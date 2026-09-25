package projects

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// maxDevCommandChars is the longest dev command a project can have.
const maxDevCommandChars = 500

// List returns every project, oldest first, with its badges. The projects and the counts come
// from one read, so they agree with each other.
func (s *Service) List(ctx context.Context) (protocol.ProjectListSnapshot, error) {
	var rows []db.Project
	var needs []db.CountNeedsByProjectRow
	err := s.store.Read(ctx, func(q *db.Queries) error {
		var err error
		if rows, err = q.ListProjects(ctx); err != nil {
			return fmt.Errorf("list projects: %w", err)
		}
		if needs, err = q.CountNeedsByProject(ctx); err != nil {
			return fmt.Errorf("count the cards that need a person: %w", err)
		}
		return nil
	})
	if err != nil {
		return protocol.ProjectListSnapshot{}, err
	}
	needing := make(map[string]int, len(needs))
	for _, n := range needs {
		needing[n.ProjectID] = int(n.Needs)
	}
	projects := make([]protocol.Project, 0, len(rows))
	for _, row := range rows {
		awake, err := s.awake.AwakeCards(ctx, row.ID)
		if err != nil {
			return protocol.ProjectListSnapshot{}, fmt.Errorf("count the awake cards of project %s: %w", row.ID, err)
		}
		project, err := toProject(row, protocol.ProjectBadges{Needs: needing[row.ID], Awake: awake})
		if err != nil {
			return protocol.ProjectListSnapshot{}, err
		}
		projects = append(projects, project)
	}
	return protocol.ProjectListSnapshot{Projects: projects, ServerTime: protocol.NewTimestamp(s.now())}, nil
}

// Get returns one project with its badges.
func (s *Service) Get(ctx context.Context, id string) (protocol.Project, error) {
	row, err := s.store.Queries().GetProject(ctx, id)
	if err != nil {
		return protocol.Project{}, notFound(fmt.Errorf("read project %s: %w", id, err), notFoundProject(id))
	}
	return s.withBadges(ctx, row)
}

// withBadges works out a project's badges from its cards and builds the wire project.
func (s *Service) withBadges(ctx context.Context, row db.Project) (protocol.Project, error) {
	needs, err := s.store.Queries().CountNeedsForProject(ctx, row.ID)
	if err != nil {
		return protocol.Project{}, fmt.Errorf("count the cards that need a person in project %s: %w", row.ID, err)
	}
	awake, err := s.awake.AwakeCards(ctx, row.ID)
	if err != nil {
		return protocol.Project{}, fmt.Errorf("count the awake cards of project %s: %w", row.ID, err)
	}
	return toProject(row, protocol.ProjectBadges{Needs: int(needs), Awake: awake})
}

// Update changes the project fields that are set. A rename changes only the display name: the
// folder and the branches stay as they are. project.updated is published only when something
// really changed.
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (protocol.Project, error) {
	if err := checkUpdate(in); err != nil {
		return protocol.Project{}, err
	}
	row, err := s.store.Queries().GetProject(ctx, id)
	if err != nil {
		return protocol.Project{}, notFound(fmt.Errorf("read project %s: %w", id, err), notFoundProject(id))
	}
	if in.DefaultBranch != nil {
		if err := s.checkBranch(ctx, row.RepoPath, strings.TrimSpace(*in.DefaultBranch)); err != nil {
			return protocol.Project{}, err
		}
	}
	changed := false
	err = s.store.Write(ctx, func(q *db.Queries) error {
		current, err := q.GetProject(ctx, id)
		if err != nil {
			return notFound(fmt.Errorf("read project %s: %w", id, err), notFoundProject(id))
		}
		next := applyUpdate(current, in)
		if next == current {
			return nil
		}
		next.UpdatedAt = s.now().UnixMilli()
		if _, err := q.UpdateProject(ctx, db.UpdateProjectParams{
			Name: next.Name, DevCommand: next.DevCommand, DefaultBranch: next.DefaultBranch,
			BypassLocked: next.BypassLocked, UpdatedAt: next.UpdatedAt, ID: id,
		}); err != nil {
			return fmt.Errorf("update project %s: %w", id, err)
		}
		changed = true
		return nil
	})
	if err != nil {
		return protocol.Project{}, err
	}
	project, err := s.Get(ctx, id)
	if err != nil {
		return protocol.Project{}, err
	}
	if changed {
		s.publish(protocol.HomeTopic, protocol.EventTypeProjectUpdated, protocol.ProjectEventData{Project: project}, false)
	}
	return project, nil
}

// checkUpdate refuses values that are not allowed, before anything is read.
func checkUpdate(in UpdateInput) error {
	if in.Name != nil {
		if err := checkName(*in.Name); err != nil {
			return err
		}
	}
	if in.DevCommand != nil {
		command := strings.TrimSpace(*in.DevCommand)
		if utf8.RuneCountInString(command) > maxDevCommandChars {
			return protocol.InvalidArgument(fmt.Sprintf("The dev command can have at most %d characters.", maxDevCommandChars))
		}
		if strings.ContainsAny(command, "\r\n\x00") {
			return protocol.InvalidArgument("The dev command must be one line.")
		}
	}
	return nil
}

// applyUpdate returns the row with the fields that are set changed. The fields are trimmed.
func applyUpdate(row db.Project, in UpdateInput) db.Project {
	if in.Name != nil {
		row.Name = strings.TrimSpace(*in.Name)
	}
	if in.DevCommand != nil {
		row.DevCommand = strings.TrimSpace(*in.DevCommand)
	}
	if in.DefaultBranch != nil {
		row.DefaultBranch = strings.TrimSpace(*in.DefaultBranch)
	}
	if in.BypassLocked != nil {
		row.BypassLocked = intFromBool(*in.BypassLocked)
	}
	return row
}

// checkBranch refuses a default branch that is not a branch of the repository.
func (s *Service) checkBranch(ctx context.Context, repo, name string) error {
	if !folderExists(repo) {
		return protocol.Unavailable(messageNoRepoFolder)
	}
	if err := s.git.ValidBranchName(ctx, name); err != nil {
		return protocol.InvalidArgument("That is not a usable branch name.").WithCause(err)
	}
	found, err := s.git.BranchExists(ctx, repo, name)
	if err != nil {
		return fmt.Errorf("look for the default branch: %w", err)
	}
	if !found {
		return protocol.InvalidArgument("That branch does not exist in the repository.").With("branch", name)
	}
	return nil
}
