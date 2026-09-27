package session

import (
	"context"
	"sort"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The sleep notice: the group of idle cards that will sleep soon, and the calls a person makes on
// it (keep awake, sleep now, keep all, sleep all, dismiss) - the automatic half of session hold,
// docs/architecture.md sections 5.1 and 5.2, checklist item B5.6, inventory N5. What decides when
// a card joins one is sleep.go; this file is the notice itself.
//
// The notice lives in the manager's memory, not in a table. What it names is runtime state: a live
// session, a wall-clock deadline, and the group of cards that share it. A daemon that restarts has
// no live sessions and restores the cards that were awake per the restore setting (section 5.3), so
// there is nothing left for an old notice to be about. The full `notices` table of section 10 and
// the `notify` module that routes member notices belong to the phase that adds mentions and
// replies; the sleep reminder is the one kind Phase 5 produces, and it is derived, not stored.
//
// A group is one project's idle cards: "grouped reminders" (section 5.2) is one reminder per
// project rather than one per card, so a person reads a line per project and the cards they hold.
// The group's deadline is when its cards sleep. A card that becomes idle while its project already
// has a notice joins it and shares its deadline; a project with none gets a fresh deadline.
//
// Every call that takes a card off a notice - keep awake, keep all awake, dismiss - also holds it
// off the idle timer for the keep-awake setting's length. Without that, the card is still idle one
// tick later and the notice the person just cleared would come straight back. A plain dismiss and
// "keep all awake" are therefore the same hold, differing only in what they answer.

// sleepGroup is one project's idle cards and the moment they sleep. It is guarded by
// Manager.noticesMu.
type sleepGroup struct {
	// id is the notice's own id on the wire: "sleep:<projectId>".
	id string
	// projectID is the project the cards belong to.
	projectID string
	// cards are the card ids in the group, oldest first.
	cards []string
	// deadline is when the group's cards sleep.
	deadline time.Time
	// createdAt is when the group was made, for the notice's own timestamp.
	createdAt time.Time
}

// sleepGroupID is the id of a project's sleep notice.
func sleepGroupID(projectID string) string {
	return string(protocol.NoticeKindSleep) + ":" + projectID
}

// Notices returns every standing notice, for GET /v1/notices. Only the sleep kind exists yet; a
// notice that is not a sleep group joins here when a later phase produces one.
func (m *Manager) Notices() []protocol.Notice {
	m.noticesMu.Lock()
	defer m.noticesMu.Unlock()
	out := make([]protocol.Notice, 0, len(m.sleepGroups))
	for _, group := range m.sleepGroups {
		out = append(out, group.notice())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// notice is the group on the wire.
func (g *sleepGroup) notice() protocol.Notice {
	deadline := protocol.NewTimestamp(g.deadline)
	cards := make([]string, len(g.cards))
	copy(cards, g.cards)
	return protocol.Notice{
		ID: g.id, Kind: protocol.NoticeKindSleep, Cards: cards, Deadline: &deadline,
		ProjectID: g.projectID, CreatedAt: protocol.NewTimestamp(g.createdAt),
	}
}

// KeepAwake keeps one card off the idle timer for the keep-awake setting's length, and takes it off
// its project's sleep notice. A session that had been moved to the sleep-warning state goes back to
// awake, so the card does not read as "will sleep soon" after the person said it should not. The
// card itself is answered, the way the other hold calls answer with the card.
func (m *Manager) KeepAwake(ctx context.Context, cardID string) (protocol.Card, error) {
	card, err := m.projects.Card(ctx, cardID)
	if err != nil {
		return protocol.Card{}, err
	}
	until := m.cfg.Now().Add(m.keepAwakeLength(ctx))
	m.holdAwake(cardID, until, protocol.EventTypeNoticeDismissed)
	m.clearSleepWarning(ctx, cardID)
	m.log.Info("kept a card awake", "card_id", cardID)
	return card, nil
}

// DismissNotice clears a notice without changing any card: the cards stay awake, held off the idle
// timer for the keep-awake length. An id that names no notice is not an error, so a client that
// dismisses the same notice twice - or dismisses one that just went away because its cards slept -
// reads the same answer.
func (m *Manager) DismissNotice(ctx context.Context, id string) error {
	m.resolveNotice(ctx, id)
	return nil
}

// KeepAllAwake clears a notice's cards the way DismissNotice does and answers how many it held off,
// for the "Kept N cards awake" toast.
func (m *Manager) KeepAllAwake(ctx context.Context, id string) (int, error) {
	return m.resolveNotice(ctx, id), nil
}

// SleepAll sleeps every card a notice names that is still allowed to sleep, and answers how many
// went to sleep, for the "N cards asleep" toast. A card that became busy or needs the person while
// the notice stood is left awake: Sleep itself refuses it, the refusal is logged, not returned,
// because the rest of the group must still sleep. Such a card is also moved out of the sleep-warning
// state, so it does not go on reading as about to sleep when its notice is gone.
func (m *Manager) SleepAll(ctx context.Context, id string) (int, error) {
	cards := m.takeSleepGroup(id, protocol.EventTypeNoticeDismissed)
	slept := 0
	for _, cardID := range cards {
		if err := m.Sleep(ctx, cardID); err != nil {
			m.log.Info("left a card awake after sleep all", "card_id", cardID, "error", err)
			m.clearSleepWarning(ctx, cardID)
			continue
		}
		slept++
	}
	return slept, nil
}

// resolveNotice removes the notice named by id, holding each of its cards off the idle timer, and
// answers how many cards it named. It is one path because a dismiss and a keep-all-awake differ only
// in their answer.
func (m *Manager) resolveNotice(ctx context.Context, id string) int {
	keep := m.keepAwakeLength(ctx)
	cards := m.takeSleepGroup(id, protocol.EventTypeNoticeDismissed)
	if len(cards) == 0 {
		return 0
	}
	until := m.cfg.Now().Add(keep)
	for _, cardID := range cards {
		m.holdAwake(cardID, until, "")
		m.clearSleepWarning(ctx, cardID)
	}
	return len(cards)
}

// joinSleepGroup adds a card to its project's sleep notice, making the notice when the project has
// none, and announces that the notice list gained one when it did. A card already in the group
// keeps the deadline it already had.
func (m *Manager) joinSleepGroup(projectID, cardID string, now time.Time, deadline time.Time) {
	id := sleepGroupID(projectID)
	created := false
	m.noticesMu.Lock()
	group := m.sleepGroups[id]
	if group == nil {
		group = &sleepGroup{id: id, projectID: projectID, deadline: deadline, createdAt: now}
		m.sleepGroups[id] = group
		created = true
	}
	if !containsString(group.cards, cardID) {
		group.cards = append(group.cards, cardID)
	}
	m.noticesMu.Unlock()
	if created {
		m.publishNotices(protocol.EventTypeNoticeCreated)
	}
}

// leaveSleepGroup takes one card off its notice, dropping the notice when it names no card any more,
// and announces that the list lost one. It is what a card that left the warning state on its own
// calls.
func (m *Manager) leaveSleepGroup(cardID string, kind protocol.EventType) {
	m.noticesMu.Lock()
	dropped := m.dropFromSleepLocked(cardID)
	m.noticesMu.Unlock()
	if dropped {
		m.publishNotices(kind)
	}
}

// sleepGroupOf reports whether a card is on a sleep notice, and the notice's id when it is. The id
// is the second answer because a caller that only wants to know need not care, and the one that
// takes a card off a notice already has the id it asks by.
func (m *Manager) sleepGroupOf(cardID string) (bool, string) {
	m.noticesMu.Lock()
	defer m.noticesMu.Unlock()
	for id, group := range m.sleepGroups {
		if containsString(group.cards, cardID) {
			return true, id
		}
	}
	return false, ""
}

// takeSleepGroup removes a notice and answers the cards it named, announcing that the list lost one.
// A group with one of its own cards left, a card that a person put to sleep by hand while the
// notice stood, is this group's rule applied again: the card is gone, and a notice that names
// nothing is not a notice.
func (m *Manager) takeSleepGroup(id string, kind protocol.EventType) []string {
	m.noticesMu.Lock()
	group := m.sleepGroups[id]
	if group == nil {
		m.noticesMu.Unlock()
		return nil
	}
	delete(m.sleepGroups, id)
	cards := append([]string(nil), group.cards...)
	m.noticesMu.Unlock()
	m.publishNotices(kind)
	return cards
}

// dueSleepGroups answers a copy of every notice whose deadline has passed, dropping each from the
// standing list and announcing that the list lost one. The groups are copies so the caller can put
// their cards to sleep without holding the notice lock while it stops agent processes.
func (m *Manager) dueSleepGroups(now time.Time) []sleepGroup {
	m.noticesMu.Lock()
	due := make([]sleepGroup, 0, len(m.sleepGroups))
	for _, group := range m.sleepGroups {
		if !group.deadline.After(now) {
			due = append(due, *group)
		}
	}
	for _, group := range due {
		delete(m.sleepGroups, group.id)
	}
	m.noticesMu.Unlock()
	if len(due) > 0 {
		m.publishNotices(protocol.EventTypeNoticeDismissed)
	}
	return due
}

// holdAwake holds a card off the idle timer until a moment, and takes it off any notice it is on.
// An empty kind publishes nothing, which is what the caller wants when it is about to publish once
// for a whole group.
func (m *Manager) holdAwake(cardID string, until time.Time, kind protocol.EventType) {
	m.noticesMu.Lock()
	m.keptAwake[cardID] = until
	m.dropFromSleepLocked(cardID)
	m.noticesMu.Unlock()
	if kind != "" {
		m.publishNotices(kind)
	}
}

// keptOffIdle reports whether a person has held a card off the idle timer and that hold has not run
// out. A hold in the past is left in the map and ignored, so nothing has to clean it up.
func (m *Manager) keptOffIdle(cardID string, now time.Time) bool {
	m.noticesMu.Lock()
	defer m.noticesMu.Unlock()
	until, ok := m.keptAwake[cardID]
	return ok && now.Before(until)
}

// forgetCard drops everything the manager remembers about a card that has gone for good: its
// keep-awake hold and its place on a sleep notice. A card whose logs are removed will not come back
// under that id, so nothing is left for a notice to be about.
func (m *Manager) forgetCard(cardID string) {
	m.noticesMu.Lock()
	delete(m.keptAwake, cardID)
	dropped := m.dropFromSleepLocked(cardID)
	m.noticesMu.Unlock()
	if dropped {
		m.publishNotices(protocol.EventTypeNoticeDismissed)
	}
}

// dropFromSleepLocked takes a card off its project's sleep notice, dropping the notice when it names
// no card any more. The caller holds Manager.noticesMu. The notice's own deadline does not move.
func (m *Manager) dropFromSleepLocked(cardID string) bool {
	for id, group := range m.sleepGroups {
		if !containsString(group.cards, cardID) {
			continue
		}
		group.cards = removeString(group.cards, cardID)
		if len(group.cards) == 0 {
			delete(m.sleepGroups, id)
		}
		return true
	}
	return false
}

// clearSleepWarning moves a card's session back to awake when it was held in the sleep-warning
// state, so a card a person has kept awake no longer reads as about to sleep. A session in any
// other state is left alone.
func (m *Manager) clearSleepWarning(ctx context.Context, cardID string) {
	row, err := m.store.Queries().GetSessionByCard(ctx, cardID)
	if err != nil {
		// A card with no session has nothing to clear, which is not worth a log line: it is a
		// card that was never started, or one that has just gone.
		return
	}
	if protocol.SessionState(row.State) != protocol.SessionStateSleepWarning {
		return
	}
	if err := m.setRowState(ctx, row, protocol.SessionStateAwake); err != nil {
		m.log.Warn("could not move a card out of the sleep warning", "card_id", cardID, "error", err)
		return
	}
	m.publishSessionState(cardID, row.ID, protocol.SessionStateAwake, "")
}

// publishNotices announces that the notice list changed, on the home topic, which every client
// follows for the shell. The two events are the ones architecture.md 11.2 names: created when the
// list gains a notice, dismissed when it loses one. A client that is watching re-reads the list on
// either, so it never has to hold the difference.
func (m *Manager) publishNotices(kind protocol.EventType) {
	if m.bus == nil {
		return
	}
	m.bus.Publish(string(protocol.HomeTopic), string(kind),
		protocol.NoticeListEventData{Notices: m.Notices()}, true)
}

func containsString(list []string, want string) bool {
	for _, one := range list {
		if one == want {
			return true
		}
	}
	return false
}

// removeString returns list without the first want.
func removeString(list []string, want string) []string {
	out := list[:0]
	removed := false
	for _, one := range list {
		if !removed && one == want {
			removed = true
			continue
		}
		out = append(out, one)
	}
	return out
}
