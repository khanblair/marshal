package projects

import (
	"context"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The quality gate before a card moves to In review (docs/architecture.md section 17.1, checklist
// B5.8): the card's own changes are checked for code smells, and a finding that blocks keeps the
// card where it is and goes back to the agent that wrote the code.
//
// It is consulted by both ways a card reaches In review: the person's own move of section 6.1, and
// the daemon's move after an agent opens a pull request (SetState, used by internal/pullrequest).
// A refusal leaves the card exactly as it was, so the only trace is the sentence the person reads
// and the message the agent was sent.

// checkQuality asks the quality module whether a card may move to In review, and answers the
// refusal when it may not. It answers nil when nothing blocks the card, when no quality module is
// wired at all, and when the checks could not run: a smell checker that could not start must never
// be the thing that keeps a person's work out of review, so that case is logged and the move goes
// ahead.
func (s *Service) checkQuality(ctx context.Context, cardID string) *protocol.Error {
	gate := s.reviewGate()
	if gate == nil {
		return nil
	}
	blocking, err := gate.BlockingFindings(ctx, cardID)
	if err != nil {
		s.log.Warn("could not check a card's code smells before review", "card_id", cardID, "error", err)
		return nil
	}
	if blocking == 0 {
		return nil
	}
	return refusedMove(protocol.MoveRefusalReasonQualityBlocking)
}
