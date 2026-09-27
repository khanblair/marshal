package harness_test

import (
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
)

// The harness is where a permission decision is made (docs/architecture.md section 13). These tests
// are the mode table of docs/marshal-product-scope.md section 14.1 as a set of expectations: each
// mode against a file write and a command, and each guard against the modes it overrides.

// base is the config Marshal ships with, in a given mode: the shipped profile, the shipped
// blocklist, and no worktree (a session that is not a card's).
func base(mode protocol.PermissionMode) harness.Config {
	return harness.Config{
		Mode: mode, Profile: security.DefaultProfile(), Blocklist: security.DefaultBlocklist(),
	}
}

// readRequest, editRequest, and commandRequest are the three shapes every mode is tested against.
func readRequest() harness.Request { return harness.Request{Kind: "read", Path: "README.md"} }
func editRequest() harness.Request { return harness.Request{Kind: "edit", Path: "src/main.go"} }
func commandRequest() harness.Request {
	return harness.Request{Kind: "execute", Command: "go test ./..."}
}
func networkRequest() harness.Request {
	return harness.Request{Kind: "fetch", Path: "https://example.com"}
}

func TestEveryModeAgainstAFileWriteAndACommand(t *testing.T) {
	tests := []struct {
		mode    protocol.PermissionMode
		req     harness.Request
		want    harness.Decision
		because string
	}{
		// Ask: reading is allowed, every edit and command is the person's.
		{protocol.PermissionModeAsk, readRequest(), harness.DecisionAllow, "reading is not one of the things Ask asks about"},
		{protocol.PermissionModeAsk, editRequest(), harness.DecisionAsk, "Ask asks before every edit"},
		{protocol.PermissionModeAsk, commandRequest(), harness.DecisionAsk, "Ask asks before every command"},

		// Auto-accept edits: edits are allowed, everything else is the person's.
		{protocol.PermissionModeAutoEdits, editRequest(), harness.DecisionAllow, "auto-accept edits allows an edit"},
		{protocol.PermissionModeAutoEdits, commandRequest(), harness.DecisionAsk, "auto-accept edits still asks about a command"},
		{protocol.PermissionModeAutoEdits, readRequest(), harness.DecisionAllow, "reading is allowed"},

		// Plan only: reading is allowed and nothing else is, but a plan is refused rather than asked.
		{protocol.PermissionModePlan, readRequest(), harness.DecisionAllow, "plan only allows reading"},
		{protocol.PermissionModePlan, editRequest(), harness.DecisionDeny, "plan only refuses an edit"},
		{protocol.PermissionModePlan, commandRequest(), harness.DecisionDeny, "plan only refuses a command"},

		// Full auto: what the rules have not refused is allowed.
		{protocol.PermissionModeFullAuto, editRequest(), harness.DecisionAllow, "full auto allows an edit"},
		{protocol.PermissionModeFullAuto, commandRequest(), harness.DecisionAllow, "full auto allows a command"},
		{protocol.PermissionModeFullAuto, readRequest(), harness.DecisionAllow, "full auto allows reading"},

		// Bypass: the guards are skipped, so the request is the daemon's to allow.
		{protocol.PermissionModeBypass, editRequest(), harness.DecisionAllow, "bypass allows an edit"},
		{protocol.PermissionModeBypass, commandRequest(), harness.DecisionAllow, "bypass allows a command"},
		{protocol.PermissionModeBypass, readRequest(), harness.DecisionAllow, "bypass allows reading"},

		// A network request is neither a read nor an edit, so each mode treats it as a command.
		{protocol.PermissionModeAsk, networkRequest(), harness.DecisionAsk, "Ask asks about a network request"},
		{protocol.PermissionModePlan, networkRequest(), harness.DecisionDeny, "plan only refuses a network request"},
		{protocol.PermissionModeFullAuto, networkRequest(), harness.DecisionAllow, "full auto allows a network request"},

		// A mode the daemon does not know is treated as the careful one.
		{protocol.PermissionMode("something-new"), commandRequest(), harness.DecisionAsk, "an unknown mode asks"},
		{protocol.PermissionMode(""), commandRequest(), harness.DecisionAsk, "no mode at all asks"},
		{protocol.PermissionMode("something-new"), readRequest(), harness.DecisionAsk, "an unknown mode asks even about a read"},
		{protocol.PermissionMode(""), readRequest(), harness.DecisionAsk, "no mode at all asks even about a read"},
		{protocol.PermissionMode("something-new"), editRequest(), harness.DecisionAsk, "an unknown mode asks about an edit"},
		{protocol.PermissionMode(""), editRequest(), harness.DecisionAsk, "no mode at all asks about an edit"},
	}
	for _, tt := range tests {
		name := string(tt.mode) + "/" + tt.req.Kind
		t.Run(name, func(t *testing.T) {
			got := base(tt.mode).Decide(tt.req)
			if got.Decision != tt.want {
				t.Errorf("decide = %s (%s), want %s: %s", got.Decision, got.Rule, tt.want, tt.because)
			}
		})
	}
}

// TestTheGuardsApplyInEveryModeButBypass is the heart of B3.3 and B3.7: a blocked command and a
// deploy are not things a mode can wave through. A blocked command is refused, and a deploy is a
// person's, in ask, auto-accept edits, plan only, and full auto alike - and only bypass skips them.
func TestTheGuardsApplyInEveryModeButBypass(t *testing.T) {
	modes := []protocol.PermissionMode{
		protocol.PermissionModeAsk, protocol.PermissionModeAutoEdits,
		protocol.PermissionModePlan, protocol.PermissionModeFullAuto,
	}
	for _, mode := range modes {
		t.Run(string(mode)+"/blocked", func(t *testing.T) {
			got := base(mode).Decide(harness.Request{Kind: "execute", Command: "rm -rf /"})
			if got.Decision != harness.DecisionDeny || got.Rule != security.RuleRecursiveDelete {
				t.Errorf("a blocked command in %s = %s (%s), want deny by %s", mode, got.Decision, got.Rule, security.RuleRecursiveDelete)
			}
		})
		t.Run(string(mode)+"/deploy", func(t *testing.T) {
			for _, command := range []string{"npm run deploy", "cd infra && terraform apply", "ls; vercel --prod"} {
				got := base(mode).Decide(harness.Request{Kind: "execute", Command: command})
				if got.Decision != harness.DecisionAsk || got.Rule != security.RuleDeploy {
					t.Errorf("%q in %s = %s (%s), want ask by %s", command, mode, got.Decision, got.Rule, security.RuleDeploy)
				}
			}
		})
	}

	t.Run("bypass/blocked", func(t *testing.T) {
		got := base(protocol.PermissionModeBypass).Decide(harness.Request{Kind: "execute", Command: "rm -rf /"})
		if got.Decision != harness.DecisionAllow || got.Rule != harness.RuleBypass {
			t.Errorf("bypass refuses a blocked command: %s (%s), want it skipped", got.Decision, got.Rule)
		}
	})
	t.Run("bypass/deploy", func(t *testing.T) {
		got := base(protocol.PermissionModeBypass).Decide(harness.Request{Kind: "execute", Command: "npm run deploy"})
		if got.Decision != harness.DecisionAllow {
			t.Errorf("bypass asks about a deploy: %s (%s), want it skipped", got.Decision, got.Rule)
		}
	})
}

// TestAProfileRefusesInEveryModeButBypass covers the profile half of B3.3: what a profile does not
// allow is blocked, and no mode widens it.
func TestAProfileRefusesInEveryModeButBypass(t *testing.T) {
	for _, mode := range []protocol.PermissionMode{
		protocol.PermissionModeAsk, protocol.PermissionModeAutoEdits,
		protocol.PermissionModePlan, protocol.PermissionModeFullAuto,
	} {
		cfg := base(mode)
		cfg.Profile = security.Profile{ReadFiles: true}
		got := cfg.Decide(editRequest())
		if got.Decision != harness.DecisionDeny || got.Rule != security.RuleProfile {
			t.Errorf("a refused edit in %s = %s (%s), want deny by %s", mode, got.Decision, got.Rule, security.RuleProfile)
		}
	}

	cfg := base(protocol.PermissionModeBypass)
	cfg.Profile = security.NothingAllowed()
	if got := cfg.Decide(editRequest()); got.Decision != harness.DecisionAllow {
		t.Errorf("bypass honoured a profile refusal: %s (%s), want it skipped", got.Decision, got.Rule)
	}
}

// card is a containment for a card whose worktree is inside the worktrees root, on its own branch.
func card() gitx.Containment {
	return gitx.NewContainment("/data/worktrees/p1", "/data/worktrees/p1/card-1", "marshal/card-1", "main")
}

// TestTheWorktreeRuleIsKeptInEveryModeIncludingBypass is what bypass never gives up (B3.2): a file
// outside the card's worktree, and a Git command that would touch the main branch, are refused even
// in bypass - and the blocklist is skipped there, so this cannot be left to the blocklist.
func TestTheWorktreeRuleIsKeptInEveryModeIncludingBypass(t *testing.T) {
	modes := []protocol.PermissionMode{
		protocol.PermissionModeAsk, protocol.PermissionModeAutoEdits, protocol.PermissionModePlan,
		protocol.PermissionModeFullAuto, protocol.PermissionModeBypass,
	}
	for _, mode := range modes {
		cfg := base(mode)
		containment := card()
		cfg.Containment = &containment

		t.Run(string(mode)+"/outside", func(t *testing.T) {
			got := cfg.Decide(harness.Request{Kind: "edit", Path: "/etc/passwd"})
			if got.Decision != harness.DecisionDeny || got.Rule != gitx.RuleOutsideWorktree {
				t.Errorf("an edit outside the worktree = %s (%s), want deny by %s", got.Decision, got.Rule, gitx.RuleOutsideWorktree)
			}
		})
		t.Run(string(mode)+"/main-branch", func(t *testing.T) {
			got := cfg.Decide(harness.Request{Kind: "execute", Command: "git push origin main"})
			if got.Decision != harness.DecisionDeny || got.Rule != gitx.RuleMainBranch {
				t.Errorf("a push to main = %s (%s), want deny by %s", got.Decision, got.Rule, gitx.RuleMainBranch)
			}
		})
		t.Run(string(mode)+"/wrapped-main-branch", func(t *testing.T) {
			got := cfg.Decide(harness.Request{Kind: "execute", Command: "env git push origin main"})
			if got.Decision != harness.DecisionDeny || got.Rule != gitx.RuleMainBranch {
				t.Errorf("a wrapped push to main = %s (%s), want deny by %s", got.Decision, got.Rule, gitx.RuleMainBranch)
			}
		})
		t.Run(string(mode)+"/wrapper-option-main-branch", func(t *testing.T) {
			for _, command := range []string{
				"env -i git push origin main",
				"sudo -u root git push origin main",
				"nice -n 5 git push origin main",
			} {
				got := cfg.Decide(harness.Request{Kind: "execute", Command: command})
				if got.Decision != harness.DecisionDeny || got.Rule != gitx.RuleMainBranch {
					t.Errorf("%q = %s (%s), want deny by %s", command, got.Decision, got.Rule, gitx.RuleMainBranch)
				}
			}
		})
		t.Run(string(mode)+"/quote-joined-main-branch", func(t *testing.T) {
			got := cfg.Decide(harness.Request{Kind: "execute", Command: `git push origin ma''in`})
			if got.Decision != harness.DecisionDeny || got.Rule != gitx.RuleMainBranch {
				t.Errorf("a quote-joined push to main = %s (%s), want deny by %s", got.Decision, got.Rule, gitx.RuleMainBranch)
			}
		})
		t.Run(string(mode)+"/git-behind-another-command", func(t *testing.T) {
			// The git program is found wherever the line puts it, not only as the first word.
			got := cfg.Decide(harness.Request{Kind: "execute", Command: "cd /tmp && git push origin main"})
			if got.Decision != harness.DecisionDeny || got.Rule != gitx.RuleMainBranch {
				t.Errorf("a push behind another command = %s (%s), want deny by %s", got.Decision, got.Rule, gitx.RuleMainBranch)
			}
		})
		t.Run(string(mode)+"/git-past-wrapper-options", func(t *testing.T) {
			got := cfg.Decide(harness.Request{Kind: "execute", Command: "sudo -u root git push origin main"})
			if got.Decision != harness.DecisionDeny || got.Rule != gitx.RuleMainBranch {
				t.Errorf("a push past a wrapper's options = %s (%s), want deny by %s", got.Decision, got.Rule, gitx.RuleMainBranch)
			}
		})
		t.Run(string(mode)+"/ref-through-a-variable", func(t *testing.T) {
			// A ref named through a variable cannot be read, so it is refused rather than guessed at:
			// it could move the main branch without ever saying so.
			got := cfg.Decide(harness.Request{Kind: "execute", Command: "git push origin $BRANCH"})
			if got.Decision != harness.DecisionDeny || got.Rule != harness.RuleUnreadable {
				t.Errorf("a push naming its ref through a variable = %s (%s), want deny by %s", got.Decision, got.Rule, harness.RuleUnreadable)
			}
		})
		t.Run(string(mode)+"/git-inside-a-substitution", func(t *testing.T) {
			got := cfg.Decide(harness.Request{Kind: "execute", Command: "echo `git push origin main`"})
			if got.Decision != harness.DecisionDeny || got.Rule != harness.RuleUnreadable {
				t.Errorf("a git command inside a substitution = %s (%s), want deny by %s", got.Decision, got.Rule, harness.RuleUnreadable)
			}
		})
		t.Run(string(mode)+"/branch-with-a-slash-is-not-main", func(t *testing.T) {
			got := cfg.Decide(harness.Request{Kind: "execute", Command: "git push origin feature/main"})
			if got.Rule == gitx.RuleMainBranch || got.Rule == harness.RuleUnreadable {
				t.Errorf("a push to feature/main was refused by %s in %s", got.Rule, mode)
			}
		})
		t.Run(string(mode)+"/commit-message-mentions-main", func(t *testing.T) {
			// A commit message is one argument, so naming the main branch inside it is not a push to
			// it - and a parenthesis inside the quotes is text, not a command separator. What the mode
			// then does with the commit is the mode's own business.
			for _, command := range []string{
				`git commit -m "work on main"`,
				`git commit -m "see (git push origin main)"`,
				`echo "(git push origin main)"`,
			} {
				got := cfg.Decide(harness.Request{Kind: "execute", Command: command})
				if got.Rule == gitx.RuleMainBranch || got.Rule == gitx.RuleOutsideWorktree {
					t.Errorf("%q was refused by %s in %s", command, got.Rule, mode)
				}
			}
		})

		t.Run(string(mode)+"/chained-main-branch", func(t *testing.T) {
			got := cfg.Decide(harness.Request{Kind: "execute", Command: "ls && git push origin main"})
			if got.Decision != harness.DecisionDeny || got.Rule != gitx.RuleMainBranch {
				t.Errorf("a chained push to main = %s (%s), want deny by %s", got.Decision, got.Rule, gitx.RuleMainBranch)
			}
		})
		t.Run(string(mode)+"/inside", func(t *testing.T) {
			// An edit inside the worktree is not the worktree rule's business; what the mode then does
			// with it (allow, ask, or a plan's refusal) is the mode's own business.
			got := cfg.Decide(harness.Request{Kind: "edit", Path: "src/main.go"})
			if got.Rule == gitx.RuleOutsideWorktree || got.Rule == gitx.RuleMainBranch {
				t.Errorf("an edit inside the worktree was refused by %s in %s", got.Rule, mode)
			}
		})
	}
}

// TestAChatHasNoWorktreeRule covers the other half of the same coin: a session with no card has no
// containment, so nothing is refused for a reason that does not exist for it.
func TestAChatHasNoWorktreeRule(t *testing.T) {
	cfg := base(protocol.PermissionModeAutoEdits)
	if got := cfg.Decide(harness.Request{Kind: "edit", Path: "/etc/passwd"}); got.Decision != harness.DecisionAllow {
		t.Errorf("a session with no worktree refused an edit as %s (%s), want it allowed", got.Decision, got.Rule)
	}
}

// TestAnEmptyBlocklistHasNothingToSay covers a config with no list: the mode decides, and a request
// the mode allows is allowed rather than refused by a rule that is not there.
func TestAnEmptyBlocklistHasNothingToSay(t *testing.T) {
	cfg := base(protocol.PermissionModeFullAuto)
	cfg.Blocklist = &security.Blocklist{}
	got := cfg.Decide(harness.Request{Kind: "execute", Command: "rm -rf /"})
	if got.Decision != harness.DecisionAllow {
		t.Errorf("an empty blocklist refused a command: %s (%s)", got.Decision, got.Rule)
	}
}

// TestABlockedCommandCannotHideInALine covers the rule that a dangerous command is refused even in
// the mode that allows the most, however it is spelled: joined to another command, wrapped, or run
// inside a shell's own script.
func TestABlockedCommandCannotHideInALine(t *testing.T) {
	for _, command := range []string{
		"git status && rm -rf /",
		"bash -c 'rm -rf /'",
		"env rm -rf /",
		"ls; chmod -R 777 /",
	} {
		t.Run(command, func(t *testing.T) {
			got := base(protocol.PermissionModeFullAuto).Decide(harness.Request{Kind: "execute", Command: command})
			if got.Decision != harness.DecisionDeny {
				t.Errorf("%q = %s (%s), want a refusal", command, got.Decision, got.Rule)
			}
		})
	}
}
