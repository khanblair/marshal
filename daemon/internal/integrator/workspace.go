package integrator

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/keyedlock"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// WorkspaceRoot is the folder every project's Integrator workspace lives under.
func WorkspaceRoot(dataDir string) string { return filepath.Join(dataDir, "integrator") }

// WorkspaceDir is the Integrator's workspace for one project: a worktree on the integrator branch,
// outside the owner's folder.
func WorkspaceDir(dataDir, projectID string) string {
	return filepath.Join(WorkspaceRoot(dataDir), projectID)
}

// WorkspaceDeps are the parts the Git workspace is built from.
type WorkspaceDeps struct {
	Projects Projects
	Git      *gitx.Git
	DataDir  string
}

// GitWorkspace is the Workspace over Git: a persistent worktree on the integrator branch.
//
// It takes its own lock, never the project's merge lock, so the Integrator's chat can ask for the
// workspace while a merge holds the project.
type GitWorkspace struct {
	projects Projects
	git      *gitx.Git
	dataDir  string
	locks    keyedlock.Locks
}

var _ Workspace = (*GitWorkspace)(nil)

// NewWorkspace builds the Git workspace.
func NewWorkspace(deps WorkspaceDeps) (*GitWorkspace, error) {
	if deps.Projects == nil || deps.Git == nil {
		return nil, errors.New("the Integrator workspace needs the projects and Git")
	}
	if !filepath.IsAbs(deps.DataDir) {
		return nil, errors.New("the Integrator workspace needs the data folder as a full path")
	}
	return &GitWorkspace{projects: deps.Projects, git: deps.Git, dataDir: filepath.Clean(deps.DataDir)}, nil
}

// Ensure makes the workspace if it is missing and answers its folder. The integrator branch starts
// at the tip of the project's integration branch. A branch that is already there keeps its commits,
// and a worktree whose folder was deleted is made again.
func (w *GitWorkspace) Ensure(ctx context.Context, projectID string) (string, error) {
	project, err := w.projects.Get(ctx, projectID)
	if err != nil {
		return "", err
	}
	target := project.Target()
	if target == "" {
		return "", errors.New("this project has no integration branch to start the Integrator branch from")
	}
	unlock := w.locks.Lock(projectID)
	defer unlock()
	dir := WorkspaceDir(w.dataDir, projectID)
	err = w.git.EnsureBranchWorktree(ctx, project.Path, dir, WorkspaceRoot(w.dataDir), protocol.IntegrationBranchName, target)
	if err != nil {
		return "", fmt.Errorf("make the Integrator workspace of project %s: %w", projectID, err)
	}
	return dir, nil
}
