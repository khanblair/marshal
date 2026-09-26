package projects

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/sample"
)

// The sample project (docs/backend-checklist.md B2.12, docs/backend-inventory.md N24). The sample
// repository ships inside the daemon (internal/sample); asking for a project from it writes the
// repository into <data>/sample/marshal-sample the first time, and then adds that folder exactly as
// a folder is added, so a card starts on it like on any other project.
//
// There is one sample folder. While its project is in Marshal, a second sample is refused with a
// plain sentence and the project's id, so the app can open the one there is. Removing the project
// never deletes the folder, so asking again after that adds the same folder back, with whatever was
// done in it.

const (
	messageSampleAdded = "The sample project is already in Marshal."
	// messageSampleNotUsable names the folder, not its path: messages never hold a path.
	messageSampleNotUsable = "Marshal's sample folder is there but is not a repository it can use. " +
		"Move the folder named sample out of Marshal's data folder, then try again."
)

// sampleRepository makes the sample repository when it is not there yet, and returns what Git says
// about it.
func (s *Service) sampleRepository(ctx context.Context) (gitx.RepoInfo, error) {
	if err := s.checkSampleFree(ctx); err != nil {
		return gitx.RepoInfo{}, err
	}
	root, err := sample.Ensure(ctx, s.dataDir)
	if errors.Is(err, sample.ErrNotUsable) {
		return gitx.RepoInfo{}, protocol.Conflict(messageSampleNotUsable).WithCause(err)
	}
	if err != nil {
		return gitx.RepoInfo{}, fmt.Errorf("make the sample repository: %w", err)
	}
	return s.inspectFolder(ctx, root)
}

// checkSampleFree refuses a sample while a project already uses the sample folder. The stored path
// is the one Git gave, with links followed, so the file system compares the two.
func (s *Service) checkSampleFree(ctx context.Context) error {
	folder := sample.Folder(s.dataDir)
	rows, err := s.store.Queries().ListProjects(ctx)
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}
	for _, row := range rows {
		if row.RepoPath == folder || sameFolder(row.RepoPath, folder) {
			return protocol.Conflict(messageSampleAdded).With("projectId", row.ID)
		}
	}
	return nil
}
