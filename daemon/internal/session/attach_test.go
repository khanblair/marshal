package session_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
)

// The internal MCP server and the context a session starts with (docs/architecture.md sections 7
// and 11.4) are attached by a module that is built after the session manager, so the manager knows
// it only through the Attacher seam. These tests drive that seam with a fake: what the manager
// hands the agent, and when it gives it back up.

// fakeAttacher is an Attacher that records what it was asked and answers what the test tells it to.
type fakeAttacher struct {
	mu       sync.Mutex
	asked    []string
	detached []string
	// order is every Attach and Detach as it happened, tagged "attach:<id>" or "detach:<id>", so a
	// test can prove a card's server was given up before the session that replaces it took the next
	// one (which is what the manager's pump drain is for).
	order  []string
	answer func(card protocol.Card) (session.Attachment, error)
}

func (f *fakeAttacher) Attach(_ context.Context, card protocol.Card) (session.Attachment, error) {
	f.mu.Lock()
	f.asked = append(f.asked, card.ID)
	f.order = append(f.order, "attach:"+card.ID)
	f.mu.Unlock()
	if f.answer == nil {
		return session.Attachment{}, nil
	}
	return f.answer(card)
}

func (f *fakeAttacher) Detach(_ context.Context, cardID string) {
	f.mu.Lock()
	f.detached = append(f.detached, cardID)
	f.order = append(f.order, "detach:"+cardID)
	f.mu.Unlock()
}

func (f *fakeAttacher) askedCards() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.asked...)
}

func (f *fakeAttacher) detachedCards() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.detached...)
}

// sequence copies every Attach and Detach in the order they happened.
func (f *fakeAttacher) sequence() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.order...)
}

// server is the internal server as a session is given it: the daemon's own program, run as `mcp`,
// told which card it speaks for.
func server(card protocol.Card) agents.MCPServer {
	return agents.MCPServer{
		Name:    "marshal",
		Command: "/usr/local/bin/marshald",
		Args:    []string{"mcp"},
		Env:     []string{"MARSHAL_MCP_CARD=" + card.ID},
	}
}

func TestStartGivesTheSessionTheServerAndItsContext(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Do the work")

	attacher := &fakeAttacher{answer: func(card protocol.Card) (session.Attachment, error) {
		return session.Attachment{
			Servers:      []agents.MCPServer{server(card)},
			Instructions: "You are the Implementer.\n\nThe board has 2 other cards.",
		}, nil
	}}
	e.mgr.SetAttacher(attacher)

	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}

	if got := attacher.askedCards(); len(got) != 1 || got[0] != card.ID {
		t.Errorf("the attacher was asked for %q, want just card %s", got, card.ID)
	}
	specs := e.agent.startSpecs()
	if len(specs) != 1 {
		t.Fatalf("the agent was started %d times, want once", len(specs))
	}
	spec := specs[0]
	if len(spec.MCPServers) != 1 || spec.MCPServers[0].Name != "marshal" {
		t.Errorf("the session's MCP servers are %+v", spec.MCPServers)
	}
	if want := "MARSHAL_MCP_CARD=" + card.ID; len(spec.MCPServers) == 1 && spec.MCPServers[0].Env[0] != want {
		t.Errorf("the server is told %q, want %q", spec.MCPServers[0].Env[0], want)
	}
	if spec.Instructions != "You are the Implementer.\n\nThe board has 2 other cards." {
		t.Errorf("the session's instructions are %q", spec.Instructions)
	}
}

func TestTheServerIsGivenUpWhenTheSessionEnds(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Do the work")

	attacher := &fakeAttacher{answer: func(card protocol.Card) (session.Attachment, error) {
		return session.Attachment{Servers: []agents.MCPServer{server(card)}}, nil
	}}
	e.mgr.SetAttacher(attacher)

	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	if err := e.mgr.StopCardSession(context.Background(), card.ID); err != nil {
		t.Fatalf("stop the card: %v", err)
	}
	// The release runs on the pump's goroutine, when the session's channel closes, so the test
	// waits for it rather than assuming it has already happened.
	waitFor(t, "the server to be given up", func() bool {
		return len(attacher.detachedCards()) > 0
	})
	if got := attacher.detachedCards(); got[0] != card.ID {
		t.Errorf("the server was given up for %q, want card %s", got, card.ID)
	}
}

// A knowledge base that cannot be read is a session without it, not a card that cannot start. The
// failure is logged by the manager; what a person sees is their card running.
func TestAGapInTheKnowledgeBaseDoesNotStopTheCard(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Do the work")

	attacher := &fakeAttacher{answer: func(protocol.Card) (session.Attachment, error) {
		return session.Attachment{}, errors.New("the vault is unreadable")
	}}
	e.mgr.SetAttacher(attacher)

	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("a card was refused because its knowledge base could not be read: %v", err)
	}
	specs := e.agent.startSpecs()
	if len(specs) != 1 {
		t.Fatalf("the agent was started %d times, want once", len(specs))
	}
	if len(specs[0].MCPServers) != 0 || specs[0].Instructions != "" {
		t.Errorf("a failed attachment still gave the session %+v / %q", specs[0].MCPServers, specs[0].Instructions)
	}
}

// A daemon with no module set up attaches nothing at all, which is what every phase before this one
// looked like.
func TestNothingIsAttachedWhenNoModuleIsSet(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Do the work")

	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	specs := e.agent.startSpecs()
	if len(specs) != 1 {
		t.Fatalf("the agent was started %d times, want once", len(specs))
	}
	if len(specs[0].MCPServers) != 0 || specs[0].Instructions != "" {
		t.Errorf("a session with no module set up was given %+v / %q", specs[0].MCPServers, specs[0].Instructions)
	}
}

// A card that never had a session is the only case that gets the context: a resume is given the
// servers again, because the request that brings the session back carries them, but not the
// instructions, which go with a new session's first message.
func TestAResumeIsGivenTheServerAndNotTheContext(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Do the work")

	attacher := &fakeAttacher{answer: func(card protocol.Card) (session.Attachment, error) {
		return session.Attachment{
			Servers:      []agents.MCPServer{server(card)},
			Instructions: "You are the Implementer.",
		}, nil
	}}
	e.mgr.SetAttacher(attacher)

	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	if err := e.mgr.StopCardSession(context.Background(), card.ID); err != nil {
		t.Fatalf("stop the card: %v", err)
	}
	waitFor(t, "the first session to end", func() bool { return len(attacher.detachedCards()) > 0 })
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("resume the card: %v", err)
	}

	specs := e.agent.startSpecs()
	if len(specs) != 2 {
		t.Fatalf("%d specs were recorded, want a start and a resume", len(specs))
	}
	resumed := specs[1]
	if len(resumed.MCPServers) != 1 {
		t.Errorf("the resumed session was given %+v, want the internal server again", resumed.MCPServers)
	}
	if resumed.Instructions != "" {
		t.Errorf("the resumed session was told %q, and a resume is not told the context again", resumed.Instructions)
	}
}
