package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/api"
	"github.com/khanblair/marshal/daemon/internal/buildinfo"
	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/platform"
)

func runCLI(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(args, terminal{stdin: strings.NewReader(stdin), stdout: &out, stderr: &errOut})
	return code, out.String(), errOut.String()
}

func TestVersion(t *testing.T) {
	code, out, _ := runCLI(t, "", "version")
	if code != exitOK || strings.TrimSpace(out) != buildinfo.Version {
		t.Errorf("version = %d %q", code, out)
	}
}

func TestNoCommandShowsUsage(t *testing.T) {
	code, _, errOut := runCLI(t, "")
	if code != exitBadInput || !strings.Contains(errOut, "Usage") {
		t.Errorf("no command = %d %q", code, errOut)
	}
}

func TestUnknownCommand(t *testing.T) {
	code, _, errOut := runCLI(t, "", "explode")
	if code != exitBadInput || !strings.Contains(errOut, "Unknown command") {
		t.Errorf("unknown = %d %q", code, errOut)
	}
}

func TestStatusWhenNothingIsRunning(t *testing.T) {
	// Port 1 is never open for an unprivileged listener.
	code, _, errOut := runCLI(t, "", "status", "--port", "1")
	if code != exitFailed || !strings.Contains(errOut, "not running") {
		t.Errorf("status = %d %q", code, errOut)
	}
}

func TestDevResetNeedsTheResetWord(t *testing.T) {
	code, _, errOut := runCLI(t, "", "dev", "nonsense")
	if code != exitBadInput || !strings.Contains(errOut, "Usage") {
		t.Errorf("dev nonsense = %d %q", code, errOut)
	}
}

// devMachine is a fake machine whose home folder is a temp folder, so a reset test can never
// reach a real folder. The dev daemon's port is one where nothing listens.
func devMachine(t *testing.T, extra map[string]string) (env platform.Env, devDir string) {
	t.Helper()
	home := t.TempDir()
	vars := map[string]string{"MARSHAL_PORT": "1"}
	for k, v := range extra {
		vars[k] = v
	}
	env = platform.Env{GOOS: "linux", Home: home, Getenv: func(k string) string { return vars[k] }}
	devDir = filepath.Join(home, ".local", "share", "marshal-dev")
	if err := os.MkdirAll(devDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devDir, "marshal.db"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	return env, devDir
}

func runDevWith(env platform.Env, stdin string, args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = runWith(env, args, terminal{stdin: strings.NewReader(stdin), stdout: &out, stderr: &errOut})
	return code, out.String(), errOut.String()
}

func TestDevResetDeletesAfterYes(t *testing.T) {
	env, dir := devMachine(t, nil)
	code, out, errOut := runDevWith(env, "yes\n", "dev", "reset")
	if code != exitOK || !strings.Contains(out, "Deleted") {
		t.Fatalf("dev reset = %d %q %q", code, out, errOut)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dev folder still exists: %v", err)
	}
}

func TestDevResetKeepsDataWithoutYes(t *testing.T) {
	env, dir := devMachine(t, nil)
	code, out, _ := runDevWith(env, "no\n", "dev", "reset")
	if code != exitOK || !strings.Contains(out, "Nothing was deleted") {
		t.Fatalf("dev reset = %d %q", code, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "marshal.db")); err != nil {
		t.Errorf("data was deleted: %v", err)
	}
}

func TestDevResetSkipsTheQuestionWithYesFlag(t *testing.T) {
	env, dir := devMachine(t, nil)
	if code, _, errOut := runDevWith(env, "", "dev", "reset", "--yes"); code != exitOK {
		t.Fatalf("dev reset --yes = %d %q", code, errOut)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dev folder still exists: %v", err)
	}
}

func TestDevResetNeverTouchesANormalFolder(t *testing.T) {
	normal := filepath.Join(t.TempDir(), "share", "marshal")
	if err := os.MkdirAll(normal, 0o700); err != nil {
		t.Fatal(err)
	}
	env, _ := devMachine(t, map[string]string{"MARSHAL_DATA_DIR": normal})
	code, _, errOut := runDevWith(env, "yes\n", "dev", "reset", "--yes")
	if code != exitFailed || !strings.Contains(errOut, "not a dev data folder") {
		t.Fatalf("dev reset on a normal folder = %d %q", code, errOut)
	}
	if _, err := os.Stat(normal); err != nil {
		t.Errorf("the normal folder was deleted: %v", err)
	}
}

func TestDevResetRefusesWhileTheDaemonRuns(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	settings := config.Settings{Mode: platform.ModeDev}
	server := api.New(settings, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now, api.Deps{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, listener) }()
	defer func() {
		cancel()
		<-done
	}()
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	env, dir := devMachine(t, map[string]string{"MARSHAL_PORT": port})
	code, _, errOut := runDevWith(env, "yes\n", "dev", "reset", "--yes")
	if code != exitFailed || !strings.Contains(errOut, "Stop it first") {
		t.Fatalf("dev reset while running = %d %q", code, errOut)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("data was deleted while the daemon ran: %v", err)
	}
}

// tokenMachine is a fake machine with a token file in the normal or the dev data folder.
func tokenMachine(t *testing.T, dev bool, token string) (env platform.Env, path string) {
	t.Helper()
	home := t.TempDir()
	env = platform.Env{GOOS: "linux", Home: home, Getenv: func(string) string { return "" }}
	folder, file := "marshal", platform.OwnerTokenFile
	if dev {
		folder, file = "marshal-dev", platform.DevTokenFile
	}
	dir := filepath.Join(home, ".local", "share", folder)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(dir, file)
	if token != "" {
		if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return env, path
}

func TestTokenPrintsThePathAndNotTheToken(t *testing.T) {
	for _, dev := range []bool{false, true} {
		env, path := tokenMachine(t, dev, "secret-token-value")
		args := []string{"token"}
		if dev {
			args = append(args, "--dev")
		}
		code, out, errOut := runDevWith(env, "", args...)
		if code != exitOK || strings.TrimSpace(out) != path {
			t.Errorf("dev=%v: token = %d %q %q, want the path %q", dev, code, out, errOut, path)
		}
		if strings.Contains(out+errOut, "secret-token-value") {
			t.Errorf("dev=%v: the token was printed without --show", dev)
		}
	}
}

func TestTokenShowPrintsOnlyTheToken(t *testing.T) {
	env, _ := tokenMachine(t, false, "secret-token-value")
	code, out, errOut := runDevWith(env, "", "token", "--show")
	if code != exitOK || out != "secret-token-value\n" || errOut != "" {
		t.Errorf("token --show = %d %q %q", code, out, errOut)
	}
}

func TestTokenWhenThereIsNoFile(t *testing.T) {
	env, path := tokenMachine(t, false, "")
	code, out, errOut := runDevWith(env, "", "token", "--show")
	if code != exitFailed || out != "" || !strings.Contains(errOut, path) || !strings.Contains(errOut, "Start the daemon") {
		t.Errorf("token without a file = %d %q %q", code, out, errOut)
	}
}

func TestTokenRejectsAnUnknownFlag(t *testing.T) {
	env, _ := tokenMachine(t, false, "x")
	if code, _, _ := runDevWith(env, "", "token", "--reveal"); code != exitBadInput {
		t.Errorf("token --reveal = %d, want %d", code, exitBadInput)
	}
}
