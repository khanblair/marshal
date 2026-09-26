package pty

import (
	"errors"
	"runtime"
	"slices"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// Claude Code's config starts a session under an id of the caller's choosing and resumes it by the
// same id, which is what lets the terminal view and the chat view share one conversation.
func TestClaudeConfigStartsAndResumesTheSameSessionId(t *testing.T) {
	cfg := ClaudeConfig("/usr/local/bin/claude", []string{"PATH=/usr/local/bin"})
	if cfg.Path != "/usr/local/bin/claude" || !slices.Equal(cfg.Env, []string{"PATH=/usr/local/bin"}) {
		t.Errorf("Path and Env = %q, %q", cfg.Path, cfg.Env)
	}
	if got := cfg.StartArgs("11111111-2222-4333-8444-555555555555"); !slices.Equal(got, []string{"--session-id=11111111-2222-4333-8444-555555555555"}) {
		t.Errorf("StartArgs = %q", got)
	}
	if got := cfg.ResumeArgs("11111111-2222-4333-8444-555555555555"); !slices.Equal(got, []string{"--resume=11111111-2222-4333-8444-555555555555"}) {
		t.Errorf("ResumeArgs = %q", got)
	}
	if cfg.LineEnd != "\r" {
		t.Errorf("LineEnd = %q, want a carriage return for a program that draws its own screen", cfg.LineEnd)
	}
	if got := cfg.SpecArgs(agents.StartSpec{Model: "sonnet", PermissionMode: "bypass", Thinking: "high"}); !slices.Equal(got, []string{"--model=sonnet"}) {
		t.Errorf("SpecArgs = %q, want only the model to be passed on", got)
	}
	if got := cfg.SpecArgs(agents.StartSpec{}); got != nil {
		t.Errorf("SpecArgs with no model = %q, want nothing", got)
	}
	if _, err := New(cfg); err != nil {
		t.Errorf("the config is not one New accepts: %v", err)
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Capabilities().Resume {
		t.Error("an adapter with Claude Code's config must say it can resume")
	}
}

// The settings of a session become arguments on Start and on Resume alike, after the ones that name
// the session.
func TestSpecArgsAreAddedOnStartAndOnResume(t *testing.T) {
	cfg := Config{
		Args:       []string{"args"},
		ResumeArgs: func(id string) []string { return []string{"args", "--resume", id} },
		SpecArgs:   func(spec agents.StartSpec) []string { return []string{"--model", spec.Model} },
	}
	a := newAdapter(t, cfg)
	spec := agents.StartSpec{Cwd: t.TempDir(), Model: "opus"}
	h, err := a.Start(callCtx(t), spec)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	watch(t, a, h).waitFor("args: --model opus")

	b := newAdapter(t, cfg)
	h, err = b.Resume(callCtx(t), "abc", spec)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	watch(t, b, h).waitFor("args: --resume abc --model opus")
}

// The size of the terminal is the configured one until the program is told another, and a session
// that is gone has none.
func TestSizeFollowsResize(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the helper reads its terminal size with ioctl, which the standard library has only on Unix")
	}
	r := startRun(t, Config{Cols: 90, Rows: 30}, "size")
	r.waitFor("size: 90x30")
	if cols, rows, err := r.a.Size(r.h); err != nil || cols != 90 || rows != 30 {
		t.Errorf("Size = %d, %d, %v, want 90, 30", cols, rows, err)
	}
	if err := r.a.Resize(callCtx(t), r.h, 100, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	r.waitFor("size: 100x40")
	if cols, rows, err := r.a.Size(r.h); err != nil || cols != 100 || rows != 40 {
		t.Errorf("Size after Resize = %d, %d, %v, want 100, 40", cols, rows, err)
	}
	// A size that was refused is not remembered.
	if err := r.a.Resize(callCtx(t), r.h, 0, 40); err == nil {
		t.Fatal("Resize(0, 40) succeeded")
	}
	if cols, rows, _ := r.a.Size(r.h); cols != 100 || rows != 40 {
		t.Errorf("Size after a refused Resize = %d, %d, want 100, 40", cols, rows)
	}
	if err := r.a.Stop(callCtx(t), r.h); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.a.Size(r.h); !errors.Is(err, agents.ErrUnknownSession) {
		t.Errorf("Size of a session that is gone = %v, want ErrUnknownSession", err)
	}
}
