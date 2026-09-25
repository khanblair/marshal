package projects

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

const (
	// maxNameChars is the longest display name a project can have.
	maxNameChars = 100
	// maxIDAttempts bounds the search for a free project id: api, api-2, api-3, and so on.
	maxIDAttempts = 1000
)

// CreateOption changes one call to Create.
type CreateOption func(*createConfig)

type createConfig struct {
	id string
}

// WithID gives the new project this id instead of one made from its name. Fixtures and tests use
// it so the seed projects can be "api", "web", and "mobile". The id must be a valid project id
// and must not be taken: it is never renumbered.
func WithID(id string) CreateOption {
	return func(c *createConfig) { c.id = id }
}

// Create adds a repository as a project. For a folder, the path must be the top folder of a Git
// working tree. For a clone, the repository is copied into Dest first and then added like a
// folder. The language, packages, and dev command are detected and stored, the board is made with
// the default columns in the same transaction, and project.created is published after the commit.
func (s *Service) Create(ctx context.Context, in CreateInput, opts ...CreateOption) (protocol.Project, error) {
	var cfg createConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	if err := checkCreate(in, cfg); err != nil {
		return protocol.Project{}, err
	}
	if err := s.git.CheckVersion(ctx); err != nil {
		return protocol.Project{}, protocol.Unsupported(messageNeedsGit).WithCause(err)
	}
	info, err := s.openRepository(ctx, in)
	if err != nil {
		return protocol.Project{}, err
	}
	if err := s.checkNotRegistered(ctx, info.Root); err != nil {
		return protocol.Project{}, err
	}
	found, err := Detect(info.Root)
	if err != nil {
		return protocol.Project{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = filepath.Base(info.Root)
	}
	project, err := s.insertProject(ctx, newProject{name: name, info: info, found: found, id: cfg.id})
	if err != nil {
		return protocol.Project{}, err
	}
	s.log.Info("added a project", "project_id", project.ID, "language", project.Language,
		"monorepo", project.IsMonorepo)
	s.publish(protocol.HomeTopic, protocol.EventTypeProjectCreated, protocol.ProjectEventData{Project: project}, true)
	return project, nil
}

// checkCreate refuses a request that cannot work before anything is asked of Git or the disk.
func checkCreate(in CreateInput, cfg createConfig) error {
	if !in.Source.Valid() {
		return protocol.InvalidArgument("Choose whether to add a folder or clone a repository.")
	}
	if strings.TrimSpace(in.Name) != "" {
		if err := checkName(in.Name); err != nil {
			return err
		}
	}
	if cfg.id != "" && !protocol.ValidProjectID(cfg.id) {
		return protocol.InvalidArgument("That project id is not allowed. Use lower case letters, digits, and hyphens.")
	}
	if in.Source == protocol.ProjectSourceFolder && strings.TrimSpace(in.Path) == "" {
		return protocol.InvalidArgument("Choose the folder of the repository.")
	}
	if in.Source == protocol.ProjectSourceClone && (strings.TrimSpace(in.URL) == "" || strings.TrimSpace(in.Dest) == "") {
		return protocol.InvalidArgument("Enter the address to clone from and the folder to clone into.")
	}
	return nil
}

// checkName refuses a name that is empty or too long.
func checkName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return protocol.InvalidArgument(messageEmptyName)
	}
	if utf8.RuneCountInString(name) > maxNameChars {
		return protocol.InvalidArgument(fmt.Sprintf("Project names can have at most %d characters.", maxNameChars))
	}
	return nil
}

// openRepository finds the repository the request names, cloning it first when asked, and
// returns what Git says about it.
func (s *Service) openRepository(ctx context.Context, in CreateInput) (gitx.RepoInfo, error) {
	if in.Source == protocol.ProjectSourceFolder {
		return s.inspectFolder(ctx, in.Path)
	}
	dest, err := expandHome(strings.TrimSpace(in.Dest))
	if err != nil {
		return gitx.RepoInfo{}, err
	}
	if !filepath.IsAbs(dest) {
		return gitx.RepoInfo{}, protocol.InvalidArgument(messageRelativePath)
	}
	if err := s.checkCloneDest(ctx, dest); err != nil {
		return gitx.RepoInfo{}, err
	}
	clone := gitx.CloneOptions{Branch: strings.TrimSpace(in.Branch), AllowLocal: s.allowLocalClone}
	if err := s.git.Clone(ctx, strings.TrimSpace(in.URL), dest, clone); err != nil {
		return gitx.RepoInfo{}, cloneError(err)
	}
	return s.inspectFolder(ctx, dest)
}

// cloneError turns what Git's clone reports into an answer for a person. The address, and the
// folder it was cloned into, stay out of the message: they are in the log through the cause.
func cloneError(err error) error {
	switch {
	case errors.Is(err, gitx.ErrBadURL):
		return protocol.InvalidArgument("Marshal cannot clone from that address. Use an https:// or ssh address.").WithCause(err)
	case errors.Is(err, gitx.ErrBadBranchName):
		return protocol.InvalidArgument("That is not a usable branch name.").WithCause(err)
	case errors.Is(err, gitx.ErrBadPath):
		return protocol.InvalidArgument("Choose a new or empty folder to clone into.").WithCause(err)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("clone the repository: %w", err)
	}
	return protocol.Unavailable("Marshal could not clone that repository. Check the address and your connection, then try again.").
		WithCause(err)
}

// checkCloneDest refuses a clone folder that is inside a project Marshal already manages, or is
// one. A clone there would put one repository inside another.
func (s *Service) checkCloneDest(ctx context.Context, dest string) error {
	rows, err := s.store.Queries().ListProjects(ctx)
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}
	target := resolveExisting(dest)
	for _, row := range rows {
		if within(resolveExisting(row.RepoPath), target) {
			return protocol.InvalidArgument(messageDestInsideAnother).With("projectId", row.ID)
		}
	}
	return nil
}

// inspectFolder checks that path is the top folder of a Git working tree and reads it.
func (s *Service) inspectFolder(ctx context.Context, path string) (gitx.RepoInfo, error) {
	path, err := expandHome(strings.TrimSpace(path))
	if err != nil {
		return gitx.RepoInfo{}, err
	}
	if !filepath.IsAbs(path) {
		return gitx.RepoInfo{}, protocol.InvalidArgument(messageRelativePath)
	}
	stat, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return gitx.RepoInfo{}, protocol.InvalidArgument(messageNoFolder)
	case err != nil:
		return gitx.RepoInfo{}, fmt.Errorf("look at the folder: %w", err)
	case !stat.IsDir():
		return gitx.RepoInfo{}, protocol.InvalidArgument(messageNotAFolder)
	}
	info, err := s.git.Inspect(ctx, path)
	if errors.Is(err, gitx.ErrNotARepo) {
		return gitx.RepoInfo{}, protocol.InvalidArgument(messageNotARepo).WithCause(err)
	}
	if err != nil {
		return gitx.RepoInfo{}, fmt.Errorf("inspect the repository: %w", err)
	}
	return info, nil
}

// checkNotRegistered refuses a repository that is already a project. Two spellings of one folder
// (a link, a different case on a case-insensitive disk) are the same repository, so the file
// system compares them, not the text. The database repeats the exact-text check inside the
// write, and its unique index backs that up.
func (s *Service) checkNotRegistered(ctx context.Context, root string) error {
	rows, err := s.store.Queries().ListProjects(ctx)
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}
	for _, row := range rows {
		if row.RepoPath == root || sameFolder(row.RepoPath, root) {
			return alreadyAdded(row.ID)
		}
	}
	return nil
}

func alreadyAdded(projectID string) *protocol.Error {
	return protocol.Conflict(messageAlreadyAdded).With("projectId", projectID)
}

// newProject is what insertProject needs to make a project row.
type newProject struct {
	name  string
	info  gitx.RepoInfo
	found Detection
	// id is set only by WithID.
	id string
}

// insertProject writes the project and its board in one transaction. It picks the id inside the
// transaction, so two projects added at the same moment can never take the same one.
func (s *Service) insertProject(ctx context.Context, in newProject) (protocol.Project, error) {
	boardID, err := s.newID()
	if err != nil {
		return protocol.Project{}, fmt.Errorf("make a board id: %w", err)
	}
	columns, err := json.Marshal(DefaultColumns())
	if err != nil {
		return protocol.Project{}, fmt.Errorf("encode the board columns: %w", err)
	}
	packages, err := json.Marshal(in.found.Packages)
	if err != nil {
		return protocol.Project{}, fmt.Errorf("encode the packages: %w", err)
	}
	now := s.now()
	row := db.Project{
		Name: in.name, RepoPath: in.info.Root, DefaultBranch: in.info.DefaultBranch,
		Language: in.found.Language, DevCommand: in.found.DevCommand,
		IsMonorepo: intFromBool(in.found.IsMonorepo), PackagesJSON: string(packages),
		NextCardNumber: 1, CreatedAt: now.UnixMilli(), UpdatedAt: now.UnixMilli(),
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		var err error
		if row.ID, err = pickID(ctx, q, in.name, in.id); err != nil {
			return err
		}
		if err := writeProject(ctx, q, row, newBoard{ID: boardID, Columns: string(columns)}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return protocol.Project{}, fmt.Errorf("add the project: %w", err)
	}
	return toProject(row, protocol.ProjectBadges{})
}

// newBoard is the board a new project gets, bundled so writeProject stays inside the parameter
// limit.
type newBoard struct {
	ID      string
	Columns string
}

// writeProject inserts the project row and its board. The path check is repeated here, inside the
// transaction, because the writer is one connection: nothing can slip in between the check and
// the insert.
func writeProject(ctx context.Context, q *db.Queries, row db.Project, board newBoard) error {
	if existing, err := q.GetProjectByRepoPath(ctx, row.RepoPath); err == nil {
		return alreadyAdded(existing.ID)
	} else if !store.IsNotFound(err) {
		return fmt.Errorf("look for the folder: %w", err)
	}
	err := q.CreateProject(ctx, db.CreateProjectParams{
		ID: row.ID, Name: row.Name, RepoPath: row.RepoPath, DefaultBranch: row.DefaultBranch,
		Language: row.Language, DevCommand: row.DevCommand, BypassLocked: row.BypassLocked,
		IsMonorepo: row.IsMonorepo, PackagesJSON: row.PackagesJSON,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	})
	if err != nil {
		return fmt.Errorf("insert the project: %w", err)
	}
	if err := q.CreateBoard(ctx, db.CreateBoardParams{ID: board.ID, ProjectID: row.ID, ColumnsJSON: board.Columns}); err != nil {
		return fmt.Errorf("insert the board: %w", err)
	}
	return nil
}

// pickID chooses the id of a new project: the one asked for, which must be free, or one made from
// the name, numbered until it is free.
func pickID(ctx context.Context, q *db.Queries, name, explicit string) (string, error) {
	if explicit != "" {
		taken, err := q.ProjectIDExists(ctx, explicit)
		if err != nil {
			return "", fmt.Errorf("look for the project id: %w", err)
		}
		if taken {
			return "", protocol.Conflict("A project with that id already exists.").With("id", explicit)
		}
		return explicit, nil
	}
	base := protocol.SlugFromName(name)
	for n := 1; n <= maxIDAttempts; n++ {
		id := protocol.NumberedProjectID(base, n)
		taken, err := q.ProjectIDExists(ctx, id)
		if err != nil {
			return "", fmt.Errorf("look for the project id: %w", err)
		}
		if !taken {
			return id, nil
		}
	}
	return "", fmt.Errorf("find a free project id for %q: %d ids are taken", base, maxIDAttempts)
}
