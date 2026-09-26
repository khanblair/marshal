package testutil

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/proc"
)

// buildGoBinary runs `go build` for one package of a module, into a fresh temporary folder, and
// returns that folder (for the caller to remove when it is done) and the path of the program it
// built. StubAgent and TerminalHelper both build a small Go program once per test process and share
// this, the only difference between them being which package they build and what to call the
// result.
func buildGoBinary(ctx context.Context, module, pkg, tempPrefix, name string) (dir, path string, err error) {
	dir, err = os.MkdirTemp("", tempPrefix)
	if err != nil {
		return "", "", fmt.Errorf("make a folder for the build: %w", err)
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path = filepath.Join(dir, name)
	build, err := proc.Start(ctx, proc.Spec{
		Path: "go", Args: []string{"build", "-o", path, pkg}, Dir: module,
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
// build that needs no C compiler, since both programs it builds are pure Go. The child process
// helper hands a program nothing else.
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
