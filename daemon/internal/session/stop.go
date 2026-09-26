package session

import (
	"context"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Stop ends a card's session: it asks the agent to end, marks the session row stopped, and
// publishes the state change. It does not touch the worktree: a project's own Remove flow already
// cleans worktrees up, and closing a card in Phase 1 has no separate "clean up now" step, since
// there is no card-remove route yet either (see the report).
func (m *Manager) Stop(ctx context.Context, cardID string) error {
	// A stop that arrives while the card's view is being switched waits for the switch, and then
	// ends the session it made.
	defer m.viewLocks.Lock(cardID)()
	ls, err := m.live(cardID)
	if err != nil {
		return err
	}
	ls.setStopRequested()
	if err := ls.agent.Stop(ctx, ls.handle); err != nil {
		return fmt.Errorf("stop the session of card %s: %w", cardID, err)
	}
	if err := m.setSessionState(ctx, ls, protocol.SessionStateStopped); err != nil {
		return err
	}
	m.publishState(ls, protocol.SessionStateStopped, "")
	m.forget(cardID, ls)
	return nil
}
