package localci

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCommandRunnerFindsBunWithAServicePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the local workflow runner uses POSIX shells")
	}
	home := t.TempDir()
	bin := filepath.Join(home, ".bun", "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "bun"), []byte("#!/bin/sh\nprintf 'fixture bun found\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", "/usr/bin:/bin:/usr/sbin:/sbin")
	runner := NewCommandRunner(0, 0, 0)
	result, err := runner.Run(t.Context(), RunRequest{Dir: home, Command: "bun run lint"})
	if err != nil || result.Failed || !strings.Contains(result.Output, "fixture bun found") {
		t.Fatalf("service PATH could not find the installed tool: %+v %v", result, err)
	}
	result, err = runner.Run(t.Context(), RunRequest{Dir: home, Command: "bun run lint", Env: []string{"PATH=/usr/bin:/bin"}})
	if err != nil || !result.Failed || !strings.Contains(result.Output, "bun") {
		t.Fatalf("explicit workflow PATH was not respected or stderr was missing: %+v %v", result, err)
	}
}

func TestCommandRunnerShowsStderrAndBoundsTheCombinedOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the local workflow runner uses POSIX shells")
	}
	runner := NewCommandRunner(0, 128, 10)
	result, err := runner.Run(t.Context(), RunRequest{Dir: t.TempDir(), Command: "printf 'missing tool diagnostic' >&2; exit 7"})
	if err != nil || !result.Failed || !strings.Contains(result.Output, "missing tool diagnostic") {
		t.Fatalf("stderr-only failure had no explanation: %+v %v", result, err)
	}
	result, err = runner.Run(t.Context(), RunRequest{Dir: t.TempDir(), Command: "i=0; while [ $i -lt 100 ]; do printf 'large stderr line\\n' >&2; i=$((i+1)); done; printf 'last error\\n' >&2; exit 7"})
	if err != nil || !result.Failed || len(result.Output) > 128 || !strings.Contains(result.Output, "last error") {
		t.Fatalf("output did not retain a bounded diagnostic tail: %+v %v", result, err)
	}
}
