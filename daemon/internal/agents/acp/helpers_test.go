package acp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

const (
	eventTimeout = 20 * time.Second
	stopTimeout  = 15 * time.Second
)

func TestMain(m *testing.M) {
	// The fake agent is this test binary started again, in a mode that speaks the protocol.
	if flags := os.Getenv(fakeEnv); flags != "" {
		runFakeAgent(flags)
		os.Exit(0)
	}
	code := m.Run()
	testutil.CleanStubAgent()
	if code == 0 {
		if err := goleak.Find(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}
	os.Exit(code)
}

// quietLogger keeps test output clean.
func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newAdapter makes an adapter for a config, with test defaults.
func newAdapter(t *testing.T, cfg Config) *Adapter {
	t.Helper()
	if cfg.Logger == nil {
		cfg.Logger = quietLogger()
	}
	if cfg.StopGrace == 0 {
		cfg.StopGrace = 2 * time.Second
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

// newStubAdapter makes an adapter for the stub agent. The state folder is where the stub keeps its
// sessions, and pauses are off unless extra arguments turn them back on.
func newStubAdapter(t *testing.T, stateDir string, extra ...string) *Adapter {
	t.Helper()
	args := append([]string{"--state-dir", stateDir, "--speed", "0"}, extra...)
	return newAdapter(t, Config{Path: testutil.StubAgent(t), Args: args})
}

// newFakeAdapter makes an adapter for the fake agent with the given behaviors. The value always
// starts with a word of its own, because an empty value would not switch the fake on.
func newFakeAdapter(t *testing.T, flags string, mods ...func(*Config)) *Adapter {
	t.Helper()
	cfg := Config{Path: os.Args[0], Env: []string{fakeEnv + "=fake," + flags}}
	for _, mod := range mods {
		mod(&cfg)
	}
	return newAdapter(t, cfg)
}

// stubSpec is a start spec with a fresh working folder.
func stubSpec(t *testing.T) agents.StartSpec {
	t.Helper()
	return agents.StartSpec{Cwd: t.TempDir(), Label: "card-1"}
}

// start starts a session and stops it when the test ends.
func start(t *testing.T, a *Adapter, spec agents.StartSpec) agents.SessionHandle {
	t.Helper()
	h, err := a.Start(t.Context(), spec)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { stopQuietly(t, a, h) })
	return h
}

// stopQuietly stops a session and reads its events until the channel closes.
func stopQuietly(t *testing.T, a *Adapter, h agents.SessionHandle) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()
	if err := a.Stop(ctx, h); err != nil {
		t.Errorf("Stop: %v", err)
	}
	drain(t, a.Events(h))
}

// drain reads a channel until it closes.
func drain(t *testing.T, ch <-chan agents.AgentEvent) []agents.AgentEvent {
	t.Helper()
	var got []agents.AgentEvent
	timer := time.After(eventTimeout)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return got
			}
			got = append(got, ev)
		case <-timer:
			t.Fatalf("the event channel did not close; events so far: %s", describe(got))
			return got
		}
	}
}

// until reads events until the test says stop, and returns everything read, including the last.
func until(t *testing.T, ch <-chan agents.AgentEvent, stop func(agents.AgentEvent) bool) []agents.AgentEvent {
	t.Helper()
	var got []agents.AgentEvent
	timer := time.After(eventTimeout)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("the event channel closed too early; events so far: %s", describe(got))
			}
			got = append(got, ev)
			if stop(ev) {
				return got
			}
		case <-timer:
			t.Fatalf("no matching event arrived; events so far: %s", describe(got))
			return got
		}
	}
}

// untilTurnEnds reads events through the TurnEnded event.
func untilTurnEnds(t *testing.T, ch <-chan agents.AgentEvent) []agents.AgentEvent {
	t.Helper()
	return until(t, ch, func(ev agents.AgentEvent) bool {
		_, ok := ev.(agents.TurnEnded)
		return ok
	})
}

// untilPermission reads events through the PermissionRequested event and returns it.
func untilPermission(t *testing.T, ch <-chan agents.AgentEvent) agents.PermissionRequested {
	t.Helper()
	got := until(t, ch, func(ev agents.AgentEvent) bool {
		_, ok := ev.(agents.PermissionRequested)
		return ok
	})
	return got[len(got)-1].(agents.PermissionRequested)
}

// describe names events briefly, for messages and for comparing an order of events.
func describe(events []agents.AgentEvent) string {
	names := make([]string, 0, len(events))
	for _, ev := range events {
		names = append(names, name(ev))
	}
	return strings.Join(names, ", ")
}

// name is a short name for an event.
func name(ev agents.AgentEvent) string {
	switch e := ev.(type) {
	case agents.MessageChunk:
		return "message"
	case agents.ThoughtChunk:
		return "thought"
	case agents.ToolCall:
		return "tool:" + e.Status
	case agents.ToolCallUpdate:
		return "update:" + e.Status
	case agents.PlanUpdate:
		return "plan"
	case agents.PermissionRequested:
		return "permission"
	case agents.TurnEnded:
		return "turn-ended:" + e.Reason
	case agents.Failed:
		return "failed"
	case agents.Exited:
		return "exited"
	}
	return fmt.Sprintf("%T", ev)
}

// messageText joins the message chunks of a list of events.
func messageText(events []agents.AgentEvent) string {
	var b strings.Builder
	for _, ev := range events {
		if m, ok := ev.(agents.MessageChunk); ok {
			b.WriteString(m.Text)
		}
	}
	return b.String()
}

// sessionFor returns the adapter's internal session, for the tests that reach past the interface.
func sessionFor(t *testing.T, a *Adapter, h agents.SessionHandle) *session {
	t.Helper()
	s, err := a.find(h)
	if err != nil {
		t.Fatalf("find the session: %v", err)
	}
	return s
}

// send sends a message and fails the test if that is refused.
func send(t *testing.T, a *Adapter, h agents.SessionHandle, text string) {
	t.Helper()
	if err := a.Send(t.Context(), h, agents.UserMessage{Text: text}); err != nil {
		t.Fatalf("Send(%q): %v", text, err)
	}
}

// mustBe fails the test unless err matches target.
func mustBe(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("error = %v, want it to be %v", err, target)
	}
}
