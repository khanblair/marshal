// Package claude is the adapter for Claude Code (agents.Agent), driven directly through its own
// streaming JSON mode rather than through ACP: Claude Code has no ACP support, so this package
// starts the CLI itself, writes stream-json lines to its standard input, and reads stream-json
// lines from its standard output. One Claude Code process serves one session for its whole life,
// kept alive across turns with --input-format stream-json, matching every other adapter's rule
// that sending a message never starts a new process.
package claude

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const (
	defaultStartTimeout = 60 * time.Second
	defaultStopGrace    = 5 * time.Second
	// defaultInterruptGrace bounds how long Interrupt waits for Claude Code to end the turn on
	// its own after the interrupt control_request, and again after the SIGINT that follows it.
	// Past it, the adapter falls back to stopping the process (see turn.go).
	defaultInterruptGrace = 10 * time.Second
	// defaultReadyWindow is how long a freshly started process gets to prove it is alive before
	// awaitReady treats it as ready even without an "init" line (see the report's Ruling on
	// this: it is not certain that Claude Code always sends "init" before it needs input).
	defaultReadyWindow = 2 * time.Second
)

// Config says how to run Claude Code.
type Config struct {
	// Path is the claude program, as the catalog found it. It is required.
	Path string
	// Env is added to the environment of every session, as KEY=value entries, after the folder
	// of Path is put first on PATH. A session's own StartSpec.Env comes after it.
	Env []string
	// Model maps a model id from StartSpec.Model to the value Claude Code's --model flag takes.
	// The default, nil, passes StartSpec.Model straight through: the catalog's own model ids
	// (see catalog/models.go) are already Claude Code's own aliases and full names.
	Model map[string]string
	// PermissionModes maps a protocol.PermissionMode value to one of Claude Code's
	// --permission-mode choices. The default, nil, is defaultPermissionModes, this package's own
	// ruling (see config_modes.go and the report).
	PermissionModes map[string]string
	// StartTimeout bounds the start of a session, from the process launch to it being ready (an
	// "init" line, or ReadyWindow passing with the process still alive). The default is 60
	// seconds.
	StartTimeout time.Duration
	// StopGrace is how long Stop, and the interrupt fallback, wait for the process to exit on its
	// own before they kill it. The default is 5 seconds.
	StopGrace time.Duration
	// InterruptGrace is how long Interrupt waits for Claude Code to end a turn on its own after
	// the interrupt control_request, and again after the SIGINT that follows it, before the
	// adapter falls back to stopping the process and resuming it on the next Send. The default
	// is 10 seconds.
	InterruptGrace time.Duration
	// ReadyWindow is how long a freshly started process gets to prove it is alive before it is
	// treated as ready even without an "init" line. The default is 2 seconds.
	ReadyWindow time.Duration
	// Logger receives the adapter's log lines. The default is slog.Default().
	Logger *slog.Logger
}

// withDefaults checks the config and fills in what was left out.
func (c Config) withDefaults() (Config, error) {
	if c.Path == "" {
		return Config{}, errors.New("claude adapter: the path of the claude program is required")
	}
	if c.StartTimeout <= 0 {
		c.StartTimeout = defaultStartTimeout
	}
	if c.StopGrace <= 0 {
		c.StopGrace = defaultStopGrace
	}
	if c.InterruptGrace <= 0 {
		c.InterruptGrace = defaultInterruptGrace
	}
	if c.ReadyWindow <= 0 {
		c.ReadyWindow = defaultReadyWindow
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return c, nil
}

// mappedModel returns what Config.Model says to send for a requested model, or the model as it
// was asked for when the config has no entry for it.
func (c Config) mappedModel(model string) string {
	if v, ok := c.Model[model]; ok {
		return v
	}
	return model
}

// permissionMode turns a protocol permission mode into Claude Code's --permission-mode choice.
// An empty mode returns an empty string, which means "let Claude Code use its own default" (the
// CLI's own default for -p is "manual"). A mode this adapter does not know returns
// ErrUnsupportedSetting.
func (c Config) permissionMode(mode string) (string, error) {
	if mode == "" {
		return "", nil
	}
	table := c.PermissionModes
	if table == nil {
		table = defaultPermissionModes()
	}
	v, ok := table[mode]
	if !ok {
		return "", fmt.Errorf("%w: permission mode %q", agents.ErrUnsupportedSetting, mode)
	}
	return v, nil
}

// defaultPermissionModes is this adapter's ruling on how Marshal's permission modes map to Claude
// Code's five --permission-mode choices. See the report for how each choice was confirmed against
// Claude Code's own documentation (docs.claude.com/en/docs/agent-sdk/permissions).
func defaultPermissionModes() map[string]string {
	return map[string]string{
		// "manual" is the CLI's own default for -p: it asks before every action. Marshal always
		// also passes --permission-prompts=none (see startArgs), so with no approval channel
		// wired in Phase 1, what would have asked is denied instead of hanging. That matches
		// protocol.AgentCapabilities.Approvals's own doc: "Ask" skips what needs approval.
		string(protocol.PermissionModeAsk): "manual",
		// "acceptEdits" auto-approves file edits and common filesystem commands (mkdir, touch,
		// mv, cp, sed, rm); other shell commands still need approval, which --permission-prompts
		// none turns into a denial, matching "allows file edits and asks for commands".
		string(protocol.PermissionModeAutoEdits): "acceptEdits",
		// "plan" explores and plans without editing source files, matching "Plan only" exactly.
		string(protocol.PermissionModePlan): "plan",
		// "auto" hands every prompt to Claude Code's own model classifier instead of a person,
		// matching "allows what the rules allow without asking". "dontAsk" was not used here: it
		// denies whatever would have prompted, which is the opposite of "full auto".
		string(protocol.PermissionModeFullAuto): "auto",
		// "bypassPermissions" skips permission checks outright, matching "Bypass permissions".
		string(protocol.PermissionModeBypass): "bypassPermissions",
	}
}

// effortFor turns a protocol thinking mode into Claude Code's --effort choice. Claude Code also
// offers "max", which no Marshal thinking mode maps to, so it is never sent.
func effortFor(mode string) (string, bool) {
	switch protocol.ThinkingMode(mode) {
	case protocol.ThinkingModeLow:
		return "low", true
	case protocol.ThinkingModeMedium:
		return "medium", true
	case protocol.ThinkingModeHigh:
		return "high", true
	case protocol.ThinkingModeExtraHigh:
		return "xhigh", true
	default:
		return "", false
	}
}
