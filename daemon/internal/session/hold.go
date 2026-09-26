package session

import (
	"context"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Session hold: the four controls a person presses, under the rules of docs/architecture.md
// section 5.1 (slice G of Phase 2, checklist item B2.15). The card carries paused and pinned; the
// session carries asleep and waking. Everything automatic (the idle timer, the sleep warning and
// its notice, the awake limit, keep awake, and sleep all) is Phase 5 and is not here.
//
// The refusal sentences below are the prototype's own, taken from mock/actions/sessions.ts, and
// architecture.md 5.1 repeats them word for word, so they must not drift.
//
// Two things the docs do not spell out, decided here and written into architecture.md 5.1:
//
//   - A message sent to a sleeping card wakes it (the state diagram's "Asleep -> Waking: Message
//     or trigger", the prototype's own send, and the card panel's own words). Because the person
//     has just asked the card to work, waking it that way also releases the pause that let it
//     sleep; otherwise the message would sit behind a hold nobody can see.
//   - A paused card whose pause is still holding a message cannot sleep, because stopping the
//     process would lose that message: the queue lives in memory with the live session, and a
//     sent message is only stored as history at the moment it reaches the agent.

// messageSessionStopped is the answer for a session that has stopped, whether the caller asked to
// wake it or to resume it: nothing is running to wake or resume, and start is the route that
// brings a stopped session back.
const messageSessionStopped = "This card's session has stopped and cannot be resumed."

// holdRefusalMessage is the sentence a person reads when a pause or a sleep is refused. The words
// are the ones in architecture.md section 5.1 and must not drift from it.
//
// It is a switch rather than a table because a mutable global is not allowed here
// (code-standards.md section 3), and five short cases are easier to read against the spec than a
// map is.
func holdRefusalMessage(reason protocol.HoldRefusalReason) string {
	switch reason {
	case protocol.HoldRefusalReasonPauseNotWorking:
		return "Only working cards can be paused."
	case protocol.HoldRefusalReasonSleepWorking:
		return "Working cards don't sleep. Pause the card first."
	case protocol.HoldRefusalReasonSleepNeedsYou:
		return "This card is waiting on you, so it stays awake."
	case protocol.HoldRefusalReasonSleepNoSession:
		return "This card has no awake session."
	case protocol.HoldRefusalReasonSleepHoldingMessages:
		return "This card has a message waiting for you to resume it. Resume the card first."
	}
	return ""
}

// refusedHold builds the refusal for a rule, with the stable reason and the sentence of 5.1.
func refusedHold(reason protocol.HoldRefusalReason) *protocol.Error {
	return protocol.Refused(holdRefusalMessage(reason)).With("reason", string(reason))
}

// awakeSessionState reports whether a session in this state is awake: it has, or is about to have,
// a process. A session in any other state (asleep, stopped) has nothing to put to sleep, so a
// sleep of it answers "This card has no awake session."
func awakeSessionState(state protocol.SessionState) bool {
	switch state {
	case protocol.SessionStateStarting, protocol.SessionStateAwake, protocol.SessionStateWorking,
		protocol.SessionStateWaking:
		return true
	}
	return false
}

// Pause holds a working card between turns: the turn that is running finishes, the card stays
// where it is, and a message sent meanwhile waits until the card is resumed (Start, or Unpause).
// Only a working card can be paused; anything else is refused with the sentence of
// architecture.md 5.1 and left exactly as it was.
func (m *Manager) Pause(ctx context.Context, cardID string) (protocol.Card, error) {
	card, err := m.projects.Card(ctx, cardID)
	if err != nil {
		return protocol.Card{}, err
	}
	if card.State != protocol.CardStateWorking {
		m.log.Info("refused to pause a card", "card_id", cardID, "state", card.State)
		return protocol.Card{}, refusedHold(protocol.HoldRefusalReasonPauseNotWorking)
	}
	if card.Paused {
		return card, nil
	}
	yes := true
	paused, err := m.projects.SetHold(ctx, cardID, &yes, nil)
	if err != nil {
		return protocol.Card{}, err
	}
	m.log.Info("paused a card", "card_id", cardID)
	return paused, nil
}

// Unpause releases a pause and delivers the message it was holding, if the session is live and no
// turn is running. A card that is not paused is left alone, so a client may call it twice.
func (m *Manager) Unpause(ctx context.Context, cardID string) (protocol.Card, error) {
	card, err := m.projects.Card(ctx, cardID)
	if err != nil {
		return protocol.Card{}, err
	}
	if !card.Paused {
		return card, nil
	}
	no := false
	released, err := m.projects.SetHold(ctx, cardID, &no, nil)
	if err != nil {
		return protocol.Card{}, err
	}
	m.deliverHeld(cardID)
	m.log.Info("released a card's pause", "card_id", cardID)
	return released, nil
}

// Pin keeps a card from sleeping on its own. Phase 5 is what sleeps a card automatically, so
// today this only records the person's choice, as the docs say. Every card can be pinned, so
// there is nothing to refuse, and a card that is already pinned is left alone.
func (m *Manager) Pin(ctx context.Context, cardID string) (protocol.Card, error) {
	card, err := m.projects.Card(ctx, cardID)
	if err != nil {
		return protocol.Card{}, err
	}
	if card.Pinned {
		return card, nil
	}
	yes := true
	return m.projects.SetHold(ctx, cardID, nil, &yes)
}

// Unpin undoes Pin, so a card may sleep on its own again.
func (m *Manager) Unpin(ctx context.Context, cardID string) (protocol.Card, error) {
	card, err := m.projects.Card(ctx, cardID)
	if err != nil {
		return protocol.Card{}, err
	}
	if !card.Pinned {
		return card, nil
	}
	no := false
	return m.projects.SetHold(ctx, cardID, nil, &no)
}

// Sleep stops a card's agent process, keeps the session id, and records that the session is
// asleep, so Wake or Start can bring the same conversation back. The card's worktree and branch
// stay, and the session row keeps its agent session id.
//
// The refusals are the rules of architecture.md 5.1, in the order it checks them: a working card
// that is not paused does not sleep; a card that is waiting on a person stays awake; a card with
// no awake session cannot sleep; and a pause that is still holding a message cannot be slept,
// because stopping the process would lose that message.
func (m *Manager) Sleep(ctx context.Context, cardID string) error {
	// A sleep that arrives while the card's view is being switched waits for the switch.
	defer m.viewLocks.Lock(cardID)()
	card, err := m.projects.Card(ctx, cardID)
	if err != nil {
		return err
	}
	if refusal := checkSleep(card); refusal != nil {
		m.log.Info("refused to put a card to sleep", "card_id", cardID, "reason", refusal.Details["reason"])
		return refusal
	}
	row, err := m.store.Queries().GetSessionByCard(ctx, cardID)
	if err != nil {
		if store.IsNotFound(err) {
			return refusedHold(protocol.HoldRefusalReasonSleepNoSession)
		}
		return fmt.Errorf("look up the session of card %s: %w", cardID, err)
	}
	if !awakeSessionState(protocol.SessionState(row.State)) {
		return refusedHold(protocol.HoldRefusalReasonSleepNoSession)
	}
	if m.heldMessages(cardID) > 0 {
		m.log.Info("refused to sleep a card whose pause is holding a message", "card_id", cardID)
		return refusedHold(protocol.HoldRefusalReasonSleepHoldingMessages)
	}
	if err := m.stopForSleep(ctx, cardID); err != nil {
		return err
	}
	if err := m.setRowState(ctx, row, protocol.SessionStateAsleep); err != nil {
		return err
	}
	m.publishSessionState(cardID, row.ID, protocol.SessionStateAsleep, "")
	m.log.Info("put a card's session to sleep", "card_id", cardID, "session_id", row.ID)
	return nil
}

// checkSleep answers whether a card's own state allows a sleep: the first two rules of
// architecture.md 5.1. The rules about the session (it must be awake, and its pause must not be
// holding a message) are checked after this, because they need the session row.
func checkSleep(card protocol.Card) *protocol.Error {
	switch {
	case card.State == protocol.CardStateWorking && !card.Paused:
		return refusedHold(protocol.HoldRefusalReasonSleepWorking)
	case card.State == protocol.CardStateNeeds:
		return refusedHold(protocol.HoldRefusalReasonSleepNeedsYou)
	}
	return nil
}

// stopForSleep ends the agent process of a card that has one, without ending the session: the
// live session is forgotten, and its exit is marked as expected so the pump does not treat it as
// a crash and move the card to needs you (see finishPump). A card whose session is awake in the
// database but has no live process (a restart in manual mode) has nothing to stop.
func (m *Manager) stopForSleep(ctx context.Context, cardID string) error {
	ls := m.liveOf(cardID)
	if ls == nil {
		return nil
	}
	ls.setStopRequested()
	if err := ls.agent.Stop(ctx, ls.handle); err != nil {
		// The process is still there, so the sleep did not happen and the session is not on its
		// way out after all.
		ls.clearStopRequested()
		return fmt.Errorf("put the session of card %s to sleep: %w", cardID, err)
	}
	m.forget(cardID, ls)
	return nil
}

// Wake resumes a card's sleeping session through its saved id: the session is recorded as waking,
// then awake, and the card moves back to working through the ordinary resume path. A session that
// is already awake has nothing to wake and is left alone. A session that has stopped cannot be
// woken; Start is the route that brings a stopped session back.
func (m *Manager) Wake(ctx context.Context, cardID string) error {
	row, err := m.store.Queries().GetSessionByCard(ctx, cardID)
	if err != nil {
		if store.IsNotFound(err) {
			return protocol.NotFound("session").With("cardId", cardID)
		}
		return fmt.Errorf("look up the session of card %s: %w", cardID, err)
	}
	switch protocol.SessionState(row.State) {
	case protocol.SessionStateAsleep:
		return m.wakeRow(ctx, row)
	case protocol.SessionStateStopped:
		return protocol.Refused(messageSessionStopped).With("cardId", cardID)
	}
	return nil
}

// wakeRow resumes one sleeping session row. The caller has checked that the row is asleep.
func (m *Manager) wakeRow(ctx context.Context, row db.Session) error {
	if err := m.reserve(row.CardID); err != nil {
		return err
	}
	defer m.release(row.CardID)
	if err := m.setRowState(ctx, row, protocol.SessionStateWaking); err != nil {
		return err
	}
	m.publishSessionState(row.CardID, row.ID, protocol.SessionStateWaking, "")
	_, err := m.resumeRow(ctx, row)
	return err
}

// wakeForSend wakes a card's session when a message arrives for it and it is asleep, and releases
// the pause that let it sleep. It does nothing for a card whose session is in any other state, so
// the caller's own answer for a card with no running agent stands.
func (m *Manager) wakeForSend(ctx context.Context, cardID string) error {
	row, err := m.store.Queries().GetSessionByCard(ctx, cardID)
	if err != nil {
		if store.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("look up the session of card %s: %w", cardID, err)
	}
	if protocol.SessionState(row.State) != protocol.SessionStateAsleep {
		return nil
	}
	if sessionIsInTheTerminal(row) {
		// The session would wake into the terminal, where a message has no place: refuse before a
		// process is started for it.
		return terminalActiveRefusal(cardID)
	}
	card, err := m.projects.Card(ctx, cardID)
	if err != nil {
		return err
	}
	if card.Paused {
		no := false
		if _, err := m.projects.SetHold(ctx, cardID, &no, nil); err != nil {
			return err
		}
	}
	m.log.Info("a message woke a sleeping session", "card_id", cardID, "session_id", row.ID)
	return m.wakeRow(ctx, row)
}

// deliverHeld delivers the oldest message a pause was holding, when the card's session is live
// and no turn is running. A turn that is already running keeps its own place in the queue: the
// pump's TurnEnded path delivers the next message then.
func (m *Manager) deliverHeld(cardID string) {
	ls := m.liveOf(cardID)
	if ls == nil {
		return
	}
	text, ok := ls.takeIfIdle()
	if !ok {
		return
	}
	m.deliverQueued(m.ctx, ls, text)
}

// heldMessages says how many messages a card's pause is holding. A card with no live session has
// none, and one whose agent does not report turns (the PTY adapter) is never treated as busy, so
// nothing waits for it.
func (m *Manager) heldMessages(cardID string) int {
	ls := m.liveOf(cardID)
	if ls == nil {
		return 0
	}
	return ls.waiting()
}
