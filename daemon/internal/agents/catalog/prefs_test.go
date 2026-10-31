package catalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// fakePrefs is the person's choices, by kind. A kind that is not in it has the default.
type fakePrefs struct {
	prefs map[protocol.AgentKind]Preference
	err   error
}

func (f fakePrefs) Preference(_ context.Context, kind protocol.AgentKind) (Preference, error) {
	return f.prefs[kind], f.err
}

// pathProbe is a probe that also checks a path the person chose, and records the path it was asked for.
type pathProbe struct {
	*fakeProbe
	asked []string
}

func (p *pathProbe) ProbePath(_ context.Context, _ protocol.AgentKind, path string) (Found, error) {
	p.asked = append(p.asked, path)
	version, _ := ParseSemver("2.1.282")
	return Found{Path: path, Version: version}, nil
}

func TestAnAgentTurnedOffIsListedButNotStartable(t *testing.T) {
	prefs := fakePrefs{prefs: map[protocol.AgentKind]Preference{protocol.AgentKindClaude: {Off: true}}}
	c := New(Options{Probe: installedProbe(), Logger: quiet(), Preferences: prefs})

	list, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	claude := agentOfKind(t, list, protocol.AgentKindClaude)
	if !claude.Off || claude.Version != "2.1.282" || !strings.Contains(claude.Warning, "turned off") {
		t.Errorf("claude = %+v, want it off, still at its version, with the reason", claude)
	}
	detected, err := c.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range detected {
		if d.Kind == protocol.AgentKindClaude && (d.Startable || !d.Off) {
			t.Errorf("detected claude = %+v, want off and not startable", d)
		}
	}
}

func TestAChosenPathIsTheOnlyOneProbed(t *testing.T) {
	probe := &pathProbe{fakeProbe: installedProbe()}
	chosen := "/Users/pat/bin/claude"
	prefs := fakePrefs{prefs: map[protocol.AgentKind]Preference{protocol.AgentKindClaude: {Path: chosen}}}
	c := New(Options{Probe: probe, Logger: quiet(), Preferences: prefs})

	detected, err := c.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range detected {
		if d.Kind == protocol.AgentKindClaude && (d.Path != chosen || d.CustomPath != chosen || !d.Startable) {
			t.Errorf("detected claude = %+v, want the chosen path and startable", d)
		}
	}
	if len(probe.asked) != 1 || probe.asked[0] != chosen {
		t.Errorf("path probes = %v, want the chosen path only", probe.asked)
	}
}

func TestAChosenPathWithoutAPathProbeIsMissing(t *testing.T) {
	chosen := "/Users/pat/bin/claude"
	prefs := fakePrefs{prefs: map[protocol.AgentKind]Preference{protocol.AgentKindClaude: {Path: chosen}}}
	c := New(Options{Probe: installedProbe(), Logger: quiet(), Preferences: prefs})

	list, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	claude := agentOfKind(t, list, protocol.AgentKindClaude)
	if claude.Status != protocol.AgentStatusMissing || claude.CustomPath != chosen {
		t.Errorf("claude = %+v, want missing with the chosen path shown", claude)
	}
}

func TestAPreferenceThatCannotBeReadIsTheDefault(t *testing.T) {
	prefs := fakePrefs{err: errors.New("the row is broken")}
	c := New(Options{Probe: installedProbe(), Logger: quiet(), Preferences: prefs})

	list, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	claude := agentOfKind(t, list, protocol.AgentKindClaude)
	if claude.Off || claude.CustomPath != "" || claude.Status != protocol.AgentStatusSupported {
		t.Errorf("claude = %+v, want the default", claude)
	}
}

func TestAChosenFileMustBeARunnableFile(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "claude")
	if err := os.WriteFile(plain, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	runnable := filepath.Join(dir, "claude-run")
	if err := os.WriteFile(runnable, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, path, reason string
	}{
		{"a relative path", "claude", "absolute"},
		{"no file", filepath.Join(dir, "missing"), "no file"},
		{"a folder", dir, "not a file"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkRunnable(tc.path)
			if err == nil || !strings.Contains(err.Error(), tc.reason) {
				t.Errorf("checkRunnable(%q) = %v, want a reason with %q", tc.path, err, tc.reason)
			}
		})
	}
	if err := checkRunnable(runnable); err != nil {
		t.Errorf("checkRunnable(runnable) = %v, want nil", err)
	}
	if err := checkRunnable(plain); err == nil && runtime.GOOS != "windows" {
		t.Errorf("checkRunnable(plain) = nil, want a file that is not marked runnable refused")
	}
}
