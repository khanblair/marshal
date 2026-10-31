package catalog

import (
	"context"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Preference is what the person set for one agent in Settings. The zero value is the default: the
// program found on this computer, and the agent turned on.
type Preference struct {
	// Path is a program the person chose, used in place of the one found on the computer. Empty
	// means the one found.
	Path string
	// Off keeps the agent from being started for a card. It is still listed and can still be tested.
	Off bool
}

// Preferences reads what the person set for each agent. The catalog reads it at each detection run,
// so a change shows at the next run, and a refresh shows it at once.
type Preferences interface {
	Preference(ctx context.Context, kind protocol.AgentKind) (Preference, error)
}

// PreferenceStore is Preferences that can also save a choice.
type PreferenceStore interface {
	Preferences
	SetPreference(ctx context.Context, kind protocol.AgentKind, pref Preference) error
}
