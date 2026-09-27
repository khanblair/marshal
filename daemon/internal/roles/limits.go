package roles

import (
	"context"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// LimitsFor reads the ceilings of the role a card runs under, so the session manager's harness can
// decide when a card has gone too far (B5.3, build-plan 5.3). It is the one thing outside this
// package that reads a role for a reason other than showing it: the harness reads a role's limits
// when a card runs, not when the role is saved, so the ceilings are always the current ones.
//
// The project's own version of the role wins when it has one, because that is the spec the project's
// cards run under. A role that is not there is not an error: it reports false and the card runs
// unlimited, which is what a card naming a role somebody deleted should do rather than being stopped
// for a limit nobody set.
func (s *Service) LimitsFor(ctx context.Context, projectID, name string) (harness.Limits, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return harness.Limits{}, false, nil
	}
	var (
		encoded string
		found   bool
	)
	err := s.store.Read(ctx, func(q *db.Queries) error {
		row, err := q.GetRoleByName(ctx, name)
		if err != nil {
			if store.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("read the role %q: %w", name, err)
		}
		found = true
		encoded = row.SpecJSON
		if projectID == "" {
			return nil
		}
		override, err := q.GetRoleOverride(ctx, db.GetRoleOverrideParams{RoleID: row.ID, ProjectID: projectID})
		if err != nil {
			if store.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("read the override of role %q for project %s: %w", name, projectID, err)
		}
		encoded = override.SpecJSON
		return nil
	})
	if err != nil {
		return harness.Limits{}, false, err
	}
	if !found {
		return harness.Limits{}, false, nil
	}
	spec, err := decodeSpec(encoded)
	if err != nil {
		return harness.Limits{}, false, fmt.Errorf("read the limits of role %q: %w", name, err)
	}
	return harnessLimits(spec.Limits), true, nil
}

// harnessLimits turns a role's own ceilings into the shape the harness decides with. The two carry
// the same three numbers under different names - the wire names them for what a person reads in the
// role editor, the harness names them for what it measures.
func harnessLimits(limits protocol.RoleLimits) harness.Limits {
	return harness.Limits{TurnMinutes: limits.Time, CostDollars: limits.Cost, Rounds: limits.Rounds}
}
