package integrator

import (
	"context"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// sweepBackups deletes the backup branches of deliveries that can no longer be undone. A backup
// branch is made before every delivery and only serves its undo; once the target has moved past the
// merge (another card landed, or the owner committed), the branch is clutter in the owner's repository.
// Only a delivery the target has moved past is swept: a delivery whose undo is merely blocked for now,
// by a busy folder for instance, keeps its branch. A branch that is already gone is not an error.
func (s *Service) sweepBackups(ctx context.Context, project protocol.Project) {
	rows, err := s.ledger.Deliveries(ctx, project.ID, historyLimit)
	if err != nil {
		s.log.Debug("could not read the merge history to sweep backup branches", "project_id", project.ID, "error", err)
		return
	}
	live := map[string]bool{}
	for _, row := range rows {
		if row.UndoneAt.IsZero() {
			if tip, err := s.git.Rev(ctx, project.Path, row.Target); err != nil || tip == row.Commit {
				live[row.BackupBranch] = true
			}
		}
	}
	for _, row := range rows {
		if strings.TrimSpace(row.BackupBranch) == "" || live[row.BackupBranch] {
			continue
		}
		exists, err := s.git.BranchExists(ctx, project.Path, row.BackupBranch)
		if err != nil || !exists {
			continue
		}
		if err := s.git.DeleteBranch(ctx, project.Path, row.BackupBranch, true); err != nil {
			s.log.Debug("could not delete a backup branch", "branch", row.BackupBranch, "error", err)
		}
	}
}
