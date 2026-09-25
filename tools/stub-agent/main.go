// Command stub-agent is a fake coding agent that speaks the Agent Client Protocol over standard
// input and output. It plays scripted scenarios instead of calling a model, so tests and dev mode
// run without cost and without a network. Sessions are saved to a state folder, so a new process
// can resume them.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
)

const (
	exitOK       = 0
	exitFailed   = 1
	exitBadInput = 2

	defaultScenarioName = "default"
	defaultStateDirName = "marshal-stub-agent"
	defaultSpeed        = 1.0

	envScenario = "STUB_SCENARIO"
	envStateDir = "STUB_STATE_DIR"
	envSpeed    = "STUB_SPEED"
)

const usage = `Usage: stub-agent [--scenario NAME] [--state-dir DIR] [--speed N] [--version]

A fake ACP agent for tests and dev mode. It speaks the Agent Client Protocol on standard input
and output and plays scripted scenarios.

Flags (each has an environment variable that sets its default):
  --scenario NAME   Scenario for prompts without an @scenario:NAME marker (STUB_SCENARIO, default)
  --state-dir DIR   Folder for saved sessions (STUB_STATE_DIR, a folder in the temp folder)
  --speed N         Multiplies every pause, 0 removes them (STUB_SPEED, default 1)
  --version         Print the version`

// config is what the flags and environment variables say.
type config struct {
	scenario    string
	stateDir    string
	speed       float64
	showVersion bool
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(
		ctx,
		os.Args[1:],
		terminal{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr},
		environment{getenv: os.Getenv, tempDir: os.TempDir()},
	)
	stop()
	os.Exit(code)
}

// run starts the agent on the given streams and returns the process exit code. It returns when
// the client closes the connection or the context ends.
func run(ctx context.Context, args []string, term terminal, env environment) int {
	cfg, err := parseConfig(args, env)
	if errors.Is(err, flag.ErrHelp) {
		say(term.stdout, "%s", usage)
		return exitOK
	}
	if err != nil {
		say(term.stderr, "stub-agent: %v\n\n%s", err, usage)
		return exitBadInput
	}
	if cfg.showVersion {
		say(term.stdout, "stub-agent %s", version)
		return exitOK
	}
	scenarios, err := loadBuiltinScenarios()
	if err != nil {
		say(term.stderr, "stub-agent: %v", err)
		return exitFailed
	}
	agent, err := newAgent(cfg, scenarios)
	if err != nil {
		say(term.stderr, "stub-agent: %v", err)
		return exitBadInput
	}
	// The connection reports its own trouble through the default logger. That must not reach
	// standard output, which carries the protocol.
	slog.SetDefault(slog.New(slog.NewTextHandler(term.stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	conn := serve(agent, term.stdout, term.stdin)
	select {
	case <-conn.Done():
	case <-ctx.Done():
	}
	return exitOK
}

// parseConfig reads the flags. A flag beats its environment variable, which beats the default.
func parseConfig(args []string, env environment) (config, error) {
	speed, err := speedFromEnv(env)
	if err != nil {
		return config{}, err
	}
	var cfg config
	flags := flag.NewFlagSet("stub-agent", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&cfg.scenario, "scenario", orDefault(env.getenv(envScenario), defaultScenarioName), "")
	flags.StringVar(&cfg.stateDir, "state-dir",
		orDefault(env.getenv(envStateDir), filepath.Join(env.tempDir, defaultStateDirName)), "")
	flags.Float64Var(&cfg.speed, "speed", speed, "")
	flags.BoolVar(&cfg.showVersion, "version", false, "")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if flags.NArg() > 0 {
		return config{}, fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if math.IsNaN(cfg.speed) || math.IsInf(cfg.speed, 0) || cfg.speed < 0 {
		return config{}, fmt.Errorf("speed %v must be a number of 0 or more", cfg.speed)
	}
	return cfg, nil
}

// speedFromEnv reads the default speed from the environment.
func speedFromEnv(env environment) (float64, error) {
	raw := env.getenv(envSpeed)
	if raw == "" {
		return defaultSpeed, nil
	}
	speed, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("%s is not a number: %w", envSpeed, err)
	}
	return speed, nil
}

// orDefault returns value, or fallback when value is empty.
func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
