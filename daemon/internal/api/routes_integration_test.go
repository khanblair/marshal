package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/api"
	"github.com/khanblair/marshal/daemon/internal/integrator"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The Integration view's routes and the route that shows a card's worktree. The merge queue is a
// fake reader, and the machine's file manager and editor are a recording opener: nothing here runs
// a merge, and nothing starts a program.

// fakeMergeQueue is an integrator.Reader that answers a state it was given and records each call.
type fakeMergeQueue struct {
	mu    sync.Mutex
	state protocol.IntegrationState
	err   error
	calls []string
}

func (f *fakeMergeQueue) record(call string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	return f.err
}

func (f *fakeMergeQueue) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeMergeQueue) answer(projectID string, state protocol.IntegratorState) protocol.IntegrationState {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state.ProjectID = projectID
	if state != "" {
		f.state.State = state
	}
	if f.state.Queue == nil {
		f.state.Queue = []protocol.IntegrationQueueItem{}
	}
	if f.state.History == nil {
		f.state.History = []protocol.IntegrationHistoryItem{}
	}
	if f.state.IntegratorBranch == "" {
		f.state.IntegratorBranch = protocol.IntegrationBranchName
	}
	if time.Time(f.state.ServerTime).IsZero() {
		f.state.ServerTime = protocol.NewTimestamp(time.Now())
	}
	return f.state
}

func (f *fakeMergeQueue) State(_ context.Context, projectID string) (protocol.IntegrationState, error) {
	if err := f.record("state " + projectID); err != nil {
		return protocol.IntegrationState{}, err
	}
	return f.answer(projectID, ""), nil
}

func (f *fakeMergeQueue) Pause(_ context.Context, projectID string) (protocol.IntegrationState, error) {
	if err := f.record("pause " + projectID); err != nil {
		return protocol.IntegrationState{}, err
	}
	return f.answer(projectID, protocol.IntegratorStatePaused), nil
}

func (f *fakeMergeQueue) Resume(_ context.Context, projectID string) (protocol.IntegrationState, error) {
	if err := f.record("resume " + projectID); err != nil {
		return protocol.IntegrationState{}, err
	}
	return f.answer(projectID, protocol.IntegratorStateIdle), nil
}

func (f *fakeMergeQueue) Retry(_ context.Context, cardID string) (protocol.Card, error) {
	if err := f.record("retry " + cardID); err != nil {
		return protocol.Card{}, err
	}
	return fakeCard(cardID, protocol.CardStateMerging), nil
}

func (f *fakeMergeQueue) Undo(_ context.Context, cardID string) (protocol.Card, error) {
	if err := f.record("undo " + cardID); err != nil {
		return protocol.Card{}, err
	}
	return fakeCard(cardID, protocol.CardStateReady), nil
}

func (f *fakeMergeQueue) SendToMerge(_ context.Context, cardID string) (protocol.Card, error) {
	if err := f.record("send " + cardID); err != nil {
		return protocol.Card{}, err
	}
	return fakeCard(cardID, protocol.CardStateReady), nil
}

// fakeCard is a card with the times a card always has, so it can be written to the wire.
func fakeCard(id string, state protocol.CardState) protocol.Card {
	now := protocol.NewTimestamp(time.Now())
	return protocol.Card{ID: id, State: state, Labels: []protocol.Label{}, CreatedAt: now, UpdatedAt: now}
}

var _ integrator.Reader = (*fakeMergeQueue)(nil)

// recordingOpener is an api.Opener that remembers what it was asked to open.
type recordingOpener struct {
	mu    sync.Mutex
	opens []openCall
	err   error
}

type openCall struct{ path, with string }

func (o *recordingOpener) Open(_ context.Context, path, with string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.opens = append(o.opens, openCall{path, with})
	return o.err
}

func (o *recordingOpener) seen() []openCall {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]openCall(nil), o.opens...)
}

var _ api.Opener = (*recordingOpener)(nil)

func withMergeQueue(q integrator.Reader) stackOption {
	return func(c *stackConfig) { c.integration = q }
}

func withoutIntegration() stackOption { return func(c *stackConfig) { c.noIntegration = true } }

func withOpener(o api.Opener) stackOption { return func(c *stackConfig) { c.opener = o } }

// goldenIntegrationState is the state the Go tests wrote the golden file from.
func goldenIntegrationState(t *testing.T) protocol.IntegrationState {
	t.Helper()
	data, err := os.ReadFile(testutil.TestdataPath(t, "golden", "integration-state.json"))
	if err != nil {
		t.Fatalf("read the golden state: %v", err)
	}
	var state protocol.IntegrationState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("decode the golden state: %v", err)
	}
	return state
}

func TestTheIntegrationStateRouteAnswersTheQueue(t *testing.T) {
	queue := &fakeMergeQueue{state: goldenIntegrationState(t)}
	st := newStack(t, withMergeQueue(queue))
	project, _ := st.addProject("small-repo")

	r := st.do(http.MethodGet, "/v1/projects/"+project.ID+"/integration", nil).want(t, http.StatusOK)
	sameShape(t, "integration-state", r.Body)
	got := decode[protocol.IntegrationState](t, r)
	if got.ProjectID != project.ID || got.State != protocol.IntegratorStateMerging || got.AheadBy != 2 {
		t.Fatalf("the state is %+v", got)
	}
	if len(got.Queue) != 2 || got.Queue[0].Phase != protocol.MergePhaseResolving {
		t.Errorf("the queue is %+v", got.Queue)
	}
	if len(got.History) != 1 || !got.History[0].CanUndo || got.History[0].Resolved != 1 {
		t.Errorf("the history is %+v", got.History)
	}
	if calls := queue.seen(); len(calls) != 1 || calls[0] != "state "+project.ID {
		t.Errorf("the reader was asked %v", calls)
	}
}

func TestAnEmptyQueueIsSentAsListsAndNeverNull(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	body := string(st.do(http.MethodGet, "/v1/projects/"+project.ID+"/integration", nil).want(t, http.StatusOK).Body)
	if !strings.Contains(body, `"queue":[]`) || !strings.Contains(body, `"history":[]`) {
		t.Errorf("an idle project's state is %s", body)
	}
}

func TestPausingAndResumingMergingAnswerTheNewState(t *testing.T) {
	queue := &fakeMergeQueue{}
	st := newStack(t, withMergeQueue(queue))
	project, _ := st.addProject("small-repo")
	base := "/v1/projects/" + project.ID + "/integration/"

	paused := decode[protocol.IntegrationState](t, st.do(http.MethodPost, base+"pause", nil).want(t, http.StatusOK))
	if paused.State != protocol.IntegratorStatePaused || paused.ProjectID != project.ID {
		t.Errorf("after pause the state is %+v", paused)
	}
	resumed := decode[protocol.IntegrationState](t, st.do(http.MethodPost, base+"resume", nil).want(t, http.StatusOK))
	if resumed.State != protocol.IntegratorStateIdle {
		t.Errorf("after resume the state is %+v", resumed)
	}
	want := []string{"pause " + project.ID, "resume " + project.ID}
	if calls := queue.seen(); strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Errorf("the reader was asked %v, want %v", calls, want)
	}
}

func TestRetryAndUndoAnswerTheCard(t *testing.T) {
	queue := &fakeMergeQueue{}
	st := newStack(t, withMergeQueue(queue))
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Add login page")

	retried := decode[protocol.Card](t, st.do(http.MethodPost, "/v1/cards/"+card.ID+"/merge/retry", nil).want(t, http.StatusOK))
	if retried.ID != card.ID || retried.State != protocol.CardStateMerging {
		t.Errorf("retry answered %+v", retried)
	}
	undone := decode[protocol.Card](t, st.do(http.MethodPost, "/v1/cards/"+card.ID+"/merge/undo", nil).want(t, http.StatusOK))
	if undone.ID != card.ID || undone.State != protocol.CardStateReady {
		t.Errorf("undo answered %+v", undone)
	}
	want := []string{"retry " + card.ID, "undo " + card.ID}
	if calls := queue.seen(); strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Errorf("the reader was asked %v, want %v", calls, want)
	}
}

func TestSendToMergeAnswersTheCardAndPassesTheRefusalOn(t *testing.T) {
	queue := &fakeMergeQueue{}
	st := newStack(t, withMergeQueue(queue))
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Add login page")

	sent := decode[protocol.Card](t, st.do(http.MethodPost, "/v1/cards/"+card.ID+"/send-to-merge", nil).want(t, http.StatusOK))
	if sent.ID != card.ID || sent.State != protocol.CardStateReady {
		t.Errorf("send-to-merge answered %+v", sent)
	}
	queue.mu.Lock()
	queue.err = protocol.Refused("This card has no commits to merge yet.").With("reason", "send_no_commits")
	queue.mu.Unlock()
	got := st.do(http.MethodPost, "/v1/cards/"+card.ID+"/send-to-merge", nil).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got.Message != "This card has no commits to merge yet." {
		t.Errorf("the message is %q, want the queue's own", got.Message)
	}
}

func TestTheReadersOwnSentenceComesThroughARefusal(t *testing.T) {
	const sentence = "This merge cannot be undone: other cards were merged after it."
	queue := &fakeMergeQueue{err: protocol.Refused(sentence)}
	st := newStack(t, withMergeQueue(queue))
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Add login page")

	got := st.do(http.MethodPost, "/v1/cards/"+card.ID+"/merge/undo", nil).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got.Message != sentence {
		t.Errorf("the message is %q, want the reader's own", got.Message)
	}
	queue.mu.Lock()
	queue.err = protocol.NotFound("project")
	queue.mu.Unlock()
	st.do(http.MethodGet, "/v1/projects/"+project.ID+"/integration", nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	queue.mu.Lock()
	queue.err = errors.New("the branch lock is held")
	queue.mu.Unlock()
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/merge/retry", nil).apiError(t, http.StatusInternalServerError, protocol.ErrorCodeInternal)
}

func TestAMalformedIdIsNotFoundWithoutAskingTheReader(t *testing.T) {
	queue := &fakeMergeQueue{}
	st := newStack(t, withMergeQueue(queue))
	st.do(http.MethodGet, "/v1/projects/Not%20A%20Project/integration", nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodPost, "/v1/projects/Not%20A%20Project/integration/pause", nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodPost, "/v1/cards/not-a-card/merge/retry", nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodPost, "/v1/cards/not-a-card/merge/undo", nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodPost, "/v1/cards/not-a-card/send-to-merge", nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	if calls := queue.seen(); len(calls) != 0 {
		t.Errorf("the reader was asked %v for ids that cannot exist", calls)
	}
}

func openRequest(card protocol.Card, with string) (string, protocol.OpenWorktreeRequest) {
	return "/v1/cards/" + card.ID + "/worktree/open", protocol.OpenWorktreeRequest{With: with}
}

func TestOpeningAWorktreeHandsTheCardsOwnFolderToTheOpener(t *testing.T) {
	opener := &recordingOpener{}
	st := newStack(t, withOpener(opener))
	project, repo := st.addProject("small-repo")
	card := st.addCard(project.ID, "Add login page")
	dir := st.startWorktree(t, project, repo, card)

	for _, with := range []string{"finder", "editor"} {
		path, body := openRequest(card, with)
		if r := st.do(http.MethodPost, path, body).want(t, http.StatusNoContent); len(r.Body) != 0 {
			t.Errorf("opening answered a body: %s", r.Body)
		}
	}
	want := []openCall{{dir, "finder"}, {dir, "editor"}}
	got := opener.seen()
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("the opener was asked %v, want %v", got, want)
	}
}

func TestOpeningAWorktreeRefusesWhatCannotBeShown(t *testing.T) {
	opener := &recordingOpener{}
	st := newStack(t, withOpener(opener))
	project, repo := st.addProject("small-repo")
	card := st.addCard(project.ID, "Add login page")
	path, _ := openRequest(card, "")

	st.do(http.MethodPost, path, protocol.OpenWorktreeRequest{With: "finder"}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	dir := st.startWorktree(t, project, repo, card)
	st.do(http.MethodPost, path, protocol.OpenWorktreeRequest{With: "vim"}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	st.do(http.MethodPost, path, `{"with":"finder","path":"/etc"}`).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	st.do(http.MethodPost, path, nil).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	st.do(http.MethodPost, "/v1/cards/not-a-card/worktree/open", protocol.OpenWorktreeRequest{With: "finder"}).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	st.do(http.MethodPost, path, protocol.OpenWorktreeRequest{With: "finder"}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got := opener.seen(); len(got) != 0 {
		t.Errorf("the opener was asked %v for requests that cannot be answered", got)
	}
}

func TestAnOpenerThatFailsIsReportedInAPlainSentence(t *testing.T) {
	opener := &recordingOpener{err: errors.New("exec: \"open\": executable file not found")}
	st := newStack(t, withOpener(opener))
	project, repo := st.addProject("small-repo")
	card := st.addCard(project.ID, "Add login page")
	st.startWorktree(t, project, repo, card)
	path, body := openRequest(card, "editor")

	got := st.do(http.MethodPost, path, body).apiError(t, http.StatusServiceUnavailable, protocol.ErrorCodeUnavailable)
	if strings.Contains(got.Message, "exec") {
		t.Errorf("the message leaks the program's error: %q", got.Message)
	}
}

// A request that did not come from this machine, such as one over the tailnet, is refused before
// anything is read. The dev token is accepted only from loopback, so this runs on a normal daemon.
func TestOpeningAWorktreeIsRefusedOverTheTailnet(t *testing.T) {
	opener := &recordingOpener{}
	st := newStack(t, normalDaemon(), withOpener(opener))
	project, repo := st.addProject("small-repo")
	card := st.addCard(project.ID, "Add login page")
	st.startWorktree(t, project, repo, card)
	path, body := openRequest(card, "finder")
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}

	for _, remote := range []string{"100.64.0.7:51000", "192.0.2.1:4000", "[fd7a:115c:a1e0::1]:51000"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(payload)))
		req.RemoteAddr = remote
		req.Header.Set("Authorization", "Bearer "+st.token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		st.server.Handler().ServeHTTP(rec, req)
		answer := reply{Status: rec.Code, Header: rec.Header(), Body: rec.Body.Bytes()}
		answer.apiError(t, http.StatusForbidden, protocol.ErrorCodeForbidden)
	}
	if got := opener.seen(); len(got) != 0 {
		t.Fatalf("the opener ran for a request from another machine: %v", got)
	}
	st.do(http.MethodPost, path, body).want(t, http.StatusNoContent)
	if got := opener.seen(); len(got) != 1 {
		t.Errorf("the opener was asked %v from this machine, want once", got)
	}
}
