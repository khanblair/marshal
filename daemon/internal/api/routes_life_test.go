package api_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
)

// gitOutput runs Git in a folder and returns what it printed.
func (st *stack) gitOutput(dir string, args ...string) string {
	st.t.Helper()
	out, err := st.git.Run(context.Background(), dir, args...)
	if err != nil {
		st.t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(out)
}

// One whole life over HTTP and the event stream: a project is added, a card is made and started,
// a message is sent and answered, the card is stopped, and the project is removed. The
// repository's own files are the same at the end as they were at the start.
func TestAFullLifeThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, repo := st.addProject("small-repo")
	before := digestTree(t, repo)

	stream := st.dial(protocol.HomeTopic, protocol.ProjectTopic(project.ID))
	card := st.addCard(project.ID, "Fix the token bug")
	created := stream.until(ofType(protocol.EventTypeCardCreated))
	if got := dataOf[protocol.CardEventData](t, created[len(created)-1]).Card; got.ID != card.ID {
		t.Fatalf("card.created is about %s, want %s", got.ID, card.ID)
	}
	if len(stream.resyncs) != 1 || stream.resyncs[0].Reason != protocol.ResyncReasonEpochChanged {
		t.Errorf("a first connection should be told to load its snapshots once, got %+v", stream.resyncs)
	}
	stream.hello(protocol.HomeTopic, protocol.ProjectTopic(project.ID), protocol.CardTopic(card.ID))

	started := decode[protocol.Card](t, st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).want(t, http.StatusOK))
	seen := stream.until(stateOf(card.ID, protocol.SessionStateAwake))
	moved := ofTypeIn(seen, protocol.EventTypeCardMoved)
	if len(moved) == 0 {
		t.Fatalf("no card.moved event while starting: %s", describeEvents(seen))
	}
	if data := dataOf[protocol.CardMovedEventData](t, moved[0]); data.Card.State != protocol.CardStateWorking || data.From != protocol.CardStateBacklog {
		t.Errorf("card.moved = %+v, want backlog to working", data)
	}

	const question = "Please fix the token refresh bug"
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/messages", protocol.SendMessageRequest{Text: question}).want(t, http.StatusNoContent)
	seen = stream.until(stateOf(card.ID, protocol.SessionStateWorking))
	seen = append(seen, stream.until(stateOf(card.ID, protocol.SessionStateAwake))...)
	if texts := strings.Join(messageTexts(seen), ""); !strings.Contains(texts, "Turn 1. I remember 0 earlier turns.") {
		t.Errorf("the answer was %q, want the stub agent's first turn", texts)
	}
	if len(ofTypeIn(seen, protocol.EventTypeSessionToolCall)) == 0 {
		t.Errorf("the stub agent's tool calls did not reach the stream: %s", describeEvents(seen))
	}
	// The agent worked in the card's own worktree, and the repository folder stayed as it was.
	worktree := filepath.Join(projects.WorktreesDir(st.dataDir, project.ID), card.ID)
	if _, err := os.Stat(filepath.Join(worktree, "notes", "plan.md")); err != nil {
		t.Errorf("the agent's file is not in the card's worktree: %v", err)
	}
	if !reflect.DeepEqual(digestTree(t, repo), before) {
		t.Error("the repository's files changed while the agent worked")
	}

	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/stop", nil).want(t, http.StatusNoContent)
	stream.until(stateOf(card.ID, protocol.SessionStateStopped))
	if got := decode[protocol.Card](t, st.do(http.MethodGet, "/v1/cards/"+card.ID, nil).want(t, http.StatusOK)); got.Branch != started.Branch {
		t.Errorf("the card's branch is %q, want %q", got.Branch, started.Branch)
	}

	st.do(http.MethodDelete, "/v1/projects/"+project.ID, nil).want(t, http.StatusNoContent)
	removed := stream.until(ofType(protocol.EventTypeProjectRemoved))
	if data := dataOf[protocol.ProjectRemovedEventData](t, removed[len(removed)-1]); data.ProjectID != project.ID {
		t.Errorf("project.removed is about %q, want %q", data.ProjectID, project.ID)
	}

	if !reflect.DeepEqual(digestTree(t, repo), before) {
		t.Error("the repository's files are not the same after the project was removed")
	}
	if status := st.gitOutput(repo, "status", "--porcelain"); status != "" {
		t.Errorf("the repository has changes after the project was removed:\n%s", status)
	}
	if branches := st.gitOutput(repo, "branch", "--list", "marshal/*"); branches != "" {
		t.Errorf("the card's branch is still there: %s", branches)
	}
	if folders := st.worktreeFolders(project.ID); len(folders) != 0 {
		t.Errorf("worktree folders are left: %v", folders)
	}
	st.mustNotLog(question)
	st.mustNotLog(st.token)
}

func ofTypeIn(events []protocol.Event, typ protocol.EventType) []protocol.Event {
	var out []protocol.Event
	for _, ev := range events {
		if ev.Type == typ {
			out = append(out, ev)
		}
	}
	return out
}

// Closing the daemon and starting it again over the same data folder brings the session back, and
// the agent remembers what was said before.
func TestARestartKeepsTheConversationThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Remember me")
	stream := st.dial(protocol.CardTopic(card.ID))
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).want(t, http.StatusOK)
	stream.until(stateOf(card.ID, protocol.SessionStateAwake))
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/messages", protocol.SendMessageRequest{Text: "first question"}).want(t, http.StatusNoContent)
	seen := stream.until(stateOf(card.ID, protocol.SessionStateWorking))
	seen = append(seen, stream.until(stateOf(card.ID, protocol.SessionStateAwake))...)
	if texts := strings.Join(messageTexts(seen), ""); !strings.Contains(texts, "Turn 1. I remember 0 earlier turns.") {
		t.Fatalf("the first answer was %q", texts)
	}

	st.restart()
	if err := st.mgr.RestoreAll(context.Background()); err != nil {
		t.Fatalf("RestoreAll: %v", err)
	}
	stream = st.dial(protocol.CardTopic(card.ID))
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/messages", protocol.SendMessageRequest{Text: "second question"}).want(t, http.StatusNoContent)
	seen = stream.until(stateOf(card.ID, protocol.SessionStateWorking))
	seen = append(seen, stream.until(stateOf(card.ID, protocol.SessionStateAwake))...)
	if texts := strings.Join(messageTexts(seen), ""); !strings.Contains(texts, "Turn 2. I remember 1 earlier turns.") {
		t.Errorf("the second answer was %q, want it to remember the first turn", texts)
	}
}

// With automatic resume off, a session waits after a restart, a message to it is refused, and the
// person resumes it with the resume route.
func TestAManualResumeThroughHTTP(t *testing.T) {
	st := newStack(t, withResumeMode(session.ResumeModeManual))
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Wait for me")
	stream := st.dial(protocol.CardTopic(card.ID))
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).want(t, http.StatusOK)
	stream.until(stateOf(card.ID, protocol.SessionStateAwake))
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/messages", protocol.SendMessageRequest{Text: "first question"}).want(t, http.StatusNoContent)
	stream.until(stateOf(card.ID, protocol.SessionStateWorking))
	stream.until(stateOf(card.ID, protocol.SessionStateAwake))

	st.restart()
	if err := st.mgr.RestoreAll(context.Background()); err != nil {
		t.Fatalf("RestoreAll: %v", err)
	}
	got := st.do(http.MethodPost, "/v1/cards/"+card.ID+"/messages", protocol.SendMessageRequest{Text: "anyone there?"}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got.Message != "This card has no agent running." {
		t.Errorf("message = %q", got.Message)
	}

	stream = st.dial(protocol.CardTopic(card.ID))
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/resume", nil).want(t, http.StatusNoContent)
	stream.until(stateOf(card.ID, protocol.SessionStateAwake))
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/messages", protocol.SendMessageRequest{Text: "second question"}).want(t, http.StatusNoContent)
	seen := stream.until(stateOf(card.ID, protocol.SessionStateWorking))
	seen = append(seen, stream.until(stateOf(card.ID, protocol.SessionStateAwake))...)
	if texts := strings.Join(messageTexts(seen), ""); !strings.Contains(texts, "Turn 2. I remember 1 earlier turns.") {
		t.Errorf("the answer after the resume was %q, want it to remember the first turn", texts)
	}
}
