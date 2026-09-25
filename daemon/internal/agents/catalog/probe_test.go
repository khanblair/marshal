package catalog

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// notOnPath is a lookup that checks a path that it is given, and never searches the real PATH: a
// bare name finds nothing.
func notOnPath(name string) (string, error) {
	if filepath.Dir(name) == "." {
		return "", exec.ErrNotFound
	}
	return exec.LookPath(name)
}

func TestProbeReadsTheVersionThatEachAgentPrints(t *testing.T) {
	tests := []struct {
		name   string
		answer string
		want   string
	}{
		{"claude code", "2.1.282 (Claude Code)\n", "2.1.282"},
		{"gemini cli", "0.35.1\n", "0.35.1"},
		{"codex", "codex-cli 0.42.0\n", "0.42.0"},
		{"a pre-release", "v0.36.0-preview.1", "0.36.0-preview.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := installFake(t, dir, "claude", tt.answer)
			probe := NewProbe(ProbeOptions{Dirs: []string{dir}, LookPath: notOnPath})
			found, err := probe.Probe(t.Context(), protocol.AgentKindClaude)
			if err != nil {
				t.Fatalf("Probe: %v", err)
			}
			if found.Path != path || found.Version.String() != tt.want {
				t.Errorf("Probe = %q %q, want %q %q", found.Path, found.Version, path, tt.want)
			}
		})
	}
}

func TestProbeFindsAProgramOnPATH(t *testing.T) {
	dir := t.TempDir()
	path := installFake(t, dir, "gemini", "0.35.1")
	t.Setenv("PATH", dir)
	found, err := NewProbe(ProbeOptions{}).Probe(t.Context(), protocol.AgentKindGemini)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if found.Path != path {
		t.Errorf("path = %q, want %q", found.Path, path)
	}
}

func TestProbeSearchesTheWellKnownFoldersAfterPATH(t *testing.T) {
	onPath, known := t.TempDir(), t.TempDir()
	installFake(t, onPath, "claude", "1.0.0")
	installFake(t, known, "claude", "2.0.0")
	installFake(t, known, "gemini", "0.35.1")
	t.Setenv("PATH", onPath)
	probe := NewProbe(ProbeOptions{Dirs: []string{known}})

	claude, err := probe.Probe(t.Context(), protocol.AgentKindClaude)
	if err != nil || claude.Version.String() != "1.0.0" {
		t.Errorf("claude = %q, %v, want the one on PATH (1.0.0)", claude.Version, err)
	}
	gemini, err := probe.Probe(t.Context(), protocol.AgentKindGemini)
	if err != nil || gemini.Path != filepath.Join(known, filepath.Base(gemini.Path)) {
		t.Errorf("gemini = %q, %v, want the one in the well-known folder", gemini.Path, err)
	}
}

func TestProbeSaysNotFoundWhenNothingIsThere(t *testing.T) {
	probe := NewProbe(ProbeOptions{Dirs: []string{t.TempDir()}, LookPath: notOnPath})
	_, err := probe.Probe(t.Context(), protocol.AgentKindCodex)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	if _, err := probe.Probe(t.Context(), protocol.AgentKindBuiltin); !errors.Is(err, ErrNotFound) {
		t.Errorf("the built-in agent has no program, so err = %v, want ErrNotFound", err)
	}
}

func TestProbeIgnoresAFileThatCannotRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a file without the execute bit is a Unix idea")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\necho 1.2.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	probe := NewProbe(ProbeOptions{Dirs: []string{dir}, LookPath: notOnPath})
	if _, err := probe.Probe(t.Context(), protocol.AgentKindClaude); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestProbeReportsAProgramThatGivesNoVersion(t *testing.T) {
	tests := []struct {
		name   string
		answer string
	}{
		{"garbage", "hello world, nothing here\n"},
		{"an error exit", "exit 3"},
		{"more than a version can be", "spam"},
		{"an empty answer", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := installFake(t, dir, "claude", tt.answer)
			probe := NewProbe(ProbeOptions{Dirs: []string{dir}, LookPath: notOnPath})
			found, err := probe.Probe(t.Context(), protocol.AgentKindClaude)
			if !errors.Is(err, ErrUnreadable) {
				t.Fatalf("err = %v, want ErrUnreadable", err)
			}
			if found.Path != path {
				t.Errorf("path = %q, want %q, so the caller can say where the program is", found.Path, path)
			}
		})
	}
}

func TestProbeStopsAProgramThatNeverAnswers(t *testing.T) {
	dir := t.TempDir()
	installFake(t, dir, "claude", "hang")
	probe := NewProbe(ProbeOptions{
		Dirs: []string{dir}, LookPath: notOnPath, VersionTimeout: 300 * time.Millisecond,
	})
	start := time.Now()
	_, err := probe.Probe(t.Context(), protocol.AgentKindClaude)
	if !errors.Is(err, ErrUnreadable) || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want ErrUnreadable caused by the deadline", err)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("the probe took %v, and should have stopped near its 300ms limit", took)
	}
}

func TestProbeGivesUpWhenTheCallerDoes(t *testing.T) {
	dir := t.TempDir()
	installFake(t, dir, "claude", "hang")
	probe := NewProbe(ProbeOptions{Dirs: []string{dir}, LookPath: notOnPath, VersionTimeout: time.Minute})
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := probe.Probe(ctx, protocol.AgentKindClaude); !errors.Is(err, ErrUnreadable) {
		t.Errorf("err = %v, want ErrUnreadable", err)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("the probe took %v after its context ended", took)
	}
}
