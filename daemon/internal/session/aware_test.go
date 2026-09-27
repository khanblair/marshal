package session_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The board-awareness summary a card's agent is given at the start of every turn
// (docs/marshal-product-scope.md section 11.3, docs/backend-checklist.md B7.2, build-plan task 7.2).
// The manager knows the module that builds it only through the Awareness seam, so these tests drive
// that seam with a fake and check exactly what reached the agent.

// fakeAwareness is an Awareness that answers what the test tells it and records who it was asked
// about.
type fakeAwareness struct {
	mu      sync.Mutex
	asked   []string
	summary string
}

func (f *fakeAwareness) TurnAwareness(_ context.Context, cardID string) string {
	f.mu.Lock()
	f.asked = append(f.asked, cardID)
	f.mu.Unlock()
	return f.summary
}

func (f *fakeAwareness) askedCards() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.asked...)
}

// boardSummary is a summary in the shape the mcpattach module builds one.
const boardSummary = "Other cards on this board:\n- small-repo#2 Ship the board (backlog)"

// sentTurn waits until the fake agent has taken at least n messages and answers them.
func sentTurn(t *testing.T, e *env, n int) []string {
	t.Helper()
	waitFor(t, "the agent to be given the turn's message", func() bool {
		return len(e.agent.sentTexts()) >= n
	})
	return e.agent.sentTexts()
}

func TestEveryTurnStartsWithTheBoardAwarenessSummary(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Do the work")

	awareness := &fakeAwareness{summary: boardSummary}
	e.mgr.SetAwareness(awareness)

	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "add the route"); err != nil {
		t.Fatalf("send a message: %v", err)
	}

	sent := sentTurn(t, e, 1)
	if want := boardSummary + "\n\nadd the route"; sent[0] != want {
		t.Errorf("the agent was given %q, want the board summary in front of the message:\n%q", sent[0], want)
	}
	if asked := awareness.askedCards(); len(asked) != 1 || asked[0] != card.ID {
		t.Errorf("the summary was asked for %q, want just card %s", asked, card.ID)
	}

	// The stored history is what the person typed, not what the agent was handed: the summary is
	// context, and a card's history says what was said, not what was wrapped around it.
	waitForHistory(t, e, card.ID, 1)
	for _, ev := range kindEvents(t, e.historyOf(t, card.ID), history.KindUser) {
		if strings.Contains(ev.Summary, "Other cards on this board") {
			t.Errorf("the board summary was stored as part of the conversation: %q", ev.Summary)
		}
	}
}

func TestAQueuedMessageIsGivenItsOwnSummaryAndOnlyOnce(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Do the work")

	e.mgr.SetAwareness(&fakeAwareness{summary: boardSummary})

	// A turn is held open so the second message has to wait for its own turn.
	hold := make(chan struct{})
	e.agent.mu.Lock()
	e.agent.hold = hold
	e.agent.mu.Unlock()

	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "first"); err != nil {
		t.Fatalf("send the first message: %v", err)
	}
	sentTurn(t, e, 1)
	if err := e.mgr.Send(context.Background(), card.ID, "second"); err != nil {
		t.Fatalf("send the second message: %v", err)
	}
	close(hold)

	sent := sentTurn(t, e, 2)
	if len(sent) != 2 {
		t.Fatalf("the agent was given %d messages, want two: %q", len(sent), sent)
	}
	// Each turn gets its own summary, and a message that waited is not carrying the summary it was
	// given where it was accepted plus a second one from where it was delivered.
	for i, want := range []string{"first", "second"} {
		if strings.Count(sent[i], "Other cards on this board") != 1 {
			t.Errorf("message %d carries the summary %d times, want once:\n%q",
				i+1, strings.Count(sent[i], "Other cards on this board"), sent[i])
		}
		if !strings.HasPrefix(sent[i], boardSummary) || !strings.HasSuffix(sent[i], want) {
			t.Errorf("message %d is %q, want the summary then %q", i+1, sent[i], want)
		}
	}
}

func TestABoardSummaryThatCannotBeBuiltLeavesTheMessageAlone(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Do the work")

	e.mgr.SetAwareness(&fakeAwareness{summary: ""})

	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "just the message"); err != nil {
		t.Fatalf("send a message: %v", err)
	}

	if sent := sentTurn(t, e, 1); sent[0] != "just the message" {
		t.Errorf("the agent was given %q, want the message on its own", sent[0])
	}
}

// A daemon with no awareness module is what every phase before this one looked like.
func TestNothingIsAddedWhenNoAwarenessModuleIsSet(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Do the work")

	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "just the message"); err != nil {
		t.Fatalf("send a message: %v", err)
	}

	if sent := sentTurn(t, e, 1); sent[0] != "just the message" {
		t.Errorf("the agent was given %q, want the message on its own", sent[0])
	}
}

// A chat has no card and so no board to be aware of: its messages go to the agent as they were
// typed.
func TestAChatIsNotGivenABoardSummary(t *testing.T) {
	e := newChatEnv(t)
	project := e.project(t, "small-repo")
	chat := e.newChat(t, project.ID, protocol.CreateChatRequest{AgentKind: protocol.AgentKindClaude})

	e.mgr.SetAwareness(&fakeAwareness{summary: boardSummary})

	if err := e.mgr.SendChat(context.Background(), chat, "just the message"); err != nil {
		t.Fatalf("send to the chat: %v", err)
	}

	var sent []string
	deadline := time.Now().Add(eventTimeout)
	for time.Now().Before(deadline) {
		if sent = e.agent.sentTexts(); len(sent) > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if len(sent) == 0 {
		t.Fatal("the chat's message never reached the agent")
	}
	if sent[0] != "just the message" {
		t.Errorf("the chat's agent was given %q, want the message on its own", sent[0])
	}
}
