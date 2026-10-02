package session

import (
	"context"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
)

// ResetChatSession ends a chat's live session and forgets the conversation its agent kept, so the
// chat's next message starts a new agent that has never talked. The chat's own history is left
// alone, and so is its row: it reads "starting", as a chat that has never talked does. The Integrator
// is reset this way between merge tasks so that one task's context cannot colour the next.
//
// A chat with no live process is reset too: only its saved conversation is forgotten. A chat that
// never had a session row has nothing to reset, and is not an error.
func (m *Manager) ResetChatSession(ctx context.Context, chatID string) error {
	unlock := m.chatLocks.Lock(chatID)
	defer unlock()
	if ls := m.liveOf(chatID); ls != nil {
		if err := m.endChatProcess(ctx, ls); err != nil {
			return err
		}
		// The ending session closes its log folder as it finishes, and the next session opens the
		// same folder, so the next message must not start before it has.
		if !m.waitPump(ls) {
			m.log.Warn("a chat's old session had not finished when it was reset", "chat_id", chatID)
		}
	}
	row, err := m.chatSession(ctx, chatID)
	if err != nil {
		if store.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("read the session of chat %s: %w", chatID, err)
	}
	row.AgentSessionID = ""
	if err := m.setRowState(ctx, row, protocol.SessionStateStarting); err != nil {
		return err
	}
	m.publishChatState(chatID, row.ID, protocol.SessionStateStarting, "")
	m.log.Info("reset a chat's session", "chat_id", chatID, "session_id", row.ID)
	return nil
}
