package history_test

import (
	"context"
	"slices"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A plan is a message in a card's chat and not a table of its own
// (docs/marshal-product-scope.md 10.3), so these tests read it back through the same store that
// pages the chat (docs/backend-checklist.md B5.2, inventory N6).

// samplePlan is a plan an agent wrote and nobody has answered yet, with every part a person reads.
func samplePlan() history.Plan {
	return history.Plan{
		State:  protocol.ChatPlanStateWaiting,
		Steps:  []string{"Read the router", "Add the route"},
		Files:  []string{"internal/api/router.go"},
		Risks:  []string{"A route that already exists is refused"},
		Checks: []string{"go test ./internal/api/"},
	}
}

func TestAppendPlanAnswersTheMessageItWrote(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	stored, err := e.history.AppendPlan(ctx, e.cardID, e.sessionID, samplePlan())
	if err != nil {
		t.Fatalf("AppendPlan: %v", err)
	}
	if stored.EventID == "" {
		t.Fatal("the stored plan has no message id, so nothing can name it in plan.updated")
	}

	// The plan a person is answering reads back as the one that was written, with the same id.
	got, ok, err := e.history.LatestPlan(ctx, e.cardID)
	if err != nil {
		t.Fatalf("LatestPlan: %v", err)
	}
	if !ok {
		t.Fatal("LatestPlan says the card has no plan after a plan was stored")
	}
	want := samplePlan()
	want.EventID, want.SessionID = stored.EventID, e.sessionID
	if !samePlan(got, want) {
		t.Errorf("the plan read back = %+v, want %+v", got, want)
	}
}

// samePlan reports whether two plans carry the same thing. A plan holds lists, so it is compared
// field by field rather than as a whole.
func samePlan(a, b history.Plan) bool {
	return a.EventID == b.EventID && a.SessionID == b.SessionID && a.State == b.State &&
		slices.Equal(a.Steps, b.Steps) && slices.Equal(a.Files, b.Files) &&
		slices.Equal(a.Risks, b.Risks) && slices.Equal(a.Checks, b.Checks)
}

func TestLatestPlanOfACardWithNoPlan(t *testing.T) {
	e := newEnv(t)
	plan, ok, err := e.history.LatestPlan(context.Background(), e.cardID)
	if err != nil {
		t.Fatalf("LatestPlan: %v", err)
	}
	if ok {
		t.Errorf("LatestPlan = %+v, true, want no plan", plan)
	}
}

// TestLatestPlanIsTheNewestOne is the rule the reader draws by: a plan is replaced rather than
// edited in place, so the newest plan message is the plan as it stands now.
func TestLatestPlanIsTheNewestOne(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	first, err := e.history.AppendPlan(ctx, e.cardID, e.sessionID, samplePlan())
	if err != nil {
		t.Fatalf("AppendPlan: %v", err)
	}
	edited := samplePlan()
	edited.State = protocol.ChatPlanStateEdited
	edited.Steps = []string{"Only this step"}
	second, err := e.history.AppendPlan(ctx, e.cardID, e.sessionID, edited)
	if err != nil {
		t.Fatalf("AppendPlan: %v", err)
	}

	got, ok, err := e.history.LatestPlan(ctx, e.cardID)
	if err != nil {
		t.Fatalf("LatestPlan: %v", err)
	}
	if !ok {
		t.Fatal("LatestPlan says the card has no plan")
	}
	if got.EventID != second.EventID {
		t.Errorf("the plan read back is message %q, want the newest %q (the older is %q)",
			got.EventID, second.EventID, first.EventID)
	}
	if got.State != protocol.ChatPlanStateEdited || len(got.Steps) != 1 {
		t.Errorf("the plan read back = %+v, want the edited one", got)
	}
}

// TestAPlanStateTheFlowCannotProduceReadsAsWaiting covers a detail stored by a writer that is not
// this one: a plan nobody has answered is where a plan stands, so an unknown state reads as that
// rather than as a state the app does not know.
func TestAPlanStateTheFlowCannotProduceReadsAsWaiting(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	plan := samplePlan()
	plan.State = protocol.ChatPlanState("halfway")

	if _, err := e.history.AppendPlan(ctx, e.cardID, e.sessionID, plan); err != nil {
		t.Fatalf("AppendPlan: %v", err)
	}
	got, ok, err := e.history.LatestPlan(ctx, e.cardID)
	if err != nil {
		t.Fatalf("LatestPlan: %v", err)
	}
	if !ok || got.State != protocol.ChatPlanStateWaiting {
		t.Errorf("the plan read back = %+v (%v), want the waiting state", got, ok)
	}
}
