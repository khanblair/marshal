package claude

import (
	"context"
	"errors"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

func TestStartReadsTheInitLineAndTheSessionIDMatches(t *testing.T) {
	a := fakeAdapter(t, "")
	h := start(t, a, startSpec(t))
	if !looksLikeUUID(h.ID) {
		t.Errorf("session id = %q, want a UUID", h.ID)
	}
	if !h.Capabilities.Resume || !h.Capabilities.StructuredEvents {
		t.Errorf("capabilities = %+v", h.Capabilities)
	}
}

func TestASessionIDMismatchIsLoggedButNotFatal(t *testing.T) {
	mh := &memHandler{}
	a := fakeAdapter(t, "badid", func(c *Config) { c.Logger = slog.New(mh) })
	h := start(t, a, startSpec(t))
	if !looksLikeUUID(h.ID) {
		t.Errorf("session id = %q, want a UUID", h.ID)
	}
	if !mh.contains("different session id") {
		t.Errorf("wanted a warning about the mismatched session id, got %v", mh.messages())
	}
}

func TestSendStreamsMessageAndToolCallEventsInOrderAndEndsWithEndTurn(t *testing.T) {
	a := fakeAdapter(t, "")
	h := start(t, a, startSpec(t))
	tr := send(t, a, h, "hello")

	if tr.text() != "echo: hello" {
		t.Errorf("text = %q, want %q", tr.text(), "echo: hello")
	}
	if len(tr.calls) != 1 || tr.calls[0].Kind != "execute" || tr.calls[0].Command != "echo hi" {
		t.Fatalf("calls = %+v", tr.calls)
	}
	if len(tr.updates) != 1 || tr.updates[0].Status != agents.StatusCompleted || tr.updates[0].Content != "hi\n" {
		t.Fatalf("updates = %+v", tr.updates)
	}
	if tr.updates[0].ID != tr.calls[0].ID {
		t.Errorf("update id = %q, want to match the call id %q", tr.updates[0].ID, tr.calls[0].ID)
	}
	if tr.ended == nil || tr.ended.Reason != agents.TurnEndTurn {
		t.Fatalf("ended = %+v, want TurnEndTurn", tr.ended)
	}
	wantOrder := []string{"message", "message", "tool_call", "tool_update", "turn_ended"}
	if strings.Join(tr.order, ",") != strings.Join(wantOrder, ",") {
		t.Errorf("order = %v, want %v (the complete assistant text block must not repeat the streamed text)", tr.order, wantOrder)
	}
}

func TestASecondSendDuringATurnReturnsErrBusy(t *testing.T) {
	a := fakeAdapter(t, "")
	h := start(t, a, startSpec(t))
	if err := a.Send(context.Background(), h, agents.UserMessage{Text: "one"}); err != nil {
		t.Fatalf("first Send: %v", err)
	}
	if err := a.Send(context.Background(), h, agents.UserMessage{Text: "two"}); !errors.Is(err, agents.ErrBusy) {
		t.Errorf("second Send error = %v, want ErrBusy", err)
	}
	collect(t, a.Events(h)) // drain the first turn so cleanup does not race it
}

func TestInterruptEndsTheTurnWhenClaudeCodeAcksIt(t *testing.T) {
	a := fakeAdapter(t, "hold")
	h := start(t, a, startSpec(t))
	if err := a.Send(context.Background(), h, agents.UserMessage{Text: "hello"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := a.Interrupt(context.Background(), h); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	tr := collect(t, a.Events(h))
	if tr.ended == nil || tr.ended.Reason != agents.TurnCancelled {
		t.Fatalf("ended = %+v, want TurnCancelled", tr.ended)
	}
}

// TestInterruptEndsTheTurnWithSIGINTWhenClaudeCodeIgnoresTheControlRequest covers the second
// interrupt stage: Claude Code's documentation names SIGINT as the way to end a turn without
// leaving it "unfinished" for a later --resume to replay, and the fake answers it that way unless
// told not to. This test proves the fast path: no process restart is needed, so a further Send
// keeps working on the very same process.
func TestInterruptEndsTheTurnWithSIGINTWhenClaudeCodeIgnoresTheControlRequest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no portable way to send SIGINT to another process on windows; see interrupt_windows.go")
	}
	a := fakeAdapter(t, "hold,ignoreinterrupt,showargs", func(c *Config) { c.InterruptGrace = 200 * time.Millisecond })
	h := start(t, a, startSpec(t))
	if err := a.Send(context.Background(), h, agents.UserMessage{Text: "hello"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := a.Interrupt(context.Background(), h); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	tr := collect(t, a.Events(h))
	if tr.ended == nil || tr.ended.Reason != agents.TurnCancelled {
		t.Fatalf("ended = %+v, want TurnCancelled", tr.ended)
	}
	pid1 := extractPid(tr.text())
	if pid1 == "" {
		t.Fatalf("text = %q, no pid found", tr.text())
	}

	// A further Send answers on the very same process: no restart, so the pid does not change.
	tr2 := send(t, a, h, "again")
	if tr2.ended == nil || tr2.ended.Reason != agents.TurnEndTurn {
		t.Fatalf("second turn ended = %+v, want a normal end with no restart", tr2.ended)
	}
	if pid2 := extractPid(tr2.text()); pid2 != pid1 {
		t.Errorf("pid = %q after SIGINT, want the same pid as before (%q): no restart should have happened", pid2, pid1)
	}
}

// TestInterruptFallsBackToRestartingWhenClaudeCodeNeverAnswers covers the last-resort stage: both
// the control_request and SIGINT get no answer, so the adapter stops the process and resumes it.
func TestInterruptFallsBackToRestartingWhenClaudeCodeNeverAnswers(t *testing.T) {
	a := fakeAdapter(t, "hold,ignoreinterrupt,ignoresigint,showargs", func(c *Config) { c.InterruptGrace = 150 * time.Millisecond })
	h := start(t, a, startSpec(t))
	if err := a.Send(context.Background(), h, agents.UserMessage{Text: "hello"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := a.Interrupt(context.Background(), h); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	tr := collect(t, a.Events(h))
	if tr.ended == nil || tr.ended.Reason != agents.TurnCancelled {
		t.Fatalf("ended = %+v, want TurnCancelled", tr.ended)
	}
	pid1 := extractPid(tr.text())
	if pid1 == "" {
		t.Fatalf("text = %q, no pid found", tr.text())
	}

	// The next Send transparently starts a fresh process (a different pid) for the same session.
	tr2 := send(t, a, h, "again")
	if tr2.ended == nil || tr2.ended.Reason != agents.TurnEndTurn {
		t.Fatalf("second turn ended = %+v, want a normal end after the restart", tr2.ended)
	}
	if !strings.HasPrefix(tr2.text(), "echo: again") {
		t.Errorf("text = %q, want it to start with %q", tr2.text(), "echo: again")
	}
	if pid2 := extractPid(tr2.text()); pid2 == pid1 || pid2 == "" {
		t.Errorf("pid = %q after the restart, want a different pid than before (%q)", pid2, pid1)
	}
}

// extractPid reads the "pid=<n>" token that the fake claude program always streams (see
// runFakeClaude), so a test can tell whether the same process answered two turns or a fresh one
// did.
func extractPid(text string) string {
	const marker = "pid="
	i := strings.Index(text, marker)
	if i < 0 {
		return ""
	}
	rest := text[i+len(marker):]
	if j := strings.IndexByte(rest, ' '); j >= 0 {
		return rest[:j]
	}
	return rest
}

func TestInterruptDoesNothingWithNoTurnRunning(t *testing.T) {
	a := fakeAdapter(t, "")
	h := start(t, a, startSpec(t))
	if err := a.Interrupt(context.Background(), h); err != nil {
		t.Errorf("Interrupt: %v", err)
	}
}

func TestResumeContinuesTheSameSessionWithAFreshProcess(t *testing.T) {
	a := fakeAdapter(t, "showargs")
	h := start(t, a, agents.StartSpec{Cwd: t.TempDir(), Instructions: "be terse"})
	tr := send(t, a, h, "one")
	if tr.ended == nil || tr.ended.Reason != agents.TurnEndTurn {
		t.Fatalf("ended = %+v", tr.ended)
	}
	pid1 := extractPid(tr.text())
	if pid1 == "" {
		t.Fatalf("text = %q, no pid found", tr.text())
	}
	stopAndDrain(t, a, h)

	h2, err := a.Resume(context.Background(), h.ID, startSpec(t))
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if h2.ID != h.ID {
		t.Errorf("resumed id = %q, want %q", h2.ID, h.ID)
	}
	t.Cleanup(func() { stopAndDrain(t, a, h2) })
	tr2 := send(t, a, h2, "two")
	if !strings.HasPrefix(tr2.text(), "echo: two") {
		t.Errorf("text = %q, want it to start with %q", tr2.text(), "echo: two")
	}
	if strings.Contains(tr2.text(), "--append-system-prompt") {
		t.Errorf("text = %q, resume must not resend the instructions", tr2.text())
	}
	if !strings.Contains(tr2.text(), "--resume="+h.ID) {
		t.Errorf("text = %q, want --resume=%s", tr2.text(), h.ID)
	}
	if pid2 := extractPid(tr2.text()); pid2 == pid1 || pid2 == "" {
		t.Errorf("pid = %q after Resume, want a different pid than the first process (%q)", pid2, pid1)
	}
}

func TestResumeWithNoIDFails(t *testing.T) {
	a := fakeAdapter(t, "")
	_, err := a.Resume(context.Background(), "", startSpec(t))
	if !errors.Is(err, agents.ErrCannotResume) {
		t.Errorf("err = %v, want ErrCannotResume", err)
	}
}

func TestResumeWithAMalformedIDFails(t *testing.T) {
	a := fakeAdapter(t, "")
	_, err := a.Resume(context.Background(), "not-a-uuid", startSpec(t))
	if !errors.Is(err, agents.ErrCannotResume) {
		t.Errorf("err = %v, want ErrCannotResume", err)
	}
}

func TestAnUnexpectedExitGivesFailedThenExitedWithTheStderrTail(t *testing.T) {
	a := fakeAdapter(t, "crashmidturn")
	h := start(t, a, startSpec(t))
	tr := send(t, a, h, "hello")

	if len(tr.failed) != 1 {
		t.Fatalf("failed events = %d, want 1: %+v", len(tr.failed), tr.failed)
	}
	if !strings.Contains(tr.failed[0].Detail, "crash mid-turn") {
		t.Errorf("detail = %q, want the stderr tail", tr.failed[0].Detail)
	}
	if tr.exited == nil {
		t.Fatalf("no Exited event; order = %v", tr.order)
	}
	last := tr.order[len(tr.order)-2:]
	if strings.Join(last, ",") != "failed,exited" {
		t.Errorf("order tail = %v, want Failed immediately before Exited", last)
	}
}

// TestAFailedResultGivesFailedAndTurnError covers a turn that Claude Code itself calls an error,
// with the process staying alive (unlike the crash above): a "result" line with is_error true.
func TestAFailedResultGivesFailedAndTurnError(t *testing.T) {
	a := fakeAdapter(t, "failresult,toolfails")
	h := start(t, a, startSpec(t))
	tr := send(t, a, h, "hello")

	if len(tr.updates) != 1 || tr.updates[0].Status != agents.StatusFailed {
		t.Fatalf("tool updates = %+v, want one failed update", tr.updates)
	}
	if len(tr.failed) != 1 || tr.failed[0].Message != "could not finish" {
		t.Fatalf("failed = %+v", tr.failed)
	}
	if tr.failed[0].Detail != "error_during_execution" {
		t.Errorf("detail = %q, want the result's subtype", tr.failed[0].Detail)
	}
	if tr.ended == nil || tr.ended.Reason != agents.TurnError {
		t.Fatalf("ended = %+v, want TurnError", tr.ended)
	}
}

func TestStartFailsWithTheStderrTailWhenClaudeCodeCrashesBeforeReady(t *testing.T) {
	a := fakeAdapter(t, "crash")
	_, err := a.Start(context.Background(), startSpec(t))
	if err == nil {
		t.Fatal("Start succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "crash before starting") {
		t.Errorf("err = %v, want the stderr tail", err)
	}
}

// TestStartTimesOutWhenClaudeCodeNeverBecomesReady covers a process that neither answers with an
// init line nor exits: Config.StartTimeout is set shorter than Config.ReadyWindow (which keeps
// its 2s default here), so the outer timeout is what fires, proving Start does not wait forever.
func TestStartTimesOutWhenClaudeCodeNeverBecomesReady(t *testing.T) {
	a := fakeAdapter(t, "noready", func(c *Config) { c.StartTimeout = 200 * time.Millisecond })
	_, err := a.Start(context.Background(), startSpec(t))
	if err == nil {
		t.Fatal("Start succeeded, want a timeout error")
	}
}

// TestStartBecomesReadyWithoutAnInitLine covers the case the brief flagged as unverified: Claude
// Code may not send "init" before it needs input at all. A process that stays alive past
// Config.ReadyWindow, with no init line, must still let Start succeed and a later Send work.
func TestStartBecomesReadyWithoutAnInitLine(t *testing.T) {
	a := fakeAdapter(t, "noready", func(c *Config) { c.ReadyWindow = 100 * time.Millisecond })
	h := start(t, a, startSpec(t))
	tr := send(t, a, h, "hello")
	if !strings.HasPrefix(tr.text(), "echo: hello") {
		t.Errorf("text = %q, want it to start with %q", tr.text(), "echo: hello")
	}
	if tr.ended == nil || tr.ended.Reason != agents.TurnEndTurn {
		t.Fatalf("ended = %+v, want TurnEndTurn", tr.ended)
	}
}

// TestStartBecomesReadyWithALateInitLine covers the other order: "init" arrives, but only once
// the first message is sent, instead of right away. Start must not depend on which order a real
// Claude Code actually uses.
func TestStartBecomesReadyWithALateInitLine(t *testing.T) {
	a := fakeAdapter(t, "lateinit", func(c *Config) { c.ReadyWindow = 100 * time.Millisecond })
	h := start(t, a, startSpec(t))
	tr := send(t, a, h, "hello")
	if !strings.HasPrefix(tr.text(), "echo: hello") {
		t.Errorf("text = %q, want it to start with %q", tr.text(), "echo: hello")
	}
	if tr.ended == nil || tr.ended.Reason != agents.TurnEndTurn {
		t.Fatalf("ended = %+v, want TurnEndTurn", tr.ended)
	}
}

func TestStopIsIdempotent(t *testing.T) {
	a := fakeAdapter(t, "")
	h := start(t, a, startSpec(t))
	ctx := context.Background()
	if err := a.Stop(ctx, h); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := a.Stop(ctx, h); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	for range a.Events(h) {
	}
}

func TestStopOnAnUnknownSessionIsNotAnError(t *testing.T) {
	a := fakeAdapter(t, "")
	if err := a.Stop(context.Background(), agents.SessionHandle{ID: "nothing-here"}); err != nil {
		t.Errorf("Stop = %v, want nil", err)
	}
}

func TestEventsOnAnUnknownSessionIsClosedAndEmpty(t *testing.T) {
	a := fakeAdapter(t, "")
	events := a.Events(agents.SessionHandle{ID: "nothing-here"})
	for range events {
		t.Fatal("expected no events")
	}
}

func TestRespondReturnsErrUnknownRequest(t *testing.T) {
	a := fakeAdapter(t, "")
	h := start(t, a, startSpec(t))
	err := a.Respond(context.Background(), h, agents.ApprovalResponse{RequestID: "x"})
	if !errors.Is(err, agents.ErrUnknownRequest) {
		t.Errorf("Respond = %v, want ErrUnknownRequest", err)
	}
}

// TestCapabilitiesMatchesTheCatalogsClaudeSpec hardcodes catalog/specs.go's claudeSpec
// capabilities: catalog has no exported accessor for one kind's spec without probing a real
// binary (see the report), so this is copied by hand and must be kept in sync with it, on the
// fields agents.Capabilities has. agents.Capabilities has no Approvals field (see the report's
// Ruling); Respond always failing is how this adapter expresses that it has none.
func TestCapabilitiesMatchesTheCatalogsClaudeSpec(t *testing.T) {
	a := fakeAdapter(t, "")
	want := agents.Capabilities{Resume: true, StructuredEvents: true, ModelSwitching: true, Thinking: true, MCP: true}
	if got := a.Capabilities(); got != want {
		t.Errorf("Capabilities = %+v, want %+v", got, want)
	}
}

func TestPermissionModesMapToClaudeCodesChoices(t *testing.T) {
	tests := []struct{ mode, want string }{
		{"ask", "manual"}, {"auto-edits", "acceptEdits"}, {"plan", "plan"},
		{"full-auto", "auto"}, {"bypass", "bypassPermissions"},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			a := fakeAdapter(t, "showargs")
			h := start(t, a, agents.StartSpec{Cwd: t.TempDir(), PermissionMode: tt.mode})
			if !h.Applied.PermissionMode || h.PermissionMode != tt.mode {
				t.Errorf("handle = %+v", h)
			}
			tr := send(t, a, h, "hi")
			if !strings.Contains(tr.text(), "--permission-mode="+tt.want) {
				t.Errorf("text = %q, want --permission-mode=%s", tr.text(), tt.want)
			}
			if !strings.Contains(tr.text(), "--permission-prompts=none") {
				t.Errorf("text = %q, want --permission-prompts=none on every mode", tr.text())
			}
		})
	}
}

func TestAnUnsupportedPermissionModeFailsStart(t *testing.T) {
	a := fakeAdapter(t, "")
	_, err := a.Start(context.Background(), agents.StartSpec{Cwd: t.TempDir(), PermissionMode: "nonsense"})
	if !errors.Is(err, agents.ErrUnsupportedSetting) {
		t.Errorf("err = %v, want ErrUnsupportedSetting", err)
	}
}

func TestAnUnsupportedThinkingModeFailsStart(t *testing.T) {
	a := fakeAdapter(t, "")
	_, err := a.Start(context.Background(), agents.StartSpec{Cwd: t.TempDir(), Thinking: "nonsense"})
	if !errors.Is(err, agents.ErrUnsupportedSetting) {
		t.Errorf("err = %v, want ErrUnsupportedSetting", err)
	}
}

func TestModelEffortAndInstructionsBecomeArguments(t *testing.T) {
	a := fakeAdapter(t, "showargs")
	h := start(t, a, agents.StartSpec{
		Cwd: t.TempDir(), Model: "opus", Thinking: "extra-high", Instructions: "be terse",
	})
	if !h.Applied.Model || !h.Applied.Thinking {
		t.Errorf("applied = %+v", h.Applied)
	}
	tr := send(t, a, h, "hi")
	for _, want := range []string{"--model=opus", "--effort=xhigh", "--append-system-prompt=be terse", "--session-id="} {
		if !strings.Contains(tr.text(), want) {
			t.Errorf("text = %q, want to contain %q", tr.text(), want)
		}
	}
}

func TestCwdMustBeAbsolute(t *testing.T) {
	a := fakeAdapter(t, "")
	_, err := a.Start(context.Background(), agents.StartSpec{Cwd: "relative/path"})
	if err == nil {
		t.Fatal("Start succeeded, want an error")
	}
}

// memHandler is a slog.Handler that keeps every message, for a test that checks a log line was
// written without caring where the daemon sends logs in production.
type memHandler struct {
	mu   sync.Mutex
	recs []string
}

func (h *memHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *memHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recs = append(h.recs, r.Message)
	return nil
}

func (h *memHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *memHandler) WithGroup(string) slog.Handler      { return h }

func (h *memHandler) messages() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.recs...)
}

func (h *memHandler) contains(substr string) bool {
	for _, m := range h.messages() {
		if strings.Contains(m, substr) {
			return true
		}
	}
	return false
}
