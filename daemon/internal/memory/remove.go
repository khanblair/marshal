package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Removing a project's memory (docs/architecture.md sections 12 and 16.1, docs/backend-checklist.md
// B7.4). The projects service asks for this when a person removes a project and did not ask to keep
// what is remembered about it, through the MemoryRemover seam it already has
// (internal/projects.MemoryRemover).
//
// Both halves of memory go: the folder on disk, which holds the card notes and the knowledge base a
// person may have written, and the rows that index those files. The cards themselves are the
// projects module's to delete, and it deletes them before it asks for this - which takes the notes
// rows with them through the foreign key - so this is written to be right either way round: it
// deletes the rows itself, and deleting none of them is not an error.

// RemoveProjectMemory deletes a project's memory folder from the vault and the rows that index it.
// It is idempotent: a project whose memory is already gone is not an error, which is what lets the
// projects service call it for a removal that was finished by hand.
func (s *Service) RemoveProjectMemory(ctx context.Context, projectID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.deleteProjectNotes(ctx, projectID); err != nil {
		return err
	}
	return s.removeProjectFolder(projectID)
}

// deleteProjectNotes removes a project's note rows. Their indexed text goes with them through the
// triggers of migration 0019, so a search cannot find a note whose project is gone.
func (s *Service) deleteProjectNotes(ctx context.Context, projectID string) error {
	err := s.store.Write(ctx, func(q *db.Queries) error {
		return q.DeleteProjectNotes(ctx, projectID)
	})
	if err != nil {
		return fmt.Errorf("delete the notes of project %s: %w", projectID, err)
	}
	return nil
}

// removeProjectFolder deletes `<vault>/<project>` from disk. The name is checked to be a single
// folder directly inside the vault root before anything is removed, the same guard a note's path
// goes through and for a stronger reason: this deletes a whole tree, and a project id that carried a
// path separator or a `..` would delete something that is not the project's memory.
func (s *Service) removeProjectFolder(projectID string) error {
	if projectID == "" || projectID == "." || projectID == ".." || projectID != filepath.Base(projectID) ||
		strings.ContainsAny(projectID, `/\`) {
		return fmt.Errorf("memory: %q is not a project folder inside the vault", projectID)
	}
	full := filepath.Join(s.root, projectID)
	if inside, err := filepath.Rel(s.root, full); err != nil || inside != projectID {
		return fmt.Errorf("memory: %q is not inside the vault root", projectID)
	}
	if err := os.RemoveAll(full); err != nil {
		return fmt.Errorf("remove the memory folder of project %s: %w", projectID, err)
	}
	return nil
}
