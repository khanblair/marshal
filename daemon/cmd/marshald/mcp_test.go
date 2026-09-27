package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/mcpattach"
)

// `marshald mcp` is the same binary in another mode: the stdio server an agent runs to reach the
// daemon's tools (mcp.go). It is dispatched before anything reads the settings, so these tests check
// the two things that would go wrong if that order changed - an argument mistake reaching the
// daemon's own flags, and the mode opening a data folder it has no use for.

func TestMCPModeSaysWhatItIsMissing(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		secret  string
		missing string
	}{
		{name: "no address", args: []string{"--card", "card-1"}, secret: "s3cret", missing: "--address"},
		{name: "no card", args: []string{"--address", "127.0.0.1:1"}, secret: "s3cret", missing: "--card"},
		{
			name: "no secret", args: []string{"--address", "127.0.0.1:1", "--card", "card-1"},
			missing: mcpattach.TokenEnv,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.secret != "" {
				t.Setenv(mcpattach.TokenEnv, tc.secret)
			} else {
				// The environment may carry the variable from a real run; an empty value is the
				// "missing" case.
				t.Setenv(mcpattach.TokenEnv, "")
			}
			var out, errOut bytes.Buffer
			if code := run(append([]string{"mcp"}, tc.args...), &out, &errOut); code != exitBadInput {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitBadInput, errOut.String())
			}
			if !strings.Contains(errOut.String(), tc.missing) {
				t.Errorf("message %q does not name %s", errOut.String(), tc.missing)
			}
		})
	}
}

// TestMCPModeNeverReadsTheDaemonsSettings: the mode's flags are its own, so a flag the daemon takes
// is a mistake here, and pointing it at a data folder creates nothing - which is what makes the mode
// safe to run from inside an agent's session.
func TestMCPModeNeverReadsTheDaemonsSettings(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(mcpattach.TokenEnv, "s3cret")
	var out, errOut bytes.Buffer
	if code := run([]string{"mcp", "--data-dir", dir, "--address", "127.0.0.1:1"}, &out, &errOut); code != exitBadInput {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitBadInput, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(dir, databaseFile)); err == nil {
		t.Error("the mode created the database")
	}
	if got := strings.TrimSpace(out.String()); got != "" {
		t.Errorf("the mode wrote %q to the agent's stdout, which carries the protocol", got)
	}
}

// TestMCPModeReportsADaemonItCannotReach: with its arguments in order the mode reaches for the
// daemon, and a daemon that is not there is a plain failure rather than a silent stdio server that
// answers nothing. The connection is refused at once, so nothing is waited for.
func TestMCPModeReportsADaemonItCannotReach(t *testing.T) {
	t.Setenv(mcpattach.TokenEnv, "s3cret")
	var out, errOut bytes.Buffer
	code := run([]string{"mcp", "--address", "127.0.0.1:1", "--card", "card-1"}, &out, &errOut)
	if code != exitFailed {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitFailed, errOut.String())
	}
	if !strings.Contains(errOut.String(), "mcp: reach the daemon") {
		t.Errorf("message %q does not say the daemon could not be reached", errOut.String())
	}
}
