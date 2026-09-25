package session

import (
	"context"
	"errors"
	"fmt"
)

// StopProjectSessions stops every live session of a project's cards. The projects service calls
// it first when a project is removed (docs/architecture.md 16.1), before the worktrees are cleaned
// up, so no agent is left running in a folder that is about to disappear. Every session is tried
// even when one fails to stop; the errors are joined.
func (m *Manager) StopProjectSessions(ctx context.Context, projectID string) error {
	var errs []error
	for _, cardID := range m.liveCardsOf(projectID) {
		if err := m.Stop(ctx, cardID); err != nil && !errors.Is(err, ErrNoLiveSession) {
			errs = append(errs, fmt.Errorf("stop card %s: %w", cardID, err))
		}
	}
	return errors.Join(errs...)
}

// AwakeCards says how many of a project's cards have a running agent, for the "awake" badge in
// the sidebar. It counts live sessions in this process, so it never touches the database.
func (m *Manager) AwakeCards(_ context.Context, projectID string) (int, error) {
	return len(m.liveCardsOf(projectID)), nil
}

// liveCardsOf lists the cards of a project that have a live session.
func (m *Manager) liveCardsOf(projectID string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var cards []string
	for cardID, ls := range m.sessions {
		if ls.projectID == projectID {
			cards = append(cards, cardID)
		}
	}
	return cards
}
