package gitx_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
)

// containmentCase is one Git command line and what the containment rule says about it.
type containmentCase struct {
	name string
	args []string
	rule string // empty means the command is allowed
	why  string
}

func TestContainmentRefusesLeavingTheWorktreeAndTouchingMain(t *testing.T) {
	worktree := "/data/worktrees/acme/card-1"
	c := gitx.NewContainment("/data/worktrees/acme", worktree, "marshal/acme-1-work", "main")
	cases := []containmentCase{
		// Reading, committing, and working on the card's own branch are the point of bypass.
		{name: "status", args: []string{"status", "--short"}},
		{name: "diff", args: []string{"diff", "HEAD", "main"}},
		{name: "log of main", args: []string{"log", "--oneline", "main"}},
		{name: "show of main", args: []string{"show", "main"}},
		{name: "rev-parse of main", args: []string{"rev-parse", "main"}},
		{name: "merge-base", args: []string{"merge-base", "main", "HEAD"}},
		{name: "add", args: []string{"add", "--all"}},
		{name: "commit", args: []string{"commit", "-m", "work on the card"}},
		{name: "commit that mentions main", args: []string{"commit", "-m", "main"}},
		{name: "checkout own branch", args: []string{"checkout", "marshal/acme-1-work"}},
		{name: "switch own branch", args: []string{"switch", "marshal/acme-1-work"}},
		{name: "branch of its own", args: []string{"branch", "marshal/acme-1-work-2"}},
		{name: "push own branch", args: []string{"push", "origin", "marshal/acme-1-work"}},
		{name: "push own branch to itself", args: []string{"push", "origin", "HEAD:marshal/acme-1-work"}},
		{name: "push a tag", args: []string{"push", "origin", "v1.0.0"}},
		{name: "fetch a branch of the remote", args: []string{"fetch", "origin", "marshal/other"}},
		{name: "run in the worktree", args: []string{"-C", worktree, "status"}},
		{name: "worktree list", args: []string{"worktree", "list"}},
		{name: "remote -v", args: []string{"remote", "-v"}},
		{name: "config --get", args: []string{"config", "--get", "user.email"}},
		{name: "no subcommand", args: nil},
		{name: "unknown subcommand", args: []string{"bisect", "start"}},

		// Leaving the worktree.
		{name: "run in another folder", rule: gitx.RuleOutsideWorktree, args: []string{"-C", "/repo", "status"}},
		{name: "run in a sibling worktree", rule: gitx.RuleOutsideWorktree, args: []string{"-C", "/data/worktrees/acme/card-2", "status"}},
		{name: "another git directory", rule: gitx.RuleOutsideWorktree, args: []string{"--git-dir=/repo/.git", "log"}},
		{name: "another working tree", rule: gitx.RuleOutsideWorktree, args: []string{"--work-tree", "/repo", "status"}},
		{name: "a namespace", rule: gitx.RuleOutsideWorktree, args: []string{"--namespace=other", "log"}},
		{name: "a folder with no value", rule: gitx.RuleOutsideWorktree, args: []string{"-C"}},
		{name: "clone", rule: gitx.RuleOutsideWorktree, args: []string{"clone", "/repo", "/tmp/other"}},
		{name: "init", rule: gitx.RuleOutsideWorktree, args: []string{"init", "/tmp/other"}},
		{name: "add a worktree", rule: gitx.RuleOutsideWorktree, args: []string{"worktree", "add", "/tmp/other"}},
		{name: "remove a worktree", rule: gitx.RuleOutsideWorktree, args: []string{"worktree", "remove", "/tmp/other"}},
		{name: "a submodule", rule: gitx.RuleOutsideWorktree, args: []string{"submodule", "update", "--init"}},
		{name: "change a remote", rule: gitx.RuleOutsideWorktree, args: []string{"remote", "set-url", "origin", "https://example.invalid/x.git"}},
		{name: "rewrite history", rule: gitx.RuleOutsideWorktree, args: []string{"filter-branch", "--all"}},
		{name: "change a repository setting", rule: gitx.RuleOutsideWorktree, args: []string{"config", "core.hooksPath", "/tmp/hooks"}},

		// Touching the main branch.
		{name: "checkout main", rule: gitx.RuleMainBranch, args: []string{"checkout", "main"}},
		{name: "checkout the full ref", rule: gitx.RuleMainBranch, args: []string{"checkout", "refs/heads/main"}},
		{name: "switch main", rule: gitx.RuleMainBranch, args: []string{"switch", "main"}},
		{name: "switch to a tracking main", rule: gitx.RuleMainBranch, args: []string{"switch", "origin/main"}},
		{name: "merge main", rule: gitx.RuleMainBranch, args: []string{"merge", "main"}},
		{name: "merge with a message", rule: gitx.RuleMainBranch, args: []string{"merge", "-m", "bring it in", "main"}},
		{name: "rebase onto main", rule: gitx.RuleMainBranch, args: []string{"rebase", "main"}},
		{name: "reset to main", rule: gitx.RuleMainBranch, args: []string{"reset", "--hard", "main"}},
		{name: "delete main", rule: gitx.RuleMainBranch, args: []string{"branch", "-D", "main"}},
		{name: "move main", rule: gitx.RuleMainBranch, args: []string{"branch", "-m", "main", "old-main"}},
		{name: "tag main", rule: gitx.RuleMainBranch, args: []string{"tag", "v1.0.0", "main"}},
		{name: "push main", rule: gitx.RuleMainBranch, args: []string{"push", "origin", "main"}},
		{name: "push the card branch onto main", rule: gitx.RuleMainBranch, args: []string{"push", "origin", "HEAD:main"}},
		{name: "push the full ref onto main", rule: gitx.RuleMainBranch, args: []string{"push", "origin", "refs/heads/marshal/acme-1-work:refs/heads/main"}},
		{name: "delete main on the remote", rule: gitx.RuleMainBranch, args: []string{"push", "origin", ":main"}},
		{name: "fetch main", rule: gitx.RuleMainBranch, args: []string{"fetch", "origin", "main"}},
		{name: "pull main", rule: gitx.RuleMainBranch, args: []string{"pull", "origin", "main"}},
		{name: "update the main ref", rule: gitx.RuleMainBranch, args: []string{"update-ref", "refs/heads/main", "HEAD"}},
		{name: "point HEAD at main", rule: gitx.RuleMainBranch, args: []string{"symbolic-ref", "HEAD", "refs/heads/main"}},
		{name: "cherry-pick from main", rule: gitx.RuleMainBranch, args: []string{"cherry-pick", "main"}},

		// Overriding a remote.
		{name: "force push the card branch", rule: gitx.RuleForcePush, args: []string{"push", "--force", "origin", "marshal/acme-1-work"}},
		{name: "force push, short flag", rule: gitx.RuleForcePush, args: []string{"push", "-f", "origin", "marshal/acme-1-work"}},
		{name: "force push with lease", rule: gitx.RuleForcePush, args: []string{"push", "--force-with-lease", "origin", "marshal/acme-1-work"}},
		{name: "push every branch", rule: gitx.RuleForcePush, args: []string{"push", "--all", "origin"}},
		{name: "mirror the repository", rule: gitx.RuleForcePush, args: []string{"push", "--mirror", "origin"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := c.CheckCommand(tc.args)
			if tc.rule == "" {
				if err != nil {
					t.Fatalf("git %s: refused, want allowed: %v", strings.Join(tc.args, " "), err)
				}
				return
			}
			var v gitx.Violation
			if !errors.As(err, &v) {
				t.Fatalf("git %s: err = %v, want a violation of the %s rule", strings.Join(tc.args, " "), err, tc.rule)
			}
			if v.Rule != tc.rule {
				t.Errorf("git %s: rule = %s, want %s", strings.Join(tc.args, " "), v.Rule, tc.rule)
			}
		})
	}
}

// TestContainmentReadsTheWorktreeGitReports is the half that holds in a real repository: the folder
// bypass runs an agent in is one Marshal made, under the worktrees root, on the card's own branch.
func TestContainmentReadsTheWorktreeGitReports(t *testing.T) {
	ctx := context.Background()
	g := testGit()
	repo := fixture(t, "small-repo")
	root := filepath.Join(t.TempDir(), "worktrees")
	spec := gitx.WorktreeSpec{
		Path:   filepath.Join(root, "acme", "card-1"),
		Branch: "marshal/acme-1-work",
		Base:   "main",
	}
	if err := g.AddWorktree(ctx, repo, spec); err != nil {
		t.Fatalf("add a worktree: %v", err)
	}
	list, err := g.ListWorktrees(ctx, repo)
	if err != nil {
		t.Fatalf("list worktrees: %v", err)
	}
	c := gitx.NewContainment(root, spec.Path, spec.Branch, "main")
	if err := c.CheckWorktrees(list); err != nil {
		t.Fatalf("the worktree Marshal just made was refused: %v", err)
	}

	// A worktree that is checked out on the main branch is the case this rule exists for. Git will
	// not check the same branch out twice, so it takes two steps: the repository folder moves off
	// main, and the card's worktree moves onto it. Both are things a person can do by hand, and the
	// daemon must not then hand a card an agent that runs without asking.
	if _, err := g.Run(ctx, repo, "switch", "--quiet", "-c", "scratch"); err != nil {
		t.Fatalf("move the repository folder off main: %v", err)
	}
	if _, err := g.Run(ctx, spec.Path, "checkout", "--quiet", "main"); err != nil {
		t.Fatalf("check out main in the worktree: %v", err)
	}
	list, err = g.ListWorktrees(ctx, repo)
	if err != nil {
		t.Fatalf("list worktrees: %v", err)
	}
	var v gitx.Violation
	if err := c.CheckWorktrees(list); !errors.As(err, &v) || v.Rule != gitx.RuleMainBranch {
		t.Fatalf("a worktree on the main branch: err = %v, want a %s violation", err, gitx.RuleMainBranch)
	}
}

// TestContainmentRefusesAWorktreeOutsideTheRoot covers the other half of the folder rule: a folder
// Marshal did not make is never one its agents run in.
func TestContainmentRefusesAWorktreeOutsideTheRoot(t *testing.T) {
	ctx := context.Background()
	g := testGit()
	repo := fixture(t, "small-repo")
	elsewhere := filepath.Join(t.TempDir(), "not-marshals")
	spec := gitx.WorktreeSpec{Path: filepath.Join(elsewhere, "card-1"), Branch: "marshal/acme-1-work", Base: "main"}
	if err := g.AddWorktree(ctx, repo, spec); err != nil {
		t.Fatalf("add a worktree: %v", err)
	}
	list, err := g.ListWorktrees(ctx, repo)
	if err != nil {
		t.Fatalf("list worktrees: %v", err)
	}
	c := gitx.NewContainment(filepath.Join(t.TempDir(), "worktrees"), spec.Path, spec.Branch, "main")
	var v gitx.Violation
	if err := c.CheckWorktrees(list); !errors.As(err, &v) || v.Rule != gitx.RuleOutsideWorktree {
		t.Fatalf("a worktree outside the root: err = %v, want a %s violation", err, gitx.RuleOutsideWorktree)
	}
	// A worktree Git has never heard of is refused too, rather than assumed to be fine.
	if err := c.CheckWorktrees(nil); !errors.As(err, &v) || v.Rule != gitx.RuleOutsideWorktree {
		t.Fatalf("an unknown worktree: err = %v, want a %s violation", err, gitx.RuleOutsideWorktree)
	}
}
