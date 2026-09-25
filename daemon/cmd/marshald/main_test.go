package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/buildinfo"
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
