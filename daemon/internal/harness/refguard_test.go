package harness_test

import (
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
)

// The ref guard is what keeps the Integrator inside its own workspace: it may commit and merge there,
// and it may not move or delete anything the owner's repository shares.

func TestTheRefGuardRefusesCommandsThatMoveRefs(t *testing.T) {
	refused := []string{
		"git push",
		"git push origin integrator",
		"git -C /tmp/work push origin HEAD",
		"git pull --rebase",
		"git fetch origin main:main",
		"git fetch --force origin",
		"git rebase main",
		"git rebase --abort",
		"git update-ref refs/heads/main HEAD",
		"git update-ref -d refs/heads/old",
		"git switch other",
		"git switch -c new",
		"git checkout main",
		"git checkout -b new-branch",
		"git checkout -B new-branch",
		"git checkout --orphan fresh",
		"git checkout --detach",
		"git checkout .",
		"git branch -D old",
		"git branch -d old",
		"git branch -f main HEAD",
		"git branch -fd old",
		"git branch -m old new",
		"git branch --delete old",
		"git branch new-branch",
		"git branch -u origin/main",
		"git tag v1",
		"git tag -a v1 -m release",
		"git tag -d v1",
		"git reset --hard",
		"git reset --hard HEAD~1",
		"git reset --soft HEAD~1",
		"git reset --merge",
		"git reset HEAD~2",
		"git reset main",
		"git worktree remove ../other",
		"git worktree prune",
		"git worktree add ../other main",
		"git stash drop",
		"git stash drop stash@{1}",
		"git stash clear",
		"git stash pop",
		"git stash branch fresh",
		"git reflog expire --all",
		"git reflog delete HEAD@{1}",
		"git symbolic-ref HEAD refs/heads/other",
		"git symbolic-ref -d HEAD",
		"git gc --prune=now",
		"git prune",
		"git filter-branch --all",
		"git replace abc def",
		// Found wherever the line puts them.
		"git status && git push origin integrator",
		"cd work; git rebase main",
		"sudo git push",
		"bash -c 'git push origin integrator'",
		"env GIT_TERMINAL_PROMPT=0 git push",
		"GIT_DIR=/x git push",
		"/usr/bin/git push",
		"git --no-pager push",
		"git -c user.name=x push",
		// A Git command the daemon cannot read is refused rather than guessed at.
		"git $(echo push)",
		"git push `echo origin`",
		"git ${CMD} origin",
	}
	for _, command := range refused {
		if rule, detail, breach := harness.RefMovingBreach(command); !breach {
			t.Errorf("%q was allowed, want it refused", command)
		} else if rule != harness.RuleRefMoving && rule != harness.RuleUnreadable {
			t.Errorf("%q was refused under %q (%s), want the ref rule or the unreadable rule", command, rule, detail)
		}
	}
}

func TestTheRefGuardAllowsWhatAMergeNeeds(t *testing.T) {
	allowed := []string{
		"git status",
		"git status --short",
		"git diff",
		"git diff --cached -- src/a.go",
		"git diff main...HEAD",
		"git log --oneline -n 20",
		"git log main..HEAD",
		"git show HEAD:src/a.go",
		"git merge-tree --write-tree main feature",
		"git merge-base main feature",
		"git ls-files -u",
		"git rev-parse HEAD",
		"git add src/a.go",
		"git add -A",
		"git rm old.go",
		"git mv a.go b.go",
		"git restore --staged src/a.go",
		"git restore src/a.go",
		"git commit -m 'resolve conflicts: keep both'",
		"git commit -m \"git push is not run here\"",
		"git commit --amend --no-edit",
		"git merge feature",
		"git merge --continue",
		"git merge --abort",
		"git cherry-pick abc123",
		"git revert abc123",
		"git checkout --ours -- src/a.go",
		"git checkout --theirs -- src/a.go",
		"git checkout --ours src/a.go",
		"git checkout -- src/a.go",
		"git checkout HEAD -- src/a.go",
		"git checkout feature -- src/a.go",
		"git reset",
		"git reset HEAD",
		"git reset HEAD -- src/a.go",
		"git reset -- src/a.go",
		"git reset -q",
		"git branch",
		"git branch --list",
		"git branch -a",
		"git branch -vv",
		"git branch --show-current",
		"git branch --contains abc123",
		"git branch --merged main",
		"git tag",
		"git tag -l",
		"git tag --list 'v*'",
		"git stash",
		"git stash push -m wip",
		"git stash list",
		"git stash show -p",
		"git stash apply",
		"git worktree list",
		"git reflog",
		"git reflog show HEAD",
		"git symbolic-ref HEAD",
		"git symbolic-ref --short HEAD",
		"git fetch",
		"git fetch origin",
		// Not Git at all.
		"ls -la",
		"go test ./...",
		"echo hello",
		"grep -rn push src",
		"",
	}
	for _, command := range allowed {
		if rule, detail, breach := harness.RefMovingBreach(command); breach {
			t.Errorf("%q was refused under %q (%s), want it allowed", command, rule, detail)
		}
	}
}

func TestTheRefGuardIsKeptInEveryModeAndRunsAheadOfTheMode(t *testing.T) {
	for _, mode := range []protocol.PermissionMode{
		protocol.PermissionModeAsk, protocol.PermissionModeAutoEdits, protocol.PermissionModeFullAuto,
		protocol.PermissionModeBypass,
	} {
		cfg := harness.Config{Mode: mode, Profile: security.DefaultProfile(), Blocklist: security.DefaultBlocklist(), ProtectRefs: true}
		got := cfg.Decide(harness.Request{Kind: "execute", Command: "git push origin integrator"})
		if got.Decision != harness.DecisionDeny || got.Rule != harness.RuleRefMoving {
			t.Errorf("%s: push = %+v, want a refusal under %s", mode, got, harness.RuleRefMoving)
		}
		if ok := cfg.Decide(harness.Request{Kind: "execute", Command: "git add -A"}); mode != protocol.PermissionModeAsk &&
			mode != protocol.PermissionModeAutoEdits && ok.Decision != harness.DecisionAllow {
			t.Errorf("%s: git add = %+v, want it allowed", mode, ok)
		}
	}
}

func TestWithoutProtectRefsAPushIsLeftToTheModeAndTheBlocklist(t *testing.T) {
	cfg := harness.Config{Mode: protocol.PermissionModeFullAuto, Profile: security.DefaultProfile(), Blocklist: security.DefaultBlocklist()}
	if got := cfg.Decide(harness.Request{Kind: "execute", Command: "git push origin feature"}); got.Decision != harness.DecisionAllow {
		t.Errorf("an ordinary session's push = %+v, want the mode to allow it", got)
	}
}

func TestTheIntegratorsRulesTogetherHoldItToItsWorkspace(t *testing.T) {
	root, workspace := "/data/worktrees/api", "/data/worktrees/api/integrator"
	containment := gitx.NewContainment(root, workspace, protocol.IntegrationBranchName, "main")
	cfg := harness.Config{
		Mode: protocol.PermissionModeFullAuto, Profile: security.DefaultProfile(), Blocklist: security.DefaultBlocklist(),
		ProtectRefs: true, Containment: &containment,
	}
	tests := []struct {
		name string
		req  harness.Request
		want harness.Decision
	}{
		{"an edit inside the workspace", harness.Request{Kind: "edit", Path: workspace + "/src/a.go"}, harness.DecisionAllow},
		{"a relative edit", harness.Request{Kind: "edit", Path: "src/a.go"}, harness.DecisionAllow},
		{"an edit in the owner's folder", harness.Request{Kind: "edit", Path: "/Users/me/api/src/a.go"}, harness.DecisionDeny},
		{"a commit", harness.Request{Kind: "execute", Command: "git commit -m done"}, harness.DecisionAllow},
		{"a merge", harness.Request{Kind: "execute", Command: "git merge card-7"}, harness.DecisionAllow},
		{"a push", harness.Request{Kind: "execute", Command: "git push origin integrator"}, harness.DecisionDeny},
		{"git run in the owner's folder", harness.Request{Kind: "execute", Command: "git -C /Users/me/api status"}, harness.DecisionDeny},
		{"a merge of the main branch", harness.Request{Kind: "execute", Command: "git merge main"}, harness.DecisionDeny},
		{"a recursive delete", harness.Request{Kind: "execute", Command: "rm -rf /"}, harness.DecisionDeny},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cfg.Decide(tc.req); got.Decision != tc.want {
				t.Errorf("Decide(%+v) = %+v, want %s", tc.req, got, tc.want)
			}
		})
	}
}
