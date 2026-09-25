package api_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/api"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// An agent that needs a sign-in gets a sentence that says so, and nothing is left behind, not even
// the branch that the try made. When the person signs in and starts the card again, nothing is in
// the way, so the second start meets the same sentence until the agent works.
func TestAnAgentThatNeedsASignIn(t *testing.T) {
	needsSignIn := &agents.AuthRequiredError{Methods: []string{"claude-login", "api-key"}}
	st := newStack(t, withAgent(fakeFactory(fakeAgent{startErr: needsSignIn})))
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Needs a sign-in")
	path := "/v1/cards/" + card.ID + "/start"

	got := st.do(http.MethodPost, path, nil).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	want := "The agent needs you to sign in before it can start. " +
		"Sign in with the agent's own command in a terminal, then try again."
	if got.Message != want {
		t.Errorf("message = %q\nwant      %q", got.Message, want)
	}
	if got.Details["methods"] != "claude-login, api-key" {
		t.Errorf("details = %v, want the ways to sign in", got.Details)
	}
	if folders := st.worktreeFolders(project.ID); len(folders) != 0 {
		t.Errorf("the failed start left the worktree folders %v", folders)
	}

	again := st.do(http.MethodPost, path, nil).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if again.Message != want {
		t.Errorf("the second start said %q\nwant %q", again.Message, want)
	}
	if folders := st.worktreeFolders(project.ID); len(folders) != 0 {
		t.Errorf("the second start left the worktree folders %v", folders)
	}
}

// A start that fails for a reason nobody named gets one plain sentence, with nothing of the inside
// in the answer. The reason goes to the log.
func TestAnUnexpectedFailureKeepsItsCauseOutOfTheAnswer(t *testing.T) {
	st := newStack(t, withAgent(fakeFactory(fakeAgent{startErr: errors.New("the program /usr/bin/secret-agent crashed"), sendErr: errors.New("socket closed unexpectedly")})))
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Breaks")

	got := st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).apiError(t, http.StatusServiceUnavailable, protocol.ErrorCodeUnavailable)
	want := "Marshal could not start the agent for this card. " +
		"Check that the agent is installed and that you are signed in to it, then try again."
	if got.Message != want || strings.Contains(got.Message, "secret-agent") {
		t.Errorf("the answer is %+v, want the plain sentence and nothing of the inside", got)
	}
	if !strings.Contains(st.logText(), "secret-agent crashed") {
		t.Error("the real reason is not in the log")
	}
}

// A session that ended on the agent's side reads as "no agent running" for a message and for a
// stop, and the session is left for the person to start again.
func TestAnAgentThatHasAlreadyEnded(t *testing.T) {
	tests := []struct {
		name   string
		script fakeAgent
		path   string
		body   any
	}{
		{"a message to a stopped agent", fakeAgent{sendErr: agents.ErrStopped}, "/messages", protocol.SendMessageRequest{Text: "hi"}},
		{"a message to an unknown session", fakeAgent{sendErr: agents.ErrUnknownSession}, "/messages", protocol.SendMessageRequest{Text: "hi"}},
		{"a stop of an unknown session", fakeAgent{stopErr: agents.ErrUnknownSession}, "/stop", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := newStack(t, withAgent(fakeFactory(tc.script)))
			project, _ := st.addProject("small-repo")
			card := st.addCard(project.ID, "Ends early")
			st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).want(t, http.StatusOK)

			got := st.do(http.MethodPost, "/v1/cards/"+card.ID+tc.path, tc.body).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
			if got.Message != "This card has no agent running." {
				t.Errorf("message = %q", got.Message)
			}
		})
	}
}

// Resuming a session that the agent cannot pick up again says so, and the card moves to needing the
// person, which is what the event stream and the board show.
func TestResumingASessionTheAgentCannotPickUp(t *testing.T) {
	st := newStack(t, withAgent(fakeFactory(fakeAgent{resumeErr: agents.ErrCannotResume})))
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Cannot resume")
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).want(t, http.StatusOK)
	st.restart()

	got := st.do(http.MethodPost, "/v1/cards/"+card.ID+"/resume", nil).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got.Message != "Marshal could not pick this session back up. The card now needs you." {
		t.Errorf("message = %q", got.Message)
	}
	after := decode[protocol.Card](t, st.do(http.MethodGet, "/v1/cards/"+card.ID, nil).want(t, http.StatusOK))
	if after.State != protocol.CardStateNeeds {
		t.Errorf("the card is %s, want it to need the person", after.State)
	}
	row, err := st.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil || row.State != string(protocol.SessionStateStopped) {
		t.Errorf("the session row is %+v (error %v), want it stopped", row.State, err)
	}
}

// A resume that fails for a reason nobody named is answered like any other resume that could not
// pick the session up, and the card is moved to needing the person: the session manager records
// the failure before it reports.
func TestResumingASessionThatFailsForAnUnnamedReason(t *testing.T) {
	st := newStack(t, withAgent(fakeFactory(fakeAgent{resumeErr: errors.New("the agent's process would not start")})))
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Fails to resume")
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).want(t, http.StatusOK)
	st.restart()

	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/resume", nil).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	after := decode[protocol.Card](t, st.do(http.MethodGet, "/v1/cards/"+card.ID, nil).want(t, http.StatusOK))
	if after.State != protocol.CardStateNeeds {
		t.Errorf("the card is %s, want it to need the person", after.State)
	}
}

// Starting takes longer than the server's usual time to write an answer, and its request context
// must not end before it is done. The limits here are far shorter than the start, so an answer
// that arrives proves both the longer write window and that the read limit stays out of the way.
func TestASlowStartOutlastsTheUsualLimits(t *testing.T) {
	limits := api.Limits{WriteTimeout: 300 * time.Millisecond, ReadTimeout: 100 * time.Millisecond}
	st := newStack(t, withLimits(limits), withAgent(fakeFactory(fakeAgent{startDelay: 900 * time.Millisecond, resumeDelay: 900 * time.Millisecond})))
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Slow to start")

	began := time.Now()
	started := decode[protocol.Card](t, st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).want(t, http.StatusOK))
	if took := time.Since(began); took < 800*time.Millisecond {
		t.Fatalf("the start took %v, the test does not prove anything", took)
	}
	if started.State != protocol.CardStateWorking {
		t.Errorf("the card is %s, want working", started.State)
	}

	// Resuming is as slow, and gets the same window.
	st.restart()
	began = time.Now()
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/resume", nil).want(t, http.StatusNoContent)
	if took := time.Since(began); took < 800*time.Millisecond {
		t.Fatalf("the resume took %v, the test does not prove anything", took)
	}
}

// When the client goes away while a card is starting, the request's context ends, the session
// manager undoes the worktree, and nothing is left behind: no folder, no registered worktree, no
// branch recorded on the card, and no session.
func TestACancelledStartLeavesNoWorktreeBehind(t *testing.T) {
	entered := make(chan struct{}, 1)
	st := newStack(t, withAgent(fakeFactory(fakeAgent{startDelay: blockForever, entered: entered})))
	project, repo := st.addProject("small-repo")
	card := st.addCard(project.ID, "Cancelled while starting")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := st.newRequest(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil)
	req = req.WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+st.token)
	failed := make(chan error, 1)
	go func() {
		resp, err := st.client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
		failed <- err
	}()

	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the agent was never asked to start")
	}
	if folders := st.worktreeFolders(project.ID); len(folders) != 1 {
		t.Fatalf("while starting there should be one worktree folder, got %v", folders)
	}
	cancel()
	if err := <-failed; err == nil {
		t.Fatal("the cancelled request did not fail")
	}

	deadline := time.Now().Add(10 * time.Second)
	for len(st.worktreeFolders(project.ID)) != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("the worktree folder is still there: %v", st.worktreeFolders(project.ID))
		}
		time.Sleep(20 * time.Millisecond)
	}
	worktrees, err := st.git.ListWorktrees(context.Background(), repo)
	if err != nil {
		t.Fatalf("list the worktrees: %v", err)
	}
	if len(worktrees) != 1 {
		t.Errorf("Git still lists %d worktrees: %+v", len(worktrees), worktrees)
	}
	// The card is cleared after the folder and the branch are gone, so wait for that step too.
	var after protocol.Card
	for {
		after = decode[protocol.Card](t, st.do(http.MethodGet, "/v1/cards/"+card.ID, nil).want(t, http.StatusOK))
		if after.Branch == "" || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if after.State != protocol.CardStateBacklog || after.Branch != "" {
		t.Errorf("the card is %+v, want it back in the backlog with no branch", after)
	}
	if _, err := st.store.Queries().GetSessionByCard(context.Background(), card.ID); !store.IsNotFound(err) {
		t.Errorf("a session row was left behind (error %v)", err)
	}
}

// A project whose repository folder was moved or deleted, or is no longer a Git repository, gets
// a sentence that says so when one of its cards is started, and nothing is left behind.
func TestStartingACardWhoseRepositoryIsGone(t *testing.T) {
	tests := []struct {
		name   string
		damage func(t *testing.T, repo string)
	}{
		{"the folder is deleted", func(t *testing.T, repo string) {
			if err := os.RemoveAll(repo); err != nil {
				t.Fatal(err)
			}
		}},
		{"the Git data is deleted", func(t *testing.T, repo string) {
			if err := os.RemoveAll(filepath.Join(repo, ".git")); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := newStack(t)
			project, repo := st.addProject("small-repo")
			card := st.addCard(project.ID, "The repository vanishes")
			tc.damage(t, repo)

			got := st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).apiError(t, http.StatusServiceUnavailable, protocol.ErrorCodeUnavailable)
			want := "Marshal cannot use the repository folder of this project. Check that the folder is still there and is a Git repository."
			if got.Message != want {
				t.Errorf("message = %q\nwant      %q", got.Message, want)
			}
			if folders := st.worktreeFolders(project.ID); len(folders) != 0 {
				t.Errorf("the failed start left the worktree folders %v", folders)
			}
			if after := decode[protocol.Card](t, st.do(http.MethodGet, "/v1/cards/"+card.ID, nil).want(t, http.StatusOK)); after.State != protocol.CardStateBacklog {
				t.Errorf("the card is %s, want it left in the backlog", after.State)
			}
		})
	}
}

// A start or a resume that ran past the agent's time limit does not say "try again": a start has
// left its branch behind, and a resume has already stopped the session and moved the card to
// needing the person.
func TestAnAgentThatRunsOutOfTime(t *testing.T) {
	slow := fmt.Errorf("start the agent: %w", context.DeadlineExceeded)
	t.Run("a start", func(t *testing.T) {
		st := newStack(t, withAgent(fakeFactory(fakeAgent{startErr: slow})))
		project, _ := st.addProject("small-repo")
		card := st.addCard(project.ID, "Slow to start")
		got := st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).apiError(t, http.StatusServiceUnavailable, protocol.ErrorCodeUnavailable)
		if want := "The agent took too long to start. Check that it runs in a terminal."; got.Message != want {
			t.Errorf("message = %q\nwant      %q", got.Message, want)
		}
	})
	t.Run("a resume", func(t *testing.T) {
		st := newStack(t, withAgent(fakeFactory(fakeAgent{resumeErr: slow})))
		project, _ := st.addProject("small-repo")
		card := st.addCard(project.ID, "Slow to resume")
		st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).want(t, http.StatusOK)
		st.restart()
		got := st.do(http.MethodPost, "/v1/cards/"+card.ID+"/resume", nil).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
		if got.Message != "Marshal could not pick this session back up. The card now needs you." {
			t.Errorf("message = %q", got.Message)
		}
		if after := decode[protocol.Card](t, st.do(http.MethodGet, "/v1/cards/"+card.ID, nil).want(t, http.StatusOK)); after.State != protocol.CardStateNeeds {
			t.Errorf("the card is %s, want it to need the person", after.State)
		}
	})
}

// The real stub agent cannot pick up a session it does not know, and that comes back through the
// resume route as the sentence for a session that cannot be picked up again.
func TestResumingASessionTheStubAgentDoesNotKnow(t *testing.T) {
	st := newStack(t, withResumeMode(session.ResumeModeManual))
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Lost session")
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).want(t, http.StatusOK)
	st.restart()

	ctx := context.Background()
	row, err := st.store.Queries().GetSessionByCard(ctx, card.ID)
	if err != nil {
		t.Fatalf("read the session: %v", err)
	}
	err = st.store.Write(ctx, func(q *db.Queries) error {
		_, err := q.UpdateSessionRuntime(ctx, db.UpdateSessionRuntimeParams{
			State: row.State, AgentSessionID: "no-such-session-on-disk",
			LastActiveAt: row.LastActiveAt, UpdatedAt: row.UpdatedAt, ID: row.ID,
		})
		return err
	})
	if err != nil {
		t.Fatalf("corrupt the agent session id: %v", err)
	}
	got := st.do(http.MethodPost, "/v1/cards/"+card.ID+"/resume", nil).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got.Message != "Marshal could not pick this session back up. The card now needs you." {
		t.Errorf("message = %q", got.Message)
	}
	if after := decode[protocol.Card](t, st.do(http.MethodGet, "/v1/cards/"+card.ID, nil).want(t, http.StatusOK)); after.State != protocol.CardStateNeeds {
		t.Errorf("the card is %s, want it to need the person", after.State)
	}
}
