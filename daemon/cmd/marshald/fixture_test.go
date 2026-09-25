package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/platform"
)

// TestLoadFixtureNeverStopsTheDaemon checks each way a fixture setting can go: none of them may
// panic or return an error, because a fixture is a dev convenience. The prototype case fails on
// purpose (a data folder that is not a full path), and must be reported as a warning.
func TestLoadFixtureNeverStopsTheDaemon(t *testing.T) {
	tests := []struct {
		name     string
		settings config.Settings
		wantLog  string
	}{
		{name: "no fixture asked for", settings: config.Settings{}},
		{
			name:     "an unknown fixture",
			settings: config.Settings{Mode: platform.ModeDev, Fixture: "small-repo"},
			wantLog:  "unknown fixture",
		},
		{
			name:     "the prototype fails",
			settings: config.Settings{Mode: platform.ModeDev, Fixture: "prototype", DataDir: "relative"},
			wantLog:  "could not load the prototype fixture",
		},
		{
			name:     "a normal install ignores a fixture",
			settings: config.Settings{Mode: platform.ModeNormal, Fixture: "prototype", DataDir: "relative"},
			wantLog:  "fixtures load only in dev mode",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			loadFixture(context.Background(), tc.settings, nil, slog.New(slog.NewTextHandler(&out, nil)))
			switch {
			case tc.wantLog == "" && out.Len() != 0:
				t.Errorf("logged %q, want nothing", out.String())
			case tc.wantLog != "" && !strings.Contains(out.String(), tc.wantLog):
				t.Errorf("log %q does not contain %q", out.String(), tc.wantLog)
			}
		})
	}
}
