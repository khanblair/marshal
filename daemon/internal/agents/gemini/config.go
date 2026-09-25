// Package gemini runs Gemini CLI through the ACP adapter (agents/acp). Gemini CLI speaks the Agent
// Client Protocol when it is started with --acp, so this package does not parse anything itself. It
// says how Gemini CLI is started and how Marshal's settings turn into Gemini's, and it fixes the
// two places where Gemini's ACP support differs from what the ACP adapter expects:
//
//   - The model. Gemini lists its models in a part of the protocol that the ACP library does not
//     have yet, so the adapter cannot set one. Gemini reads the GEMINI_MODEL variable, and each
//     session has its own process, so the model goes in the environment of the process.
//   - Signing in. Gemini asks for a sign-in when a session starts and the person has not signed
//     in. Marshal never signs in for the person, so that becomes a plain sentence that says what
//     to do in a terminal.
package gemini

import (
	"log/slog"
	"slices"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents/acp"
	"github.com/khanblair/marshal/daemon/internal/agents/catalog"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Config says how to run Gemini CLI.
type Config struct {
	// Path is the gemini program, as the catalog found it. It is required.
	Path string
	// Args replaces the arguments that start Gemini CLI in ACP mode (--acp). Tests use it to point
	// the adapter at the scripted stub agent. Leave it empty in normal use.
	Args []string
	// Env is added to the environment of every session, as KEY=value entries, after the entries
	// that this package sets, so it can override them. A session's own StartSpec.Env comes after.
	Env []string
	// StopGrace, StartTimeout, and InterruptGrace are passed to the ACP adapter, which has the
	// defaults.
	StopGrace, StartTimeout, InterruptGrace time.Duration
	// Logger receives the adapter's log lines. The default is slog.Default().
	Logger *slog.Logger
}

// modelVariable is the environment variable that Gemini CLI reads for the model of a session.
const modelVariable = "GEMINI_MODEL"

// acpArgs are the arguments that start Gemini CLI as an ACP agent. The older spelling,
// --experimental-acp, is deprecated.
func acpArgs() []string { return []string{"--acp"} }

// permissionModes turns Marshal's permission modes into the ids of Gemini's approval modes, which
// the ACP session lists as its modes. The CLI's own flag spells the second one auto_edit, but the
// id on the protocol is autoEdit.
//
// Full auto and bypass both mean yolo, which approves every tool. Gemini has no lesser step
// between asking and approving everything else; the harness decides whether bypass is allowed, and
// this adapter only maps the word. Gemini CLI 0.35.1 offers the plan mode unless the person's own
// settings turn planning off, and when it is not offered, a card in plan mode fails to start with
// a message that lists the modes that Gemini does offer.
func permissionModes() map[string]string {
	return map[string]string{
		string(protocol.PermissionModeAsk):       "default",
		string(protocol.PermissionModeAutoEdits): "autoEdit",
		string(protocol.PermissionModePlan):      "plan",
		string(protocol.PermissionModeFullAuto):  "yolo",
		string(protocol.PermissionModeBypass):    "yolo",
	}
}

// acpConfig builds the configuration of the ACP adapter.
//
// Nothing here signs in: AutoAuthMethods stays empty, so a needed sign-in is reported and never
// done. The environment sets NO_BROWSER so that Gemini does not open a browser from a background
// daemon, and it puts the folder of the program first on PATH, because Gemini CLI starts through
// "env node".
//
// Gemini has no setting for how hard a model thinks, so ThinkingModes is empty and the adapter
// reports that thinking cannot be set.
func acpConfig(cfg Config) acp.Config {
	args := cfg.Args
	if len(args) == 0 {
		args = acpArgs()
	}
	env := slices.Concat(catalog.ProgramEnv(cfg.Path), []string{"NO_BROWSER=true"}, cfg.Env)
	return acp.Config{
		Path: cfg.Path, Args: args, Env: env,
		PermissionModes: permissionModes(),
		StopGrace:       cfg.StopGrace, StartTimeout: cfg.StartTimeout, InterruptGrace: cfg.InterruptGrace,
		Logger: cfg.Logger,
	}
}
