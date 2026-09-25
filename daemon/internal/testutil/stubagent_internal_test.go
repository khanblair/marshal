package testutil

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestGoEnvironmentPassesOnlyGoSettings(t *testing.T) {
	got := goEnvironment([]string{"GOFLAGS=-mod=mod", "GOCACHE=/c", "HOME=/h", "MARSHAL_TOKEN=x", "XDG_CACHE_HOME=/x", "PATH=/bin"})
	want := []string{"CGO_ENABLED=0", "GOFLAGS=-mod=mod", "GOCACHE=/c", "XDG_CACHE_HOME=/x"}
	if !slices.Equal(got, want) {
		t.Errorf("goEnvironment = %q, want %q", got, want)
	}
}

func TestBuildStubAgentReportsABuildThatFails(t *testing.T) {
	notAModule := t.TempDir()
	if err := os.WriteFile(filepath.Join(notAModule, "main.go"), []byte("package main\nfunc main() { undefined() }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir, _, err := buildStubAgent(t.Context(), notAModule)
	if dir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
	}
	if err == nil || !strings.Contains(err.Error(), "go build") {
		t.Errorf("error = %v, want a go build failure", err)
	}
}
