package projects

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// DefaultColumns are the columns of a new board, in board order: one for each card state the
// screens show. A card in the "merging" state has no column, because it is shown in "ready".
func DefaultColumns() []protocol.CardState {
	return []protocol.CardState{
		protocol.CardStateBacklog, protocol.CardStatePlanning, protocol.CardStateWorking,
		protocol.CardStateNeeds, protocol.CardStateReview, protocol.CardStateReady, protocol.CardStateDone,
	}
}

// Board returns a project's columns and all of its cards, from one read, so they agree.
func (s *Service) Board(ctx context.Context, projectID string) (protocol.BoardSnapshot, error) {
	var board db.Board
	var rows []db.Card
	err := s.store.Read(ctx, func(q *db.Queries) error {
		var err error
		if board, err = q.GetBoardByProject(ctx, projectID); err != nil {
			return notFound(fmt.Errorf("read the board of project %s: %w", projectID, err), notFoundProject(projectID))
		}
		if rows, err = q.ListCardsByProject(ctx, projectID); err != nil {
			return fmt.Errorf("list the cards of project %s: %w", projectID, err)
		}
		return nil
	})
	if err != nil {
		return protocol.BoardSnapshot{}, err
	}
	var columns []protocol.CardState
	if err := json.Unmarshal([]byte(board.ColumnsJSON), &columns); err != nil {
		return protocol.BoardSnapshot{}, fmt.Errorf("read the columns of project %s: %w", projectID, err)
	}
	return protocol.BoardSnapshot{
		ProjectID:  projectID,
		Columns:    columns,
		Cards:      toCards(rows),
		ServerTime: protocol.NewTimestamp(s.now()),
	}, nil
}
