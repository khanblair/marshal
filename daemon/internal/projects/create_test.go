package projects_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

func TestCreateFromAFolder(t *testing.T) {
	e := newEnv(t)
	path := testutil.Fixture(t, "small-repo")
	project, err := e.svc.Create(context.Background(), protocol.CreateProjectRequest{
		Source: protocol.ProjectSourceFolder, Path: path,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := protocol.Project{
		ID: "small-repo", Name: "small-repo", Language: projects.LanguageJavaScript, DefaultBranch: "main",
		Packages: []string{}, CreatedAt: project.CreatedAt,
	}
	if project.Path == "" || !sameFolder(t, project.Path, path) {
		t.Errorf("path = %q, want the folder %q", project.Path, path)
	}
	project.Path = ""
	if !equalProjects(project, want) {
		t.Errorf("project = %+v, want %+v", project, want)
	}
	stored, err := e.svc.Get(context.Background(), project.ID)
	if err != nil || stored.Name != "small-repo" {
		t.Errorf("Get = %+v, %v", stored, err)
	}
	board, err := e.svc.Board(context.Background(), project.ID)
	if err != nil || len(board.Columns) != len(projects.DefaultColumns()) || len(board.Cards) != 0 {
		t.Errorf("Board = %+v, %v; want the default columns and no cards", board, err)
	}
}

func equalProjects(a, b protocol.Project) bool {
	return a.ID == b.ID && a.Name == b.Name && a.Language == b.Language && a.DefaultBranch == b.DefaultBranch &&
		a.DevCommand == b.DevCommand && a.BypassLocked == b.BypassLocked && a.IsMonorepo == b.IsMonorepo &&
		strings.Join(a.Packages, ",") == strings.Join(b.Packages, ",") && a.Badges == b.Badges &&
		a.Path == b.Path && a.CreatedAt == b.CreatedAt
}

func TestCreateUsesTheNameAndTrimsIt(t *testing.T) {
	e := newEnv(t)
	project, err := e.svc.Create(context.Background(), protocol.CreateProjectRequest{
		Source: protocol.ProjectSourceFolder, Path: testutil.Fixture(t, "small-repo"), Name: "  Web Dashboard  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if project.Name != "Web Dashboard" || project.ID != "web-dashboard" {
		t.Errorf("name %q, id %q; want %q, %q", project.Name, project.ID, "Web Dashboard", "web-dashboard")
	}
}

func TestCreateFromAMonorepoFindsItsPackages(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "monorepo")
	want := []string{"packages/api", "packages/shared", "packages/web"}
	if project.Language != projects.LanguageMonorepo || !project.IsMonorepo || strings.Join(project.Packages, ",") != strings.Join(want, ",") {
		t.Errorf("project = %+v, want a monorepo with %v", project, want)
	}
	list, err := e.svc.List(context.Background())
	if err != nil || len(list.Projects) != 1 || len(list.Projects[0].Packages) != 3 {
		t.Errorf("List = %+v, %v; the packages must survive a round trip through the store", list, err)
	}
}

func TestCreateFromAFolderWithATildePath(t *testing.T) {
	e := newEnv(t)
	repo := testutil.Fixture(t, "small-repo")
	t.Setenv("HOME", filepath.Dir(repo))
	t.Setenv("USERPROFILE", filepath.Dir(repo))
	project, err := e.svc.Create(context.Background(), protocol.CreateProjectRequest{
		Source: protocol.ProjectSourceFolder, Path: "~/" + filepath.Base(repo),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sameFolder(t, project.Path, repo) {
		t.Errorf("path = %q, want %q", project.Path, repo)
	}
}

func TestCreateFromAClone(t *testing.T) {
	e := newEnv(t)
	source := testutil.Fixture(t, "small-repo")
	if _, err := e.git.Run(context.Background(), source, "branch", "develop"); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "cloned-app")
	project, err := e.svc.Create(context.Background(), protocol.CreateProjectRequest{
		Source: protocol.ProjectSourceClone, URL: source, Dest: dest, Branch: "develop",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sameFolder(t, project.Path, dest) || project.Name != "cloned-app" || project.ID != "cloned-app" {
		t.Errorf("project = %+v, want the clone at %s named cloned-app", project, dest)
	}
	branch, err := e.git.Run(context.Background(), dest, "branch", "--show-current")
	if err != nil || branch != "develop" {
		t.Errorf("the clone is on %q, %v; want develop", branch, err)
	}
	if project.Language != projects.LanguageJavaScript {
		t.Errorf("language = %q, want a clone to be detected like a folder", project.Language)
	}
}

func TestACloneCannotGoInsideAProject(t *testing.T) {
	e := newEnv(t)
	first := e.folder(t, "small-repo")
	dest := filepath.Join(first.Path, "inside")
	tests := []struct {
		name string
		dest string
	}{
		{"inside a project", dest},
		{"the project folder itself", first.Path},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.svc.Create(context.Background(), protocol.CreateProjectRequest{
				Source: protocol.ProjectSourceClone, URL: testutil.Fixture(t, "small-repo"), Dest: tc.dest,
			})
			perr := wantCode(t, err, protocol.ErrorCodeInvalidArgument)
			if !strings.Contains(perr.Message, "cannot be inside a project") {
				t.Errorf("message = %q", perr.Message)
			}
			if _, statErr := os.Stat(dest); statErr == nil {
				t.Error("the clone was made anyway")
			}
		})
	}
}

func TestCloneRefusesWhatIsNotAllowed(t *testing.T) {
	source := testutil.Fixture(t, "small-repo")
	full := filepath.Join(t.TempDir(), "dest")
	notEmpty := t.TempDir()
	if err := os.WriteFile(filepath.Join(notEmpty, "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		opts []projects.Option
		in   protocol.CreateProjectRequest
	}{
		{"a folder as the address without the dev option", []projects.Option{projects.WithLocalClones(false)},
			protocol.CreateProjectRequest{Source: protocol.ProjectSourceClone, URL: source, Dest: full}},
		{"an address that is not one", nil,
			protocol.CreateProjectRequest{Source: protocol.ProjectSourceClone, URL: "ext::sh -c id", Dest: full}},
		{"a folder with files in it", nil,
			protocol.CreateProjectRequest{Source: protocol.ProjectSourceClone, URL: source, Dest: notEmpty}},
		{"a relative folder", nil,
			protocol.CreateProjectRequest{Source: protocol.ProjectSourceClone, URL: source, Dest: "relative/dest"}},
		{"no address", nil, protocol.CreateProjectRequest{Source: protocol.ProjectSourceClone, Dest: full}},
		{"no folder", nil, protocol.CreateProjectRequest{Source: protocol.ProjectSourceClone, URL: source}},
		{"a branch that is not a name", nil,
			protocol.CreateProjectRequest{Source: protocol.ProjectSourceClone, URL: source, Dest: full, Branch: "-x"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, tc.opts...)
			_, err := e.svc.Create(context.Background(), tc.in)
			_ = wantCode(t, err, protocol.ErrorCodeInvalidArgument)
			if _, statErr := os.Stat(full); statErr == nil {
				t.Error("a folder was made for a refused clone")
			}
			e.noEvent(t)
		})
	}
}

func TestCloneOfAMissingRepositoryIsUnavailableAndLeavesNothing(t *testing.T) {
	e := newEnv(t)
	dest := filepath.Join(t.TempDir(), "dest")
	_, err := e.svc.Create(context.Background(), protocol.CreateProjectRequest{
		Source: protocol.ProjectSourceClone, URL: filepath.Join(t.TempDir(), "no-such-repo"), Dest: dest,
	})
	perr := wantCode(t, err, protocol.ErrorCodeUnavailable)
	if strings.Contains(perr.Message, dest) {
		t.Errorf("the message %q names the folder", perr.Message)
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Error("a failed clone left its folder behind")
	}
}

func TestCreateRefusesTheSameFolderTwice(t *testing.T) {
	e := newEnv(t)
	first := e.folder(t, "small-repo")
	e.drainEvents()
	spellings := map[string]string{
		"the same text":      first.Path,
		"a trailing slash":   first.Path + string(filepath.Separator),
		"an unclean segment": filepath.Join(first.Path, "src", ".."),
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(first.Path, link); err == nil {
		spellings["a link to it"] = link
	}
	for name, path := range spellings {
		t.Run(name, func(t *testing.T) {
			_, err := e.svc.Create(context.Background(), protocol.CreateProjectRequest{
				Source: protocol.ProjectSourceFolder, Path: path,
			})
			perr := wantCode(t, err, protocol.ErrorCodeConflict)
			if perr.Details["projectId"] != first.ID {
				t.Errorf("details = %v, want the id of the project that has the folder", perr.Details)
			}
		})
	}
	e.noEvent(t)
}

func TestCreateAtTheSameTimeAddsOneProject(t *testing.T) {
	e := newEnv(t)
	path := testutil.Fixture(t, "small-repo")
	const attempts = 6
	errs := make([]error, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = e.svc.Create(context.Background(), protocol.CreateProjectRequest{
				Source: protocol.ProjectSourceFolder, Path: path,
			})
		}()
	}
	wg.Wait()
	added := 0
	for _, err := range errs {
		if err == nil {
			added++
			continue
		}
		_ = wantCode(t, err, protocol.ErrorCodeConflict)
	}
	list, err := e.svc.List(context.Background())
	if err != nil || added != 1 || len(list.Projects) != 1 {
		t.Errorf("added %d, listed %d (%v); want exactly one project", added, len(list.Projects), err)
	}
}

func TestProjectIDsAreUnique(t *testing.T) {
	e := newEnv(t)
	names := []string{"api", "api", "API", "Web Dashboard!", "3d viewer", "a-very-long-project-name-that-goes-past-the-limit"}
	wantIDs := []string{"api", "api-2", "api-3", "web-dashboard", "project", "a-very-long-project-name"}
	for i, name := range names {
		project, err := e.svc.Create(context.Background(), protocol.CreateProjectRequest{
			Source: protocol.ProjectSourceFolder, Path: testutil.Fixture(t, "small-repo"), Name: name,
		})
		if err != nil {
			t.Fatalf("create %q: %v", name, err)
		}
		if project.ID != wantIDs[i] {
			t.Errorf("name %q got id %q, want %q", name, project.ID, wantIDs[i])
		}
		if !protocol.ValidProjectID(project.ID) {
			t.Errorf("id %q is not a valid project id", project.ID)
		}
	}
}

func TestALongIDStillHasRoomForItsNumber(t *testing.T) {
	e := newEnv(t)
	name := strings.Repeat("a", 30)
	first := e.folder(t, "small-repo", projects.WithID(strings.Repeat("a", 24)))
	second, err := e.svc.Create(context.Background(), protocol.CreateProjectRequest{
		Source: protocol.ProjectSourceFolder, Path: testutil.Fixture(t, "small-repo"), Name: name,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || !protocol.ValidProjectID(second.ID) || len(second.ID) > 24 {
		t.Errorf("ids %q and %q: the second must be valid, at most 24 characters, and different", first.ID, second.ID)
	}
}

func TestCreateWithAnExplicitID(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo", projects.WithID("api"))
	if project.ID != "api" || project.Name != "small-repo" {
		t.Errorf("project = %+v, want id api and the folder's name", project)
	}
	_, err := e.svc.Create(context.Background(), protocol.CreateProjectRequest{
		Source: protocol.ProjectSourceFolder, Path: testutil.Fixture(t, "small-repo"),
	}, projects.WithID("api"))
	_ = wantCode(t, err, protocol.ErrorCodeConflict)
	for _, bad := range []string{"A", "1abc", "has space", strings.Repeat("a", 25)} {
		_, err := e.svc.Create(context.Background(), protocol.CreateProjectRequest{
			Source: protocol.ProjectSourceFolder, Path: testutil.Fixture(t, "small-repo"),
		}, projects.WithID(bad))
		_ = wantCode(t, err, protocol.ErrorCodeInvalidArgument)
	}
}

func TestCreateRefusesFoldersThatAreNotRepositories(t *testing.T) {
	e := newEnv(t)
	repo := testutil.Fixture(t, "small-repo")
	plain := t.TempDir()
	file := filepath.Join(plain, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		in      protocol.CreateProjectRequest
		code    protocol.ErrorCode
		message string
	}{
		{"a folder that is not a repository", folderRequest(plain), protocol.ErrorCodeInvalidArgument,
			"That folder is not a Git repository. Choose the top folder of a repository."},
		{"a subfolder of a repository", folderRequest(filepath.Join(repo, "src")), protocol.ErrorCodeInvalidArgument,
			"That folder is not a Git repository. Choose the top folder of a repository."},
		{"a folder that does not exist", folderRequest(filepath.Join(plain, "missing")), protocol.ErrorCodeInvalidArgument,
			"That folder does not exist. Check the path and try again."},
		{"a file", folderRequest(file), protocol.ErrorCodeInvalidArgument,
			"That is not a folder. Choose the top folder of a repository."},
		{"a relative path", folderRequest("code/app"), protocol.ErrorCodeInvalidArgument,
			"Enter the full path of the folder, starting from the top of the disk or with ~."},
		{"no path", folderRequest("  "), protocol.ErrorCodeInvalidArgument, "Choose the folder of the repository."},
		{"no source", protocol.CreateProjectRequest{Path: repo}, protocol.ErrorCodeInvalidArgument,
			"Choose whether to add a folder or clone a repository."},
		{"an unknown source", protocol.CreateProjectRequest{Source: "github", Path: repo}, protocol.ErrorCodeInvalidArgument,
			"Choose whether to add a folder or clone a repository."},
		{"a name that is too long", protocol.CreateProjectRequest{Source: protocol.ProjectSourceFolder, Path: repo, Name: strings.Repeat("n", 101)},
			protocol.ErrorCodeInvalidArgument, "Project names can have at most 100 characters."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.svc.Create(context.Background(), tc.in)
			perr := wantCode(t, err, tc.code)
			if perr.Message != tc.message {
				t.Errorf("message = %q, want %q", perr.Message, tc.message)
			}
		})
	}
	list, err := e.svc.List(context.Background())
	if err != nil || len(list.Projects) != 0 {
		t.Errorf("List = %+v, %v; a refused create must add nothing", list, err)
	}
	e.noEvent(t)
}

func folderRequest(path string) protocol.CreateProjectRequest {
	return protocol.CreateProjectRequest{Source: protocol.ProjectSourceFolder, Path: path}
}

func TestCreateAcceptsARepositoryWithoutCommits(t *testing.T) {
	e := newEnv(t)
	dir := filepath.Join(t.TempDir(), "fresh")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := e.git.Run(context.Background(), dir, "init", "--quiet", "--initial-branch=main"); err != nil {
		t.Fatal(err)
	}
	project, err := e.svc.Create(context.Background(), folderRequest(dir))
	if err != nil || project.DefaultBranch != "main" || project.Language != projects.LanguageUnknown {
		t.Errorf("Create = %+v, %v; want a project on main with an unknown language", project, err)
	}
}
