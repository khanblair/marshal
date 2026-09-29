package integrations

// The Marshal-to-Trello half of the card sync (B8.2, the other direction from trello_sync.go): a
// card that reaches Marshal's done column, and is already linked to a Trello card, is moved to
// Trello's own done-reading list too. It follows the daemon's own card.moved event - never a poll -
// the same way internal/dashboard's Subscriber follows card.moved for its own stream.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/integrations/trello"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// OutboundSync watches Marshal's own cards for a move into done, and mirrors it onto a linked
// Trello card. Built once; Start begins following, Close stops it before the bus closes.
type OutboundSync struct {
	svc *Service
	bus *events.Bus

	mu  sync.Mutex
	sub *events.Subscription
	wg  sync.WaitGroup
}

// NewOutboundSync builds the watcher. Both svc and bus are required.
func NewOutboundSync(svc *Service, bus *events.Bus) (*OutboundSync, error) {
	if svc == nil || svc.store == nil {
		return nil, fmt.Errorf("integrations: the outbound sync needs a service with a store")
	}
	if bus == nil {
		return nil, fmt.Errorf("integrations: the outbound sync needs the event bus")
	}
	return &OutboundSync{svc: svc, bus: bus}, nil
}

// Start subscribes to every project's own topic, the same way internal/dashboard's Subscriber
// does, and begins applying card.moved events. A second call is a no-op.
func (o *OutboundSync) Start(ctx context.Context) error {
	var ids []string
	err := o.svc.store.Read(ctx, func(q *db.Queries) error {
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
		return err
	}
	topics := make([]string, 0, len(ids)+1)
	topics = append(topics, string(protocol.HomeTopic))
	for _, id := range ids {
		topics = append(topics, string(protocol.ProjectTopic(id)))
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.sub != nil {
		return nil
	}
	o.sub = o.bus.Subscribe(events.Topics(topics...))
	o.wg.Add(1)
	go o.run(ctx)
	return nil
}

// Close stops following and waits for the event it is applying to finish.
func (o *OutboundSync) Close() error {
	o.mu.Lock()
	sub := o.sub
	o.mu.Unlock()
	if sub != nil {
		sub.Close()
	}
	o.wg.Wait()
	return nil
}

func (o *OutboundSync) run(ctx context.Context) {
	defer o.wg.Done()
	o.mu.Lock()
	sub := o.sub
	o.mu.Unlock()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-sub.C():
			if !ok {
				return
			}
			o.apply(ctx, ev)
		}
	}
}

// apply reacts to a card moved into done, past a project just being created (whose own topic this
// follows too, mirroring the dashboard Subscriber's own project-tracking, kept minimal here since
// a project made after this daemon started is not yet linked to any Trello board anyway).
func (o *OutboundSync) apply(ctx context.Context, ev events.Event) {
	if ev.Type != string(protocol.EventTypeCardMoved) {
		return
	}
	data, ok := ev.Data.(protocol.CardMovedEventData)
	if !ok || data.From == protocol.CardStateDone || data.Card.State != protocol.CardStateDone {
		return
	}
	if err := o.pushDone(ctx, data.Card); err != nil {
		o.svc.log.Error("could not push a card's move to Trello", "card", data.Card.ID, "error", err)
	}
}

// pushDone moves cardID's linked Trello card to a list that reads as done, when the card is
// linked and a done-reading list exists on the board. Neither missing is quiet: most cards are
// never linked to Trello at all, and looking for that is the ordinary path, not a failure.
func (o *OutboundSync) pushDone(ctx context.Context, card protocol.Card) error {
	trelloCardID, linked, err := o.externalIDFor(ctx, card.ID)
	if err != nil {
		return err
	}
	if !linked {
		return nil
	}
	client, boardID, err := o.svc.TrelloClient(ctx)
	if errors.Is(err, ErrNotConnected) {
		return nil
	}
	if err != nil {
		return err
	}
	lists, err := client.ListLists(ctx, boardID)
	if err != nil {
		return fmt.Errorf("read %s's lists: %w", boardID, err)
	}
	doneList, found := doneListOf(lists)
	if !found {
		return nil
	}
	if err := client.MoveCard(ctx, trelloCardID, doneList.ID); err != nil {
		return fmt.Errorf("move the Trello card %s to %s: %w", trelloCardID, doneList.Name, err)
	}
	o.svc.log.Info("moved a card to done on Trello", "card", card.ID, "trello_card", trelloCardID)
	return nil
}

// externalIDFor reads the Trello card id a Marshal card is linked to, when it has one.
func (o *OutboundSync) externalIDFor(ctx context.Context, cardID string) (string, bool, error) {
	var externalID string
	var found bool
	err := o.svc.store.Read(ctx, func(q *db.Queries) error {
		id, err := q.CardExternalID(ctx, db.CardExternalIDParams{CardID: cardID, Kind: trello.ID})
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		externalID, found = id, true
		return nil
	})
	return externalID, found, err
}
