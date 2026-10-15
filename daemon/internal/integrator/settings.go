package integrator

import (
	"context"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// settingsOf is how a project ships its cards, as the wire says it.
func settingsOf(s Settings) protocol.IntegrationSettings {
	return protocol.IntegrationSettings{
		AutoMerge: s.AutoMerge, AutoShip: s.AutoShip, Review: s.Review, Checks: s.Checks, AllowPush: s.AllowPush,
	}
}

// UpdateSettings changes the switches a person sent and leaves the rest as they are. Turning merging
// on merges the cards that are already waiting. The answer is the project's state after the change.
func (s *Service) UpdateSettings(
	ctx context.Context, projectID string, patch protocol.IntegrationSettingsPatch,
) (protocol.IntegrationState, error) {
	if _, err := s.projects.Get(ctx, projectID); err != nil {
		return protocol.IntegrationState{}, err
	}
	current, err := s.ledger.Settings(ctx, projectID)
	if err != nil {
		return protocol.IntegrationState{}, err
	}
	flags := Flags{
		AutoMerge: pick(patch.AutoMerge, current.AutoMerge), AutoShip: pick(patch.AutoShip, current.AutoShip),
		Review: pick(patch.Review, current.Review), Checks: pick(patch.Checks, current.Checks),
		AllowPush: pick(patch.AllowPush, current.AllowPush),
	}
	if err := s.ledger.SetFlags(ctx, projectID, flags); err != nil {
		return protocol.IntegrationState{}, err
	}
	if flags.AutoMerge && !current.AutoMerge {
		s.kick(ctx, projectID)
	}
	return s.State(ctx, projectID)
}

// AllowPush says whether agents may push and Marshal may publish for a project. A project whose
// settings cannot be read is not allowed: nothing leaves the machine on a guess.
func (s *Service) AllowPush(ctx context.Context, projectID string) bool {
	settings, err := s.ledger.Settings(ctx, projectID)
	return err == nil && settings.AllowPush
}

// pick is the value a patch sets, or the current one when it sets nothing.
func pick(set *bool, current bool) bool {
	if set == nil {
		return current
	}
	return *set
}
