package session

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/khanblair/marshal/daemon/internal/store"
)

// StopProjectSessions stops every live session of a project: its cards' and its chats'. The
// projects service calls it first when a project is removed (docs/architecture.md 16.1), before the
// worktrees are cleaned up, so no agent is left running in a folder that is about to disappear, and
// a chat's agent runs in the repository folder itself. Every session is tried even when one fails to
// stop; the errors are joined.
func (m *Manager) StopProjectSessions(ctx context.Context, projectID string) error {
	var errs []error
	for _, cardID := range m.liveCardsOf(projectID) {
		if err := m.Stop(ctx, cardID); err != nil && !errors.Is(err, ErrNoLiveSession) {
			errs = append(errs, fmt.Errorf("stop card %s: %w", cardID, err))
		}
	}
	if err := m.stopProjectChats(ctx, projectID); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// StopCardSession stops one card's session. A card that has no session running in this process is
// not an error: a card is often deleted after its session already ended.
func (m *Manager) StopCardSession(ctx context.Context, cardID string) error {
	if err := m.Stop(ctx, cardID); err != nil && !errors.Is(err, ErrNoLiveSession) {
		return err
	}
	return nil
}

// RemoveCardLogs deletes the on-disk log folder of a card's session. The session manager owns
// where those folders live, so a card delete asks it rather than building the path itself. A card
// that never had a session has nothing to remove, and a missing folder is not an error.
func (m *Manager) RemoveCardLogs(ctx context.Context, cardID string) error {
	row, err := m.store.Queries().GetSessionByCard(ctx, cardID)
	if err != nil {
		if store.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("read the session of card %s: %w", cardID, err)
	}
	dir := sessionLogDir(m.cfg.DataDir, row.ID)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove the log folder of card %s: %w", cardID, err)
	}
	m.cfg.Logger.Info("removed a card's session logs", "card_id", cardID, "session_id", row.ID)
	return nil
}

// AwakeCards says how many of a project's cards have a running agent, for the "awake" badge in
// the sidebar. It counts live sessions in this process, so it never touches the database. A chat's
// session is not counted: the badge is about cards, and the awake limits of Phase 5 (B5.6) are the
// place a chat's agent starts to count.
func (m *Manager) AwakeCards(_ context.Context, projectID string) (int, error) {
	return len(m.liveCardsOf(projectID)), nil
}

// liveCardsOf lists the cards of a project that have a live session.
func (m *Manager) liveCardsOf(projectID string) []string {
	return m.liveOwnersOf(projectID, false)
}

// liveChatsOf lists the chats of a project that have a live session.
func (m *Manager) liveChatsOf(projectID string) []string {
	return m.liveOwnersOf(projectID, true)
}

// liveOwnersOf lists the ids of a project's live sessions that are chats' or that are cards'.
func (m *Manager) liveOwnersOf(projectID string, chats bool) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var ids []string
	for key, ls := range m.sessions {
		if ls.projectID == projectID && ls.isChat() == chats {
			ids = append(ids, key)
		}
	}
	return ids
}
