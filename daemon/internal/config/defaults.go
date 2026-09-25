package config

import "log/slog"

// Ports. Dev mode uses its own port, so a dev daemon and a normal install can run together.
const (
	DefaultPort = 47800
	DevPort     = 47801
	maxPort     = 65535
)

// Default log levels: detail in dev mode, normal events otherwise.
const (
	defaultLogLevel = slog.LevelInfo
	devLogLevel     = slog.LevelDebug
)

// AgentMode says which agents a daemon starts.
type AgentMode string

// The stub agent is the default in dev mode, so development costs nothing and works offline.
const (
	AgentStub AgentMode = "stub"
	AgentReal AgentMode = "real"
)

// Names of the environment settings, as listed in development.md section 3.5.
const (
	envDataDir  = "MARSHAL_DATA_DIR"
	envPort     = "MARSHAL_PORT"
	envAgent    = "MARSHAL_AGENT"
	envLogLevel = "MARSHAL_LOG_LEVEL"
	envFixture  = "MARSHAL_FIXTURE"
)
