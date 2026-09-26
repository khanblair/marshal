package session

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Send delivers a message into a card's running session. If a turn is already in flight it queues
// the message (bounded to maxQueuedMessages) and delivers it once the current turn ends; otherwise
// it starts the turn at once. A session whose agent does not report turns (Capabilities.StructuredEvents
// is false, the PTY adapter, not reachable through a card's chat in Phase 1) is never treated as
// busy by this bookkeeping and always sends at once: see liveSession.claimTurn and the report.
//
// Two holds change that. A card a pause holds queues the message without starting a turn, because
// the pause means "the turn that is running finishes, and a message sent meanwhile waits until the
// person resumes" (docs/architecture.md 5.1); Start or Unpause delivers it. A card whose session
// is asleep is woken first, which is what the state diagram's "Asleep -> Waking: Message or
// trigger" and the card panel's own words say.
//
// A card in the terminal view takes no message: what a person types goes to the terminal, and a
// chat message would only be typed into it beside their keys, without the chat's history seeing the
// answer. It is refused with the reason view_terminal_active, and the card is left as it was.
func (m *Manager) Send(ctx context.Context, cardID, text string) error {
	// A message that arrives while the card's view is being switched waits for the switch, and then
	// goes to whichever session it made.
	defer m.viewLocks.Lock(cardID)()
	ls, err := m.live(cardID)
	if err != nil {
		if !errors.Is(err, ErrNoLiveSession) {
			return err
		}
		if err := m.wakeForSend(ctx, cardID); err != nil {
			return err
		}
		if ls, err = m.live(cardID); err != nil {
			return err
		}
	}
	if ls.term != nil {
		return terminalActiveRefusal(cardID)
	}
	if m.cardPaused(ctx, cardID) {
		return m.queueOrRefuse(ls, text)
	}
	return m.deliver(ctx, ls, text)
}

// deliver puts a message into a live session, a card's or a chat's: it starts the turn at once when
// none is running, and queues the message when one is.
func (m *Manager) deliver(ctx context.Context, ls *liveSession, text string) error {
	if !ls.claimTurn() {
		return m.queueOrRefuse(ls, text)
	}
	// The state is written before the agent is asked, not after: a fast turn can end, and the pump
	// write "awake", before this goroutine gets back to write "working", which would leave an idle
	// session marked as working. A send that then fails is reverted in afterSendFailed.
	m.markTurnStarting(ctx, ls)
	if err := ls.agent.Send(ctx, ls.handle, agents.UserMessage{Text: text}); err != nil {
		return m.afterSendFailed(ctx, ls, text, err)
	}
	// Stored only here, once the agent has taken the message: a message that never reached the
	// agent is not part of the history, and one that did must not be lost with the request that
	// carried it.
	m.recordUserMessage(ls, text)
	m.markCardWorking(ctx, ls)
	return nil
}

// queueOrRefuse is the path for a session this bookkeeping already believes is busy, or that a
// pause holds.
func (m *Manager) queueOrRefuse(ls *liveSession, text string) error {
	if !ls.enqueue(text) {
		return ls.about(protocol.Refused(messageTooManyWaiting))
	}
	return nil
}

// messageTooManyWaiting is what a person reads when the queue of a busy session is full.
const messageTooManyWaiting = "There are already too many messages waiting for this agent. Wait for it to catch up."

// afterSendFailed handles a failed agent.Send. ErrBusy means this bookkeeping and the agent
// disagreed about whether a turn was running: a turn genuinely is in progress (that is what the
// agent just said), so the working mark Send already wrote stands, and the message is queued
// instead of lost, unless the queue is already full, in which case the session is still left
// working (the turn the agent reported is still running) and only this one message is refused.
// Any other error means no turn started at all, so the working mark is reverted to awake.
func (m *Manager) afterSendFailed(ctx context.Context, ls *liveSession, text string, sendErr error) error {
	if errors.Is(sendErr, agents.ErrBusy) {
		ls.forceBusy()
		if !ls.enqueue(text) {
			return ls.about(protocol.Refused(messageTooManyWaiting))
		}
		return nil
	}
	ls.release()
	m.revertTurnStarting(ctx, ls)
	return fmt.Errorf("send a message to %s %s: %w", ls.noun(), ls.key(), sendErr)
}

// markTurnStarting records that a turn is starting: for a structured session, the session's own
// state moves to working, matching docs/architecture.md 5.1's Awake -> Working move. A session
// whose agent does not report turns is left alone (see liveSession.claimTurn).
func (m *Manager) markTurnStarting(ctx context.Context, ls *liveSession) {
	if !ls.structured {
		return
	}
	if err := m.setSessionState(ctx, ls, protocol.SessionStateWorking); err != nil {
		m.log.Warn("could not record that a session started a turn", ls.noun()+"_id", ls.key(), "error", err)
	}
	m.publishState(ls, protocol.SessionStateWorking, "")
}

// revertTurnStarting undoes markTurnStarting when the turn it announced never actually started.
func (m *Manager) revertTurnStarting(ctx context.Context, ls *liveSession) {
	if !ls.structured {
		return
	}
	if err := m.setSessionState(ctx, ls, protocol.SessionStateAwake); err != nil {
		m.log.Warn("could not record that a turn never started", ls.noun()+"_id", ls.key(), "error", err)
	}
	m.publishState(ls, protocol.SessionStateAwake, "")
}

// markCardWorking moves a card to working once a turn has actually started (it may already be
// there). Errors are logged: the card catches up on the next card or session event either way. A
// chat has no card to move, so it does nothing for one.
func (m *Manager) markCardWorking(ctx context.Context, ls *liveSession) {
	if ls.isChat() {
		return
	}
	if _, err := m.projects.SetState(ctx, ls.cardID, protocol.CardStateWorking); err != nil {
		m.log.Warn("could not move a card to working after a message was sent", "card_id", ls.cardID, "error", err)
	}
}
