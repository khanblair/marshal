package gitx_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
)

func TestSparseWorktreeHasOnlyTheListedFolders(t *testing.T) {
	tests := []struct {
		name    string
		sparse  []string
		present []string
		absent  []string
	}{
		{
			name:    "one package",
			sparse:  []string{"packages/api"},
			present: []string{"package.json", "pnpm-workspace.yaml", "README.md", "packages/api/package.json", "packages/api/src/index.js"},
			absent:  []string{"packages/web", "packages/shared", "packages/web/src/index.js", "packages/shared/package.json"},
		},
		{
			name:    "two packages",
			sparse:  []string{"packages/api", "packages/shared"},
			present: []string{"packages/api/src/index.js", "packages/shared/src/index.js", "package.json"},
			absent:  []string{"packages/web"},
		},
		{
			name:    "the same folder spelled twice",
			sparse:  []string{"packages/web", "packages/web/", "./packages/web"},
			present: []string{"packages/web/src/index.js"},
			absent:  []string{"packages/api", "packages/shared"},
		},
		{
			name:    "a folder that is not in the repository",
			sparse:  []string{"packages/nope"},
			present: []string{"package.json"},
			absent:  []string{"packages/api", "packages/web", "packages/shared"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			c := newWorktreeCase(t, "monorepo")
			c.spec.Sparse = tc.sparse
			if err := testGit().AddWorktree(ctx, c.repo, c.spec); err != nil {
				t.Fatalf("AddWorktree: %v", err)
			}
			for _, name := range tc.present {
				if _, err := os.Stat(filepath.Join(c.spec.Path, filepath.FromSlash(name))); err != nil {
					t.Errorf("%s should be checked out: %v", name, err)
				}
			}
			for _, name := range tc.absent {
				if exists(filepath.Join(c.spec.Path, filepath.FromSlash(name))) {
					t.Errorf("%s should not be checked out", name)
				}
			}
			info, err := testGit().Inspect(ctx, c.spec.Path)
			if err != nil || !info.Clean || info.CurrentBranch != c.spec.Branch {
				t.Errorf("Inspect(worktree) = %+v, %v; want a clean tree on %s", info, err, c.spec.Branch)
			}
		})
	}
}

func TestSparseWorktreeStaysSparseAfterLaterCommands(t *testing.T) {
	ctx := context.Background()
	c := newWorktreeCase(t, "monorepo")
	c.spec.Sparse = []string{"packages/api"}
	if err := testGit().AddWorktree(ctx, c.repo, c.spec); err != nil {
		t.Fatal(err)
	}
	commitFile(t, c.spec.Path, "packages/api/change.txt", "change")
	git(t, c.spec.Path, "checkout", "--quiet", "--", ".")
	if exists(filepath.Join(c.spec.Path, "packages", "web")) {
		t.Error("a later Git command checked out a folder outside the limit")
	}
	if got := git(t, c.spec.Path, "sparse-checkout", "list"); got != "packages/api" {
		t.Errorf("sparse-checkout list = %q, want packages/api", got)
	}
	// The main folder of the repository is not affected by the limit on the worktree.
	if !exists(filepath.Join(c.repo, "packages", "web", "package.json")) {
		t.Error("the main working tree lost files")
	}
}

func TestSparseWorktreeCanBeRemoved(t *testing.T) {
	ctx := context.Background()
	c := newWorktreeCase(t, "monorepo")
	c.spec.Sparse = []string{"packages/api"}
	before := worktreePaths(t, c.repo)
	if err := testGit().AddWorktree(ctx, c.repo, c.spec); err != nil {
		t.Fatal(err)
	}
	if err := testGit().RemoveWorktree(ctx, c.repo, c.spec.Path, c.root, false); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	if !reflect.DeepEqual(worktreePaths(t, c.repo), before) {
		t.Errorf("worktrees = %+v, want %+v", worktreePaths(t, c.repo), before)
	}
}

func TestSparseFoldersAreChecked(t *testing.T) {
	bad := []string{
		"", ".", "./", "/", "/etc", "..", "../x", "a/../../x", "a/..", "-x", "-", "!x", "#x", "a*", "a?b", "a[b]",
		"a]b", "C:/x", "c:", "a\x00b", "a\nb", "a\tb",
	}
	for _, folder := range bad {
		t.Run(folder, func(t *testing.T) {
			c := newWorktreeCase(t, "monorepo")
			c.spec.Sparse = []string{"packages/api", folder}
			before := branchNames(t, c.repo)
			err := testGit().AddWorktree(context.Background(), c.repo, c.spec)
			if !errors.Is(err, gitx.ErrBadPath) {
				t.Fatalf("AddWorktree with sparse folder %q = %v, want ErrBadPath", folder, err)
			}
			assertPlainMessage(t, err)
			if exists(c.spec.Path) || !reflect.DeepEqual(branchNames(t, c.repo), before) {
				t.Error("a refused sparse folder left a worktree or a branch behind")
			}
		})
	}
}

func TestSparseFoldersWithSpaces(t *testing.T) {
	ctx := context.Background()
	c := newWorktreeCase(t, "monorepo")
	commitFile(t, c.repo, "apps/my app/main.js", "x")
	c.spec.Sparse = []string{"apps/my app"}
	if err := testGit().AddWorktree(ctx, c.repo, c.spec); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(c.spec.Path, "apps", "my app", "main.js")); err != nil {
		t.Errorf("the folder with a space is missing: %v", err)
	}
	if exists(filepath.Join(c.spec.Path, "packages", "api")) {
		t.Error("a folder outside the limit was checked out")
	}
}
