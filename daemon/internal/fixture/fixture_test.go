package fixture_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/goleak"

	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/fixture"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// env is a projects service on a real store, with a data folder to make the fixture in.
type env struct {
	svc     *projects.Service
	dataDir string
	git     *gitx.Git
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, "marshal.db"))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("close the store: %v", err)
		}
	})
	bus, err := events.New()
	if err != nil {
		t.Fatalf("make the bus: %v", err)
	}
	t.Cleanup(bus.Close)
	git := testutil.Git()
	dataDir := filepath.Join(dir, "data")
	svc, err := projects.New(projects.Deps{Store: st, Bus: bus, Git: git, DataDir: dataDir})
	if err != nil {
		t.Fatalf("make the service: %v", err)
	}
	return &env{svc: svc, dataDir: dataDir, git: git}
}

// load runs LoadPrototype against the repository fixtures of this checkout.
func (e *env) load(t *testing.T, opts ...fixture.Option) error {
	t.Helper()
	all := append([]fixture.Option{fixture.WithSourceDir(testutil.TestdataPath(t, "repos"))}, opts...)
	return fixture.LoadPrototype(context.Background(), e.dataDir, e.svc, all...)
}

func (e *env) list(t *testing.T) []protocol.Project {
	t.Helper()
	snap, err := e.svc.List(context.Background())
	if err != nil {
		t.Fatalf("list the projects: %v", err)
	}
	return snap.Projects
}

func (e *env) head(t *testing.T, dir string) string {
	t.Helper()
	head, err := e.git.Run(context.Background(), dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("read HEAD of %s: %v", dir, err)
	}
	return head
}

func sameFolder(t *testing.T, a, b string) bool {
	t.Helper()
	infoA, err := os.Stat(a)
	if err != nil {
		t.Fatalf("stat %s: %v", a, err)
	}
	infoB, err := os.Stat(b)
	if err != nil {
		t.Fatalf("stat %s: %v", b, err)
	}
	return os.SameFile(infoA, infoB)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestLoadPrototypeMakesThreeWorkingProjectsAndIsSafeToRepeat(t *testing.T) {
	e := newEnv(t)
	var logs bytes.Buffer
	log := fixture.WithLogger(slog.New(slog.NewTextHandler(&logs, nil)))
	if err := e.load(t, log); err != nil {
		t.Fatalf("first load: %v", err)
	}
	want := []struct {
		id, name, marker, absent string
		monorepo                 bool
	}{
		{id: "api", name: "api-gateway", marker: filepath.Join("src", "util.js"), absent: "packages"},
		{id: "web", name: "web-dashboard", marker: filepath.Join("src", "util.js"), absent: "packages"},
		{id: "mobile", name: "mobile-app", marker: filepath.Join("packages", "api", "package.json"), absent: "src", monorepo: true},
	}
	got := e.list(t)
	if len(got) != len(want) {
		t.Fatalf("projects = %d, want %d: %+v", len(got), len(want), got)
	}
	heads := make(map[string]string)
	for i, w := range want {
		t.Run(w.id, func(t *testing.T) {
			p := got[i]
			if p.ID != w.id || p.Name != w.name {
				t.Fatalf("project %d = %s (%s), want %s (%s)", i, p.ID, p.Name, w.id, w.name)
			}
			folder := filepath.Join(e.dataDir, "fixtures", w.name)
			if !sameFolder(t, p.Path, folder) {
				t.Errorf("path = %s, want the folder %s", p.Path, folder)
			}
			info, err := e.git.Inspect(context.Background(), folder)
			if err != nil || !info.HasCommits || !info.Clean {
				t.Fatalf("Inspect = %+v, %v; want a clean repository with a commit", info, err)
			}
			if !exists(filepath.Join(folder, ".git")) {
				t.Error("the folder has no .git of its own")
			}
			if !exists(filepath.Join(folder, w.marker)) {
				t.Errorf("the repository is missing %s", w.marker)
			}
			if exists(filepath.Join(folder, w.absent)) {
				t.Errorf("the repository has %s, which belongs to the other fixture", w.absent)
			}
			if p.IsMonorepo != w.monorepo || (len(p.Packages) > 0) != w.monorepo {
				t.Errorf("IsMonorepo = %v with packages %v, want monorepo %v", p.IsMonorepo, p.Packages, w.monorepo)
			}
			heads[w.id] = e.head(t, folder)
		})
	}

	logs.Reset()
	if err := e.load(t, log); err != nil {
		t.Fatalf("second load: %v", err)
	}
	if again := e.list(t); len(again) != len(want) {
		t.Fatalf("after the second load there are %d projects, want %d", len(again), len(want))
	}
	if n := strings.Count(logs.String(), "already exists"); n != len(want) {
		t.Errorf("the second load reported %d existing projects, want %d:\n%s", n, len(want), logs.String())
	}
	if strings.Contains(logs.String(), "project created") {
		t.Errorf("the second load created something:\n%s", logs.String())
	}
	for i, w := range want {
		if head := e.head(t, filepath.Join(e.dataDir, "fixtures", w.name)); head != heads[w.id] {
			t.Errorf("%s: the second load changed HEAD from %s to %s", w.id, heads[w.id], head)
		}
		if got[i].ID != w.id {
			t.Errorf("project %d changed to %s", i, got[i].ID)
		}
	}
}

func TestLoadPrototypeCreatesOnlyWhatIsMissing(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	own := testutil.Fixture(t, "small-repo")
	_, err := e.svc.Create(ctx, protocol.CreateProjectRequest{Source: protocol.ProjectSourceFolder, Path: own, Name: "mine"},
		projects.WithID("api"))
	if err != nil {
		t.Fatalf("create the project the fixture should leave alone: %v", err)
	}
	if err := e.load(t); err != nil {
		t.Fatalf("load: %v", err)
	}
	got := e.list(t)
	if len(got) != 3 {
		t.Fatalf("projects = %d, want 3: %+v", len(got), got)
	}
	api, err := e.svc.Get(ctx, "api")
	if err != nil {
		t.Fatalf("get api: %v", err)
	}
	if api.Name != "mine" || !sameFolder(t, api.Path, own) {
		t.Errorf("api = %s at %s, want the project that was there first", api.Name, api.Path)
	}
	if exists(filepath.Join(e.dataDir, "fixtures", "api-gateway")) {
		t.Error("the fixture made a repository for a project it skipped")
	}
}

func TestLoadPrototypeReusesTheFolderOfARemovedProject(t *testing.T) {
	e := newEnv(t)
	if err := e.load(t); err != nil {
		t.Fatalf("first load: %v", err)
	}
	folder := filepath.Join(e.dataDir, "fixtures", "mobile-app")
	before := e.head(t, folder)
	if err := e.svc.Remove(context.Background(), "mobile", protocol.RemoveProjectRequest{KeepBranches: true, KeepMemory: true}); err != nil {
		t.Fatalf("remove the project: %v", err)
	}
	if err := e.load(t); err != nil {
		t.Fatalf("second load: %v", err)
	}
	if got := e.list(t); len(got) != 3 || got[2].ID != "mobile" {
		t.Fatalf("projects = %+v, want mobile back", got)
	}
	if after := e.head(t, folder); after != before {
		t.Errorf("the folder was made again: HEAD %s, want %s", after, before)
	}
}

func TestLoadPrototypeFindsTheFixtureRepositoriesByItself(t *testing.T) {
	e := newEnv(t)
	if err := fixture.LoadPrototype(context.Background(), e.dataDir, e.svc); err != nil {
		t.Fatalf("load without a source folder: %v", err)
	}
	if got := e.list(t); len(got) != 3 {
		t.Fatalf("projects = %d, want 3", len(got))
	}
}

func TestLoadPrototypeReturnsAnErrorWhenItCannotWork(t *testing.T) {
	tests := []struct {
		name string
		// arrange changes the world so the load must fail, and returns the options to load with.
		arrange func(t *testing.T, e *env) []fixture.Option
		// want is text the error must contain.
		want string
	}{
		{
			name: "the fixtures folder cannot be made",
			arrange: func(t *testing.T, e *env) []fixture.Option {
				makeFile(t, filepath.Join(e.dataDir, "fixtures"))
				return nil
			},
			want: "make the fixtures folder",
		},
		{
			name: "Git is missing",
			arrange: func(t *testing.T, _ *env) []fixture.Option {
				t.Setenv("PATH", t.TempDir())
				return nil
			},
			want: "Git is not installed",
		},
		{
			name: "the source folder does not exist",
			arrange: func(t *testing.T, _ *env) []fixture.Option {
				return []fixture.Option{fixture.WithSourceDir(filepath.Join(t.TempDir(), "nothing-here"))}
			},
			want: fixture.ErrSourceNotFound.Error(),
		},
		{
			name: "the fixture folder is there but is not a repository",
			arrange: func(t *testing.T, e *env) []fixture.Option {
				if err := os.MkdirAll(filepath.Join(e.dataDir, "fixtures", "api-gateway"), 0o750); err != nil {
					t.Fatal(err)
				}
				return nil
			},
			want: "delete it and start again",
		},
		{
			name: "the fixture folder is a repository with no commits",
			arrange: func(t *testing.T, e *env) []fixture.Option {
				dir := filepath.Join(e.dataDir, "fixtures", "api-gateway")
				if err := os.MkdirAll(dir, 0o750); err != nil {
					t.Fatal(err)
				}
				if _, err := e.git.Run(context.Background(), dir, "init", "--quiet"); err != nil {
					t.Fatal(err)
				}
				return nil
			},
			want: "no commits",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			opts := tc.arrange(t, e)
			err := e.load(t, opts...)
			if err == nil {
				t.Fatal("load returned no error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err, tc.want)
			}
			if got := e.list(t); len(got) != 0 {
				t.Errorf("a failed load left %d projects", len(got))
			}
			assertNoStagingFolder(t, e.dataDir)
		})
	}
}

func TestLoadPrototypeDoesNotTreatAnErrorAsAMissingProject(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := fixture.LoadPrototype(ctx, e.dataDir, e.svc, fixture.WithSourceDir(testutil.TestdataPath(t, "repos")))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want the cancel", err)
	}
	if exists(filepath.Join(e.dataDir, "fixtures")) {
		t.Error("the fixture made files although it could not tell whether it needed to")
	}
}

func TestLoadPrototypeChecksItsInputs(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := fixture.LoadPrototype(ctx, "relative/data", e.svc); err == nil {
		t.Error("a relative data folder was accepted")
	}
	if err := fixture.LoadPrototype(ctx, e.dataDir, nil); err == nil {
		t.Error("a missing projects module was accepted")
	}
}

func makeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("in the way"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// assertNoStagingFolder fails if a failed load left a half-made repository behind.
func assertNoStagingFolder(t *testing.T, dataDir string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dataDir, "fixtures"))
	if err != nil {
		return
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".staging-") {
			t.Errorf("a staging folder was left behind: %s", entry.Name())
		}
	}
}
