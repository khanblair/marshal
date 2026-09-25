package testutil

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/proc"
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

// buildStubAgent runs go build in the stub agent's module folder and returns the folder that holds
// the result, and the path of the program.
func buildStubAgent(ctx context.Context, moduleDir string) (dir, path string, err error) {
	dir, err = os.MkdirTemp("", "marshal-stub-agent-")
	if err != nil {
		return "", "", fmt.Errorf("make a folder for the build: %w", err)
	}
	name := "stub-agent"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path = filepath.Join(dir, name)
	build, err := proc.Start(ctx, proc.Spec{
		Path: "go", Args: []string{"build", "-o", path, "."}, Dir: moduleDir,
		Env: goEnvironment(os.Environ()),
	})
	if err != nil {
		return dir, "", err
	}
	// go build prints nothing on success, and its complaints go to standard error.
	_, _ = io.Copy(io.Discard, build.Stdout)
	exit := build.Wait()
	_ = build.Stdout.Close()
	if exit.Err != nil {
		return dir, "", fmt.Errorf("go build: %w\n%s", exit.Err, build.StderrTail())
	}
	return dir, path, nil
}

// goEnvironment picks what the Go tool needs from an environment: its own settings, and a
// build that needs no C compiler, since the stub agent is pure Go. The child process helper hands
// a program nothing else.
func goEnvironment(environ []string) []string {
	env := []string{"CGO_ENABLED=0"}
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "GO") || name == "XDG_CACHE_HOME" {
			env = append(env, entry)
		}
	}
	return env
}
