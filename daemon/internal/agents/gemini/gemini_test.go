package gemini

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

const (
	eventTimeout = 30 * time.Second
	stopTimeout  = 15 * time.Second
)

// fakeAdapter makes an adapter for the fake Gemini CLI with the given behaviors.
func fakeAdapter(t *testing.T, flags string) agents.Agent {
	t.Helper()
	a, err := New(Config{
		Path: os.Args[0], Env: []string{fakeEnv + "=fake," + flags},
		StopGrace: 2 * time.Second, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

// start starts a session and stops it when the test ends.
func start(t *testing.T, a agents.Agent, spec agents.StartSpec) agents.SessionHandle {
	t.Helper()
	if spec.Cwd == "" {
		spec.Cwd = t.TempDir()
	}
	h, err := a.Start(t.Context(), spec)
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

// ask sends a message and returns the text of the answer, read up to the end of the turn.
func ask(t *testing.T, a agents.Agent, h agents.SessionHandle, text string) string {
	t.Helper()
	if err := a.Send(t.Context(), h, agents.UserMessage{Text: text}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	var answer strings.Builder
	timer := time.After(eventTimeout)
	for {
		select {
		case ev, ok := <-a.Events(h):
			if !ok {
				t.Fatalf("the event channel closed before the turn ended; answer so far: %q", answer.String())
			}
			switch e := ev.(type) {
			case agents.MessageChunk:
				answer.WriteString(e.Text)
			case agents.TurnEnded:
				return answer.String()
			case agents.Failed:
				t.Fatalf("the agent failed: %s (%s)", e.Message, e.Detail)
			}
		case <-timer:
			t.Fatalf("the turn did not end; answer so far: %q", answer.String())
		}
	}
}

func TestPermissionModesBecomeGeminisApprovalModes(t *testing.T) {
	tests := []struct {
		mode string
		want string
	}{
		{"ask", "default"},
		{"auto-edits", "autoEdit"},
		{"plan", "plan"},
		{"full-auto", "yolo"},
		{"bypass", "yolo"},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			a := fakeAdapter(t, "")
			h := start(t, a, agents.StartSpec{PermissionMode: tt.mode})
			if !h.Applied.PermissionMode || h.PermissionMode != tt.mode {
				t.Errorf("handle = mode %q applied %v, want %q applied", h.PermissionMode, h.Applied.PermissionMode, tt.mode)
			}
			if got := ask(t, a, h, "hello"); !strings.Contains(got, "mode="+tt.want+" ") {
				t.Errorf("answer = %q, want Gemini to be in mode %q", got, tt.want)
			}
		})
	}
}

func TestAModeThatGeminiDoesNotOfferStopsTheStart(t *testing.T) {
	a := fakeAdapter(t, "noplan")
	_, err := a.Start(t.Context(), agents.StartSpec{Cwd: t.TempDir(), PermissionMode: "plan"})
	if !errors.Is(err, agents.ErrUnsupportedSetting) {
		t.Fatalf("err = %v, want ErrUnsupportedSetting", err)
	}
	if !strings.Contains(err.Error(), "default") || !strings.Contains(err.Error(), "yolo") {
		t.Errorf("err = %q, want it to list what Gemini offers", err)
	}
}

func TestTheModelGoesInTheEnvironmentOfTheProcess(t *testing.T) {
	a := fakeAdapter(t, "")
	h := start(t, a, agents.StartSpec{Model: "gemini-2.5-flash", Env: []string{"EXTRA=1"}})
	if h.Model != "gemini-2.5-flash" || !h.Applied.Model || !h.Capabilities.ModelSwitching {
		t.Errorf("handle = model %q applied %v switching %v, want the model taken", h.Model, h.Applied.Model,
			h.Capabilities.ModelSwitching)
	}
	if got := ask(t, a, h, "hello"); !strings.Contains(got, "model=gemini-2.5-flash ") {
		t.Errorf("answer = %q, want the process to have GEMINI_MODEL set", got)
	}
	if !a.Capabilities().ModelSwitching {
		t.Error("Capabilities().ModelSwitching = false, want true")
	}
}

func TestNoModelLeavesGeminiItsOwnChoice(t *testing.T) {
	a := fakeAdapter(t, "")
	h := start(t, a, agents.StartSpec{})
	if h.Applied.Model || h.Model != "" {
		t.Errorf("handle = model %q applied %v, want none", h.Model, h.Applied.Model)
	}
	if got := ask(t, a, h, "hello"); !strings.Contains(got, "model= ") {
		t.Errorf("answer = %q, want no GEMINI_MODEL", got)
	}
}

func TestAModelNameThatCouldBeMischiefIsRefused(t *testing.T) {
	a := fakeAdapter(t, "")
	for _, model := range []string{"gemini pro", "x\nPATH=/evil", "a=b", strings.Repeat("m", 200), "$(id)"} {
		_, err := a.Start(t.Context(), agents.StartSpec{Cwd: t.TempDir(), Model: model})
		if !errors.Is(err, agents.ErrUnsupportedSetting) {
			t.Errorf("Start with model %q: err = %v, want ErrUnsupportedSetting", model, err)
		}
	}
}

func TestThinkingCannotBeSetOnGemini(t *testing.T) {
	a := fakeAdapter(t, "")
	h := start(t, a, agents.StartSpec{Thinking: "high"})
	if h.Applied.Thinking || h.Capabilities.Thinking {
		t.Errorf("handle = thinking applied %v, capability %v, want neither", h.Applied.Thinking, h.Capabilities.Thinking)
	}
	if a.Capabilities().Thinking {
		t.Error("Capabilities().Thinking = true, but Gemini CLI has no thinking setting")
	}
}

func TestSignInIsReportedInPlainWordsAndNeverDone(t *testing.T) {
	// The fake ends the run if it is asked to authenticate, so a start that reports a sign-in
	// proves that the adapter did not try.
	a := fakeAdapter(t, "signin,authcall")
	_, err := a.Start(t.Context(), agents.StartSpec{Cwd: t.TempDir()})
	if err == nil {
		t.Fatal("Start succeeded, want a sign-in error")
	}
	var signIn *SignInError
	if !errors.As(err, &signIn) {
		t.Fatalf("err = %T %v, want a *SignInError", err, err)
	}
	want := "Sign in to Gemini CLI in a terminal first. Run \"gemini\" there once, and choose how to sign in."
	if err.Error() != want {
		t.Errorf("message = %q, want %q", err.Error(), want)
	}
	var auth *agents.AuthRequiredError
	if !errors.As(err, &auth) {
		t.Fatal("the *agents.AuthRequiredError inside is lost")
	}
	if len(auth.Methods) != 4 || auth.Methods[0] != "Log in with Google" {
		t.Errorf("sign-in methods = %v, want the four that Gemini lists", auth.Methods)
	}
}

func TestSignInIsReportedOnResumeToo(t *testing.T) {
	a := fakeAdapter(t, "signin,authcall")
	_, err := a.Resume(t.Context(), "gemini-session-1", agents.StartSpec{Cwd: t.TempDir()})
	var signIn *SignInError
	if !errors.As(err, &signIn) {
		t.Fatalf("err = %v, want a *SignInError", err)
	}
}

func TestOtherErrorsAreNotSignInErrors(t *testing.T) {
	a := fakeAdapter(t, "")
	_, err := a.Start(t.Context(), agents.StartSpec{Cwd: "relative/path"})
	var signIn *SignInError
	if err == nil || errors.As(err, &signIn) {
		t.Errorf("err = %v, want an ordinary error", err)
	}
}

func TestGeminiIsStartedInAcpModeWithoutABrowser(t *testing.T) {
	a := fakeAdapter(t, "")
	h := start(t, a, agents.StartSpec{})
	got := ask(t, a, h, "hello")
	if !strings.Contains(got, "args=--acp") {
		t.Errorf("answer = %q, want Gemini started with --acp", got)
	}
	if !strings.Contains(got, "no_browser=true") {
		t.Errorf("answer = %q, want NO_BROWSER=true so that a daemon never opens a browser", got)
	}
}

func TestResumeLoadsTheSession(t *testing.T) {
	a := fakeAdapter(t, "")
	first := start(t, a, agents.StartSpec{PermissionMode: "auto-edits"})
	stopAndDrain(t, a, first)

	h, err := a.Resume(t.Context(), first.ID, agents.StartSpec{Cwd: t.TempDir(), PermissionMode: "plan", Model: "gemini-2.5-pro"})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	t.Cleanup(func() { stopAndDrain(t, a, h) })
	if h.ID != first.ID || !h.Capabilities.Resume || !h.Applied.Model {
		t.Errorf("handle = %+v, want the same session, resumable, with the model taken", h)
	}
	if got := ask(t, a, h, "again"); !strings.Contains(got, "mode=plan model=gemini-2.5-pro") {
		t.Errorf("answer = %q, want the new settings", got)
	}
}

func TestTheStubAgentRunsBehindTheWrapper(t *testing.T) {
	stub := testutil.StubAgent(t)
	a, err := New(Config{
		Path: stub, Args: []string{"--state-dir", t.TempDir(), "--speed", "0"},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// The stub offers no controls, so a model that goes in the environment is harmless, and the
	// settings that it cannot take are reported as not applied.
	h := start(t, a, agents.StartSpec{Model: "gemini-2.5-pro", Thinking: "high", PermissionMode: "ask"})
	if !h.Applied.Model || h.Applied.Thinking || h.Applied.PermissionMode {
		t.Errorf("applied = %+v, want only the model", h.Applied)
	}
	if got := ask(t, a, h, "hello"); got == "" {
		t.Error("the stub agent gave no answer through the wrapper")
	}
}

func TestNewNeedsAPathAndFactoryMakesAgents(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Error("New without a path succeeded, want an error")
	}
	agent, err := Factory(Config{Path: "/bin/gemini"})()
	if err != nil || agent == nil {
		t.Errorf("Factory()() = %v, %v, want an agent", agent, err)
	}
	// Two factory calls make two agents that do not share sessions.
	other, _ := Factory(Config{Path: "/bin/gemini"})()
	if agent == other {
		t.Error("the factory returned the same agent twice")
	}
}

func TestConfigBuildsTheAcpAdapterSettings(t *testing.T) {
	cfg := acpConfig(Config{Path: "/opt/homebrew/bin/gemini", Env: []string{"NO_BROWSER=false", "X=1"}})
	if len(cfg.Args) != 1 || cfg.Args[0] != "--acp" {
		t.Errorf("args = %v, want --acp", cfg.Args)
	}
	if len(cfg.AutoAuthMethods) != 0 {
		t.Errorf("AutoAuthMethods = %v, want none: Marshal never signs in for the person", cfg.AutoAuthMethods)
	}
	if len(cfg.ThinkingModes) != 0 {
		t.Errorf("ThinkingModes = %v, want none: Gemini has no thinking setting", cfg.ThinkingModes)
	}
	// The person's own entries come last, so they win.
	if last := cfg.Env[len(cfg.Env)-2:]; last[0] != "NO_BROWSER=false" || last[1] != "X=1" {
		t.Errorf("env = %v, want the config's entries last", cfg.Env)
	}
	if !strings.HasPrefix(cfg.Env[0], "PATH=/opt/homebrew/bin") {
		t.Errorf("env[0] = %q, want a PATH that starts with the program's folder", cfg.Env[0])
	}
	for mode, want := range map[string]string{
		"ask": "default", "auto-edits": "autoEdit", "plan": "plan", "full-auto": "yolo", "bypass": "yolo",
	} {
		if cfg.PermissionModes[mode] != want {
			t.Errorf("PermissionModes[%q] = %q, want %q", mode, cfg.PermissionModes[mode], want)
		}
	}
	custom := acpConfig(Config{Path: "/x/gemini", Args: []string{"--state-dir", "/tmp/s"}})
	if len(custom.Args) != 2 || custom.Args[0] != "--state-dir" {
		t.Errorf("custom args = %v, want the ones that were given instead of --acp", custom.Args)
	}
}
