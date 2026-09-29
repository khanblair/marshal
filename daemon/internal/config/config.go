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
	// Tailnet joins the tailnet from inside the daemon, so the daemon is reachable on this
	// machine and on the person's own devices, with no separate Tailscale install (B9.1). It is
	// off unless it is asked for: a daemon nobody asked to join a tailnet listens on loopback
	// only, exactly as it did before this field existed (docs/architecture.md section 13).
	Tailnet bool
	// TailnetHostname is the node's name on the tailnet. Empty lets the node use the program's
	// own name.
	TailnetHostname string
	// Funnel exposes /hooks/* to the public internet through Tailscale Funnel. Only the webhook
	// routes are served on it, and each is still signature-verified the way Phase 8 built it. It
	// means nothing without Tailnet: there would be no node to expose.
	Funnel bool
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
	tailnet  bool
	hostname string
	funnel   bool
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
	fs.StringVar(&raw.fixture, "fixture", "", "dev mode only: load a fixture on start (the only one is prototype: the three projects the prototype shows)")
	fs.BoolVar(&raw.tailnet, "tailnet", false, "join the tailnet from inside the daemon, so phones reach this daemon without a separate Tailscale install (also MARSHAL_TAILNET)")
	fs.StringVar(&raw.hostname, "tailnet-hostname", "", "this node's name on the tailnet (default: the program's own name; also MARSHAL_TAILNET_HOSTNAME)")
	fs.BoolVar(&raw.funnel, "funnel", false, "expose /hooks/* publicly through Tailscale Funnel, every request still signature-verified (also MARSHAL_FUNNEL)")
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
	// The two switches are on when either the flag or the environment setting says so. There is
	// no "off": a daemon only ever joins a tailnet when someone asked it to, and the way to stop
	// asking is to remove both.
	s.Tailnet = raw.tailnet || resolveBool(env.Getenv(envTailnet))
	s.Funnel = raw.funnel || resolveBool(env.Getenv(envFunnel))
	s.TailnetHostname = firstNonEmpty(raw.hostname, env.Getenv(envTailnetHostname))
	return s, nil
}

// resolveBool reads an environment setting that is a switch: "1", "yes", "true", or "on" is on,
// and anything else - including an empty setting, which means it was never set - is off.
func resolveBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "yes", "true", "on":
		return true
	default:
		return false
	}
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
