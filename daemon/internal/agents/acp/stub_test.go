package acp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

func TestStubDefaultScenarioStreamsInOrder(t *testing.T) {
	a := newStubAdapter(t, t.TempDir())
	spec := stubSpec(t)
	h := start(t, a, spec)
	events := a.Events(h)

	send(t, a, h, "please fix the token bug")
	got := untilTurnEnds(t, events)

	wantOrder := "message, message, tool:in_progress, update:completed, message, tool:in_progress, " +
		"update:completed, message, turn-ended:end_turn"
	if order := describe(got); order != wantOrder {
		t.Fatalf("events = %s\nwant     %s", order, wantOrder)
	}
	if text := messageText(got); !strings.HasPrefix(text, "Turn 1. I remember 0 earlier turns.") ||
		!strings.HasSuffix(text, "Done. The plan is in notes/plan.md.") {
		t.Errorf("message = %q", text)
	}
	read := got[2].(agents.ToolCall)
	if read.Kind != "read" || read.Title != "Read README.md" || read.Path != filepath.Join(spec.Cwd, "README.md") {
		t.Errorf("read call = %+v", read)
	}
	if done := got[3].(agents.ToolCallUpdate); done.ID != read.ID || !strings.Contains(done.Content, "known bug") {
		t.Errorf("read result = %+v", done)
	}
	edit := got[6].(agents.ToolCallUpdate)
	if len(edit.Diffs) != 1 || filepath.Base(edit.Diffs[0].Path) != "plan.md" ||
		!strings.Contains(edit.Diffs[0].NewText, "# Plan") {
		t.Errorf("edit diffs = %+v", edit.Diffs)
	}
	// The edit really happened in the working folder that the session was given.
	if _, err := os.Stat(filepath.Join(spec.Cwd, "notes", "plan.md")); err != nil {
		t.Errorf("the plan file is missing: %v", err)
	}
}

func TestStubHandleAndCapabilities(t *testing.T) {
	a := newStubAdapter(t, t.TempDir())
	spec := stubSpec(t)
	spec.Model, spec.Thinking, spec.PermissionMode = "some-model", "high", "ask"
	h := start(t, a, spec)

	if h.ID == "" || h.Label != "card-1" {
		t.Errorf("handle = %+v, want an id and the label", h)
	}
	// The stub has no controls, so the values are kept in the handle but not applied.
	if h.Model != "some-model" || h.Thinking != "high" || h.PermissionMode != "ask" || h.Applied != (agents.Applied{}) {
		t.Errorf("handle settings = %+v", h)
	}
	want := agents.Capabilities{Resume: true, LoadSession: true, StructuredEvents: true}
	if h.Capabilities != want {
		t.Errorf("handle capabilities = %+v, want %+v", h.Capabilities, want)
	}
	if got := a.Capabilities(); got != want {
		t.Errorf("adapter capabilities = %+v, want %+v", got, want)
	}
}

func TestStubPermissionAnswers(t *testing.T) {
	tests := []struct {
		name       string
		optionID   string
		wantFile   bool
		wantInText string
	}{
		{"allow", "allow", true, "The build folder is gone"},
		{"deny", "deny", false, "Sorry, I should not have asked"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newStubAdapter(t, t.TempDir())
			spec := stubSpec(t)
			h := start(t, a, spec)
			events := a.Events(h)

			send(t, a, h, "@scenario:approve clean up")
			ask := untilPermission(t, events)
			if ask.Command != "rm -rf build" || ask.Kind != "execute" || ask.Title != "Run rm -rf build" {
				t.Errorf("request = %+v", ask)
			}
			if len(ask.Options) != 2 || ask.Options[0].ID != "allow" || ask.Options[0].Kind != agents.OptionAllowOnce ||
				ask.Options[1].ID != "deny" || ask.Options[1].Kind != agents.OptionRejectOnce {
				t.Errorf("options = %+v", ask.Options)
			}
			resp := agents.ApprovalResponse{RequestID: ask.RequestID, OptionID: tt.optionID}
			if err := a.Respond(t.Context(), h, resp); err != nil {
				t.Fatalf("Respond: %v", err)
			}
			rest := untilTurnEnds(t, events)
			if !strings.Contains(messageText(rest), tt.wantInText) {
				t.Errorf("message = %q, want it to contain %q", messageText(rest), tt.wantInText)
			}
			_, err := os.Stat(filepath.Join(spec.Cwd, "notes", "fix.md"))
			if (err == nil) != tt.wantFile {
				t.Errorf("fix file exists = %v, want %v", err == nil, tt.wantFile)
			}
			if end := rest[len(rest)-1].(agents.TurnEnded); end.Reason != agents.TurnEndTurn {
				t.Errorf("turn ended with %q", end.Reason)
			}
		})
	}
}

func TestStubRespondValidatesTheAnswer(t *testing.T) {
	a := newStubAdapter(t, t.TempDir())
	h := start(t, a, stubSpec(t))
	events := a.Events(h)
	send(t, a, h, "@scenario:approve")
	ask := untilPermission(t, events)

	mustBe(t, a.Respond(t.Context(), h, agents.ApprovalResponse{RequestID: "perm-99", OptionID: "allow"}),
		agents.ErrUnknownRequest)
	mustBe(t, a.Respond(t.Context(), h, agents.ApprovalResponse{RequestID: ask.RequestID, OptionID: "maybe"}),
		agents.ErrUnknownOption)
	// The wrong answer left the request waiting, so a right one still works.
	if err := a.Respond(t.Context(), h, agents.ApprovalResponse{RequestID: ask.RequestID, Cancelled: true}); err != nil {
		t.Fatalf("Respond(cancelled): %v", err)
	}
	if end := untilTurnEnds(t, events); end[len(end)-1].(agents.TurnEnded).Reason != agents.TurnCancelled {
		t.Errorf("a dismissed request should end the turn as cancelled, got %s", describe(end))
	}
	mustBe(t, a.Respond(t.Context(), h, agents.ApprovalResponse{RequestID: ask.RequestID, OptionID: "allow"}),
		agents.ErrUnknownRequest)
}

func TestStubSecondSendIsBusyUntilTheTurnEnds(t *testing.T) {
	a := newStubAdapter(t, t.TempDir())
	h := start(t, a, stubSpec(t))
	events := a.Events(h)
	send(t, a, h, "@scenario:approve")
	ask := untilPermission(t, events)

	err := a.Send(t.Context(), h, agents.UserMessage{Text: "again"})
	mustBe(t, err, agents.ErrBusy)

	if err := a.Respond(t.Context(), h, agents.ApprovalResponse{RequestID: ask.RequestID, OptionID: "deny"}); err != nil {
		t.Fatalf("Respond: %v", err)
	}
	untilTurnEnds(t, events)
	// The turn is over, so the next message is accepted, and the agent remembers the first turn.
	send(t, a, h, "@scenario:resume next")
	next := untilTurnEnds(t, events)
	if text := messageText(next); !strings.HasPrefix(text, "Turn 2. I remember 1 earlier turns.") {
		t.Errorf("message = %q", text)
	}
}

func TestStubInterrupt(t *testing.T) {
	tests := []struct {
		name string
		args []string
		text string
		// ready reads events until the turn is at the point where it is interrupted.
		ready func(*testing.T, <-chan agents.AgentEvent)
	}{
		{"while waiting for permission", nil, "@scenario:approve", func(t *testing.T, ch <-chan agents.AgentEvent) {
			untilPermission(t, ch)
		}},
		{"while working", []string{"--speed", "1"}, "go", func(t *testing.T, ch <-chan agents.AgentEvent) {
			until(t, ch, func(ev agents.AgentEvent) bool { _, ok := ev.(agents.MessageChunk); return ok })
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newStubAdapter(t, t.TempDir(), tt.args...)
			spec := stubSpec(t)
			h := start(t, a, spec)
			events := a.Events(h)
			send(t, a, h, tt.text)
			tt.ready(t, events)

			if err := a.Interrupt(t.Context(), h); err != nil {
				t.Fatalf("Interrupt: %v", err)
			}
			got := untilTurnEnds(t, events)
			if end := got[len(got)-1].(agents.TurnEnded); end.Reason != agents.TurnCancelled {
				t.Fatalf("turn ended with %q, want cancelled (events: %s)", end.Reason, describe(got))
			}
			if _, err := os.Stat(filepath.Join(spec.Cwd, "notes", "plan.md")); err == nil {
				t.Error("the cancelled turn still wrote its file")
			}
			// The session is kept, and it counts the cancelled turn as one it remembers.
			send(t, a, h, "@scenario:resume again")
			next := untilTurnEnds(t, events)
			if text := messageText(next); !strings.HasPrefix(text, "Turn 2. I remember 1 earlier turns.") {
				t.Errorf("message after the interrupt = %q", text)
			}
		})
	}
}

func TestStubInterruptWithoutATurnDoesNothing(t *testing.T) {
	a := newStubAdapter(t, t.TempDir())
	h := start(t, a, stubSpec(t))
	if err := a.Interrupt(t.Context(), h); err != nil {
		t.Errorf("Interrupt with no turn: %v", err)
	}
}

func TestStubStuckLoopReportsSixFailures(t *testing.T) {
	a := newStubAdapter(t, t.TempDir())
	h := start(t, a, stubSpec(t))
	events := a.Events(h)
	send(t, a, h, "@scenario:stuck-loop build it")
	got := untilTurnEnds(t, events)

	var starts, failures int
	ids := map[string]bool{}
	for _, ev := range got {
		switch e := ev.(type) {
		case agents.ToolCall:
			starts++
			ids[e.ID] = true
			if e.Command != "npm run build" {
				t.Errorf("command = %q", e.Command)
			}
		case agents.ToolCallUpdate:
			if e.Status == agents.StatusFailed && strings.Contains(e.Content, "TS2304") {
				failures++
			}
		}
	}
	if starts != 6 || failures != 6 || len(ids) != 6 {
		t.Errorf("tool calls = %d, failed updates = %d, distinct ids = %d, want 6 of each", starts, failures, len(ids))
	}
}

func TestStubResumeInANewAdapter(t *testing.T) {
	state := t.TempDir()
	first := newStubAdapter(t, state)
	spec := stubSpec(t)
	h, err := first.Start(t.Context(), spec)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	send(t, first, h, "first question")
	untilTurnEnds(t, first.Events(h))
	stopQuietly(t, first, h)

	// A new adapter is a new process that only shares the state folder with the old one.
	second := newStubAdapter(t, state)
	resumed, err := second.Resume(t.Context(), h.ID, spec)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	t.Cleanup(func() { stopQuietly(t, second, resumed) })
	if resumed.ID != h.ID {
		t.Errorf("resumed id = %q, want %q", resumed.ID, h.ID)
	}
	events := second.Events(resumed)
	send(t, second, resumed, "second question")
	got := untilTurnEnds(t, events)
	// Nothing of the old conversation comes back as events.
	if text := messageText(got); !strings.HasPrefix(text, "Turn 2. I remember 1 earlier turns.") {
		t.Errorf("message = %q", text)
	}
	if order := describe(got[:1]); order != "message" {
		t.Errorf("first event = %s, want the new turn's message", order)
	}
}

func TestStubResumeOfAnUnknownSessionCannotResume(t *testing.T) {
	a := newStubAdapter(t, t.TempDir())
	_, err := a.Resume(t.Context(), "stub-does-not-exist", stubSpec(t))
	mustBe(t, err, agents.ErrCannotResume)
	_, err = a.Resume(t.Context(), "", stubSpec(t))
	mustBe(t, err, agents.ErrCannotResume)
}

func TestStubProcessKilledMidTurnFailsAndExits(t *testing.T) {
	a := newStubAdapter(t, t.TempDir())
	h := start(t, a, stubSpec(t))
	events := a.Events(h)
	send(t, a, h, "@scenario:approve")
	untilPermission(t, events)

	proc, err := os.FindProcess(sessionFor(t, a, h).proc.Pid())
	if err != nil {
		t.Fatalf("find the process: %v", err)
	}
	if err := proc.Kill(); err != nil {
		t.Fatalf("kill the agent: %v", err)
	}

	rest := drain(t, events)
	order := describe(rest)
	if !strings.HasSuffix(order, "failed, exited") || strings.Count(order, "failed") != 1 {
		t.Fatalf("events after the kill = %s, want one Failed and then Exited", order)
	}
	failed := rest[len(rest)-2].(agents.Failed)
	exited := rest[len(rest)-1].(agents.Exited)
	if !strings.Contains(failed.Message, "middle of its work") || failed.Detail == "" {
		t.Errorf("failed = %+v", failed)
	}
	// A signal shows as -1. Windows has no signals: it ends a killed process with exit code 1.
	killed := exited.Code == -1
	if runtime.GOOS == "windows" {
		killed = exited.Code != 0
	}
	if !killed || exited.Err == nil {
		t.Errorf("exited = %+v, want a kill", exited)
	}
	// The dead session is gone from the adapter, and stopping it is still fine.
	mustBe(t, a.Send(t.Context(), h, agents.UserMessage{Text: "hello?"}), agents.ErrUnknownSession)
	if err := a.Stop(t.Context(), h); err != nil {
		t.Errorf("Stop after the crash: %v", err)
	}
}

func TestStubStopIsIdempotentAndEndsWithExited(t *testing.T) {
	a := newStubAdapter(t, t.TempDir())
	h, err := a.Start(t.Context(), stubSpec(t))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	events := a.Events(h)

	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			if err := a.Stop(t.Context(), h); err != nil {
				t.Errorf("Stop: %v", err)
			}
		})
	}
	wg.Wait()
	if err := a.Stop(t.Context(), h); err != nil {
		t.Errorf("Stop again: %v", err)
	}

	got := drain(t, events)
	if len(got) == 0 {
		t.Fatal("no events after Stop, want Exited")
	}
	if _, ok := got[len(got)-1].(agents.Exited); !ok || strings.Contains(describe(got), "failed") {
		t.Errorf("events = %s, want them to end with Exited and hold no Failed", describe(got))
	}
	mustBe(t, a.Send(t.Context(), h, agents.UserMessage{Text: "hi"}), agents.ErrUnknownSession)
	mustBe(t, a.Respond(t.Context(), h, agents.ApprovalResponse{RequestID: "perm-1"}), agents.ErrUnknownSession)
	mustBe(t, a.Interrupt(t.Context(), h), agents.ErrUnknownSession)
	if _, open := <-a.Events(h); open {
		t.Error("Events of a stopped session should be a closed channel")
	}
}

func TestStubStopDuringATurn(t *testing.T) {
	a := newStubAdapter(t, t.TempDir())
	h, err := a.Start(t.Context(), stubSpec(t))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	events := a.Events(h)
	send(t, a, h, "@scenario:approve")
	untilPermission(t, events)

	begin := time.Now()
	if err := a.Stop(t.Context(), h); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if took := time.Since(begin); took > 10*time.Second {
		t.Errorf("Stop took %v with a request waiting", took)
	}
	got := drain(t, events)
	if strings.Contains(describe(got), "failed") {
		t.Errorf("a requested stop must not report a failure: %s", describe(got))
	}
}

func TestStubSessionOutlivesTheContextThatStartedIt(t *testing.T) {
	a := newStubAdapter(t, t.TempDir())
	ctx, cancel := context.WithCancel(t.Context())
	h, err := a.Start(ctx, stubSpec(t))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { stopQuietly(t, a, h) })
	cancel()

	send(t, a, h, "still here?")
	got := untilTurnEnds(t, a.Events(h))
	if end := got[len(got)-1].(agents.TurnEnded); end.Reason != agents.TurnEndTurn {
		t.Errorf("turn ended with %q", end.Reason)
	}
}

func TestStubStartWithACancelledContextFails(t *testing.T) {
	a := newStubAdapter(t, t.TempDir())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := a.Start(ctx, stubSpec(t)); err == nil {
		t.Fatal("Start with a cancelled context succeeded")
	}
}

func TestStubInstructionsRideWithTheFirstMessageOnly(t *testing.T) {
	state := t.TempDir()
	a := newStubAdapter(t, state)
	spec := stubSpec(t)
	spec.Instructions = "You are a careful reviewer."
	h := start(t, a, spec)
	events := a.Events(h)
	send(t, a, h, "first")
	untilTurnEnds(t, events)
	send(t, a, h, "second")
	untilTurnEnds(t, events)

	// The stub saves what it was asked, and joins the blocks of a prompt with a newline.
	saved, err := os.ReadFile(filepath.Join(state, h.ID+".json"))
	if err != nil {
		t.Fatalf("read the stub's saved session: %v", err)
	}
	text := string(saved)
	if !strings.Contains(text, `"user": "You are a careful reviewer.\nfirst"`) ||
		!strings.Contains(text, `"user": "second"`) {
		t.Errorf("saved session does not show the instructions once, with the first message:\n%s", text)
	}
}

func TestUnknownHandleIsRefused(t *testing.T) {
	a := newStubAdapter(t, t.TempDir())
	ghost := agents.SessionHandle{ID: "nobody"}
	mustBe(t, a.Send(t.Context(), ghost, agents.UserMessage{Text: "hi"}), agents.ErrUnknownSession)
	mustBe(t, a.Interrupt(t.Context(), ghost), agents.ErrUnknownSession)
	mustBe(t, a.Respond(t.Context(), ghost, agents.ApprovalResponse{}), agents.ErrUnknownSession)
	if err := a.Stop(t.Context(), ghost); err != nil {
		t.Errorf("Stop of an unknown session = %v, want nil", err)
	}
	if _, open := <-a.Events(ghost); open {
		t.Error("Events of an unknown session should be a closed channel")
	}
	if err := a.Send(cancelledContext(t), ghost, agents.UserMessage{}); err == nil {
		t.Error("Send with a cancelled context succeeded")
	}
	if err := a.Respond(cancelledContext(t), ghost, agents.ApprovalResponse{}); err == nil {
		t.Error("Respond with a cancelled context succeeded")
	}
}

func cancelledContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return ctx
}

func TestTwoSessionsRunSideBySide(t *testing.T) {
	a := newStubAdapter(t, t.TempDir())
	h1 := start(t, a, stubSpec(t))
	h2 := start(t, a, stubSpec(t))
	if h1.ID == h2.ID {
		t.Fatalf("both sessions have the id %q", h1.ID)
	}
	send(t, a, h1, "one")
	send(t, a, h2, "two")
	for _, h := range []agents.SessionHandle{h1, h2} {
		got := untilTurnEnds(t, a.Events(h))
		if !strings.HasPrefix(messageText(got), "Turn 1.") {
			t.Errorf("session %s: %s", h.ID, messageText(got))
		}
	}
}

func TestStartErrors(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		spec func(*testing.T) agents.StartSpec
		want string
	}{
		{"relative folder", Config{Path: "unused"}, func(*testing.T) agents.StartSpec {
			return agents.StartSpec{Cwd: "relative/dir"}
		}, "absolute"},
		{"missing program", Config{Path: filepath.Join(os.TempDir(), "no-such-agent-program")}, stubSpec, "start the agent program"},
		{"missing folder", Config{Path: os.Args[0]}, func(t *testing.T) agents.StartSpec {
			return agents.StartSpec{Cwd: filepath.Join(t.TempDir(), "absent")}
		}, "absent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newAdapter(t, tt.cfg)
			_, err := a.Start(t.Context(), tt.spec(t))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Start error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
	if _, err := New(Config{}); err == nil {
		t.Error("New without a program succeeded")
	}
}
