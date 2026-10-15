package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// finishCardInput is finish_card's arguments.
type finishCardInput struct {
	// Summary is one sentence saying what was done. It is shown with the card.
	Summary string `json:"summary"`
}

// finishCardOut is finish_card's answer.
type finishCardOut struct {
	// Message says what happens next, so the agent does not wait on it.
	Message string `json:"message"`
}

// finish_card says the card's work is done. The merge flow starts when the agent's turn ends, so a
// commit made after this call, in the same turn, is still part of what is reviewed.
func (s *Server) finishCard(ctx context.Context, _ *mcp.CallToolRequest, in finishCardInput) (*mcp.CallToolResult, finishCardOut, error) {
	if err := s.allow("finish_card", kindWrite); err != nil {
		return nil, finishCardOut{}, err
	}
	if s.deps.Finisher == nil {
		return nil, finishCardOut{}, errors.New("finishing a card is not available here")
	}
	summary := strings.TrimSpace(in.Summary)
	if summary == "" {
		return nil, finishCardOut{}, errors.New("finish_card needs a summary: one sentence saying what you did")
	}
	if err := s.deps.Finisher.Finish(ctx, s.identity.CardID, summary); err != nil {
		return nil, finishCardOut{}, fmt.Errorf("finish the card: %w", err)
	}
	return nil, finishCardOut{Message: "Noted. When this turn ends Marshal reviews your changes, runs the checks, and merges them."}, nil
}
