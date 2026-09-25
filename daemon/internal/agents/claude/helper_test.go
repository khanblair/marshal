package claude

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

const (
	// eventTimeout is how long a test waits for a turn to end. It is generous for a race-detector
	// build on a slow CI machine, but every scenario here finishes in well under a second.
	eventTimeout = 15 * time.Second
	stopTimeout  = 10 * time.Second
)

// fakeAdapter makes an adapter for the fake claude program (see fake_test.go) with the given
// behaviors.
func fakeAdapter(t *testing.T, flags string, mods ...func(*Config)) agents.Agent {
	t.Helper()
	cfg := Config{
		Path: os.Args[0], Env: []string{fakeEnv + "=" + flags},
		StartTimeout: 5 * time.Second, StopGrace: 2 * time.Second, InterruptGrace: 2 * time.Second,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	for _, mod := range mods {
		mod(&cfg)
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

// startSpec returns a start spec pointed at a fresh temp folder.
func startSpec(t *testing.T) agents.StartSpec {
	t.Helper()
	return agents.StartSpec{Cwd: t.TempDir(), Label: "test"}
}

// start starts a session and stops it when the test ends.
func start(t *testing.T, a agents.Agent, spec agents.StartSpec) agents.SessionHandle {
	t.Helper()
	h, err := a.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { stopAndDrain(t, a, h) })
	return h
}

// stopAndDrain stops a session and reads its events until the channel closes.
func stopAndDrain(t *testing.T, a agents.Agent, h agents.SessionHandle) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()
	if err := a.Stop(ctx, h); err != nil {
		t.Errorf("Stop: %v", err)
	}
	for range a.Events(h) {
	}
}

// turn is what one call to send collected, in the order the events arrived.
type turn struct {
	chunks   []string
	thoughts []string
	calls    []agents.ToolCall
	updates  []agents.ToolCallUpdate
	failed   []agents.Failed
	ended    *agents.TurnEnded
	exited   *agents.Exited
	order    []string
}

// text joins the message chunks collected.
func (tr turn) text() string { return strings.Join(tr.chunks, "") }

// send sends a message and reads events until the turn ends (TurnEnded) or the channel closes
// (Exited), whichever comes first.
func send(t *testing.T, a agents.Agent, h agents.SessionHandle, text string) turn {
	t.Helper()
	if err := a.Send(context.Background(), h, agents.UserMessage{Text: text}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	return collect(t, a.Events(h))
}

// collect reads events until the turn ends or the channel closes.
func collect(t *testing.T, events <-chan agents.AgentEvent) turn {
	t.Helper()
	var tr turn
	deadline := time.After(eventTimeout)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return tr
			}
			tr.absorb(ev)
			if tr.ended != nil || tr.exited != nil {
				return tr
			}
		case <-deadline:
			t.Fatalf("timed out waiting for the turn to end; order so far: %v", tr.order)
		}
	}
}

// absorb records one event.
func (tr *turn) absorb(ev agents.AgentEvent) {
	switch e := ev.(type) {
	case agents.MessageChunk:
		tr.chunks = append(tr.chunks, e.Text)
		tr.order = append(tr.order, "message")
	case agents.ThoughtChunk:
		tr.thoughts = append(tr.thoughts, e.Text)
		tr.order = append(tr.order, "thought")
	case agents.ToolCall:
		tr.calls = append(tr.calls, e)
		tr.order = append(tr.order, "tool_call")
	case agents.ToolCallUpdate:
		tr.updates = append(tr.updates, e)
		tr.order = append(tr.order, "tool_update")
	case agents.Failed:
		tr.failed = append(tr.failed, e)
		tr.order = append(tr.order, "failed")
	case agents.TurnEnded:
		got := e
		tr.ended = &got
		tr.order = append(tr.order, "turn_ended")
	case agents.Exited:
		got := e
		tr.exited = &got
		tr.order = append(tr.order, "exited")
	}
}
