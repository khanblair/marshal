package gitx_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
)

// worktreeCase is a repository, a folder for its worktrees, and a spec that works.
type worktreeCase struct {
	repo string
	root string
	spec gitx.WorktreeSpec
}

func newWorktreeCase(t *testing.T, fixtureName string) worktreeCase {
	t.Helper()
	root := filepath.Join(t.TempDir(), "worktrees")
	return worktreeCase{
		repo: fixture(t, fixtureName),
		root: root,
		spec: gitx.WorktreeSpec{
			Path:   filepath.Join(root, "acme", "card-1"),
			Branch: "marshal/acme-1-work",
			Base:   "main",
		},
	}
}

func branchNames(t *testing.T, repo string) []string {
	t.Helper()
	names, err := testGit().Branches(context.Background(), repo, "")
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func worktreePaths(t *testing.T, repo string) []gitx.Worktree {
	t.Helper()
	list, err := testGit().ListWorktrees(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func TestAddAndRemoveWorktreeLeavesTheRepoAsItWas(t *testing.T) {
	ctx := context.Background()
	g := testGit()
	tc := newWorktreeCase(t, "small-repo")
	branchesBefore, worktreesBefore := branchNames(t, tc.repo), worktreePaths(t, tc.repo)

	if err := g.AddWorktree(ctx, tc.repo, tc.spec); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tc.spec.Path, "src", "util.js")); err != nil {
		t.Errorf("the worktree is missing a file: %v", err)
	}
	list := worktreePaths(t, tc.repo)
	if len(list) != 2 || list[1].Branch != tc.spec.Branch || !sameFolder(t, list[1].Path, tc.spec.Path) {
		t.Fatalf("worktrees = %+v, want the main one and the new one", list)
	}
	info, err := g.Inspect(ctx, tc.spec.Path)
	if err != nil || info.CurrentBranch != tc.spec.Branch || !info.Clean {
		t.Errorf("Inspect(worktree) = %+v, %v", info, err)
	}

	if err := g.RemoveWorktree(ctx, tc.repo, tc.spec.Path, tc.root, false); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	if exists(tc.spec.Path) {
		t.Error("the worktree folder is still there")
	}
	if !reflect.DeepEqual(worktreePaths(t, tc.repo), worktreesBefore) {
		t.Errorf("worktrees = %+v, want %+v", worktreePaths(t, tc.repo), worktreesBefore)
	}
	// The branch stays, because it can hold work that is not merged. The caller deletes it.
	if found, _ := g.BranchExists(ctx, tc.repo, tc.spec.Branch); !found {
		t.Error("RemoveWorktree removed the branch")
	}
	if err := g.DeleteBranch(ctx, tc.repo, tc.spec.Branch, false); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	if !reflect.DeepEqual(branchNames(t, tc.repo), branchesBefore) {
		t.Errorf("branches = %v, want %v", branchNames(t, tc.repo), branchesBefore)
	}
}

func TestAddWorktreeAcceptsDifferentPlaces(t *testing.T) {
	sep := string(os.PathSeparator)
	tests := map[string]func(t *testing.T, tc *worktreeCase){
		"a path with spaces": func(t *testing.T, tc *worktreeCase) {
			tc.spec.Path = filepath.Join(tc.root, "my project", "card 1")
		},
		"a trailing separator": func(t *testing.T, tc *worktreeCase) {
			tc.spec.Path += sep
		},
		"an empty folder that exists": func(t *testing.T, tc *worktreeCase) {
			if err := os.MkdirAll(tc.spec.Path, 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"a parent that is a link": func(t *testing.T, tc *worktreeCase) {
			real := filepath.Join(t.TempDir(), "real")
			if err := os.MkdirAll(real, 0o755); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(t.TempDir(), "link")
			symlink(t, real, link)
			tc.spec.Path = filepath.Join(link, "card-1")
		},
	}
	for name, prepare := range tests {
		t.Run(name, func(t *testing.T) {
			tc := newWorktreeCase(t, "small-repo")
			prepare(t, &tc)
			if err := testGit().AddWorktree(context.Background(), tc.repo, tc.spec); err != nil {
				t.Fatalf("AddWorktree(%q): %v", tc.spec.Path, err)
			}
			if _, err := os.Stat(filepath.Join(tc.spec.Path, "README.md")); err != nil {
				t.Errorf("the worktree is empty: %v", err)
			}
		})
	}
}

func TestAddWorktreeRefusesAndChangesNothing(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T, tc *worktreeCase)
		want    error
	}{
		{"a relative path", func(_ *testing.T, tc *worktreeCase) { tc.spec.Path = "wt/card-1" }, gitx.ErrBadPath},
		{"an empty path", func(_ *testing.T, tc *worktreeCase) { tc.spec.Path = "" }, gitx.ErrBadPath},
		{"a dot dot segment", func(_ *testing.T, tc *worktreeCase) {
			tc.spec.Path = tc.root + string(os.PathSeparator) + "a" + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "b"
		}, gitx.ErrBadPath},
		{"the repository folder", func(_ *testing.T, tc *worktreeCase) { tc.spec.Path = tc.repo }, gitx.ErrBadPath},
		{"a place inside the repository", func(_ *testing.T, tc *worktreeCase) {
			tc.spec.Path = filepath.Join(tc.repo, "wt", "card-1")
		}, gitx.ErrBadPath},
		{"a folder that has files", func(t *testing.T, tc *worktreeCase) {
			if err := os.MkdirAll(tc.spec.Path, 0o755); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(tc.spec.Path, "keep.txt"), "keep")
		}, gitx.ErrWorktreeExists},
		{"a file", func(t *testing.T, tc *worktreeCase) {
			if err := os.MkdirAll(filepath.Dir(tc.spec.Path), 0o755); err != nil {
				t.Fatal(err)
			}
			writeFile(t, tc.spec.Path, "a file")
		}, gitx.ErrBadPath},
		{"a branch that exists", func(_ *testing.T, tc *worktreeCase) { tc.spec.Branch = "main" }, gitx.ErrBranchExists},
		{"a bad branch name", func(_ *testing.T, tc *worktreeCase) { tc.spec.Branch = "a..b" }, gitx.ErrBadBranchName},
		{"a branch name that is an option", func(_ *testing.T, tc *worktreeCase) { tc.spec.Branch = "--force" }, gitx.ErrBadBranchName},
		{"a base that is an option", func(_ *testing.T, tc *worktreeCase) { tc.spec.Base = "--orphan" }, gitx.ErrBadBranchName},
		{"a folder that is not a repository", func(t *testing.T, tc *worktreeCase) { tc.repo = t.TempDir() }, gitx.ErrNotARepo},
		{"a subfolder of the repository", func(_ *testing.T, tc *worktreeCase) { tc.repo = filepath.Join(tc.repo, "src") }, gitx.ErrNotARepo},
		{"a bad sparse folder", func(_ *testing.T, tc *worktreeCase) { tc.spec.Sparse = []string{"../outside"} }, gitx.ErrBadPath},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newWorktreeCase(t, "small-repo")
			realRepo := c.repo
			branchesBefore, worktreesBefore := branchNames(t, realRepo), worktreePaths(t, realRepo)
			tc.prepare(t, &c)

			err := testGit().AddWorktree(context.Background(), c.repo, c.spec)
			if !errors.Is(err, tc.want) {
				t.Fatalf("AddWorktree = %v, want %v", err, tc.want)
			}
			assertPlainMessage(t, err)
			if !reflect.DeepEqual(branchNames(t, realRepo), branchesBefore) {
				t.Errorf("branches changed: %v, was %v", branchNames(t, realRepo), branchesBefore)
			}
			if !reflect.DeepEqual(worktreePaths(t, realRepo), worktreesBefore) {
				t.Errorf("worktrees changed: %+v", worktreePaths(t, realRepo))
			}
		})
	}
}

func TestAddWorktreeFromAMissingBaseLeavesNothing(t *testing.T) {
	tc := newWorktreeCase(t, "small-repo")
	tc.spec.Base = "no-such-base"
	err := testGit().AddWorktree(context.Background(), tc.repo, tc.spec)
	if err == nil {
		t.Fatal("AddWorktree accepted a base that does not exist")
	}
	if found, _ := testGit().BranchExists(context.Background(), tc.repo, tc.spec.Branch); found {
		t.Error("the failed AddWorktree left its branch behind")
	}
	if exists(tc.spec.Path) {
		t.Error("the failed AddWorktree left its folder behind")
	}
}

func TestAddWorktreeStopsWhenTheContextIsCancelled(t *testing.T) {
	tc := newWorktreeCase(t, "small-repo")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := testGit().AddWorktree(ctx, tc.repo, tc.spec)
	if !errors.Is(err, context.Canceled) || errors.Is(err, gitx.ErrNotARepo) {
		t.Errorf("AddWorktree = %v, want a cancel", err)
	}
	if exists(tc.spec.Path) {
		t.Error("a cancelled AddWorktree left a folder behind")
	}
}

func TestRemoveWorktreeNeedsForceForChanges(t *testing.T) {
	changes := map[string]func(t *testing.T, dir string){
		"an edited file":    func(t *testing.T, dir string) { writeFile(t, filepath.Join(dir, "README.md"), "edited") },
		"an untracked file": func(t *testing.T, dir string) { writeFile(t, filepath.Join(dir, "new.txt"), "new") },
		"a staged new file": func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "new.txt"), "new")
			git(t, dir, "add", "new.txt")
		},
		"a removed file": func(t *testing.T, dir string) { _ = os.Remove(filepath.Join(dir, "README.md")) },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			tc := newWorktreeCase(t, "small-repo")
			if err := testGit().AddWorktree(ctx, tc.repo, tc.spec); err != nil {
				t.Fatal(err)
			}
			change(t, tc.spec.Path)
			err := testGit().RemoveWorktree(ctx, tc.repo, tc.spec.Path, tc.root, false)
			if !errors.Is(err, gitx.ErrDirty) {
				t.Fatalf("RemoveWorktree = %v, want ErrDirty", err)
			}
			if !exists(tc.spec.Path) || len(worktreePaths(t, tc.repo)) != 2 {
				t.Fatal("the worktree was removed although it had changes")
			}
			if err := testGit().RemoveWorktree(ctx, tc.repo, tc.spec.Path, tc.root, true); err != nil {
				t.Fatalf("RemoveWorktree with force: %v", err)
			}
			if exists(tc.spec.Path) || len(worktreePaths(t, tc.repo)) != 1 {
				t.Error("force did not remove the worktree")
			}
		})
	}
}

func TestRemoveWorktreeIgnoresIgnoredFiles(t *testing.T) {
	ctx := context.Background()
	tc := newWorktreeCase(t, "small-repo")
	if err := testGit().AddWorktree(ctx, tc.repo, tc.spec); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tc.spec.Path, "node_modules", "dep"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(tc.spec.Path, "node_modules", "dep", "index.js"), "x")
	if err := testGit().RemoveWorktree(ctx, tc.repo, tc.spec.Path, tc.root, false); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	if exists(tc.spec.Path) {
		t.Error("the worktree folder is still there")
	}
}

func TestRemoveWorktreeRefusesPathsOutsideTheRoot(t *testing.T) {
	ctx := context.Background()
	tc := newWorktreeCase(t, "small-repo")
	outside := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(outside, "precious.txt"), "keep")
	sibling := tc.root + "-evil"
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(tc.root, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tc.root, "link-out")
	linked := os.Symlink(outside, link) == nil
	sep := string(os.PathSeparator)
	tests := map[string]struct{ path, root string }{
		"a folder elsewhere":            {outside, tc.root},
		"the root itself":               {tc.root, tc.root},
		"the root with a separator":     {tc.root + sep, tc.root},
		"a parent of the root":          {filepath.Dir(tc.root), tc.root},
		"a dot dot escape":              {tc.root + sep + ".." + sep + "elsewhere", tc.root},
		"a sibling with the same start": {sibling, tc.root},
		"the repository itself":         {tc.repo, tc.root},
		"a relative path":               {"card-1", tc.root},
		"an empty path":                 {"", tc.root},
		"an empty root":                 {outside, ""},
		"a relative root":               {outside, "worktrees"},
	}
	if linked {
		tests["a link that leaves the root"] = struct{ path, root string }{link, tc.root}
	}
	for name, c := range tests {
		t.Run(name, func(t *testing.T) {
			err := testGit().RemoveWorktree(ctx, tc.repo, c.path, c.root, true)
			if !errors.Is(err, gitx.ErrOutsideRoot) {
				t.Errorf("RemoveWorktree(%q, root %q) = %v, want ErrOutsideRoot", c.path, c.root, err)
			}
			assertPlainMessage(t, err)
		})
	}
	for _, path := range []string{filepath.Join(outside, "precious.txt"), sibling, tc.repo, tc.root} {
		if !exists(path) {
			t.Errorf("%s was removed", path)
		}
	}
}

func TestRemoveWorktreeHandlesFoldersGitDoesNotKnow(t *testing.T) {
	ctx := context.Background()
	tc := newWorktreeCase(t, "small-repo")
	full := filepath.Join(tc.root, "acme", "orphan")
	empty := filepath.Join(tc.root, "acme", "empty")
	for _, dir := range []string{full, empty} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(full, "work.txt"), "unsaved work")

	if err := testGit().RemoveWorktree(ctx, tc.repo, full, tc.root, false); !errors.Is(err, gitx.ErrDirty) {
		t.Errorf("RemoveWorktree(full) = %v, want ErrDirty", err)
	}
	if !exists(filepath.Join(full, "work.txt")) {
		t.Fatal("a folder with files was removed without force")
	}
	if err := testGit().RemoveWorktree(ctx, tc.repo, full, tc.root, true); err != nil || exists(full) {
		t.Errorf("RemoveWorktree(full, force) = %v, exists = %v", err, exists(full))
	}
	if err := testGit().RemoveWorktree(ctx, tc.repo, empty, tc.root, false); err != nil || exists(empty) {
		t.Errorf("RemoveWorktree(empty) = %v, exists = %v", err, exists(empty))
	}
	missing := filepath.Join(tc.root, "acme", "never-existed")
	if err := testGit().RemoveWorktree(ctx, tc.repo, missing, tc.root, false); err != nil {
		t.Errorf("RemoveWorktree(missing) = %v, want nil so a second clean up is harmless", err)
	}
}

func TestRemoveWorktreeAfterTheFolderWasDeletedByHand(t *testing.T) {
	ctx := context.Background()
	tc := newWorktreeCase(t, "small-repo")
	if err := testGit().AddWorktree(ctx, tc.repo, tc.spec); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(tc.spec.Path); err != nil {
		t.Fatal(err)
	}
	if err := testGit().RemoveWorktree(ctx, tc.repo, tc.spec.Path, tc.root, false); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	if len(worktreePaths(t, tc.repo)) != 1 {
		t.Errorf("worktrees = %+v, want only the main one", worktreePaths(t, tc.repo))
	}
}

func TestRemoveWorktreeThroughALinkToTheRoot(t *testing.T) {
	ctx := context.Background()
	tc := newWorktreeCase(t, "small-repo")
	if err := testGit().AddWorktree(ctx, tc.repo, tc.spec); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	symlink(t, tc.root, link)
	viaLink := filepath.Join(link, "acme", "card-1")
	if err := testGit().RemoveWorktree(ctx, tc.repo, viaLink, tc.root, false); err != nil {
		t.Fatalf("RemoveWorktree(%q): %v", viaLink, err)
	}
	if exists(tc.spec.Path) {
		t.Error("the worktree is still there")
	}
}

func TestPruneWorktreesForgetsMissingFolders(t *testing.T) {
	ctx := context.Background()
	tc := newWorktreeCase(t, "small-repo")
	if err := testGit().AddWorktree(ctx, tc.repo, tc.spec); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(tc.spec.Path); err != nil {
		t.Fatal(err)
	}
	list := worktreePaths(t, tc.repo)
	if len(list) != 2 || !list[1].Prunable {
		t.Fatalf("worktrees = %+v, want the missing one to be prunable", list)
	}
	if err := testGit().PruneWorktrees(ctx, tc.repo); err != nil {
		t.Fatal(err)
	}
	if len(worktreePaths(t, tc.repo)) != 1 {
		t.Error("the missing worktree is still listed")
	}
}

func TestListWorktreesMainFirst(t *testing.T) {
	ctx := context.Background()
	tc := newWorktreeCase(t, "small-repo")
	tc.spec.Path = filepath.Join(tc.root, "with space")
	if err := testGit().AddWorktree(ctx, tc.repo, tc.spec); err != nil {
		t.Fatal(err)
	}
	list := worktreePaths(t, tc.repo)
	if len(list) != 2 || !sameFolder(t, list[0].Path, tc.repo) || list[0].Branch != "main" {
		t.Fatalf("worktrees = %+v", list)
	}
	if !strings.Contains(list[1].Path, "with space") || len(list[1].Head) < 40 {
		t.Errorf("second worktree = %+v, want the path with a space and a full commit id", list[1])
	}
}

func TestAddWorktreeFromARemoteBranchDoesNotTrackIt(t *testing.T) {
	ctx := context.Background()
	tc := newWorktreeCase(t, "small-repo")
	git(t, tc.repo, "update-ref", "refs/remotes/origin/develop", "HEAD")
	tc.spec.Base = "origin/develop"
	if err := testGit().AddWorktree(ctx, tc.repo, tc.spec); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	if _, err := testGit().Run(ctx, tc.repo, "config", "--get", "branch."+tc.spec.Branch+".remote"); err == nil {
		t.Error("the new branch follows a remote branch, so a pull or a push would go there")
	}
}

func TestAddWorktreeWhereAnOldOneIsRegisteredButGone(t *testing.T) {
	ctx := context.Background()
	tc := newWorktreeCase(t, "small-repo")
	if err := testGit().AddWorktree(ctx, tc.repo, tc.spec); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(tc.spec.Path); err != nil {
		t.Fatal(err)
	}
	branchesBefore, worktreesBefore := branchNames(t, tc.repo), worktreePaths(t, tc.repo)

	tc.spec.Branch = "marshal/acme-2-again"
	err := testGit().AddWorktree(ctx, tc.repo, tc.spec)
	if !errors.Is(err, gitx.ErrWorktreeExists) {
		t.Fatalf("AddWorktree = %v, want ErrWorktreeExists", err)
	}
	assertPlainMessage(t, err)
	if !reflect.DeepEqual(branchNames(t, tc.repo), branchesBefore) || !reflect.DeepEqual(worktreePaths(t, tc.repo), worktreesBefore) {
		t.Error("the refused AddWorktree changed the repository, for example by removing the old registration")
	}
	// After the caller removes the old one, the place is free again.
	if err := testGit().RemoveWorktree(ctx, tc.repo, tc.spec.Path, tc.root, false); err != nil {
		t.Fatal(err)
	}
	if err := testGit().AddWorktree(ctx, tc.repo, tc.spec); err != nil {
		t.Errorf("AddWorktree after the clean up: %v", err)
	}
}
