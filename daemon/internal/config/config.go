// Package config resolves the settings a daemon starts with, from flags, MARSHAL_*
// environment settings, and the defaults for its mode, in that order.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/platform"
)

// Settings is everything the daemon needs to start.
type Settings struct {
	Mode        platform.Mode
	DataDir     string
	Port        int
	LogLevel    slog.Level
	Agent       AgentMode
	Fixture     string
	ShowVersion bool
}

// Dev reports whether this is a dev daemon.
func (s Settings) Dev() bool { return s.Mode == platform.ModeDev }

// ErrHelp is returned when the user asked for help. The usage text has been written already.
var ErrHelp = flag.ErrHelp

type rawFlags struct {
	dev      bool
	version  bool
	port     int
	dataDir  string
	agent    string
	logLevel string
	fixture  string
}

func parseFlags(args []string, usage io.Writer) (rawFlags, error) {
	var raw rawFlags
	fs := flag.NewFlagSet("marshald", flag.ContinueOnError)
	fs.SetOutput(usage)
	fs.BoolVar(&raw.dev, "dev", false, "run as a dev daemon with its own data folder, port, and stub agent")
	fs.BoolVar(&raw.version, "version", false, "print the version and exit")
	fs.IntVar(&raw.port, "port", 0, "port to listen on (default 47800, or 47801 in dev mode)")
	fs.StringVar(&raw.dataDir, "data-dir", "", "data folder (default depends on the operating system)")
	fs.StringVar(&raw.agent, "agent", "", "agents to start: stub or real (default stub in dev mode, real otherwise)")
	fs.StringVar(&raw.logLevel, "log-level", "", "debug, info, warn, or error")
	fs.StringVar(&raw.fixture, "fixture", "", "load a fixture project on start, for example small-repo")
	if err := fs.Parse(args); err != nil {
		return rawFlags{}, err
	}
	return raw, nil
}

// Load resolves the settings. A flag beats an environment setting, which beats the default.
func Load(args []string, env platform.Env, usage io.Writer) (Settings, error) {
	raw, err := parseFlags(args, usage)
	if err != nil {
		return Settings{}, err
	}
	s := Settings{Mode: platform.ModeNormal, ShowVersion: raw.version}
	if raw.dev {
		s.Mode = platform.ModeDev
	}
	if s.Port, err = resolvePort(raw.port, env.Getenv(envPort), s.Mode); err != nil {
		return Settings{}, err
	}
	if s.Agent, err = resolveAgent(raw.agent, env.Getenv(envAgent), s.Mode); err != nil {
		return Settings{}, err
	}
	if s.LogLevel, err = resolveLogLevel(raw.logLevel, env.Getenv(envLogLevel), s.Mode); err != nil {
		return Settings{}, err
	}
	if s.DataDir, err = resolveDataDir(raw.dataDir, env, s.Mode); err != nil {
		return Settings{}, err
	}
	s.Fixture = firstNonEmpty(raw.fixture, env.Getenv(envFixture))
	return s, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func resolvePort(flagValue int, envValue string, mode platform.Mode) (int, error) {
	port := flagValue
	if port == 0 && envValue != "" {
		parsed, err := strconv.Atoi(envValue)
		if err != nil {
			return 0, fmt.Errorf("%s must be a number, not %q", envPort, envValue)
		}
		port = parsed
	}
	if port == 0 {
		port = DefaultPort
		if mode == platform.ModeDev {
			port = DevPort
		}
	}
	if port < 1 || port > maxPort {
		return 0, fmt.Errorf("the port must be between 1 and %d, not %d", maxPort, port)
	}
	return port, nil
}

func resolveAgent(flagValue, envValue string, mode platform.Mode) (AgentMode, error) {
	value := firstNonEmpty(flagValue, envValue)
	switch AgentMode(value) {
	case AgentStub, AgentReal:
		return AgentMode(value), nil
	case "":
		if mode == platform.ModeDev {
			return AgentStub, nil
		}
		return AgentReal, nil
	default:
		return "", fmt.Errorf("the agent setting must be %q or %q, not %q", AgentStub, AgentReal, value)
	}
}

func resolveLogLevel(flagValue, envValue string, mode platform.Mode) (slog.Level, error) {
	value := firstNonEmpty(flagValue, envValue)
	if value == "" {
		if mode == platform.ModeDev {
			return devLogLevel, nil
		}
		return defaultLogLevel, nil
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.ToUpper(value))); err != nil {
		return 0, fmt.Errorf("the log level must be debug, info, warn, or error, not %q", value)
	}
	return level, nil
}

func resolveDataDir(flagValue string, env platform.Env, mode platform.Mode) (string, error) {
	if dir := firstNonEmpty(flagValue, env.Getenv(envDataDir)); dir != "" {
		return dir, nil
	}
	dir, err := platform.DataDir(env, mode)
	if err != nil {
		return "", errors.Join(errors.New("could not choose a data folder"), err)
	}
	return dir, nil
}
