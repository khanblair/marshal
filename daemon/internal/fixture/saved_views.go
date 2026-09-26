package fixture

import (
	"context"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// savedViewSeed is one of the saved views the prototype gives a project.
type savedViewSeed struct {
	project string
	name    string
	filters []protocol.Filter
	lane    protocol.Swimlane
}

// prototypeSavedViews are the prototype's saved views for each project, in the order its menu shows
// them. The prototype's "All cards" is not here: it is the app's own view (every card, no grouping),
// which every project has and the daemon does not store.
func prototypeSavedViews() []savedViewSeed {
	return []savedViewSeed{
		{projectAPI, "Needs me", []protocol.Filter{{Key: protocol.FilterKeyStatus, Value: string(protocol.CardStateNeeds)}}, protocol.SwimlaneNone},
		{projectAPI, "Claude Code by role", []protocol.Filter{{Key: protocol.FilterKeyAgent, Value: "Claude Code"}}, protocol.SwimlaneRole},
		{projectWeb, "UI work", []protocol.Filter{{Key: protocol.FilterKeyLabel, Value: "ui"}}, protocol.SwimlaneNone},
		{projectWeb, "By agent", []protocol.Filter{}, protocol.SwimlaneAgent},
		{projectMobile, "By package", []protocol.Filter{}, protocol.SwimlanePackage},
		{projectMobile, "api-client only", []protocol.Filter{{Key: protocol.FilterKeyPackage, Value: "packages/api-client"}}, protocol.SwimlaneNone},
	}
}

// loadSavedViews writes the prototype's saved views for a project, and says how many it made. It is
// safe to call on every start: a project that already has any saved view is left alone, so a view a
// person deleted stays deleted.
func (l *loader) loadSavedViews(ctx context.Context, projectID string) (int, error) {
	existing, err := l.projects.SavedViews(ctx, projectID)
	if err != nil {
		return 0, fmt.Errorf("read the saved views of %s: %w", projectID, err)
	}
	if len(existing.Views) > 0 {
		return 0, nil
	}
	made := 0
	for _, seed := range prototypeSavedViews() {
		if seed.project != projectID {
			continue
		}
		in := protocol.CreateSavedViewRequest{Name: seed.name, Filters: seed.filters, Swimlane: seed.lane}
		if _, _, err := l.projects.CreateSavedView(ctx, projectID, in); err != nil {
			return made, fmt.Errorf("add the saved view %q of %s: %w", seed.name, projectID, err)
		}
		made++
	}
	return made, nil
}
