package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/agents/acp"
	"github.com/khanblair/marshal/daemon/internal/agents/builtin"
	"github.com/khanblair/marshal/daemon/internal/agents/catalog"
	"github.com/khanblair/marshal/daemon/internal/agents/claude"
	"github.com/khanblair/marshal/daemon/internal/agents/codex"
	"github.com/khanblair/marshal/daemon/internal/agents/gemini"
	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/platform"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/providers"
	"github.com/khanblair/marshal/daemon/internal/security"
)

// buildAgents makes the agent registry the session manager starts processes through, and the
// catalog the API layer will list agents from. In stub mode (settings.Agent == config.AgentStub)
// every kind is registered behind the stub agent binary. Real mode registers the
// Claude, Gemini, and Codex CLIs plus Marshal's built-in provider agent.
func buildAgents(settings config.Settings, env platform.Env, log *slog.Logger, providerSvc *providers.Service, prefs catalog.Preferences, saved builtin.History) (*agents.Registry, catalog.Source, error) {
	registry := agents.NewRegistry()
	if settings.Agent == config.AgentStub {
		return registerStubAgents(registry, settings, env, log)
	}
	// BuiltinModels is the key store's own list: the built-in agent can run the models of exactly the
	// providers that have a key saved, so its row is asked again on every List and shows a key the
	// person saved a moment ago.
	cat := catalog.New(catalog.Options{Logger: log, BuiltinModels: providerSvc.Models, Preferences: prefs})
	for _, kind := range []protocol.AgentKind{protocol.AgentKindClaude, protocol.AgentKindGemini, protocol.AgentKindCodex} {
		if err := registry.Register(kind, realAgentFactory(cat, kind, log)); err != nil {
			return nil, nil, fmt.Errorf("register the %s agent: %w", kind, err)
		}
	}
	if err := registry.Register(protocol.AgentKindBuiltin, builtinAgentFactory(log, providerSvc, saved)); err != nil {
		return nil, nil, fmt.Errorf("register the built-in agent: %w", err)
	}
	return registry, cat, nil
}

// builtinAgentFactory returns the factory for Marshal's own agent. It runs in the daemon, so there
// is no path to resolve and nothing to detect: what it needs is a model provider.
//
// The resolver is the provider service itself, which is the one place that knows which providers
// have a key saved: builtin.Resolver and providers.Service.Resolve have the same shape on purpose,
// so the key store the screens write and the resolver the agent reads are the same object. A model
// no saved key can run is reported as one nobody can run, in plain words, rather than pretending to
// start.
func builtinAgentFactory(log *slog.Logger, resolver builtin.Resolver, saved builtin.History) agents.Factory {
	return builtin.Factory(builtin.Config{
		Resolver: resolver,
		History:  saved,
		Profile:  security.DefaultProfile(),
		Logger:   log,
	})
}

// registerStubAgents registers every kind catalog.Kinds() lists behind the same agents/acp
// adapter configuration, pointed at the built stub agent binary (catalog.Stub's own doc comment:
// "The session manager starts the scripted stub agent behind each kind"). Its --state-dir is a
// fixed folder under the data directory, so its own sessions resume across a daemon restart the
// same way real ones do.
func registerStubAgents(registry *agents.Registry, settings config.Settings, env platform.Env, log *slog.Logger) (*agents.Registry, catalog.Source, error) {
	path, err := stubAgentPath(env)
	if err != nil {
		return nil, nil, err
	}
	stateDir := filepath.Join(settings.DataDir, "dev-agent-state")
	args := []string{"--state-dir", stateDir, "--speed", stubAgentSpeed}
	factory := func() (agents.Agent, error) {
		return acp.New(acp.Config{Path: path, Args: args, Logger: log})
	}
	for _, kind := range catalog.Kinds() {
		if err := registry.Register(kind, factory); err != nil {
			return nil, nil, fmt.Errorf("register the stub agent for %s: %w", kind, err)
		}
	}
	return registry, catalog.NewStub(), nil
}

// stubAgentPath resolves the stub agent binary: the MARSHAL_STUB_AGENT environment override if
// set, else a file named stub-agent (stub-agent.exe on Windows) in the same folder as the running
// marshald executable. It fails with a clear, actionable message if neither is found: the stub
// agent is never built at runtime.
func stubAgentPath(env platform.Env) (string, error) {
	if override := env.Getenv(stubAgentEnv); override != "" {
		return override, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find the running program: %w", err)
	}
	name := stubAgentBinary
	if env.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(filepath.Dir(exe), name)
	if _, statErr := os.Stat(path); statErr != nil {
		return "", fmt.Errorf(
			"find the stub agent at %s: run `pnpm build` first, or set %s to its path: %w", path, stubAgentEnv, statErr)
	}
	return path, nil
}

// realAgentFactory returns a Factory that detects the installed agents the first time it is
// called, never at start-up (the catalog package's own doc comment: detection "never runs when
// the daemon starts"), and starts the real adapter for kind if it is installed and startable.
func realAgentFactory(cat catalog.Source, kind protocol.AgentKind, log *slog.Logger) agents.Factory {
	return func() (agents.Agent, error) {
		detected, err := cat.Detect(context.Background())
		if err != nil {
			return nil, fmt.Errorf("detect installed agents: %w", err)
		}
		for _, d := range detected {
			if d.Kind == kind {
				return realAgentOf(kind, d, log)
			}
		}
		return nil, fmt.Errorf("%w: %s", agents.ErrUnknownKind, kind)
	}
}

// realAgentOf starts the real adapter for a detected agent, or reports agents.ErrUnknownKind when
// it is not installed or Marshal cannot start it yet.
func realAgentOf(kind protocol.AgentKind, d catalog.Detected, log *slog.Logger) (agents.Agent, error) {
	if d.Off {
		return nil, fmt.Errorf("%w: %s is turned off in Settings", agents.ErrUnknownKind, kind)
	}
	if !d.Startable {
		return nil, fmt.Errorf("%w: %s is not installed, or Marshal cannot start it yet", agents.ErrUnknownKind, kind)
	}
	switch kind {
	case protocol.AgentKindClaude:
		return claude.New(claude.Config{Path: d.Path, Logger: log})
	case protocol.AgentKindGemini:
		return gemini.New(gemini.Config{Path: d.Path, Logger: log})
	case protocol.AgentKindCodex:
		return codex.New(codex.Config{Path: d.Path, Logger: log})
	default:
		return nil, fmt.Errorf("%w: %s", agents.ErrUnknownKind, kind)
	}
}
