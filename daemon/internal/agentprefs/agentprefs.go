// Package agentprefs keeps what the person set for each agent in Settings: a program to use in
// place of the one found, and whether the agent is turned off. Each agent is one row of the settings
// table, so an agent with nothing set has no row at all and reads as the default.
package agentprefs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/agents/catalog"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// keyPrefix names an agent's row in the settings table, such as "agent.claude".
const keyPrefix = "agent."

// Service reads and writes the agent choices.
type Service struct {
	store *store.Store
}

var _ catalog.PreferenceStore = (*Service)(nil)

// New returns the service. It needs the store, because a choice that cannot be read would look like
// the default.
func New(st *store.Store) (*Service, error) {
	if st == nil {
		return nil, errors.New("the agent settings need the store")
	}
	return &Service{store: st}, nil
}

// stored is the JSON kept for one agent.
type stored struct {
	Path string `json:"path"`
	Off  bool   `json:"off"`
}

// Preference reads the choice for an agent. An agent with no row answers the default.
func (s *Service) Preference(ctx context.Context, kind protocol.AgentKind) (catalog.Preference, error) {
	raw, err := s.store.Queries().GetSetting(ctx, keyPrefix+string(kind))
	switch {
	case err == nil:
		var in stored
		if err := json.Unmarshal([]byte(raw), &in); err != nil {
			return catalog.Preference{}, fmt.Errorf("read the settings of %s: %w", kind, err)
		}
		return catalog.Preference{Path: in.Path, Off: in.Off}, nil
	case store.IsNotFound(err):
		return catalog.Preference{}, nil
	default:
		return catalog.Preference{}, fmt.Errorf("read the settings of %s: %w", kind, err)
	}
}

// SetPreference saves the choice for an agent, replacing what was there.
func (s *Service) SetPreference(ctx context.Context, kind protocol.AgentKind, pref catalog.Preference) error {
	encoded, err := json.Marshal(stored{Path: pref.Path, Off: pref.Off})
	if err != nil {
		return fmt.Errorf("encode the settings of %s: %w", kind, err)
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		return q.SetSetting(ctx, db.SetSettingParams{Key: keyPrefix + string(kind), ValueJSON: string(encoded)})
	})
	if err != nil {
		return fmt.Errorf("save the settings of %s: %w", kind, err)
	}
	return nil
}
