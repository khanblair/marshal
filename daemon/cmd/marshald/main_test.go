package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/buildinfo"
	"github.com/khanblair/marshal/daemon/internal/platform"
)

func TestVersionFlagPrintsTheVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--version"}, &out, &errOut); code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, errOut.String())
	}
	if got := strings.TrimSpace(out.String()); got != buildinfo.Version {
		t.Errorf("printed %q, want %q", got, buildinfo.Version)
	}
}

func TestBadSettingsExitWithAMessage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--port", "70000"}, &out, &errOut); code != exitBadInput {
		t.Fatalf("exit code = %d, want %d", code, exitBadInput)
	}
	if !strings.Contains(errOut.String(), "port") {
		t.Errorf("message %q does not mention the port", errOut.String())
	}
}

// TestRunWhenAlreadyRunningNeverTouchesTheDatabase proves the lock is checked before anything
// else opens: a second daemon pointed at a data folder whose lock is already held must exit with
// exitAlreadyRunning and never create the database file.
func TestRunWhenAlreadyRunningNeverTouchesTheDatabase(t *testing.T) {
	dir := t.TempDir()
	lock, err := platform.AcquireLock(filepath.Join(dir, lockFileName))
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	defer func() { _ = lock.Release() }()

	var out, errOut bytes.Buffer
	code := run([]string{"--data-dir", dir, "--port", "1"}, &out, &errOut)
	if code != exitAlreadyRunning {
		t.Fatalf("run = %d, want %d (stderr: %s)", code, exitAlreadyRunning, errOut.String())
	}
	// The structured log line (written by serve, with the OS-level detail) and the plain sentence
	// (written by run, for the person) both land on stderr; the plain sentence is the last line.
	lines := strings.Split(strings.TrimRight(errOut.String(), "\n"), "\n")
	if got := lines[len(lines)-1]; got != "Marshal is already running for this user." {
		t.Errorf("last stderr line = %q, want the plain sentence \"Marshal is already running for this user.\"", got)
	}
	if _, statErr := os.Stat(filepath.Join(dir, databaseFile)); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("the database was created although the lock was already held: %v", statErr)
	}
}
