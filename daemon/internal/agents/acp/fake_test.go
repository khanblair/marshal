package acp

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// allowLogin lets the adapter sign in with the fake agent's "login" method on its own.
func allowLogin(c *Config) { c.AutoAuthMethods = []string{"login"} }

func TestSignsInByItselfWhenTheConfigAllowsIt(t *testing.T) {
	a := newFakeAdapter(t, "auth-agent", allowLogin)
	h := start(t, a, stubSpec(t))
	send(t, a, h, "hello")
	untilTurnEnds(t, a.Events(h))
}

func TestAuthRequiredNeedsTheUser(t *testing.T) {
	tests := []struct {
		name      string
		flags     string
		mods      []func(*Config)
		wantNames []string
		wantCause bool
	}{
		{"only a terminal sign-in", "auth-terminal", nil, []string{"Terminal login"}, false},
		{"an agent method that the config does not allow", "auth-agent", nil, []string{"Log in"}, false},
		{"automatic sign-in fails", "auth-agent,auth-fails", []func(*Config){allowLogin}, []string{"Log in"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newFakeAdapter(t, tt.flags, tt.mods...)
			_, err := a.Start(t.Context(), stubSpec(t))
			var authErr *agents.AuthRequiredError
			if !errors.As(err, &authErr) {
				t.Fatalf("Start error = %v, want an AuthRequiredError", err)
			}
			if strings.Join(authErr.Methods, ",") != strings.Join(tt.wantNames, ",") {
				t.Errorf("methods = %v, want %v", authErr.Methods, tt.wantNames)
			}
			if (authErr.Cause != nil) != tt.wantCause || errors.Unwrap(authErr) != authErr.Cause {
				t.Errorf("cause = %v, want a cause: %v", authErr.Cause, tt.wantCause)
			}
			if !strings.Contains(err.Error(), "sign in") {
				t.Errorf("error = %q, want a plain sentence about signing in", err)
			}
		})
	}
}

func TestSettingsAreAppliedThroughTheAgentsControls(t *testing.T) {
	withMaps := func(c *Config) {
		c.PermissionModes = map[string]string{"ask": "default", "auto-edits": "acceptEdits", "bypass": "bypassPermissions"}
		c.ThinkingModes = map[string]string{"extra-high": "high"}
	}
	tests := []struct {
		name     string
		spec     func(*agents.StartSpec)
		wantText string
		want     agents.Applied
	}{
		{"all three", func(s *agents.StartSpec) {
			s.Model, s.Thinking, s.PermissionMode = "opus", "extra-high", "auto-edits"
		}, "mode=acceptEdits model=opus thinking=high", agents.Applied{Model: true, Thinking: true, PermissionMode: true}},
		{"values that are already current", func(s *agents.StartSpec) {
			s.Model, s.Thinking, s.PermissionMode = "Sonnet", "low", "ask"
		}, "mode=default model=sonnet thinking=low", agents.Applied{Model: true, Thinking: true, PermissionMode: true}},
		{"a mode by its own name", func(s *agents.StartSpec) { s.PermissionMode = "plan" },
			"mode=plan model=sonnet", agents.Applied{PermissionMode: true}},
		{"nothing asked", func(*agents.StartSpec) {}, "mode=default model=sonnet thinking=low", agents.Applied{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newFakeAdapter(t, "controls,close", withMaps)
			spec := stubSpec(t)
			tt.spec(&spec)
			h := start(t, a, spec)
			if h.Applied != tt.want {
				t.Errorf("applied = %+v, want %+v", h.Applied, tt.want)
			}
			if !h.Capabilities.ModelSwitching || !h.Capabilities.Thinking {
				t.Errorf("capabilities = %+v, want model switching and thinking", h.Capabilities)
			}
			send(t, a, h, "status")
			if text := messageText(untilTurnEnds(t, a.Events(h))); !strings.Contains(text, tt.wantText) {
				t.Errorf("agent state = %q, want it to contain %q", text, tt.wantText)
			}
		})
	}
}

func TestSettingsTheAgentDoesNotOfferFailTheStart(t *testing.T) {
	tests := []struct {
		name string
		spec func(*agents.StartSpec)
		want string
	}{
		{"model", func(s *agents.StartSpec) { s.Model = "gpt-9" }, "sonnet, opus"},
		{"thinking", func(s *agents.StartSpec) { s.Thinking = "extra-high" }, "low, high"},
		{"permission mode", func(s *agents.StartSpec) { s.PermissionMode = "full-auto" }, "acceptEdits"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newFakeAdapter(t, "controls")
			spec := stubSpec(t)
			tt.spec(&spec)
			_, err := a.Start(t.Context(), spec)
			mustBe(t, err, agents.ErrUnsupportedSetting)
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to list what the agent offers (%s)", err, tt.want)
			}
		})
	}
}

func TestPermissionModeThroughAConfigOption(t *testing.T) {
	a := newFakeAdapter(t, "controls,mode-option")
	spec := stubSpec(t)
	spec.PermissionMode = "plan"
	h := start(t, a, spec)
	if !h.Applied.PermissionMode {
		t.Fatalf("applied = %+v, want the permission mode applied", h.Applied)
	}
	send(t, a, h, "status")
	if text := messageText(untilTurnEnds(t, a.Events(h))); !strings.Contains(text, "mode=plan") {
		t.Errorf("agent state = %q", text)
	}
}

func TestCapabilitiesFollowTheAgent(t *testing.T) {
	a := newFakeAdapter(t, "controls,resume,load,mcp")
	if got := a.Capabilities(); got != (agents.Capabilities{StructuredEvents: true}) {
		t.Errorf("before any session = %+v, want only structured events", got)
	}
	start(t, a, stubSpec(t))
	want := agents.Capabilities{
		Resume: true, LoadSession: true, StructuredEvents: true, ModelSwitching: true, Thinking: true, MCP: true,
	}
	if got := a.Capabilities(); got != want {
		t.Errorf("capabilities = %+v, want %+v", got, want)
	}
}

func TestResumeWays(t *testing.T) {
	tests := []struct {
		name  string
		flags string
	}{
		{"with the resume request", "resume"},
		{"by loading the session, and dropping the replay", "load"},
		{"resume is preferred when both exist", "resume,load"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newFakeAdapter(t, tt.flags)
			h, err := a.Resume(t.Context(), fakeSessionID, stubSpec(t))
			if err != nil {
				t.Fatalf("Resume: %v", err)
			}
			t.Cleanup(func() { stopQuietly(t, a, h) })
			events := a.Events(h)
			send(t, a, h, "carry on")
			got := untilTurnEnds(t, events)
			if strings.Contains(messageText(got), "old answer") {
				t.Errorf("the replayed history came back as events: %s", messageText(got))
			}
			if text := messageText(got); !strings.Contains(text, "history=1") {
				t.Errorf("the agent did not load its history: %q", text)
			}
		})
	}
}

func TestResumeCannotResume(t *testing.T) {
	tests := []struct{ name, flags string }{
		{"the agent cannot resume or load", ""},
		{"the agent does not know the session (load)", "load,unknown-session"},
		{"the agent does not know the session (resume)", "resume,unknown-session"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newFakeAdapter(t, tt.flags)
			_, err := a.Resume(t.Context(), fakeSessionID, stubSpec(t))
			mustBe(t, err, agents.ErrCannotResume)
		})
	}
}

func TestStartFailsClearly(t *testing.T) {
	tests := []struct{ name, flags, want string }{
		{"the agent crashes while starting", "crash-init", "boom: the agent cannot start"},
		{"the agent speaks another protocol version", "bad-version", "protocol version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newFakeAdapter(t, tt.flags)
			_, err := a.Start(t.Context(), stubSpec(t))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Start error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestStartTimesOutOnASilentAgent(t *testing.T) {
	a := newFakeAdapter(t, "hang-init", func(c *Config) { c.StartTimeout = 300 * time.Millisecond })
	begin := time.Now()
	_, err := a.Start(t.Context(), stubSpec(t))
	if err == nil {
		t.Fatal("Start succeeded, want a timeout")
	}
	if took := time.Since(begin); took > 10*time.Second {
		t.Errorf("Start took %v, want about the timeout", took)
	}
}

func TestRichTurnBecomesEvents(t *testing.T) {
	a := newFakeAdapter(t, "")
	h := start(t, a, stubSpec(t))
	send(t, a, h, "@rich please")
	got := untilTurnEnds(t, a.Events(h))

	wantOrder := "message, thought, plan, tool:pending, update:completed, message, turn-ended:end_turn"
	if order := describe(got); order != wantOrder {
		t.Fatalf("events = %s\nwant     %s", order, wantOrder)
	}
	if thought := got[1].(agents.ThoughtChunk); thought.Text != "thinking hard" {
		t.Errorf("thought = %+v", thought)
	}
	plan := got[2].(agents.PlanUpdate)
	if len(plan.Steps) != 2 || plan.Steps[0] != (agents.PlanStep{Text: "Read the code", Status: agents.PlanCompleted}) ||
		plan.Steps[1].Status != agents.PlanInProgress {
		t.Errorf("plan = %+v", plan)
	}
	call := got[3].(agents.ToolCall)
	if call.Path != "big.txt" || call.Command != "cat big.txt" || call.Kind != "read" {
		t.Errorf("call = %+v", call)
	}
	if len(call.Content) != agents.MaxContentBytes || !call.Truncated {
		t.Errorf("content has %d bytes and truncated = %v, want %d and true",
			len(call.Content), call.Truncated, agents.MaxContentBytes)
	}
	update := got[4].(agents.ToolCallUpdate)
	if update.Title != "Read big.txt fully" || len(update.Diffs) != 1 ||
		update.Diffs[0] != (agents.FileDiff{Path: "/work/a.txt", OldText: "old text", NewText: "new text"}) {
		t.Errorf("update = %+v", update)
	}
	if last := got[5].(agents.MessageChunk); last.Text != "the client says method not found" {
		t.Errorf("the file request was not refused: %q", last.Text)
	}
}

func TestTurnEndReasons(t *testing.T) {
	tests := []struct{ marker, want string }{
		{"", agents.TurnEndTurn},
		{"@stop:max_tokens", agents.TurnMaxTokens},
		{"@stop:refusal", agents.TurnRefusal},
		{"@stop:max_turn_requests", agents.TurnMaxRequests},
		{"@stop:cancelled", agents.TurnCancelled},
		{"@stop:something_new", agents.TurnEndTurn},
	}
	a := newFakeAdapter(t, "")
	h := start(t, a, stubSpec(t))
	events := a.Events(h)
	for _, tt := range tests {
		send(t, a, h, "go "+tt.marker)
		got := untilTurnEnds(t, events)
		if end := got[len(got)-1].(agents.TurnEnded); end.Reason != tt.want {
			t.Errorf("marker %q ended with %q, want %q", tt.marker, end.Reason, tt.want)
		}
	}
}

func TestAnErrorFromTheAgentFailsTheTurnAndKeepsTheSession(t *testing.T) {
	a := newFakeAdapter(t, "prompt-error")
	h := start(t, a, stubSpec(t))
	events := a.Events(h)
	for range 2 {
		send(t, a, h, "go")
		got := untilTurnEnds(t, events)
		if order := describe(got); order != "failed, turn-ended:error" {
			t.Fatalf("events = %s, want Failed and then TurnEnded(error)", order)
		}
		if failed := got[0].(agents.Failed); failed.Message == "" || !strings.Contains(failed.Detail, "overloaded") {
			t.Errorf("failed = %+v", failed)
		}
	}
}

func TestAProcessThatExitsDuringATurnReportsWhatItPrinted(t *testing.T) {
	a := newFakeAdapter(t, "exit-on-prompt")
	h, err := a.Start(t.Context(), stubSpec(t))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	events := a.Events(h)
	send(t, a, h, "go")
	got := drain(t, events)
	if order := describe(got); order != "failed, exited" {
		t.Fatalf("events = %s, want Failed and then Exited", order)
	}
	if failed := got[0].(agents.Failed); !strings.Contains(failed.Detail, "out of memory") {
		t.Errorf("failed detail = %q, want the agent's last words", failed.Detail)
	}
	if exited := got[1].(agents.Exited); exited.Code != 1 || exited.Err == nil {
		t.Errorf("exited = %+v, want code 1", exited)
	}
}

func TestInterruptGivesUpOnAnAgentThatIgnoresCancel(t *testing.T) {
	a := newFakeAdapter(t, "hang,close", func(c *Config) { c.InterruptGrace = 200 * time.Millisecond })
	h := start(t, a, stubSpec(t))
	events := a.Events(h)
	send(t, a, h, "go")
	if err := a.Interrupt(t.Context(), h); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	got := untilTurnEnds(t, events)
	if end := got[len(got)-1].(agents.TurnEnded); end.Reason != agents.TurnCancelled {
		t.Errorf("turn ended with %q, want cancelled", end.Reason)
	}
}

func TestStopWhileTheAgentIsHung(t *testing.T) {
	a := newFakeAdapter(t, "hang,close")
	h, err := a.Start(t.Context(), stubSpec(t))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	events := a.Events(h)
	send(t, a, h, "go")
	if err := a.Stop(t.Context(), h); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	got := drain(t, events)
	if strings.Contains(describe(got), "failed") {
		t.Errorf("a requested stop reported a failure: %s", describe(got))
	}
	mustBe(t, a.Send(t.Context(), h, agents.UserMessage{Text: "x"}), agents.ErrUnknownSession)
}
