package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/agents/catalog"
	"github.com/khanblair/marshal/daemon/internal/agents/pty"
	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/platform"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const (
	// stubTerminalProgram is the program that runs in a card's terminal when the stub agent is in use.
	stubTerminalProgram = "/bin/sh"
	// stubTerminalScript is what the stub terminal does: it says which session it was given, which
	// shows that a switch between the views kept the session id, and then echoes what is typed. The
	// session id is the script's first argument. It runs no command that a person typed, so a stub
	// daemon never gives a shell.
	stubTerminalScript = `printf 'terminal session %s\n' "$1"; exec cat`
)

// buildTerminals makes the registry the session manager runs a card's agent in a terminal through
// (the terminal view, docs/architecture.md 4.3). It has a factory only for a kind whose interactive
// command can pick a session up by the id the chat view gave it, and asking for the terminal of any
// other kind is refused with a plain sentence before anything is stopped.
//
// In real mode that is Claude Code. Its factory detects the installed program the first time it is
// called, as the chat view's does, so nothing is looked for at start-up. Gemini CLI and Codex are
// left out on purpose: nothing in this repository shows an interactive command of either that resumes
// the session id its chat mode was given (the ACP session id of Gemini CLI, and Codex has no chat
// adapter yet), and a terminal that started a fresh conversation in its place would lose the context
// the switch promises to keep. Adding one is a config in agents/pty and a line here.
//
// In stub mode every kind that the stub catalog lists gets the stub terminal.
func buildTerminals(settings config.Settings, env platform.Env, cat catalog.Source, log *slog.Logger) (*agents.Registry, error) {
	registry := agents.NewRegistry()
	if settings.Agent == config.AgentStub {
		if env.GOOS == "windows" {
			return registry, nil
		}
		for _, kind := range catalog.Kinds() {
			if err := registry.Register(kind, stubTerminalFactory(log)); err != nil {
				return nil, fmt.Errorf("register the stub terminal for %s: %w", kind, err)
			}
		}
		return registry, nil
	}
	if err := registry.Register(protocol.AgentKindClaude, realTerminalFactory(cat, log)); err != nil {
		return nil, fmt.Errorf("register the terminal of the claude agent: %w", err)
	}
	return registry, nil
}

// stubTerminalFactory makes the adapter that runs the stub terminal.
func stubTerminalFactory(log *slog.Logger) agents.Factory {
	return func() (agents.Agent, error) {
		args := func(sessionID string) []string { return []string{"-c", stubTerminalScript, "sh", sessionID} }
		return pty.New(pty.Config{Path: stubTerminalProgram, StartArgs: args, ResumeArgs: args, Logger: log})
	}
}

// realTerminalFactory makes the adapter that runs Claude Code's own interactive command, once the
// program has been found. An agent that is not installed, or that Marshal cannot start, is
// agents.ErrUnknownKind, which the session manager answers with "this agent has no terminal view".
func realTerminalFactory(cat catalog.Source, log *slog.Logger) agents.Factory {
	return func() (agents.Agent, error) {
		detected, err := cat.Detect(context.Background())
		if err != nil {
			return nil, fmt.Errorf("detect installed agents: %w", err)
		}
		for _, d := range detected {
			if d.Kind == protocol.AgentKindClaude && d.Startable {
				cfg := pty.ClaudeConfig(d.Path, catalog.ProgramEnv(d.Path))
				cfg.Logger = log
				return pty.New(cfg)
			}
		}
		return nil, fmt.Errorf("%w: claude is not installed, or Marshal cannot start it", agents.ErrUnknownKind)
	}
}
