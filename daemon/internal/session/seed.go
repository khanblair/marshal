package session

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// SeedSession writes a session row for a card without starting an agent. It exists for the
// prototype fixture, which has to show sessions the screens can draw (an agent that is awake, one
// that is working) without starting a process for every card in the prototype at every daemon
// start. Nothing else calls it, and it never touches the live map: a seeded session has no
// process, so the manager does not know it as live.
//
// A card that already has a session row is left alone, because a card has at most one session row
// for its whole life. That also makes it safe to call on every start.
func (m *Manager) SeedSession(ctx context.Context, cardID string, state protocol.SessionState) error {
	if !state.Valid() {
		return fmt.Errorf("seed a session of card %s: %q is not a session state", cardID, state)
	}
	card, err := m.projects.Card(ctx, cardID)
	if err != nil {
		return fmt.Errorf("seed a session of card %s: %w", cardID, err)
	}
	now := m.cfg.Now()
	rowID, err := protocol.NewID(now, rand.Reader)
	if err != nil {
		return fmt.Errorf("make a session id: %w", err)
	}
	params := db.CreateCardSessionParams{
		ID: rowID, CardID: cardID, AgentKind: string(card.Agent),
		State: string(state), Model: card.Model,
		LastActiveAt: now.UnixMilli(), CreatedAt: now.UnixMilli(), UpdatedAt: now.UnixMilli(),
	}
	if card.Thinking != nil {
		params.Thinking = string(*card.Thinking)
	}
	params.PermissionMode = string(card.PermissionMode)
	err = m.store.Write(ctx, func(q *db.Queries) error {
		// A card that already has a row keeps it: one session per card, and a fixture load must not
		// fail because the card was seeded on an earlier start.
		if _, err := q.GetSessionByCard(ctx, cardID); err == nil {
			return nil
		}
		return q.CreateCardSession(ctx, params)
	})
	if err != nil {
		return fmt.Errorf("seed the session of card %s: %w", cardID, err)
	}
	m.cfg.Logger.Debug("seeded a fixture session", "card_id", cardID, "state", state)
	return nil
}
