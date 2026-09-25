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
func (m *Manager) Send(ctx context.Context, cardID, text string) error {
	ls, err := m.live(cardID)
	if err != nil {
		return err
	}
	if !ls.claimTurn() {
		return m.queueOrRefuse(ls, cardID, text)
	}
	// The state is written before the agent is asked, not after: a fast turn can end, and the pump
	// write "awake", before this goroutine gets back to write "working", which would leave an idle
	// session marked as working. A send that then fails is reverted in afterSendFailed.
	m.markTurnStarting(ctx, ls)
	if err := ls.agent.Send(ctx, ls.handle, agents.UserMessage{Text: text}); err != nil {
		return m.afterSendFailed(ctx, ls, cardID, text, err)
	}
	m.markCardWorking(ctx, ls)
	return nil
}

// queueOrRefuse is Send's path for a session this bookkeeping already believes is busy.
func (m *Manager) queueOrRefuse(ls *liveSession, cardID, text string) error {
	if !ls.enqueue(text) {
		return protocol.Refused("There are already too many messages waiting for this agent. Wait for it to catch up.").
			With("cardId", cardID)
	}
	return nil
}

// afterSendFailed handles a failed agent.Send. ErrBusy means this bookkeeping and the agent
// disagreed about whether a turn was running: a turn genuinely is in progress (that is what the
// agent just said), so the working mark Send already wrote stands, and the message is queued
// instead of lost, unless the queue is already full, in which case the session is still left
// working (the turn the agent reported is still running) and only this one message is refused.
// Any other error means no turn started at all, so the working mark is reverted to awake.
func (m *Manager) afterSendFailed(ctx context.Context, ls *liveSession, cardID, text string, sendErr error) error {
	if errors.Is(sendErr, agents.ErrBusy) {
		ls.forceBusy()
		if !ls.enqueue(text) {
			return protocol.Refused("There are already too many messages waiting for this agent. Wait for it to catch up.").
				With("cardId", cardID)
		}
		return nil
	}
	ls.release()
	m.revertTurnStarting(ctx, ls)
	return fmt.Errorf("send a message to card %s: %w", cardID, sendErr)
}

// markTurnStarting records that a turn is starting: for a structured session, the session's own
// state moves to working, matching docs/architecture.md 5.1's Awake -> Working move. A session
// whose agent does not report turns is left alone (see liveSession.claimTurn).
func (m *Manager) markTurnStarting(ctx context.Context, ls *liveSession) {
	if !ls.structured {
		return
	}
	if err := m.setSessionState(ctx, ls, protocol.SessionStateWorking); err != nil {
		m.log.Warn("could not record that a session started a turn", "card_id", ls.cardID, "error", err)
	}
	m.publishState(ls, protocol.SessionStateWorking, "")
}

// revertTurnStarting undoes markTurnStarting when the turn it announced never actually started.
func (m *Manager) revertTurnStarting(ctx context.Context, ls *liveSession) {
	if !ls.structured {
		return
	}
	if err := m.setSessionState(ctx, ls, protocol.SessionStateAwake); err != nil {
		m.log.Warn("could not record that a turn never started", "card_id", ls.cardID, "error", err)
	}
	m.publishState(ls, protocol.SessionStateAwake, "")
}

// markCardWorking moves a card to working once a turn has actually started (it may already be
// there). Errors are logged: the card catches up on the next card or session event either way.
func (m *Manager) markCardWorking(ctx context.Context, ls *liveSession) {
	if _, err := m.projects.SetState(ctx, ls.cardID, protocol.CardStateWorking); err != nil {
		m.log.Warn("could not move a card to working after a message was sent", "card_id", ls.cardID, "error", err)
	}
}
