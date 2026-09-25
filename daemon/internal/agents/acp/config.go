// Package acp is the adapter for agents that speak the Agent Client Protocol over standard input
// and output. It runs one child process per session, through internal/proc, and turns the
// protocol's messages into agents.AgentEvent values.
//
// The adapter tells the agent that it has no file system and no terminal to offer, so the agent
// uses its own tools in its working folder. Every other request that an agent makes of its client
// is answered with "method not found".
package acp

import (
	"errors"
	"log/slog"
	"time"
)

const (
	defaultStopGrace      = 3 * time.Second
	defaultStartTimeout   = 60 * time.Second
	defaultInterruptGrace = 10 * time.Second
)

// Config says how to run one agent program. Point it at the stub agent, or at a real agent, and
// the rest of the daemon cannot tell the difference.
type Config struct {
	// Path is the agent program. It is required.
	Path string
	// Args are its arguments. The stub agent takes --state-dir, --scenario, and --speed here.
	Args []string
	// Env is added to the environment of every session, as KEY=value entries. A session's own
	// StartSpec.Env comes after it.
	Env []string
	// PermissionModes turns a protocol permission mode ("ask", "auto-edits", "plan", "full-auto",
	// "bypass") into the id or name that this agent uses for it. A mode that is not in the map is
	// matched as it is, with dashes, underscores, spaces, and case ignored. Most real agents use
	// other words, and a start that asks for a mode the agent does not recognize fails, so every
	// real agent needs this map.
	PermissionModes map[string]string
	// ThinkingModes does the same for thinking modes, and every real agent needs it too.
	ThinkingModes map[string]string
	// AutoAuthMethods lists the ids of sign-in methods that the adapter may use without asking
	// the user, when the agent says that it needs a sign-in. A method of the plain agent type can
	// still open a browser, so nothing is used unless it is named here. The default is empty, and
	// then a needed sign-in always returns agents.AuthRequiredError.
	AutoAuthMethods []string
	// StopGrace is how long Stop waits for the agent to exit on its own before it kills it. The
	// default is 3 seconds.
	StopGrace time.Duration
	// StartTimeout bounds the start of a session, from the process launch to a ready session. The
	// default is 60 seconds.
	StartTimeout time.Duration
	// InterruptGrace is how long an interrupted turn may take to end before the adapter stops
	// waiting for the agent's answer. The default is 10 seconds.
	InterruptGrace time.Duration
	// Logger receives the adapter's log lines. The default is slog.Default().
	Logger *slog.Logger
}

// withDefaults checks the config and fills in what was left out.
func (c Config) withDefaults() (Config, error) {
	if c.Path == "" {
		return Config{}, errors.New("acp adapter: the path of the agent program is required")
	}
	if c.StopGrace <= 0 {
		c.StopGrace = defaultStopGrace
	}
	if c.StartTimeout <= 0 {
		c.StartTimeout = defaultStartTimeout
	}
	if c.InterruptGrace <= 0 {
		c.InterruptGrace = defaultInterruptGrace
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return c, nil
}
