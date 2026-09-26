package testutil

import (
	"context"
	"os"
	"sync"
	"testing"
)

// helperBuild is the one build of the terminal helper that a test process makes.
var helperBuild struct { //nolint:gochecknoglobals // one build per test process, shared by every test
	once sync.Once
	dir  string
	path string
	err  error
}

// TerminalHelper builds the small program that the PTY adapter's own tests run inside a terminal
// (agents/pty/testdata/helper) once per test process, into a temporary folder, and returns its path.
// It has no shell in it, so it builds and behaves the same on every platform, and its modes (echo,
// size, args, and the rest) are what a test of the terminal view runs in place of a real coding
// agent. A package that uses it calls CleanTerminalHelper from TestMain.
func TerminalHelper(t testing.TB) string {
	t.Helper()
	helperBuild.once.Do(func() {
		helperBuild.dir, helperBuild.path, helperBuild.err = buildTerminalHelper(context.Background(), moduleDir(t))
	})
	if helperBuild.err != nil {
		t.Fatalf("build the terminal helper: %v", helperBuild.err)
	}
	return helperBuild.path
}

// CleanTerminalHelper removes the build that TerminalHelper made. It is for TestMain, and does
// nothing when no build was made.
func CleanTerminalHelper() {
	if helperBuild.dir != "" {
		_ = os.RemoveAll(helperBuild.dir)
	}
}

// buildTerminalHelper runs go build for the helper in the daemon module, and returns the folder
// that holds the result and the path of the program. It shares its build step with buildStubAgent
// through buildGoBinary (buildgo.go).
func buildTerminalHelper(ctx context.Context, module string) (dir, path string, err error) {
	return buildGoBinary(ctx, module, "./internal/agents/pty/testdata/helper", "marshal-terminal-helper-", "helper")
}
