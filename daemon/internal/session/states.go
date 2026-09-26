package session

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
)

// StoredStates reads the state and view of card sessions as they were last stored, for the `session`
// and `viewMode` fields of the wire card (projects.SessionStates). It reads the sessions table only, never a Manager's
// memory: the manager writes every state change to the session row before it announces it, so the
// row is the truth, and it survives a restart. That is also why it needs no Manager at all, and why
// the projects service can be given one before the manager exists and keep it while a manager is
// replaced.
type StoredStates struct {
	store *store.Store
}

var _ projects.SessionStates = (*StoredStates)(nil)

// NewStoredStates builds a StoredStates on an open database.
func NewStoredStates(st *store.Store) (*StoredStates, error) {
	if st == nil {
		return nil, errors.New("the session states need the store")
	}
	return &StoredStates{store: st}, nil
}

// CardSession is the stored state and view of one card's session, and nil when the card has none.
func (s *StoredStates) CardSession(ctx context.Context, cardID string) (*projects.SessionInfo, error) {
	row, err := s.store.Queries().GetSessionByCard(ctx, cardID)
	switch {
	case err == nil:
		return &projects.SessionInfo{State: protocol.SessionState(row.State), View: protocol.CardViewMode(row.ViewMode)}, nil
	case store.IsNotFound(err):
		return nil, nil
	default:
		return nil, fmt.Errorf("read the session of card %s: %w", cardID, err)
	}
}

// ProjectSessions are the stored states and views of the sessions of a project's cards, by card id.
// A card that never had a session is not in the map.
func (s *StoredStates) ProjectSessions(ctx context.Context, projectID string) (map[string]projects.SessionInfo, error) {
	rows, err := s.store.Queries().ListCardSessionStatesByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("read the sessions of project %s: %w", projectID, err)
	}
	infos := make(map[string]projects.SessionInfo, len(rows))
	for _, row := range rows {
		infos[row.CardID] = projects.SessionInfo{State: protocol.SessionState(row.State), View: protocol.CardViewMode(row.ViewMode)}
	}
	return infos, nil
}
