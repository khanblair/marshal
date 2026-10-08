package dashboard

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Subscriber keeps `daily_stats` and `activity` current from the events the daemon already
// publishes (docs/architecture.md 16.3, docs/backend-checklist.md B2.3). It is the only writer of
// those two tables, and it never reads a card's board: a card's finish is carried by the event
// itself, so no range, page, or chart ever scans every card.
//
// It follows the `home` topic for the project events, and one `project:<id>` topic per project for
// the card events. It learns the projects that exist when it starts, and follows them as they come
// and go, so it never subscribes to the chatty card topics of a session it has no use for.
//
// What it writes today:
//
//   - A card that reaches done becomes one row of the stream (kind `merge`) and one card finished
//     and one merge on its project's day. A card created already done counts the same way.
//     `card.moved` carries the state it came from, so a card moved done to done is not counted
//     twice; a card moved out of done and back is counted again, which is the truth of the stream
//     rather than a number that can be edited after the fact.
//   - A project that is removed loses its rows, and its topic is dropped.
//
// A CI run that newly failed is one line of the stream (kind `ci`) and one CI failure on its
// project's day, written by CIFailed, which the CI module calls: the `ci.updated` event carries a
// whole snapshot, so it cannot tell a new failure from the same one seen again. Cost is written
// where the model call is recorded (internal/providers). The trim runs after every append, which
// keeps the stream bounded at ninety days without a timer.
type Subscriber struct {
	svc     *Service
	bus     *events.Bus
	entropy io.Reader

	mu  sync.Mutex
	sub *events.Subscription
	wg  sync.WaitGroup
}

// NewSubscriber builds the subscriber on a service and a bus. Both are required: the service holds
// the store, the clock, and the logger, and the bus is what the events arrive on.
func NewSubscriber(svc *Service, bus *events.Bus) (*Subscriber, error) {
	if svc == nil || svc.store == nil {
		return nil, fmt.Errorf("dashboard: the subscriber needs a service with a store")
	}
	if bus == nil {
		return nil, fmt.Errorf("dashboard: the subscriber needs the event bus")
	}
	return &Subscriber{svc: svc, bus: bus, entropy: rand.Reader}, nil
}

// Start subscribes and begins applying events. It reads the projects that exist first, so a project
// created before this daemon started is followed too. It returns an error only when that first read
// fails; after that it keeps running until ctx ends or Close is called.
func (s *Subscriber) Start(ctx context.Context) error {
	projects, err := s.projectIDs(ctx)
	if err != nil {
		return err
	}
	topics := make([]string, 0, len(projects)+1)
	topics = append(topics, string(protocol.HomeTopic))
	for _, id := range projects {
		topics = append(topics, projectTopic(id))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sub != nil {
		// Starting twice would double every count, so a second call is a no-op.
		return nil
	}
	s.sub = s.bus.Subscribe(events.Topics(topics...))
	s.wg.Add(1)
	go s.run(ctx)
	return nil
}

// Close stops the subscriber and waits for the event it is applying to finish. It is safe to call
// more than once, and it must be called before the bus and the store close (cmd/marshald's close
// order).
func (s *Subscriber) Close() error {
	s.mu.Lock()
	sub := s.sub
	s.mu.Unlock()
	if sub != nil {
		sub.Close()
	}
	s.wg.Wait()
	return nil
}

// run applies one event at a time until the context ends or the subscription closes.
func (s *Subscriber) run(ctx context.Context) {
	defer s.wg.Done()
	s.mu.Lock()
	sub := s.sub
	s.mu.Unlock()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-sub.C():
			if !ok {
				return
			}
			s.apply(ctx, ev)
		}
	}
}

// apply writes what one event means, and does nothing for the events that mean nothing here. It is
// the only place that reads an event, so the mapping from the daemon's events to Home's numbers and
// stream is one switch.
func (s *Subscriber) apply(ctx context.Context, ev events.Event) {
	if s.sub.TakeLagged() {
		// An ordinary event was dropped, so a card may have finished without being counted. The
		// numbers cannot be rebuilt from the stream, so this is logged rather than fixed up: the
		// next daemon start reads the projects again and the tiles stay the source of truth for
		// "today".
		s.svc.log.Warn("the Home subscriber fell behind, so a change may not be counted",
			"topic", ev.Topic, "type", ev.Type, "seq", ev.Seq)
	}
	switch protocol.EventType(ev.Type) {
	case protocol.EventTypeCardCreated:
		data, ok := ev.Data.(protocol.CardEventData)
		if !ok {
			break
		}
		// A card created directly in done (a fixture, or a create that asks for the done state) is
		// finished the moment it exists. Any other new card is a line of its own.
		if data.Card.State == protocol.CardStateDone {
			s.cardFinished(ctx, data.Card)
		} else {
			s.cardNote(ctx, data.Card, fmt.Sprintf("#%d %s was added", data.Card.Number, data.Card.Title))
		}
	case protocol.EventTypeCardMoved:
		data, ok := ev.Data.(protocol.CardMovedEventData)
		if !ok || data.From == data.Card.State {
			break
		}
		switch {
		case data.Card.State == protocol.CardStateDone && data.From != protocol.CardStateDone:
			s.cardFinished(ctx, data.Card)
		case data.Card.State != protocol.CardStateDone:
			s.cardNote(ctx, data.Card, fmt.Sprintf("#%d %s moved to %s",
				data.Card.Number, data.Card.Title, stateLabel(data.Card.State)))
		}
	case protocol.EventTypeProjectCreated:
		if data, ok := ev.Data.(protocol.ProjectEventData); ok {
			s.sub.Add(projectTopic(data.Project.ID))
		}
	case protocol.EventTypeProjectRemoved:
		if data, ok := ev.Data.(protocol.ProjectRemovedEventData); ok {
			s.sub.Remove(projectTopic(data.ProjectID))
			s.dropProject(ctx, data.ProjectID)
		}
	}
}

// cardFinished writes one finished card: the day's two numbers and the stream row that goes with
// them, in one transaction, and then tells the home topic so a client's chart and feed update
// without asking again.
func (s *Subscriber) cardFinished(ctx context.Context, card protocol.Card) {
	now := s.svc.now()
	at := now.UnixMilli()
	day := startOfDay(now)
	id, err := protocol.NewID(now, s.entropy)
	if err != nil {
		s.svc.log.Error("could not make an activity id", "error", err)
		return
	}
	projectID := card.ProjectID
	entry := protocol.FeedEntry{
		ID:        id,
		Kind:      protocol.FeedKindMerge,
		Text:      fmt.Sprintf("#%d %s merged into main", card.Number, card.Title),
		ProjectID: &projectID,
		At:        protocol.NewTimestamp(now),
		CardID:    card.ID,
		CardKey:   card.Key,
	}
	err = s.svc.store.Write(ctx, func(q *db.Queries) error {
		seq, err := q.NextActivitySeq(ctx)
		if err != nil {
			return fmt.Errorf("take the next activity number: %w", err)
		}
		if err := q.UpsertDailyStat(ctx, db.UpsertDailyStatParams{
			Day: day.UnixMilli(), ProjectID: card.ProjectID, CardsFinished: 1, Merges: 1,
		}); err != nil {
			return fmt.Errorf("add to the day's numbers: %w", err)
		}
		if err := q.InsertActivity(ctx, db.InsertActivityParams{
			ID:          id,
			Seq:         seq,
			ProjectID:   card.ProjectID,
			Kind:        string(protocol.FeedKindMerge),
			SubjectKind: subjectCard,
			SubjectID:   card.ID,
			SubjectKey:  card.Key,
			Summary:     entry.Text,
			CreatedAt:   at,
		}); err != nil {
			return fmt.Errorf("append to the activity stream: %w", err)
		}
		if err := q.TrimActivity(ctx, now.AddDate(0, 0, -activityRetentionDays).UnixMilli()); err != nil {
			return fmt.Errorf("trim the activity stream: %w", err)
		}
		return nil
	})
	if err != nil {
		s.svc.log.Error("could not store a finished card", "card", card.Key, "error", err)
		return
	}
	// The event carries the project's own whole day, so a client that applies it twice ends in the
	// same place and can add the projects up itself. A day that cannot be read back is left out of
	// the event rather than guessed at: the client's next load has it.
	dayStats, err := s.dayStats(ctx, day, card.ProjectID)
	if err != nil {
		s.svc.log.Error("could not read back the day's numbers", "day", day, "error", err)
	}
	s.bus.Publish(string(protocol.HomeTopic), string(protocol.EventTypeActivityCreated),
		protocol.ActivityCreatedEventData{Entry: entry, Day: dayStats}, false)
}

// CIFailure is a CI run that newly failed. The card is empty for a run on a branch no card owns.
type CIFailure struct {
	ProjectID, CardID, CardKey, CardTitle, Branch, Workflow string
}

// CIFailed counts one failed CI run: a line in the stream and one failure on the project's day, in
// one transaction, and then tells the home topic so the tile and the feed update without asking
// again. A failure that cannot be stored is logged and dropped: counting is never a reason to fail
// the run's own handling.
func (s *Subscriber) CIFailed(ctx context.Context, failure CIFailure) {
	now := s.svc.now()
	day := startOfDay(now)
	id, err := protocol.NewID(now, s.entropy)
	if err != nil {
		s.svc.log.Error("could not make an activity id", "error", err)
		return
	}
	projectID := failure.ProjectID
	entry := protocol.FeedEntry{
		ID: id, Kind: protocol.FeedKindCI, Text: ciFailureText(failure), ProjectID: &projectID,
		At: protocol.NewTimestamp(now), CardID: failure.CardID, CardKey: failure.CardKey,
	}
	err = s.svc.store.Write(ctx, func(q *db.Queries) error {
		seq, err := q.NextActivitySeq(ctx)
		if err != nil {
			return fmt.Errorf("take the next activity number: %w", err)
		}
		if err := q.UpsertDailyStat(ctx, db.UpsertDailyStatParams{
			Day: day.UnixMilli(), ProjectID: failure.ProjectID, CiFailures: 1,
		}); err != nil {
			return fmt.Errorf("add to the day's numbers: %w", err)
		}
		subjectKind := ""
		if failure.CardID != "" {
			subjectKind = subjectCard
		}
		if err := q.InsertActivity(ctx, db.InsertActivityParams{
			ID: id, Seq: seq, ProjectID: failure.ProjectID, Kind: string(protocol.FeedKindCI),
			SubjectKind: subjectKind, SubjectID: failure.CardID, SubjectKey: failure.CardKey,
			Summary: entry.Text, CreatedAt: now.UnixMilli(),
		}); err != nil {
			return fmt.Errorf("append to the activity stream: %w", err)
		}
		return q.TrimActivity(ctx, now.AddDate(0, 0, -activityRetentionDays).UnixMilli())
	})
	if err != nil {
		s.svc.log.Error("could not store a CI failure", "project", failure.ProjectID, "error", err)
		return
	}
	dayStats, err := s.dayStats(ctx, day, failure.ProjectID)
	if err != nil {
		s.svc.log.Error("could not read back the day's numbers", "day", day, "error", err)
	}
	s.bus.Publish(string(protocol.HomeTopic), string(protocol.EventTypeActivityCreated),
		protocol.ActivityCreatedEventData{Entry: entry, Day: dayStats}, false)
}

// ciFailureText is the line a failed run writes: the card it is on, or the branch when no card owns it.
func ciFailureText(failure CIFailure) string {
	where := "branch " + failure.Branch
	if failure.CardKey != "" {
		where = fmt.Sprintf("%s %s", failure.CardKey, failure.CardTitle)
	}
	if failure.Workflow == "" {
		return "CI failed on " + where
	}
	return fmt.Sprintf("CI failed on %s (%s)", where, failure.Workflow)
}

// stateLabel is a state in the words the app uses for it.
func stateLabel(state protocol.CardState) string {
	switch state {
	case protocol.CardStateBacklog:
		return "Backlog"
	case protocol.CardStatePlanning:
		return "Planning"
	case protocol.CardStateWorking:
		return "Working"
	case protocol.CardStateNeeds:
		return "Needs you"
	case protocol.CardStateReview:
		return "In review"
	case protocol.CardStateReady:
		return "Ready to merge"
	case protocol.CardStateMerging:
		return "Merging"
	default:
		return string(state)
	}
}

// cardNote writes a line about a card to the stream, for something that is not a finished card: it
// was added, or it moved. It adds nothing to the day's numbers. A line that cannot be stored is
// logged and dropped, because the feed is a record of the work and never a reason to fail it.
func (s *Subscriber) cardNote(ctx context.Context, card protocol.Card, text string) {
	now := s.svc.now()
	id, err := protocol.NewID(now, s.entropy)
	if err != nil {
		s.svc.log.Error("could not make an activity id", "error", err)
		return
	}
	projectID := card.ProjectID
	entry := protocol.FeedEntry{
		ID: id, Kind: protocol.FeedKindTool, Text: text, ProjectID: &projectID,
		At: protocol.NewTimestamp(now), CardID: card.ID, CardKey: card.Key,
	}
	err = s.svc.store.Write(ctx, func(q *db.Queries) error {
		seq, err := q.NextActivitySeq(ctx)
		if err != nil {
			return fmt.Errorf("take the next activity number: %w", err)
		}
		if err := q.InsertActivity(ctx, db.InsertActivityParams{
			ID: id, Seq: seq, ProjectID: card.ProjectID, Kind: string(protocol.FeedKindTool),
			SubjectKind: subjectCard, SubjectID: card.ID, SubjectKey: card.Key, Summary: text,
			CreatedAt: now.UnixMilli(),
		}); err != nil {
			return fmt.Errorf("append to the activity stream: %w", err)
		}
		return q.TrimActivity(ctx, now.AddDate(0, 0, -activityRetentionDays).UnixMilli())
	})
	if err != nil {
		s.svc.log.Error("could not store a card's activity line", "card", card.Key, "error", err)
		return
	}
	s.bus.Publish(string(protocol.HomeTopic), string(protocol.EventTypeActivityCreated),
		protocol.ActivityCreatedEventData{Entry: entry}, false)
}

// dropProject removes a project's rows from the stream. It does not write a line about the removal:
// the only event is the project's id, so there is no name to write a sentence with, and the screens
// already drop the project row itself.
func (s *Subscriber) dropProject(ctx context.Context, projectID string) {
	err := s.svc.store.Write(ctx, func(q *db.Queries) error {
		if err := q.DeleteActivityForProject(ctx, projectID); err != nil {
			return err
		}
		return q.DeleteDailyStatsForProject(ctx, projectID)
	})
	if err != nil {
		s.svc.log.Error("could not drop a removed project's activity and numbers", "project", projectID, "error", err)
	}
}

// dayStats reads one project's stored numbers for one day back, for the day the event carries. It is
// one indexed read of at most one row. The rows of the range are all that is touched: nothing here
// counts cards.
func (s *Subscriber) dayStats(ctx context.Context, day time.Time, projectID string) (*protocol.HomeStatDay, error) {
	at := day.UnixMilli()
	var rows []db.DailyStat
	err := s.svc.store.Read(ctx, func(q *db.Queries) error {
		var err error
		rows, err = q.ListDailyStats(ctx, db.ListDailyStatsParams{FromDay: at, ToDay: at})
		return err
	})
	if err != nil {
		return nil, err
	}
	stats := protocol.HomeStatDay{Day: protocol.NewTimestamp(day)}
	for _, row := range rows {
		if row.ProjectID != projectID {
			continue
		}
		stats.CardsFinished += int(row.CardsFinished)
		stats.Merges += int(row.Merges)
		stats.CIFailures += int(row.CiFailures)
		stats.CostMicros += row.CostMicros
	}
	return &stats, nil
}

// projectIDs reads the ids of the projects that exist, so the subscriber follows their topics from
// the first event on.
func (s *Subscriber) projectIDs(ctx context.Context) ([]string, error) {
	var ids []string
	err := s.svc.store.Read(ctx, func(q *db.Queries) error {
		projects, err := q.ListProjects(ctx)
		if err != nil {
			return err
		}
		for _, project := range projects {
			ids = append(ids, project.ID)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read the projects to follow: %w", err)
	}
	return ids, nil
}

// projectTopic is the topic one project's card events arrive on.
func projectTopic(projectID string) string { return string(protocol.ProjectTopic(projectID)) }
