package fixture_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestLoadPrototypeIgnoresThePersonsOwnGitSettings makes the process look like a machine with a
// Git identity, and a global hook that refuses every commit, and checks the fixture still commits
// with its own identity and date, so every machine makes the same commit.
func TestLoadPrototypeIgnoresThePersonsOwnGitSettings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the refusing hook is a shell script")
	}
	dir := t.TempDir()
	hooks := filepath.Join(dir, "hooks")
	if err := os.MkdirAll(hooks, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "gitconfig")
	hostile := "[user]\n\tname = Someone Else\n\temail = else@example.com\n[core]\n\thooksPath = " + hooks + "\n"
	if err := os.WriteFile(config, []byte(hostile), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	t.Setenv("GIT_AUTHOR_NAME", "Someone Else")
	t.Setenv("GIT_COMMITTER_NAME", "Someone Else")
	e := newEnv(t)
	if err := e.load(t); err != nil {
		t.Fatalf("load: %v", err)
	}
	folder := filepath.Join(e.dataDir, "fixtures", "api-gateway")
	got, err := e.git.Run(context.Background(), folder, "log", "-1", "--format=%an|%ae|%cn|%ce|%aI|%s|%D")
	if err != nil {
		t.Fatalf("read the commit: %v", err)
	}
	const want = "Marshal Fixture|fixture@marshal.invalid|Marshal Fixture|fixture@marshal.invalid|2026-01-01T00:00:00Z|Fixture: api-gateway|HEAD -> main"
	if got != want {
		t.Errorf("commit = %q, want %q", got, want)
	}
}

func TestLoadPrototypeMakesTheSameFilesForTheSameSource(t *testing.T) {
	e := newEnv(t)
	if err := e.load(t); err != nil {
		t.Fatalf("load: %v", err)
	}
	tree := func(name string) string {
		t.Helper()
		out, err := e.git.Run(context.Background(), filepath.Join(e.dataDir, "fixtures", name), "rev-parse", "HEAD^{tree}")
		if err != nil {
			t.Fatalf("read the tree of %s: %v", name, err)
		}
		return out
	}
	if tree("api-gateway") != tree("web-dashboard") {
		t.Error("the two copies of small-repo hold different files")
	}
	if tree("api-gateway") == tree("mobile-app") {
		t.Error("mobile-app holds the same files as small-repo")
	}
}
