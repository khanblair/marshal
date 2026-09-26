package testutil

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// stubBuild is the one build of the stub agent that a test process makes. Every test in the
// process shares it, so it has to live in a package variable.
var stubBuild struct { //nolint:gochecknoglobals // one build per test process, shared by every test
	once sync.Once
	dir  string
	path string
	err  error
}

// StubAgent builds the scripted stub agent (tools/stub-agent, its own Go module) once per test
// process, into a temporary folder, and returns the path of the program. Point an agent adapter at
// it and no real model is needed. A package that uses it calls CleanStubAgent from TestMain, to
// remove the build when its tests are done.
func StubAgent(t testing.TB) string {
	t.Helper()
	stubBuild.once.Do(func() {
		stubBuild.dir, stubBuild.path, stubBuild.err = buildStubAgent(
			context.Background(), filepath.Join(moduleDir(t), "..", "tools", "stub-agent"))
	})
	if stubBuild.err != nil {
		t.Fatalf("build the stub agent: %v", stubBuild.err)
	}
	return stubBuild.path
}

// CleanStubAgent removes the build that StubAgent made. It is for TestMain, and does nothing when
// no build was made.
func CleanStubAgent() {
	if stubBuild.dir != "" {
		_ = os.RemoveAll(stubBuild.dir)
	}
}

// buildStubAgent runs go build in the stub agent's own module folder and returns the folder that
// holds the result, and the path of the program. It shares its build step with buildTerminalHelper
// through buildGoBinary (buildgo.go): the two differ only in which package they build and what to
// call the result.
func buildStubAgent(ctx context.Context, moduleDir string) (dir, path string, err error) {
	return buildGoBinary(ctx, moduleDir, ".", "marshal-stub-agent-", "stub-agent")
}
