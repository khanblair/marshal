package history

import (
	"context"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Plan is a card's plan as the daemon reads and writes it: the parts a person reviews and where
// the plan stands (docs/marshal-product-scope.md 10.3, docs/backend-inventory.md 4.3). A plan is
// a message in the card's chat and not a table of its own: the person reads it beside the work it
// plans, and it is paged back with the rest of the chat.
//
// A plan is replaced rather than edited in place (kinds.go, KindPlan): an answer to a plan is
// another plan message whose state says how it was answered, so the stored message a person
// answered is never rewritten behind them.
type Plan struct {
	// EventID is the stored message's own id. It is empty for a plan that has not been written
	// yet, and it is what an answer names in plan.updated.
	EventID string
	// SessionID is the session that wrote the message, so an answer is stored against the same
	// one and a card's chat does not appear to change hands.
	SessionID string
	// State is where the plan stands.
	State protocol.ChatPlanState
	// Steps are the plan's steps, in order.
	Steps []string
	// Files are the files the plan says it will touch.
	Files []string
	// Risks are the risks the plan names.
	Risks []string
	// Checks are the checks the plan says it will run.
	Checks []string
}

// Wire is the plan as a client reads it, which is also the shape plan.updated carries. It is
// exported because the session manager answers a plan with the plan message it just stored
// (internal/session/plan.go).
func (p Plan) Wire() protocol.ChatPlan {
	return protocol.ChatPlan{
		State:  planStateOf(string(p.State)),
		Steps:  listOrEmpty(p.Steps),
		Files:  listOrEmpty(p.Files),
		Risks:  listOrEmpty(p.Risks),
		Checks: listOrEmpty(p.Checks),
	}
}

// storeDetail is the plan as one message holds it. The steps keep no status of their own: a plan
// message's steps are what the person reads and answers, so a status stored by the agent's own
// update is not carried onto the plan the person sees.
func (p Plan) storeDetail() planDetail {
	stored := make([]planStep, len(p.Steps))
	for i, step := range p.Steps {
		stored[i] = planStep{Text: step}
	}
	return planDetail{
		State:  string(planStateOf(string(p.State))),
		Steps:  stored,
		Files:  p.Files,
		Risks:  p.Risks,
		Checks: p.Checks,
	}
}

// planStateOf maps the state a stored plan carries to the wire's. A state the plan flow cannot
// produce reads as waiting, which is where a plan nobody has answered stands.
func planStateOf(state string) protocol.ChatPlanState {
	asked := protocol.ChatPlanState(state)
	if asked.Valid() {
		return asked
	}
	return protocol.ChatPlanStateWaiting
}

// AppendPlan stores a plan as the card's newest plan message, replacing the plan on screen (the
// newest plan message of a card is the one a chat draws). It is how both the agent's own plan and
// a person's answer to one are written, so a plan's history is the whole story of how it was
// answered.
//
// It returns the plan as stored, with the id of the message it was written under: that id is what
// the answer announces in plan.updated and what a later read pages back.
func (s *Store) AppendPlan(ctx context.Context, cardID, sessionID string, plan Plan) (Plan, error) {
	detail, err := encodeDetail(plan.storeDetail())
	if err != nil {
		return Plan{}, fmt.Errorf("store the plan of card %s: %w", cardID, err)
	}
	ids, err := s.append(ctx, cardOwner(cardID), sessionID, []Record{{
		Kind:    KindPlan,
		Summary: planSummary(len(plan.Steps)),
		Detail:  detail,
	}})
	if err != nil {
		return Plan{}, err
	}
	plan.EventID = ids[0]
	return plan, nil
}

// LatestPlan returns the card's newest plan message, and false when the card has none. It is the
// plan a person is answering: the newest plan message is the one the card's chat draws.
func (s *Store) LatestPlan(ctx context.Context, cardID string) (Plan, bool, error) {
	page, err := s.PageByKind(ctx, cardID, KindPlan, 0, 1)
	if err != nil {
		return Plan{}, false, err
	}
	if len(page.Events) == 0 {
		return Plan{}, false, nil
	}
	ev := page.Events[0]
	block, err := planOf(ev)
	if err != nil {
		return Plan{}, false, fmt.Errorf("read the plan of card %s: %w", cardID, err)
	}
	return Plan{
		EventID: ev.ID, SessionID: ev.SessionID, State: block.State,
		Steps: block.Steps, Files: block.Files, Risks: block.Risks, Checks: block.Checks,
	}, true, nil
}
