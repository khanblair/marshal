package api_test

import (
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The four Trello apply routes: pending, apply, dismiss, and refresh. Nothing here dials Trello:
// pending and dismiss are local, and a project with no board is refused first. What a refresh and an
// apply do against Trello is tested in internal/integrations.

const trelloSyncSecret = "a-trello-webhook-secret"

// pendingOf reads a project's pending changes as raw JSON, so a null where an empty list belongs is
// seen and not hidden by the decoder.
func pendingOf(t *testing.T, st *stack, projectID string) (protocol.TrelloPending, map[string]json.RawMessage) {
	t.Helper()
	r := st.do(http.MethodGet, "/v1/projects/"+projectID+"/trello/pending", nil).want(t, http.StatusOK)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(r.Body, &raw); err != nil {
		t.Fatalf("read the pending answer: %v", err)
	}
	return decode[protocol.TrelloPending](t, r), raw
}

func wantEmptyArrays(t *testing.T, raw map[string]json.RawMessage, names ...string) {
	t.Helper()
	for _, name := range names {
		if got := string(raw[name]); got != "[]" {
			t.Errorf("%q is %s, want [] and never null", name, got)
		}
	}
}

func TestTrelloPendingOfAProjectWithNoBoardIsTheGoldenUnlinkedAnswer(t *testing.T) {
	tests := map[string]func(st *stack, projectID string){
		"no connection": func(*stack, string) {},
		"a connection linked to another project": func(st *stack, _ string) {
			other, _ := st.addProject("monorepo")
			st.do(http.MethodPut, "/v1/integrations/trello", trelloRequest(trelloSyncSecret, other.ID)).want(t, http.StatusOK)
		},
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			st := newTrelloStack(t)
			project, _ := st.addProject("small-repo")
			setup(st, project.ID)

			pending, raw := pendingOf(t, st, project.ID)

			if pending.Linked || pending.BoardName != "" || pending.FetchedAt != nil {
				t.Errorf("pending = %+v, want linked false and nothing else", pending)
			}
			wantEmptyArrays(t, raw, "changes", "cards")
			assertGoldenShape(t, "trello-pending-unlinked", raw)
		})
	}
}

// assertGoldenShape checks an answer has the keys and values of a golden file, apart from the times,
// which the golden fixes and a live answer cannot.
func assertGoldenShape(t *testing.T, name string, raw map[string]json.RawMessage) {
	t.Helper()
	want, err := os.ReadFile(testutil.TestdataPath(t, "golden", name+".json"))
	if err != nil {
		t.Fatalf("read the golden file %s: %v", name, err)
	}
	var golden map[string]json.RawMessage
	if err := json.Unmarshal(want, &golden); err != nil {
		t.Fatalf("read the golden file %s: %v", name, err)
	}
	delete(golden, "serverTime")
	got := map[string]json.RawMessage{}
	for key, value := range raw {
		if key != "serverTime" {
			got[key] = value
		}
	}
	if len(got) != len(golden) {
		t.Errorf("answer keys = %v, want the golden's %v", keysOf(got), keysOf(golden))
	}
	for key, value := range golden {
		var a, b any
		_ = json.Unmarshal(value, &a)
		_ = json.Unmarshal(got[key], &b)
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%q = %s, want %s from the golden file", key, got[key], value)
		}
	}
	if _, ok := raw["serverTime"]; !ok {
		t.Error("the answer has no serverTime")
	}
}

func TestTrelloApplyAndRefreshOfAProjectWithNoBoardAreRefused(t *testing.T) {
	st := newTrelloStack(t)
	project, _ := st.addProject("small-repo")
	other, _ := st.addProject("monorepo")
	st.do(http.MethodPut, "/v1/integrations/trello", trelloRequest(trelloSyncSecret, other.ID)).want(t, http.StatusOK)

	routes := map[string]any{
		"apply":   protocol.ApplyTrelloRequest{ChangeIDs: []string{"x"}},
		"refresh": protocol.RefreshTrelloRequest{},
	}
	for name, body := range routes {
		t.Run(name, func(t *testing.T) {
			st.do(http.MethodPost, "/v1/projects/"+project.ID+"/trello/"+name, body).
				apiError(t, http.StatusConflict, protocol.ErrorCodeConflict)
		})
	}
}

func TestTrelloPostRoutesRefuseABodyThatIsNotAJSONObject(t *testing.T) {
	st := newTrelloStack(t)
	project, _ := st.addProject("small-repo")
	st.do(http.MethodPut, "/v1/integrations/trello", trelloRequest(trelloSyncSecret, project.ID)).want(t, http.StatusOK)

	for _, route := range []string{"apply", "dismiss", "refresh"} {
		t.Run(route, func(t *testing.T) {
			st.do(http.MethodPost, "/v1/projects/"+project.ID+"/trello/"+route, "not json").
				apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
			st.do(http.MethodPost, "/v1/projects/"+project.ID+"/trello/"+route, `{"unknownField":true}`).
				apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
		})
	}
}

// linkedTrelloStack is a stack whose Trello board is linked to a project that holds one card Trello
// has never heard of.
func linkedTrelloStack(t *testing.T) (st *stack, projectID string, card protocol.Card) {
	t.Helper()
	st = newTrelloStack(t)
	project, _ := st.addProject("small-repo")
	st.do(http.MethodPut, "/v1/integrations/trello", trelloRequest(trelloSyncSecret, project.ID)).want(t, http.StatusOK)
	return st, project.ID, st.addCard(project.ID, "Fix the flaky test")
}

func TestTrelloPendingOfALinkedProjectOffersToCreateACardTrelloHasNotSeen(t *testing.T) {
	st, projectID, card := linkedTrelloStack(t)

	pending, raw := pendingOf(t, st, projectID)

	if !pending.Linked || len(pending.Changes) != 1 {
		t.Fatalf("pending = %+v, want a linked board with one change", pending)
	}
	change := pending.Changes[0]
	if change.Kind != protocol.TrelloChangeKindCreateCard || change.CardID != card.ID || change.CardTitle != card.Title {
		t.Errorf("change = %+v, want to create %q in Trello", change, card.Title)
	}
	wantEmptyArrays(t, raw, "cards")
	if got := string(raw["fetchedAt"]); got != "null" {
		t.Errorf("fetchedAt = %s, want null: the board was never read", got)
	}
}

func TestTrelloDismissHidesAChangeOverTheWireAndAnswersWhatIsPending(t *testing.T) {
	st, projectID, _ := linkedTrelloStack(t)
	change := func() string { p, _ := pendingOf(t, st, projectID); return p.Changes[0].ID }()

	r := st.do(http.MethodPost, "/v1/projects/"+projectID+"/trello/dismiss",
		protocol.DismissTrelloRequest{ChangeIDs: []string{change}}).want(t, http.StatusOK)

	answer := decode[protocol.TrelloPending](t, r)
	if !answer.Linked || len(answer.Changes) != 0 {
		t.Errorf("dismiss answered %+v, want what is pending now: nothing", answer)
	}
	if again, _ := pendingOf(t, st, projectID); len(again.Changes) != 0 {
		t.Errorf("a dismissed change came back: %+v", again.Changes)
	}
}

func TestTrelloApplyOfAChangeThatIsNotPendingAnswersWhyAndWritesNothing(t *testing.T) {
	st, projectID, _ := linkedTrelloStack(t)

	r := st.do(http.MethodPost, "/v1/projects/"+projectID+"/trello/apply",
		protocol.ApplyTrelloRequest{ChangeIDs: []string{"not-a-change"}}).want(t, http.StatusOK)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(r.Body, &raw); err != nil {
		t.Fatalf("read the apply answer: %v", err)
	}
	wantEmptyArrays(t, raw, "applied")
	result := decode[protocol.ApplyTrelloResult](t, r)
	if len(result.Skipped) != 1 || result.Skipped[0].ChangeID != "not-a-change" || result.Skipped[0].Reason == "" {
		t.Errorf("skipped = %+v, want the unknown change with a reason", result.Skipped)
	}
	if len(result.Pending.Changes) != 1 {
		t.Errorf("pending after = %+v, want the card still waiting to be created", result.Pending.Changes)
	}
}
