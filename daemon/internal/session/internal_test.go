package session

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// stubbedAgent is a minimal agents.Agent for the one internal test that needs to call
// Manager.goLive directly: every method but Stop is unused and panics if it ever is called.
type stubbedAgent struct {
	stopped bool
}

func (*stubbedAgent) Start(context.Context, agents.StartSpec) (agents.SessionHandle, error) {
	panic("not used by this test")
}

func (*stubbedAgent) Resume(context.Context, string, agents.StartSpec) (agents.SessionHandle, error) {
	panic("not used by this test")
}

func (*stubbedAgent) Send(context.Context, agents.SessionHandle, agents.UserMessage) error {
	panic("not used by this test")
}

func (*stubbedAgent) Interrupt(context.Context, agents.SessionHandle) error {
	panic("not used by this test")
}

func (*stubbedAgent) Events(agents.SessionHandle) <-chan agents.AgentEvent {
	panic("not used by this test")
}

func (*stubbedAgent) Respond(context.Context, agents.SessionHandle, agents.ApprovalResponse) error {
	panic("not used by this test")
}

func (a *stubbedAgent) Stop(context.Context, agents.SessionHandle) error {
	a.stopped = true
	return nil
}

func (*stubbedAgent) Capabilities() agents.Capabilities { return agents.Capabilities{} }

// TestGoLiveStopsTheAgentWhenTheManagerIsAlreadyClosed covers the race goLive exists for: a
// Start or Resume that was in flight when Close ran must not register its session (which would
// orphan the process Close already believes it accounted for), and must stop the agent it just
// got instead.
func TestGoLiveStopsTheAgentWhenTheManagerIsAlreadyClosed(t *testing.T) {
	agent := &stubbedAgent{}
	m := &Manager{log: slog.New(slog.DiscardHandler), closed: true}
	diskLog, err := newSessionLog(t.TempDir(), "sess-1", 1024)
	if err != nil {
		t.Fatalf("newSessionLog: %v", err)
	}
	ls := &liveSession{
		owner: owner{cardID: "card-1"}, agent: agent, handle: agents.SessionHandle{ID: "sess-1"}, diskLog: diskLog,
	}

	err = m.goLive(ls)
	if err == nil {
		t.Fatal("goLive on an already-closed manager should return an error")
	}
	if !agent.stopped {
		t.Error("goLive should stop the agent it was about to register")
	}
	// No pump will ever run for the session, so goLive closes its log itself: a second close is the
	// no-op that proves it was already closed, and the flush goroutine is gone (the package's
	// leak check would fail the run otherwise).
	diskLog.mu.Lock()
	closed := diskLog.closed
	diskLog.mu.Unlock()
	if !closed {
		t.Error("goLive left the log of a session that never went live open")
	}
}

func TestEntryRingDropsTheOldestWholeEntry(t *testing.T) {
	r := newEntryRing(10)
	r.add(LogEntry{Kind: "a"}, 4)
	r.add(LogEntry{Kind: "b"}, 4)
	r.add(LogEntry{Kind: "c"}, 4) // pushes total to 12, over the bound of 10: "a" is dropped whole
	got := r.snapshot()
	if len(got) != 2 || got[0].Kind != "b" || got[1].Kind != "c" {
		t.Fatalf("snapshot = %+v, want [b c]", got)
	}
}

func TestEntryRingKeepsAtLeastOneEntryEvenWhenItAloneIsOverBudget(t *testing.T) {
	r := newEntryRing(4)
	r.add(LogEntry{Kind: "huge"}, 1000)
	r.add(LogEntry{Kind: "next"}, 1)
	got := r.snapshot()
	if len(got) != 1 || got[0].Kind != "next" {
		t.Fatalf("snapshot = %+v, want [next] (the huge entry dropped, one entry always kept)", got)
	}
}

func TestSessionLogRotatesAndKeepsOldSegments(t *testing.T) {
	dir := t.TempDir()
	l, err := newSessionLog(dir, "sess-1", 20)
	if err != nil {
		t.Fatalf("newSessionLog: %v", err)
	}
	for i := range 5 {
		line, err := marshalLogLine(time.Now(), agents.MessageChunk{Text: "0123456789"})
		if err != nil {
			t.Fatalf("marshalLogLine #%d: %v", i, err)
		}
		if err := l.write(line, false); err != nil {
			t.Fatalf("write #%d: %v", i, err)
		}
	}
	if err := l.close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	segDir := sessionLogDir(dir, "sess-1")
	entries, err := os.ReadDir(segDir)
	if err != nil {
		t.Fatalf("read the log folder: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("segments = %d, want more than one (each line is well over the 20 byte limit alone)", len(entries))
	}
	for _, e := range entries {
		assertValidJSONL(t, filepath.Join(segDir, e.Name()))
	}
}

func TestSessionLogContinuesItsHighestSegmentOnReopen(t *testing.T) {
	dir := t.TempDir()
	first, err := newSessionLog(dir, "sess-1", defaultLogSegmentBytes)
	if err != nil {
		t.Fatalf("newSessionLog: %v", err)
	}
	line, err := marshalLogLine(time.Now(), agents.TurnEnded{Reason: agents.TurnEndTurn})
	if err != nil {
		t.Fatalf("marshalLogLine: %v", err)
	}
	if err := first.write(line, true); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := first.close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	second, err := newSessionLog(dir, "sess-1", defaultLogSegmentBytes)
	if err != nil {
		t.Fatalf("reopen newSessionLog: %v", err)
	}
	if err := second.write(line, true); err != nil {
		t.Fatalf("write after reopen: %v", err)
	}
	if err := second.close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	segDir := sessionLogDir(dir, "sess-1")
	entries, err := os.ReadDir(segDir)
	if err != nil {
		t.Fatalf("read the log folder: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("segments = %d, want exactly one (the reopen must continue it, not start a new one)", len(entries))
	}
	assertValidJSONL(t, filepath.Join(segDir, entries[0].Name()))
	data, err := os.ReadFile(filepath.Join(segDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("read the segment: %v", err)
	}
	if lines := strings.Count(string(data), "\n"); lines != 2 {
		t.Errorf("lines in the segment = %d, want 2 (one from each newSessionLog)", lines)
	}
}

func TestMarshalLogLineCoversEveryEventKind(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		ev   agents.AgentEvent
		kind string
	}{
		{"message", agents.MessageChunk{Text: "hi"}, "message"},
		{"thought", agents.ThoughtChunk{Text: "hmm"}, "thought"},
		{"tool call", agents.ToolCall{ID: "1", Title: "Read", Kind: "read", Status: agents.StatusInProgress}, "tool_call"},
		{"tool call update", agents.ToolCallUpdate{ID: "1", Status: agents.StatusCompleted}, "tool_call_update"},
		{"plan", agents.PlanUpdate{Steps: []agents.PlanStep{{Text: "step", Status: agents.PlanPending}}}, "plan"},
		{"permission requested", agents.PermissionRequested{RequestID: "p1", Title: "Run rm"}, "permission_requested"},
		{"turn ended", agents.TurnEnded{Reason: agents.TurnEndTurn}, "turn_ended"},
		{"failed", agents.Failed{Message: "broke", Detail: "stack"}, "failed"},
		{"exited clean", agents.Exited{Code: 0}, "exited"},
		{"exited with error", agents.Exited{Code: -1, Err: errBoom}, "exited"},
		{"terminal output", agents.TerminalOutput{Data: []byte("$ ls\n")}, "terminal_output"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := eventKind(tt.ev); got != tt.kind {
				t.Errorf("eventKind = %q, want %q", got, tt.kind)
			}
			line, err := marshalLogLine(now, tt.ev)
			if err != nil {
				t.Fatalf("marshalLogLine: %v", err)
			}
			var v map[string]any
			if err := json.Unmarshal(line, &v); err != nil {
				t.Fatalf("the line is not valid JSON: %v\n%s", err, line)
			}
			if v["kind"] != tt.kind {
				t.Errorf("line kind = %v, want %q", v["kind"], tt.kind)
			}
			if _, ok := v["at"]; !ok {
				t.Error("the line has no \"at\" field")
			}
		})
	}
}

var errBoom = &testError{"boom"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

// assertValidJSONL fails the test unless every line of path is valid JSON.
func assertValidJSONL(t *testing.T, path string) {
	t.Helper()
	f, err := os.Open(path) //nolint:gosec // test fixture path built from t.TempDir
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var v map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &v); err != nil {
			t.Errorf("%s: invalid JSON line %q: %v", path, scanner.Text(), err)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
}
