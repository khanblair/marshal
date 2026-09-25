package gitx_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
)

func TestValidBranchName(t *testing.T) {
	valid := []string{"marshal/p-1-fix-login", "feature/a", "a.b", "release-1.2", "é", "x/y/z"}
	invalid := []string{
		"", "-x", "-", "a b", "a..b", "x.lock", "a/", "/a", "a//b", "a~1", "a^", "a:b", "a?b", "a*",
		"a[b", "a\\b", "a\x01b", "a\nb", "@{-1}", "@", "HEAD", "refs/heads/x", "a@{b", ".hidden/x/.y",
	}
	for _, name := range valid {
		if err := testGit().ValidBranchName(context.Background(), name); err != nil {
			t.Errorf("ValidBranchName(%q) = %v, want nil", name, err)
		}
	}
	for _, name := range invalid {
		err := testGit().ValidBranchName(context.Background(), name)
		if !errors.Is(err, gitx.ErrBadBranchName) {
			t.Errorf("ValidBranchName(%q) = %v, want ErrBadBranchName", name, err)
		}
	}
}

func TestBranchLifecycle(t *testing.T) {
	ctx := context.Background()
	g := testGit()
	dir := fixture(t, "small-repo")
	found, err := g.BranchExists(ctx, dir, "marshal/p-1-x")
	if err != nil || found {
		t.Fatalf("BranchExists before = %v, %v; want false", found, err)
	}
	if err := g.CreateBranch(ctx, dir, "marshal/p-1-x", "main"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if found, err = g.BranchExists(ctx, dir, "marshal/p-1-x"); err != nil || !found {
		t.Fatalf("BranchExists after = %v, %v; want true", found, err)
	}
	err = g.CreateBranch(ctx, dir, "marshal/p-1-x", "main")
	var gitErr *gitx.Error
	if !errors.Is(err, gitx.ErrBranchExists) || !errors.As(err, &gitErr) {
		t.Errorf("CreateBranch twice = %v, want ErrBranchExists that wraps the Git error", err)
	}
	if got, _ := g.Branches(ctx, dir, ""); !reflect.DeepEqual(got, []string{"main", "marshal/p-1-x"}) {
		t.Errorf("Branches = %v", got)
	}
	if err := g.DeleteBranch(ctx, dir, "marshal/p-1-x", false); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	if found, _ = g.BranchExists(ctx, dir, "marshal/p-1-x"); found {
		t.Error("the branch is still there after DeleteBranch")
	}
}

func TestDeleteBranchNeedsForceForUnmergedWork(t *testing.T) {
	ctx := context.Background()
	g := testGit()
	dir := fixture(t, "small-repo")
	git(t, dir, "switch", "--quiet", "--create", "marshal/p-2-work")
	commitFile(t, dir, "work.txt", "work")
	git(t, dir, "switch", "--quiet", "main")
	if err := g.DeleteBranch(ctx, dir, "marshal/p-2-work", false); err == nil {
		t.Fatal("DeleteBranch without force deleted a branch with unmerged work")
	}
	if found, _ := g.BranchExists(ctx, dir, "marshal/p-2-work"); !found {
		t.Fatal("the unmerged branch was lost")
	}
	if err := g.DeleteBranch(ctx, dir, "marshal/p-2-work", true); err != nil {
		t.Fatalf("DeleteBranch with force: %v", err)
	}
}

func TestIsMerged(t *testing.T) {
	ctx := context.Background()
	g := testGit()
	dir := fixture(t, "small-repo")
	git(t, dir, "branch", "same")
	git(t, dir, "switch", "--quiet", "--create", "ahead")
	commitFile(t, dir, "ahead.txt", "ahead")
	git(t, dir, "switch", "--quiet", "main")
	tests := []struct {
		branch, into string
		want         bool
	}{
		{"same", "main", true},
		{"ahead", "main", false},
		{"main", "ahead", true},
	}
	for _, tc := range tests {
		got, err := g.IsMerged(ctx, dir, tc.branch, tc.into)
		if err != nil || got != tc.want {
			t.Errorf("IsMerged(%s, %s) = %v, %v; want %v", tc.branch, tc.into, got, err, tc.want)
		}
	}
	if _, err := g.IsMerged(ctx, dir, "nope", "main"); err == nil {
		t.Error("IsMerged accepted a branch that does not exist")
	}
	if _, err := g.IsMerged(ctx, dir, "--all", "main"); !errors.Is(err, gitx.ErrBadBranchName) {
		t.Errorf("IsMerged(--all) = %v, want ErrBadBranchName", err)
	}
}

func TestBranchesMatchesThePrefixLiterally(t *testing.T) {
	ctx := context.Background()
	g := testGit()
	dir := fixture(t, "small-repo")
	for _, name := range []string{"marshal/a", "marshal/b/c", "marshal-other", "mar", "feature/x"} {
		git(t, dir, "branch", name)
	}
	tests := map[string][]string{
		"marshal/": {"marshal/a", "marshal/b/c"},
		"marshal":  {"marshal-other", "marshal/a", "marshal/b/c"},
		"mar":      {"mar", "marshal-other", "marshal/a", "marshal/b/c"},
		"none/":    {},
		"marshal*": {},
	}
	for prefix, want := range tests {
		got, err := g.Branches(ctx, dir, prefix)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("Branches(%q) = %v, %v; want %v", prefix, got, err, want)
		}
	}
}

func TestBranchesIgnoresTagsWithTheSameName(t *testing.T) {
	dir := fixture(t, "small-repo")
	git(t, dir, "branch", "release")
	git(t, dir, "tag", "release")
	got, err := testGit().Branches(context.Background(), dir, "rel")
	if err != nil || !reflect.DeepEqual(got, []string{"release"}) {
		t.Errorf("Branches = %v, %v; want [release]", got, err)
	}
}

func TestBranchCommandsRefuseNamesThatAreOptions(t *testing.T) {
	ctx := context.Background()
	g := testGit()
	dir := fixture(t, "small-repo")
	if err := g.CreateBranch(ctx, dir, "marshal/ok", "--force"); !errors.Is(err, gitx.ErrBadBranchName) {
		t.Errorf("CreateBranch with base --force = %v, want ErrBadBranchName", err)
	}
	if err := g.CreateBranch(ctx, dir, "-x", "main"); !errors.Is(err, gitx.ErrBadBranchName) {
		t.Errorf("CreateBranch(-x) = %v, want ErrBadBranchName", err)
	}
	if err := g.DeleteBranch(ctx, dir, "--force", true); !errors.Is(err, gitx.ErrBadBranchName) {
		t.Errorf("DeleteBranch(--force) = %v, want ErrBadBranchName", err)
	}
}

func TestCreateBranchFromAMissingBase(t *testing.T) {
	dir := fixture(t, "small-repo")
	err := testGit().CreateBranch(context.Background(), dir, "marshal/p-1-x", "no-such-base")
	if err == nil || errors.Is(err, gitx.ErrBranchExists) {
		t.Errorf("CreateBranch = %v, want a plain failure", err)
	}
	if found, _ := testGit().BranchExists(context.Background(), dir, "marshal/p-1-x"); found {
		t.Error("a failed CreateBranch left the branch behind")
	}
}

func TestBranchExistsWithOddNames(t *testing.T) {
	dir := fixture(t, "small-repo")
	for _, name := range []string{"", "a..b", "-x", "a b", "main/"} {
		found, err := testGit().BranchExists(context.Background(), dir, name)
		if err != nil || found {
			t.Errorf("BranchExists(%q) = %v, %v; want false with no error", name, found, err)
		}
	}
}
