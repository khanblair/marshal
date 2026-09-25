package config_test

import (
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/platform"
)

func testEnv(vars map[string]string) platform.Env {
	return platform.Env{GOOS: "linux", Home: filepath.Join("home", "sam"), Getenv: func(k string) string { return vars[k] }}
}

func load(t *testing.T, vars map[string]string, args ...string) config.Settings {
	t.Helper()
	s, err := config.Load(args, testEnv(vars), io.Discard)
	if err != nil {
		t.Fatalf("Load(%v): %v", args, err)
	}
	return s
}

func TestNormalDefaults(t *testing.T) {
	s := load(t, nil)
	if s.Mode != platform.ModeNormal || s.Dev() {
		t.Errorf("mode = %q, want normal", s.Mode)
	}
	if s.Port != config.DefaultPort || s.Agent != config.AgentReal || s.LogLevel != slog.LevelInfo {
		t.Errorf("defaults = port %d, agent %q, level %v", s.Port, s.Agent, s.LogLevel)
	}
	if want := filepath.Join("home", "sam", ".local", "share", "marshal"); s.DataDir != want {
		t.Errorf("DataDir = %q, want %q", s.DataDir, want)
	}
}

func TestDevDefaultsAreSeparate(t *testing.T) {
	normal := load(t, nil)
	dev := load(t, nil, "--dev")
	if !dev.Dev() || dev.Port != config.DevPort || dev.Agent != config.AgentStub || dev.LogLevel != slog.LevelDebug {
		t.Errorf("dev defaults = %+v", dev)
	}
	if dev.DataDir == normal.DataDir || dev.Port == normal.Port {
		t.Errorf("dev and normal share a folder or port: %+v vs %+v", dev, normal)
	}
}

func TestFlagBeatsEnvironmentBeatsDefault(t *testing.T) {
	vars := map[string]string{"MARSHAL_PORT": "5000", "MARSHAL_AGENT": "stub", "MARSHAL_LOG_LEVEL": "warn", "MARSHAL_DATA_DIR": "envdir", "MARSHAL_FIXTURE": "monorepo"}
	fromEnv := load(t, vars)
	if fromEnv.Port != 5000 || fromEnv.Agent != config.AgentStub || fromEnv.LogLevel != slog.LevelWarn || fromEnv.DataDir != "envdir" || fromEnv.Fixture != "monorepo" {
		t.Errorf("environment settings not used: %+v", fromEnv)
	}
	fromFlags := load(t, vars, "--port", "6000", "--agent", "real", "--log-level", "error", "--data-dir", "flagdir", "--fixture", "small-repo")
	if fromFlags.Port != 6000 || fromFlags.Agent != config.AgentReal || fromFlags.LogLevel != slog.LevelError || fromFlags.DataDir != "flagdir" || fromFlags.Fixture != "small-repo" {
		t.Errorf("flags did not win: %+v", fromFlags)
	}
}

func TestInvalidSettingsGetPlainMessages(t *testing.T) {
	tests := []struct {
		name string
		vars map[string]string
		args []string
	}{
		{"port not a number", map[string]string{"MARSHAL_PORT": "abc"}, nil},
		{"port too high", nil, []string{"--port", "70000"}},
		{"unknown agent", nil, []string{"--agent", "robot"}},
		{"unknown level", nil, []string{"--log-level", "loud"}},
		{"unknown flag", nil, []string{"--nope"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := config.Load(tc.args, testEnv(tc.vars), io.Discard); err == nil {
				t.Errorf("Load(%v, %v) returned no error", tc.vars, tc.args)
			}
		})
	}
}

func TestHelpAndVersion(t *testing.T) {
	if _, err := config.Load([]string{"--help"}, testEnv(nil), io.Discard); !errors.Is(err, config.ErrHelp) {
		t.Errorf("--help returned %v, want ErrHelp", err)
	}
	if !load(t, nil, "--version").ShowVersion {
		t.Error("--version did not set ShowVersion")
	}
}
