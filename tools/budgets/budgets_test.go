package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseClockTime(t *testing.T) {
	tests := map[string]time.Duration{
		"0:00.05":    50 * time.Millisecond,
		"0:01.25":    1250 * time.Millisecond,
		"2:03.50":    2*time.Minute + 3500*time.Millisecond,
		"1:02:03.50": time.Hour + 2*time.Minute + 3500*time.Millisecond,
		"12":         12 * time.Second,
	}
	for text, want := range tests {
		got, err := parseClockTime(text)
		if err != nil || got != want {
			t.Errorf("parseClockTime(%q) = %v, %v; want %v", text, got, err, want)
		}
	}
	if _, err := parseClockTime("soon"); err == nil {
		t.Error("parseClockTime accepted text that is not a time")
	}
}

func TestParsePS(t *testing.T) {
	got, err := parsePS("  10240 0:00.12\n")
	if err != nil || got.rssBytes != 10240*1024 || got.cpu != 120*time.Millisecond {
		t.Errorf("parsePS = %+v, %v", got, err)
	}
	if _, err := parsePS("only-one"); err == nil {
		t.Error("parsePS accepted a short line")
	}
}

func TestParseProcStat(t *testing.T) {
	// The process name has a space and a bracket, which must not shift the fields.
	stat := "1234 (my (odd) app) S 1 1234 1234 0 -1 4194560 100 0 0 0 250 50 0 0 20 0 5 0 100 1000 200"
	got, err := parseProcStat(stat)
	if err != nil || got != 3*time.Second {
		t.Errorf("parseProcStat = %v, %v; want 3s (250 + 50 ticks)", got, err)
	}
	if _, err := parseProcStat("1234 (short) S 1"); err == nil {
		t.Error("parseProcStat accepted a short line")
	}
}

func TestParseVmRSS(t *testing.T) {
	got, err := parseVMRSS("Name:\tmarshald\nVmRSS:\t   20480 kB\nThreads:\t9\n")
	if err != nil || got != 20480*1024 {
		t.Errorf("parseVMRSS = %v, %v", got, err)
	}
	if _, err := parseVMRSS("Name:\tx\n"); err == nil {
		t.Error("parseVMRSS accepted text with no VmRSS line")
	}
}

func TestParsePowerShell(t *testing.T) {
	got, err := parsePowerShell("52428800 1.25\r\n")
	if err != nil || got.rssBytes != 52428800 || got.cpu != 1250*time.Millisecond {
		t.Errorf("parsePowerShell = %+v, %v", got, err)
	}
}

func TestVerdictsFailWhenOverABudget(t *testing.T) {
	opts := options{maxRSSMB: 50, maxCPUPercent: 1}
	var out bytes.Buffer
	if code := report(&out, result{rssMB: 30, cpuPercent: 0.2}.verdicts(opts)); code != exitOK {
		t.Errorf("within budget gave exit %d\n%s", code, out.String())
	}
	out.Reset()
	if code := report(&out, result{rssMB: 80, cpuPercent: 0.2}.verdicts(opts)); code != exitOver || !strings.Contains(out.String(), "FAIL") {
		t.Errorf("over budget gave exit %d\n%s", code, out.String())
	}
}

func TestRunNeedsADaemonPath(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(nil, &out, &errOut); code != exitBadInput || !strings.Contains(errOut.String(), "Usage") {
		t.Errorf("run with no arguments = %d %q", code, errOut.String())
	}
}

// TestMeasuresARealDaemon builds the real daemon and measures it for a moment. The budgets here
// are loose, because the test only proves the whole pipeline works.
func TestMeasuresARealDaemon(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the daemon")
	}
	binary := filepath.Join(t.TempDir(), "marshald")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-C", filepath.Join("..", "..", "daemon"), "-o", binary, "./cmd/marshald")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build the daemon: %v\n%s", err, out)
	}
	var out, errOut bytes.Buffer
	code := run([]string{"-daemon", binary, "-idle", "1s", "-max-rss-mb", "500", "-max-cpu-percent", "100"}, &out, &errOut)
	if code != exitOK {
		t.Fatalf("exit code %d\nstdout: %s\nstderr: %s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "Daemon idle RAM") || !strings.Contains(out.String(), "Daemon idle CPU") {
		t.Errorf("report is missing a budget line:\n%s", out.String())
	}
}
