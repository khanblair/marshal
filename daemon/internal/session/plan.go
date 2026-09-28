package session

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// PlanStore reads and writes the plans a card's chat holds. The history module implements it
// (history.Store), the way it implements HistoryRecorder.
//
// A plan is a message in the card's chat and not a table of its own
// (docs/marshal-product-scope.md 10.3): the person reads it beside the work it plans. An answer to
// a plan is stored as another plan message whose state says how it was answered, so nothing a
// person already read is rewritten behind them.
type PlanStore interface {
	// LatestPlan returns the card's newest plan message, and false when the card has none. It is
	// the plan a person is answering.
	LatestPlan(ctx context.Context, cardID string) (history.Plan, bool, error)
	// AppendPlan stores a plan as the card's newest plan message and returns it as stored, with
	// the id an answer names.
	AppendPlan(ctx context.Context, cardID, sessionID string, plan history.Plan) (history.Plan, error)
}

// planMaxSteps is how many steps a plan may hold. A plan is a few lines a person reads before the
// work starts (docs/marshal-product-scope.md 10.3), so a very long one is a wrong plan rather than
// a long one.
const planMaxSteps = 200

// planMaxStepRunes is how long one step of a plan may be.
const planMaxStepRunes = 1000

// ApprovePlan answers the plan a card is waiting on: the plan is stored as approved, the card
// starts working on it, and its permission mode moves off plan-only, which is what lets the agent
// edit files rather than plan them.
//
// The card is the plan's card and the plan is the one it is waiting on. A card with no plan, or a
// plan that has already been answered, is refused rather than answered twice.
func (m *Manager) ApprovePlan(ctx context.Context, cardID string) (protocol.Card, error) {
	return m.answerPlan(ctx, cardID, planAnswer{
		state:    protocol.ChatPlanStateApproved,
		summary:  "You approved the plan",
		act:      audit.ActionApprove,
		perm:     protocol.PermissionModeAutoEdits,
		doing:    "Starting work on the plan",
		toState:  protocol.CardStateWorking,
		keepLine: true,
	})
}

// RejectPlan answers the plan a card is waiting on by sending it back: the plan is stored as
// rejected and the card returns to planning, where the agent writes another one.
func (m *Manager) RejectPlan(ctx context.Context, cardID string) (protocol.Card, error) {
	return m.answerPlan(ctx, cardID, planAnswer{
		state:   protocol.ChatPlanStateRejected,
		summary: "You rejected the plan",
		act:     audit.ActionDeny,
		doing:   "Reworking the plan",
		toState: protocol.CardStatePlanning,
	})
}

// EditPlan replaces the steps of the plan a card is waiting on with the ones a person left, and
// stores it as edited: still waiting for an answer, but no longer the plan the agent wrote.
//
// Only the steps are editable. The files, the risks, and the checks are the agent's own reading of
// the work, and a person changing them would be writing the agent's plan for it.
func (m *Manager) EditPlan(ctx context.Context, cardID string, steps []string) (protocol.Card, error) {
	edited, err := cleanSteps(steps)
	if err != nil {
		return protocol.Card{}, err
	}
	plan, card, err := m.answerablePlan(ctx, cardID)
	if err != nil {
		return protocol.Card{}, err
	}
	plan.Steps, plan.State = edited, protocol.ChatPlanStateEdited
	return m.storePlanAnswer(ctx, card, plan, planAnswer{
		summary: planEditSummary(len(plan.Steps)),
		act:     audit.ActionPlanEdited,
	})
}

// planEditSummary is the line an edit leaves in the activity list. The count of steps is in the line
// itself because an activity row only draws a second line when it stands for a stored tool call
// (internal/history/wire.go, activityResultOf), and an edit is a system note rather than one. The
// count would otherwise be lost, and how long the plan now is is the point of an edit.
func planEditSummary(steps int) string {
	if steps == 1 {
		return "You edited the plan to 1 step"
	}
	return fmt.Sprintf("You edited the plan to %d steps", steps)
}

// planAnswer is one answer to a plan: what the plan becomes, the activity row and the audit row it
// leaves behind, and what it does to the card.
type planAnswer struct {
	// state is where the plan stands after the answer.
	state protocol.ChatPlanState
	// summary is the one line the activity list shows.
	summary string
	// act is the audit verb.
	act string
	// perm is the card's permission mode after the answer, or empty to leave it.
	perm protocol.PermissionMode
	// doing replaces the "doing now" line, or is empty to clear it.
	doing string
	// toState is where the card moves, or empty to leave it where it is.
	toState protocol.CardState
	// keepLine says the first step of the plan replaces an empty "doing now" line, so a card that
	// started work says what it started on.
	keepLine bool
}

// answerPlan answers the plan a card is waiting on with one decision.
func (m *Manager) answerPlan(ctx context.Context, cardID string, answer planAnswer) (protocol.Card, error) {
	plan, card, err := m.answerablePlan(ctx, cardID)
	if err != nil {
		return protocol.Card{}, err
	}
	plan.State = answer.state
	return m.storePlanAnswer(ctx, card, plan, answer)
}

// answerablePlan reads the card and the plan it is waiting on, and refuses an answer to anything
// else: a card that is not there, a card with no plan, and a plan that has already been answered.
func (m *Manager) answerablePlan(ctx context.Context, cardID string) (history.Plan, protocol.Card, error) {
	card, err := m.projects.Card(ctx, cardID)
	if err != nil {
		return history.Plan{}, protocol.Card{}, err
	}
	plan, ok, err := m.plans.LatestPlan(ctx, cardID)
	if err != nil {
		return history.Plan{}, protocol.Card{}, fmt.Errorf("read the plan of card %s: %w", cardID, err)
	}
	if !ok {
		return history.Plan{}, protocol.Card{}, protocol.NotFound("plan").
			With("cardId", cardID).WithCause(errNoPlan)
	}
	if !planAwaiting(plan.State) {
		return history.Plan{}, protocol.Card{}, protocol.Conflict("That plan has already been answered.").
			With("cardId", cardID).With("state", string(plan.State))
	}
	return plan, card, nil
}

// planAwaiting reports whether a plan is still waiting for a person to answer it. A plan an agent
// wrote and a plan a person has already edited both are.
func planAwaiting(state protocol.ChatPlanState) bool {
	return state == protocol.ChatPlanStateWaiting || state == protocol.ChatPlanStateEdited
}

// errNoPlan is the cause carried by the not-found answer for a card that has no plan. It says in
// the log why the card was asked about at all.
var errNoPlan = errors.New("the card has no plan")

// storePlanAnswer stores the answered plan, writes what it did, and carries the answer out on the
// card. The plan message is stored first: it is the thing a person asked for, and a failure to
// move the card afterwards must not leave the plan looking unanswered.
func (m *Manager) storePlanAnswer(ctx context.Context, card protocol.Card, plan history.Plan, answer planAnswer) (protocol.Card, error) {
	stored, err := m.plans.AppendPlan(ctx, card.ID, plan.SessionID, plan)
	if err != nil {
		return protocol.Card{}, err
	}
	m.recordPlanAnswer(card, stored, answer)
	m.auditPlanAnswer(ctx, card, stored, answer)
	m.announcePlan(card.ID, stored)
	return m.carryOut(ctx, card, stored, answer)
}

// carryOut applies an answer to the card itself. A card the answer does not move is only
// re-read, so the caller always answers with the card as it now stands.
func (m *Manager) carryOut(ctx context.Context, card protocol.Card, plan history.Plan, answer planAnswer) (protocol.Card, error) {
	change := protocol.UpdateCardRequest{}
	if answer.perm != "" {
		perm := answer.perm
		change.PermissionMode = &perm
	}
	doing := answer.doing
	if answer.keepLine && len(plan.Steps) > 0 && card.DoingNow == "" {
		doing = plan.Steps[0]
	}
	if doing != "" {
		change.DoingNow = &doing
	}
	if change.PermissionMode != nil || change.DoingNow != nil {
		updated, err := m.projects.UpdateCard(ctx, card.ID, change)
		if err != nil {
			return protocol.Card{}, err
		}
		card = updated
	}
	if answer.toState == "" || card.State == answer.toState {
		return card, nil
	}
	return m.projects.SetState(ctx, card.ID, answer.toState)
}

// recordPlanAnswer writes the activity row one answer leaves: an approved or rejected plan is an
// approval, and an edited one is a note about the session, exactly as the prototype's activity
// list draws them.
func (m *Manager) recordPlanAnswer(card protocol.Card, plan history.Plan, answer planAnswer) {
	if m.cfg.History == nil {
		return
	}
	record := history.Record{Kind: history.KindApproval, State: history.StateOK, Summary: answer.summary}
	if answer.act == audit.ActionDeny {
		record.State = history.StateFailed
	}
	if answer.act == audit.ActionPlanEdited {
		record.Kind = history.KindSystem
	}
	if err := m.cfg.History.Append(m.ctx, card.ID, plan.SessionID, []history.Record{record}); err != nil {
		m.log.Error("could not store an answer to a plan", "card_id", card.ID, "error", err)
	}
}

// auditPlanAnswer records who answered a plan, for the same reason an approval is recorded: an
// answer on a person's behalf is a thing a person may need to account for.
func (m *Manager) auditPlanAnswer(ctx context.Context, card protocol.Card, plan history.Plan, answer planAnswer) {
	if m.cfg.Audit == nil {
		return
	}
	m.cfg.Audit.LogAndForget(ctx, audit.Entry{
		Actor: audit.ActorPerson, Action: answer.act, Target: plan.EventID, SessionID: plan.SessionID,
		Detail: map[string]any{cardIDDetailKey: card.ID, "state": string(plan.State)},
	})
}

// announcePlan publishes plan.updated on the card's topic, so every view of the card's plan
// follows one answer without reading the chat back.
func (m *Manager) announcePlan(cardID string, plan history.Plan) {
	m.bus.Publish(string(protocol.CardTopic(cardID)), string(protocol.EventTypePlanUpdated),
		protocol.PlanUpdatedEventData{
			CardID: cardID, MessageID: plan.EventID, Plan: plan.Wire(), At: protocol.NewTimestamp(m.cfg.Now().UTC()),
		}, true)
}

// cleanSteps checks the steps a person left in a plan, dropping the empty ones. A plan needs at
// least one step: a plan with none is a plan nobody can read, and saving one would leave the card
// waiting on nothing.
func cleanSteps(steps []string) ([]string, error) {
	if len(steps) > planMaxSteps {
		return nil, protocol.InvalidArgument(fmt.Sprintf("A plan holds at most %d steps.", planMaxSteps)).
			With("steps", strconv.Itoa(len(steps)))
	}
	cleaned := make([]string, 0, len(steps))
	for _, step := range steps {
		line := strings.TrimSpace(step)
		if line == "" {
			continue
		}
		if len([]rune(line)) > planMaxStepRunes {
			return nil, protocol.InvalidArgument(fmt.Sprintf("A step holds at most %d characters.", planMaxStepRunes)).
				With("step", strconv.Itoa(len([]rune(line))))
		}
		cleaned = append(cleaned, line)
	}
	if len(cleaned) == 0 {
		return nil, protocol.InvalidArgument("A plan needs at least one step.").With("steps", strconv.Itoa(len(steps)))
	}
	return cleaned, nil
}
