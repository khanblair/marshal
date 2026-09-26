package projects

import (
	"context"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The manual moves of docs/architecture.md section 6.1. A person drags a card, or uses its menu,
// and the daemon decides: the app may pre-check with the same rules to refuse a drop early, but it
// never moves a card on its own authority.
//
// These rules belong to this entry point only. SetState stays a plain state change, because the
// session manager uses it for changes that no rule governs (working on start, needs on failure).

// StartingWithoutAPlan is the "doing now" line a card gets when it is moved from planning to
// working, where the agent never wrote a plan.
const startingWithoutAPlan = "Starting without a plan"

// AddressingReviewComments is the "doing now" line a card gets when it is moved from review back
// to working.
const addressingReviewComments = "Addressing review comments"

// refusalMessage is the sentence a person reads when a move is refused. The words are the ones in
// architecture.md section 6.1 and must not drift from it.
//
// It is a switch rather than a package-level table because a mutable global is not allowed here
// (code-standards.md section 3), and seven short cases are easier to read against the spec than a
// map is.
func refusalMessage(reason protocol.MoveRefusalReason) string {
	switch reason {
	case protocol.MoveRefusalReasonFromDone:
		return "Done cards are merged. Fork the card to keep working on it."
	case protocol.MoveRefusalReasonToDone:
		return "Cards move to Done by themselves after they merge."
	case protocol.MoveRefusalReasonToNeeds:
		return "Cards move to Needs you by themselves when an agent is waiting on you."
	case protocol.MoveRefusalReasonNeedsPullRequest:
		return "In review needs an open pull request. The agent opens one when the work is ready."
	case protocol.MoveRefusalReasonNeedsReview:
		return "Ready to merge needs an approved review and passing checks."
	case protocol.MoveRefusalReasonChecksNotPassed:
		return "Checks haven't passed on this card yet, so it can't be ready to merge."
	case protocol.MoveRefusalReasonCardMerging:
		return "The Integrator is merging this card. Wait for the merge to finish."
	}
	return ""
}

// refusedMove builds the refusal for a rule, with the reason and the sentence of section 6.1.
func refusedMove(reason protocol.MoveRefusalReason) *protocol.Error {
	return protocol.Refused(refusalMessage(reason)).With("reason", string(reason))
}

// checkMove answers whether a manual move is allowed. It returns nil when it is, and the refusal
// when it is not. The rules are checked in the order of architecture.md section 6.1, and the first
// one that applies wins.
//
// A move to the column the card is already in is not a rule: it does nothing and succeeds, so it
// is checked before the rules (a drag that lands back on its own column must not show a refusal).
func checkMove(card protocol.Card, target protocol.CardState) *protocol.Error {
	if target == card.State {
		return nil
	}
	switch {
	case card.State == protocol.CardStateDone:
		return refusedMove(protocol.MoveRefusalReasonFromDone)
	case target == protocol.CardStateDone:
		return refusedMove(protocol.MoveRefusalReasonToDone)
	case target == protocol.CardStateNeeds:
		return refusedMove(protocol.MoveRefusalReasonToNeeds)
	case target == protocol.CardStateReview &&
		(card.PullRequest == nil || card.State == protocol.CardStateBacklog || card.State == protocol.CardStatePlanning):
		return refusedMove(protocol.MoveRefusalReasonNeedsPullRequest)
	case target == protocol.CardStateReady && card.State != protocol.CardStateReview:
		return refusedMove(protocol.MoveRefusalReasonNeedsReview)
	case target == protocol.CardStateReady && !checksPassed(card):
		return refusedMove(protocol.MoveRefusalReasonChecksNotPassed)
	case card.State == protocol.CardStateMerging:
		return refusedMove(protocol.MoveRefusalReasonCardMerging)
	}
	return nil
}

// checksPassed reports whether a card's checks have passed. Until Phase 5 and Phase 6 run real
// checks, the only card that has a CI state at all is one the fixture gave it, so a card with no
// CI data cannot be ready to merge.
func checksPassed(card protocol.Card) bool {
	return card.CI != nil && *card.CI == protocol.CIStatePassed
}

// doingNowAfterMove is the "doing now" line an allowed move leaves behind, or the card's current
// line when the move does not change it.
func doingNowAfterMove(card protocol.Card, target protocol.CardState) string {
	switch {
	case card.State == protocol.CardStateReview && target == protocol.CardStateWorking:
		return addressingReviewComments
	case card.State == protocol.CardStatePlanning && target == protocol.CardStateWorking:
		return startingWithoutAPlan
	default:
		return card.DoingNow
	}
}

// MoveCard carries out a manual move after the rules of section 6.1 allow it. A refused move is a
// *protocol.Error with the refused code, the reason, and the sentence the app shows, and the card
// is left exactly as it was.
func (s *Service) MoveCard(ctx context.Context, id string, in protocol.MoveCardRequest) (protocol.Card, error) {
	if !in.State.Valid() {
		return protocol.Card{}, protocol.InvalidArgument("Marshal does not know that column.").With("state", string(in.State))
	}
	if in.State == protocol.CardStateMerging {
		return protocol.Card{}, protocol.InvalidArgument(
			"That column belongs to the Integrator. A card is shown in Ready to merge while it merges.").With("state", string(in.State))
	}
	card, err := s.Card(ctx, id)
	if err != nil {
		return protocol.Card{}, err
	}
	if refusal := checkMove(card, in.State); refusal != nil {
		s.log.Info("refused a manual move", "project_id", card.ProjectID, "card_id", id,
			"from", card.State, "to", in.State, "reason", refusal.Details["reason"])
		return protocol.Card{}, refusal
	}
	if card.State == in.State {
		return card, nil
	}
	return s.applyMove(ctx, card, in.State)
}

// applyMove writes the new state and the effects of an allowed move, and publishes card.moved. The
// caller has already checked the rules.
func (s *Service) applyMove(ctx context.Context, card protocol.Card, target protocol.CardState) (protocol.Card, error) {
	var after db.Card
	err := s.store.Write(ctx, func(q *db.Queries) error {
		row, err := q.GetCard(ctx, card.ID)
		if err != nil {
			return notFound(fmt.Errorf("read card %s: %w", card.ID, err), notFoundCard(card.ID))
		}
		after = row
		after.State, after.UpdatedAt = string(target), s.now().UnixMilli()
		after.DoingNow = doingNowAfterMove(card, target)
		// A card that leaves the needs state has waited as long as it is going to.
		if target != protocol.CardStateNeeds {
			after.NeedsReasonKind, after.NeedsReasonText = "", ""
			after.NeedsSince = nil
		}
		return updateCardRow(ctx, q, after)
	})
	if err != nil {
		return protocol.Card{}, err
	}
	moved, err := s.cardWithLabels(ctx, after)
	if err != nil {
		return protocol.Card{}, err
	}
	s.log.Info("moved a card", "project_id", moved.ProjectID, "card_id", moved.ID,
		"from", card.State, "to", target)
	s.publish(protocol.ProjectTopic(moved.ProjectID), protocol.EventTypeCardMoved,
		protocol.CardMovedEventData{Card: moved, From: card.State}, true)
	return moved, nil
}

// updateCardRow writes every field of a card row back. It is the one place a card row is written,
// so an update and a move cannot disagree about what the columns mean.
func updateCardRow(ctx context.Context, q *db.Queries, row db.Card) error {
	if _, err := q.UpdateCardFields(ctx, db.UpdateCardFieldsParams{
		Number: row.Number, State: row.State, Title: row.Title, Body: row.Body,
		AgentKind: row.AgentKind, Model: row.Model,
		Thinking: row.Thinking, PermissionMode: row.PermissionMode, Role: row.Role,
		Package: row.Package, PlannedStart: row.PlannedStart, PlannedEnd: row.PlannedEnd,
		Due: row.Due, ActualStart: row.ActualStart, ActualEnd: row.ActualEnd,
		PullRequestNumber: row.PullRequestNumber, PullRequestUrl: row.PullRequestUrl,
		CiState: row.CiState, ContextUsed: row.ContextUsed,
		NeedsReasonKind: row.NeedsReasonKind, NeedsReasonText: row.NeedsReasonText,
		NeedsSince: row.NeedsSince, DoingNow: row.DoingNow, Paused: row.Paused,
		Pinned: row.Pinned, UpdatedAt: row.UpdatedAt, ID: row.ID,
	}); err != nil {
		return fmt.Errorf("update card %s: %w", row.ID, err)
	}
	return nil
}
