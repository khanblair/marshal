package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The plan routes (docs/backend-checklist.md B5.2, inventory N6): a person approves the plan a card
// waits on, rejects it, or changes its steps. A plan is a message in the card's chat and not a
// table of its own (docs/marshal-product-scope.md 10.3), so these tests write one through the same
// store the session manager reads, the way a plan-first agent's own plan arrives.

// planCard is a card in a fresh project, waiting in Planning in plan-only mode, which is where a
// card stands when its agent has written a plan and nobody has answered it.
func planCard(t *testing.T, st *stack) protocol.Card {
	t.Helper()
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Plan the route")
	mode := protocol.PermissionModePlan
	st.do(http.MethodPatch, "/v1/cards/"+card.ID, protocol.UpdateCardRequest{PermissionMode: &mode}).
		want(t, http.StatusOK)
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/move", protocol.MoveCardRequest{State: protocol.CardStatePlanning}).
		want(t, http.StatusOK)
	return st.getCard(card.ID)
}

// planSession is the session row a stored plan message points at. A card no session has run for has
// no row, and every stored event names one, so the row is made first.
func planSession(t *testing.T, st *stack, cardID string) string {
	t.Helper()
	ctx := context.Background()
	session, err := st.store.Queries().GetSessionByCard(ctx, cardID)
	if store.IsNotFound(err) {
		now := time.Now().UTC()
		session.ID = "session-" + cardID
		err = st.store.Write(ctx, func(q *db.Queries) error {
			return q.CreateCardSession(ctx, db.CreateCardSessionParams{
				ID: session.ID, CardID: cardID, AgentKind: "claude", State: "awake",
				LastActiveAt: now.UnixMilli(), CreatedAt: now.UnixMilli(), UpdatedAt: now.UnixMilli(),
			})
		})
	}
	if err != nil {
		t.Fatalf("make the session row of card %s: %v", cardID, err)
	}
	return session.ID
}

// addPlan stores a plan as the card's newest plan message and answers with it as stored.
func addPlan(t *testing.T, st *stack, cardID string, plan history.Plan) history.Plan {
	t.Helper()
	plan.SessionID = planSession(t, st, cardID)
	stored, err := st.hist.AppendPlan(context.Background(), cardID, plan.SessionID, plan)
	if err != nil {
		t.Fatalf("store the plan of card %s: %v", cardID, err)
	}
	return stored
}

// cardPlan reads the plan a card's chat draws: the newest plan message of the chat route.
func cardPlan(t *testing.T, st *stack, cardID string) *protocol.ChatPlan {
	t.Helper()
	page := decode[protocol.Page[protocol.ChatMessage]](t,
		st.do(http.MethodGet, "/v1/cards/"+cardID+"/messages", nil).want(t, http.StatusOK))
	var found *protocol.ChatPlan
	for i, item := range page.Items {
		if item.Kind != protocol.ChatMessageKindPlan {
			continue
		}
		if found != nil {
			t.Fatalf("the chat draws more than one plan message: %+v", page.Items)
		}
		found = page.Items[i].Plan
	}
	if found == nil {
		t.Fatal("the card's chat draws no plan")
	}
	return found
}

// firstActivity is the newest row of a card's activity list.
func firstActivity(t *testing.T, st *stack, cardID string) protocol.ActivityItem {
	t.Helper()
	page := decode[protocol.Page[protocol.ActivityItem]](t,
		st.do(http.MethodGet, "/v1/cards/"+cardID+"/activity", nil).want(t, http.StatusOK))
	if len(page.Items) == 0 {
		t.Fatal("the card's activity list is empty")
	}
	return page.Items[0]
}

// nextOfType reads bus events until one of the type arrives, and returns it.
func nextOfType(t *testing.T, sub *events.Subscription, typ protocol.EventType) events.Event {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-sub.C():
			if !ok {
				t.Fatalf("the subscription closed while waiting for %s", typ)
			}
			if ev.Type == string(typ) {
				return ev
			}
		case <-timeout:
			t.Fatalf("no %s event arrived", typ)
			return events.Event{}
		}
	}
}

func TestPlanApprovalStartsTheWork(t *testing.T) {
	st := newStack(t)
	card := planCard(t, st)
	addPlan(t, st, card.ID, history.Plan{
		State:  protocol.ChatPlanStateWaiting,
		Steps:  []string{"Read the router", "Add the route"},
		Files:  []string{"internal/api/router.go"},
		Risks:  []string{"A route that already exists is refused"},
		Checks: []string{"go test ./internal/api/"},
	})
	sub := st.bus.Subscribe(events.AllTopics())
	defer sub.Close()

	after := decode[protocol.Card](t,
		st.do(http.MethodPost, "/v1/cards/"+card.ID+"/plan/approve", nil).want(t, http.StatusOK))
	// Approving starts the work and takes the card out of plan-only, which is what lets its agent
	// change files rather than plan them. This is why a card waits in Planning until it is approved.
	if after.State != protocol.CardStateWorking {
		t.Errorf("the card after approval is %q, want working", after.State)
	}
	if after.PermissionMode != protocol.PermissionModeAutoEdits {
		t.Errorf("the card's permission mode after approval = %q, want auto-edits", after.PermissionMode)
	}
	if after.DoingNow != "Read the router" {
		t.Errorf("the card's doing-now line after approval = %q, want the plan's first step", after.DoingNow)
	}

	// The plan reads as approved and keeps every part the person reviewed.
	plan := cardPlan(t, st, card.ID)
	if plan.State != protocol.ChatPlanStateApproved {
		t.Errorf("the plan after approval = %+v, want approved", plan)
	}
	if len(plan.Steps) != 2 || plan.Steps[0] != "Read the router" || plan.Steps[1] != "Add the route" {
		t.Errorf("the approved plan's steps = %v, want the two the agent wrote", plan.Steps)
	}
	if len(plan.Files) != 1 || len(plan.Risks) != 1 || len(plan.Checks) != 1 {
		t.Errorf("the approved plan = %+v, want its files, risks, and checks kept", plan)
	}

	// The answer is one activity row: an approval that ended well. The plan message itself is a chat
	// message and never a row, so a card's plan cannot flood the activity list.
	page := decode[protocol.Page[protocol.ActivityItem]](t,
		st.do(http.MethodGet, "/v1/cards/"+card.ID+"/activity", nil).want(t, http.StatusOK))
	if len(page.Items) != 1 {
		t.Fatalf("the activity list after approval = %+v, want one row", page.Items)
	}
	row := page.Items[0]
	if row.Kind != protocol.ActivityKindApproval || row.State != protocol.ActivityStateOK {
		t.Errorf("the activity row = %+v, want an approval that ended ok", row)
	}
	if row.Text != "You approved the plan" {
		t.Errorf("the activity row's line = %q, want the approval", row.Text)
	}

	// plan.updated carries the plan as it now stands, on the card's own topic, so every view of the
	// same plan follows one answer without reading the chat back.
	ev := nextOfType(t, sub, protocol.EventTypePlanUpdated)
	if ev.Topic != string(protocol.CardTopic(card.ID)) {
		t.Errorf("plan.updated arrived on %q, want the card's topic", ev.Topic)
	}
	if !ev.Critical {
		t.Error("plan.updated is not critical, so a slow client could miss an answer to a plan")
	}
	data, ok := ev.Data.(protocol.PlanUpdatedEventData)
	if !ok {
		t.Fatalf("plan.updated carries %T, want protocol.PlanUpdatedEventData", ev.Data)
	}
	if data.CardID != card.ID || data.MessageID == "" {
		t.Errorf("plan.updated = %+v, want the card and the message it changed", data)
	}
	if data.Plan.State != protocol.ChatPlanStateApproved || len(data.Plan.Steps) != 2 {
		t.Errorf("the plan in plan.updated = %+v, want the approved one", data.Plan)
	}
}

func TestPlanRejectionSendsThePlanBack(t *testing.T) {
	st := newStack(t)
	card := planCard(t, st)
	addPlan(t, st, card.ID, history.Plan{State: protocol.ChatPlanStateWaiting, Steps: []string{"A", "B"}})

	after := decode[protocol.Card](t,
		st.do(http.MethodPost, "/v1/cards/"+card.ID+"/plan/reject", nil).want(t, http.StatusOK))
	// The card goes back to planning, where the agent writes another plan, and its permission mode
	// is left alone: a card whose plan was refused is not one that may change files.
	if after.State != protocol.CardStatePlanning {
		t.Errorf("the card after rejection is %q, want planning", after.State)
	}
	if after.PermissionMode != protocol.PermissionModePlan {
		t.Errorf("the card's permission mode after rejection = %q, want plan", after.PermissionMode)
	}
	if after.DoingNow != "Reworking the plan" {
		t.Errorf("the card's doing-now line after rejection = %q, want reworking", after.DoingNow)
	}
	if plan := cardPlan(t, st, card.ID); plan.State != protocol.ChatPlanStateRejected {
		t.Errorf("the plan after rejection = %+v, want rejected", plan)
	}
	row := firstActivity(t, st, card.ID)
	if row.Kind != protocol.ActivityKindApproval || row.State != protocol.ActivityStateFailed {
		t.Errorf("the activity row = %+v, want an approval that failed", row)
	}
	if row.Text != "You rejected the plan" {
		t.Errorf("the activity row's line = %q, want the rejection", row.Text)
	}
}

func TestPlanEditReplacesTheStepsAndLeavesItWaiting(t *testing.T) {
	st := newStack(t)
	card := planCard(t, st)
	addPlan(t, st, card.ID, history.Plan{
		State:  protocol.ChatPlanStateWaiting,
		Steps:  []string{"Read the router", "Add the route"},
		Files:  []string{"internal/api/router.go"},
		Risks:  []string{"A route that already exists is refused"},
		Checks: []string{"go test ./internal/api/"},
	})

	after := decode[protocol.Card](t,
		st.do(http.MethodPut, "/v1/cards/"+card.ID+"/plan", protocol.EditPlanRequest{
			Steps: []string{"Read the router", "   ", "Add the route, then the test"},
		}).want(t, http.StatusOK))
	// An edited plan is still waiting: a person changed what the agent proposed rather than
	// answering it, so the card is left exactly where it was.
	if after.State != protocol.CardStatePlanning {
		t.Errorf("the card after an edit is %q, want planning", after.State)
	}
	if after.PermissionMode != protocol.PermissionModePlan {
		t.Errorf("the card's permission mode after an edit = %q, want plan", after.PermissionMode)
	}
	plan := cardPlan(t, st, card.ID)
	if plan.State != protocol.ChatPlanStateEdited {
		t.Errorf("the plan after an edit = %+v, want edited", plan)
	}
	// The blank step is dropped, the way the plan's own writer drops it.
	if len(plan.Steps) != 2 || plan.Steps[0] != "Read the router" || plan.Steps[1] != "Add the route, then the test" {
		t.Errorf("the edited plan's steps = %v, want the two a person left", plan.Steps)
	}
	// The files, the risks, and the checks are the agent's own reading of the work and are kept.
	if len(plan.Files) != 1 || len(plan.Risks) != 1 || len(plan.Checks) != 1 {
		t.Errorf("the edited plan = %+v, want the agent's files, risks, and checks kept", plan)
	}
	// One activity row is left behind. An edit is the session's own note rather than a tool call, so
	// it is drawn as the prototype's "any other tool call or system note" kind, and its count of
	// steps is in the line: a row carries a second line only when it stands for a stored tool call,
	// and nothing here did (internal/history/wire.go, activityResultOf).
	row := firstActivity(t, st, card.ID)
	if row.Kind != protocol.ActivityKindTool || row.State != protocol.ActivityStateOK {
		t.Errorf("the activity row = %+v, want a note that ended ok", row)
	}
	if row.Text != "You edited the plan to 2 steps" {
		t.Errorf("the activity row's line = %q, want the edit and its step count", row.Text)
	}
	if row.Result != "" {
		t.Errorf("the activity row's second line = %q, want none: an edit is not a tool call", row.Result)
	}

	// An edited plan is still waiting, so it can be edited again: this is what keeps a person at the
	// plan until they approve it, and it is why the count is in the line rather than only in the plan.
	st.do(http.MethodPut, "/v1/cards/"+card.ID+"/plan", protocol.EditPlanRequest{
		Steps: []string{"Read the router"},
	}).want(t, http.StatusOK)
	plan = cardPlan(t, st, card.ID)
	if plan.State != protocol.ChatPlanStateEdited || len(plan.Steps) != 1 || plan.Steps[0] != "Read the router" {
		t.Errorf("the plan after a second edit = %+v, want the one step a person left", plan)
	}
	if row = firstActivity(t, st, card.ID); row.Text != "You edited the plan to 1 step" {
		t.Errorf("the activity row's line after a one-step edit = %q, want the count in the singular", row.Text)
	}
	if after := st.getCard(card.ID); after.State != protocol.CardStatePlanning {
		t.Errorf("the card after a second edit is %q, want planning", after.State)
	}
}

func TestPlanRoutesRefuseWhatTheyCannotAnswer(t *testing.T) {
	st := newStack(t)

	// A card with no plan has nothing to answer, whichever answer is asked for.
	card := planCard(t, st)
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/plan/approve", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/plan/reject", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodPut, "/v1/cards/"+card.ID+"/plan", protocol.EditPlanRequest{Steps: []string{"A"}}).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)

	// A plan that has already been answered is refused rather than answered twice.
	addPlan(t, st, card.ID, history.Plan{State: protocol.ChatPlanStateWaiting, Steps: []string{"A"}})
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/plan/approve", nil).want(t, http.StatusOK)
	for _, call := range []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPost, "/plan/approve", nil},
		{http.MethodPost, "/plan/reject", nil},
		{http.MethodPut, "/plan", protocol.EditPlanRequest{Steps: []string{"B"}}},
	} {
		got := st.do(call.method, "/v1/cards/"+card.ID+call.path, call.body).
			apiError(t, http.StatusConflict, protocol.ErrorCodeConflict)
		if got.Message == "" {
			t.Errorf("%s %s on an answered plan carries no sentence", call.method, call.path)
		}
	}

	// An id that cannot be a card's is not found, without asking the service.
	st.do(http.MethodPost, "/v1/cards/not-an-id/plan/approve", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)

	// A plan needs at least one step, and a body that is not JSON at all is a bad request.
	other := planCard(t, st)
	addPlan(t, st, other.ID, history.Plan{State: protocol.ChatPlanStateWaiting, Steps: []string{"A"}})
	st.do(http.MethodPut, "/v1/cards/"+other.ID+"/plan", protocol.EditPlanRequest{}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	st.do(http.MethodPut, "/v1/cards/"+other.ID+"/plan", protocol.EditPlanRequest{Steps: []string{"   "}}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	st.do(http.MethodPut, "/v1/cards/"+other.ID+"/plan", nil).want(t, http.StatusBadRequest)
}
