package projects_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// calls records the order in which the fakes are called.
type calls struct {
	mu   sync.Mutex
	list []string
}

func (c *calls) add(call string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.list = append(c.list, call)
}

func (c *calls) get() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.list)
}

type fakeSessions struct {
	log *calls
	err error
}

func (f fakeSessions) StopProjectSessions(_ context.Context, projectID string) error {
	f.log.add("stop sessions of " + projectID)
	return f.err
}

type fakeMemory struct {
	log *calls
	err error
}

func (f fakeMemory) RemoveProjectMemory(_ context.Context, projectID string) error {
	f.log.add("remove memory of " + projectID)
	return f.err
}

// hashTree returns a digest of every file in a folder, its name, its mode, and its content,
// leaving out the .git folder.
func hashTree(t *testing.T, root string) map[string]string {
	t.Helper()
	sums := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == ".git" {
			return filepath.SkipDir
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		digest := sha256.New()
		digest.Write([]byte(info.Mode().String()))
		if entry.Type().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			digest.Write(data)
		}
		sums[rel] = hex.EncodeToString(digest.Sum(nil))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return sums
}

// startCard gives a card a branch and a worktree, made with Git the way the card starter will.
func (e *env) startCard(t *testing.T, project protocol.Project, card protocol.Card) (path, branch string) {
	t.Helper()
	ctx := context.Background()
	branch = gitx.CardBranchName(project.ID, card.Number, card.Title)
	path = filepath.Join(projects.WorktreesDir(e.dataDir, project.ID), card.ID)
	spec := gitx.WorktreeSpec{Path: path, Branch: branch, Base: project.DefaultBranch}
	if err := e.git.AddWorktree(ctx, project.Path, spec); err != nil {
		t.Fatalf("add a worktree: %v", err)
	}
	got, err := e.svc.SetWorktree(ctx, card.ID, path, branch)
	if err != nil || got.Branch != branch {
		t.Fatalf("SetWorktree = %+v, %v", got, err)
	}
	return path, branch
}

func (e *env) branchExists(t *testing.T, project protocol.Project, branch string) bool {
	t.Helper()
	found, err := e.git.BranchExists(context.Background(), project.Path, branch)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// assertGone checks that the project and everything of it is out of the database.
func (e *env) assertGone(t *testing.T, project protocol.Project, cards ...protocol.Card) {
	t.Helper()
	ctx := context.Background()
	_, err := e.svc.Get(ctx, project.ID)
	_ = wantCode(t, err, protocol.ErrorCodeNotFound)
	_, err = e.svc.Board(ctx, project.ID)
	_ = wantCode(t, err, protocol.ErrorCodeNotFound)
	for _, card := range cards {
		_, err = e.svc.Card(ctx, card.ID)
		_ = wantCode(t, err, protocol.ErrorCodeNotFound)
	}
	list, err := e.svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, listed := range list.Projects {
		if listed.ID == project.ID {
			t.Errorf("the project is still listed")
		}
	}
}

func TestRemoveCleansUpAndDeletesBranches(t *testing.T) {
	log := &calls{}
	e := newEnv(t, projects.WithSessionStopper(fakeSessions{log: log}), projects.WithMemoryRemover(fakeMemory{log: log}))
	project := e.folder(t, "small-repo")
	first, second := e.card(t, project.ID, "First card"), e.card(t, project.ID, "Second card")
	firstPath, firstBranch := e.startCard(t, project, first)
	secondPath, secondBranch := e.startCard(t, project, second)
	e.drainEvents()

	if err := e.svc.Remove(context.Background(), project.ID, protocol.RemoveProjectRequest{}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{firstPath, secondPath, projects.WorktreesDir(e.dataDir, project.ID)} {
		if exists(path) {
			t.Errorf("%s is still there", path)
		}
	}
	trees, err := e.git.ListWorktrees(context.Background(), project.Path)
	if err != nil || len(trees) != 1 {
		t.Errorf("worktrees = %+v, %v; want only the main folder", trees, err)
	}
	for _, branch := range []string{firstBranch, secondBranch} {
		if e.branchExists(t, project, branch) {
			t.Errorf("branch %s was kept", branch)
		}
	}
	if !e.branchExists(t, project, "main") {
		t.Error("the user's own branch was deleted")
	}
	e.assertGone(t, project, first, second)
	if got, want := log.get(), []string{"stop sessions of " + project.ID, "remove memory of " + project.ID}; !slices.Equal(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
	ev := e.nextType(t, protocol.EventTypeProjectRemoved, protocol.HomeTopic)
	if data, ok := ev.Data.(protocol.ProjectRemovedEventData); !ok || data.ProjectID != project.ID || !ev.Critical {
		t.Errorf("event = %+v (critical %v)", ev.Data, ev.Critical)
	}
	e.noEvent(t)
}

func TestRemoveKeepsBranchesAndMemoryWhenAsked(t *testing.T) {
	log := &calls{}
	e := newEnv(t, projects.WithSessionStopper(fakeSessions{log: log}), projects.WithMemoryRemover(fakeMemory{log: log}))
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Keep my work")
	path, branch := e.startCard(t, project, card)
	if err := os.WriteFile(filepath.Join(path, "work.txt"), []byte("committed work"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "work.txt"}, {"-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "Work"}} {
		if _, err := e.git.Run(context.Background(), path, args...); err != nil {
			t.Fatal(err)
		}
	}

	err := e.svc.Remove(context.Background(), project.ID, protocol.RemoveProjectRequest{KeepBranches: true, KeepMemory: true})
	if err != nil {
		t.Fatal(err)
	}
	if exists(path) {
		t.Error("the worktree folder was kept; only the branch is kept")
	}
	if !e.branchExists(t, project, branch) {
		t.Error("the branch with unmerged work was deleted")
	}
	if got, want := log.get(), []string{"stop sessions of " + project.ID}; !slices.Equal(got, want) {
		t.Errorf("calls = %v; the memory must be kept when asked", got)
	}
	e.assertGone(t, project, card)
}

// The repository folder must come out of the whole life of a project exactly as it went in.
func TestRemoveNeverTouchesTheRepositoryFolder(t *testing.T) {
	e := newEnv(t)
	path := testutil.Fixture(t, "monorepo")
	before := hashTree(t, path)
	mainStatus, err := e.git.Run(context.Background(), path, "status", "--porcelain")
	if err != nil {
		t.Fatal(err)
	}

	project, err := e.svc.Create(context.Background(), folderRequest(path))
	if err != nil {
		t.Fatal(err)
	}
	card := e.card(t, project.ID, "Touch nothing")
	worktree, _ := e.startCard(t, project, card)
	if err := os.WriteFile(filepath.Join(worktree, "packages", "api", "new.js"), []byte("in the worktree only"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Update(context.Background(), project.ID, protocol.UpdateProjectRequest{Name: str("Renamed")}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Remove(context.Background(), project.ID, protocol.RemoveProjectRequest{}); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		t.Fatalf("the repository folder is gone: %v", err)
	}
	after := hashTree(t, path)
	if len(after) != len(before) {
		t.Errorf("the folder has %d entries, it had %d", len(after), len(before))
	}
	for name, sum := range before {
		if after[name] != sum {
			t.Errorf("%s changed", name)
		}
	}
	names := make([]string, 0, len(after))
	for name := range after {
		if _, ok := before[name]; !ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) > 0 {
		t.Errorf("new files appeared: %v", names)
	}
	if status, _ := e.git.Run(context.Background(), path, "status", "--porcelain"); status != mainStatus {
		t.Errorf("git status = %q, want %q", status, mainStatus)
	}
}

func TestRemoveDoesNotHoldBackOnUncommittedWork(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Dirty")
	path, _ := e.startCard(t, project, card)
	if err := os.WriteFile(filepath.Join(path, "scratch.txt"), []byte("not committed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Remove(context.Background(), project.ID, protocol.RemoveProjectRequest{KeepBranches: true}); err != nil {
		t.Fatal(err)
	}
	if exists(path) {
		t.Error("the dirty worktree was kept")
	}
	e.assertGone(t, project, card)
}

func TestRemoveCleansUpAWorktreeNoCardRecorded(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	orphan := filepath.Join(projects.WorktreesDir(e.dataDir, project.ID), "orphan")
	spec := gitx.WorktreeSpec{Path: orphan, Branch: "marshal/orphan", Base: "main"}
	if err := e.git.AddWorktree(context.Background(), project.Path, spec); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Remove(context.Background(), project.ID, protocol.RemoveProjectRequest{KeepBranches: true}); err != nil {
		t.Fatal(err)
	}
	if exists(orphan) {
		t.Error("a worktree that no card recorded was left behind")
	}
	trees, err := e.git.ListWorktrees(context.Background(), project.Path)
	if err != nil || len(trees) != 1 {
		t.Errorf("worktrees = %+v, %v; want only the main folder", trees, err)
	}
	if !e.branchExists(t, project, "marshal/orphan") {
		t.Error("a branch that no card recorded is not Marshal's to delete")
	}
}

func TestRemoveOnlyDeletesBranchesWithMarshalsPrefix(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Odd branch")
	if _, err := e.git.Run(context.Background(), project.Path, "branch", "feature/mine"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SetWorktree(context.Background(), card.ID, "", "feature/mine"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Remove(context.Background(), project.ID, protocol.RemoveProjectRequest{}); err != nil {
		t.Fatal(err)
	}
	if !e.branchExists(t, project, "feature/mine") {
		t.Error("a branch without the marshal/ prefix was deleted")
	}
}

func TestRemoveKeepsABranchThatIsCheckedOutInTheRepositoryFolder(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Checked out")
	if _, err := e.git.Run(context.Background(), project.Path, "checkout", "--quiet", "-b", "marshal/checked-out"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SetWorktree(context.Background(), card.ID, "", "marshal/checked-out"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Remove(context.Background(), project.ID, protocol.RemoveProjectRequest{}); err != nil {
		t.Fatalf("Remove: %v; a branch that cannot be deleted must not stop the removal", err)
	}
	if !e.branchExists(t, project, "marshal/checked-out") {
		t.Error("the branch that the person has checked out was deleted")
	}
	e.assertGone(t, project, card)
}

func TestRemoveWhenTheRepositoryFolderIsGone(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Lost repo")
	path, _ := e.startCard(t, project, card)
	if err := os.Rename(project.Path, project.Path+"-moved"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Remove(context.Background(), project.ID, protocol.RemoveProjectRequest{}); err != nil {
		t.Fatal(err)
	}
	if exists(path) || exists(projects.WorktreesDir(e.dataDir, project.ID)) {
		t.Error("the worktree folders of a project whose repository is gone were kept")
	}
	e.assertGone(t, project, card)
}

func TestRemoveStopsWhenSessionsCannotBeStopped(t *testing.T) {
	log := &calls{}
	e := newEnv(t, projects.WithSessionStopper(fakeSessions{log: log, err: errors.New("agent will not stop")}))
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Busy")
	path, branch := e.startCard(t, project, card)
	e.drainEvents()
	err := e.svc.Remove(context.Background(), project.ID, protocol.RemoveProjectRequest{})
	perr := wantCode(t, err, protocol.ErrorCodeUnavailable)
	if strings.Contains(perr.Message, "will not stop") {
		t.Errorf("the message %q shows the cause", perr.Message)
	}
	if !exists(path) || !e.branchExists(t, project, branch) {
		t.Error("something was cleaned up although the sessions were still running")
	}
	if got, err := e.svc.Get(context.Background(), project.ID); err != nil || got.ID != project.ID {
		t.Errorf("the project is gone: %v", err)
	}
	e.noEvent(t)
}

func TestRemoveReportsAMemoryFailureButStillRemovesTheProject(t *testing.T) {
	log := &calls{}
	e := newEnv(t, projects.WithMemoryRemover(fakeMemory{log: log, err: errors.New("disk is read only")}))
	project := e.folder(t, "small-repo")
	e.drainEvents()
	err := e.svc.Remove(context.Background(), project.ID, protocol.RemoveProjectRequest{})
	perr := wantCode(t, err, protocol.ErrorCodeInternal)
	if !strings.Contains(perr.Message, "memory folder") || strings.Contains(perr.Message, "read only") {
		t.Errorf("message = %q", perr.Message)
	}
	e.assertGone(t, project)
	e.nextType(t, protocol.EventTypeProjectRemoved, protocol.HomeTopic)
}

func TestRemoveOfAnUnknownProject(t *testing.T) {
	e := newEnv(t)
	err := e.svc.Remove(context.Background(), "nope", protocol.RemoveProjectRequest{})
	_ = wantCode(t, err, protocol.ErrorCodeNotFound)
	project := e.folder(t, "small-repo")
	if err := e.svc.Remove(context.Background(), project.ID, protocol.RemoveProjectRequest{}); err != nil {
		t.Fatal(err)
	}
	_ = wantCode(t, e.svc.Remove(context.Background(), project.ID, protocol.RemoveProjectRequest{}), protocol.ErrorCodeNotFound)
}

func TestAProjectCanBeAddedAgainAfterItIsRemoved(t *testing.T) {
	e := newEnv(t)
	path := testutil.Fixture(t, "small-repo")
	first, err := e.svc.Create(context.Background(), folderRequest(path))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Remove(context.Background(), first.ID, protocol.RemoveProjectRequest{}); err != nil {
		t.Fatal(err)
	}
	again, err := e.svc.Create(context.Background(), folderRequest(path))
	if err != nil || again.ID != first.ID {
		t.Fatalf("Create after Remove = %+v, %v; want the same folder and id to be free again", again, err)
	}
	card := e.card(t, again.ID, "Numbers start over")
	if card.Number != 1 {
		t.Errorf("number = %d; a project that is added again starts its own count", card.Number)
	}
}
