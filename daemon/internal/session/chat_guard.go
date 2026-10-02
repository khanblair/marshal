package session

import (
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
)

// The rules a chat's agent is held to, from two sides. The harness decides every permission request
// an agent raises (HarnessConfigForChat). Claude Code raises none, because Marshal runs it with its
// permission prompts off, so for it the same rules are given as tool rules the program enforces
// itself (chatToolRules). Neither side is a sandbox: an agent that is handed a shell can still type
// anything the rules do not name.

// internalToolRule allows every tool of Marshal's own MCP server, whose name the agent knows it by.
const internalToolRule = "mcp__marshal"

// chatDecisionMode is the mode the harness decides a chat's requests with. The Orchestrator is
// plan-only whatever it was made with, because it runs in the owner's folder and must not change it.
// The Integrator decides as full auto, so a command inside its own workspace does not wait for a
// person; what it may not do is refused by its ref guard and its folder rule, in every mode. Another
// chat is decided by the mode it was made with.
func chatDecisionMode(kind chatKind, stored protocol.PermissionMode) protocol.PermissionMode {
	switch kind {
	case chatKindOrchestrator:
		return protocol.PermissionModePlan
	case chatKindIntegrator:
		return protocol.PermissionModeFullAuto
	}
	return stored
}

// HarnessConfigForChat answers the harness's view of a chat as it is now, for its session's
// permission requests and for the internal MCP server's checks. It answers false when the chat
// cannot be read or has no mode, and then every request is a person's.
func (m *Manager) HarnessConfigForChat(chatID string) (harness.Config, bool) {
	row, err := m.store.Queries().GetChat(m.ctx, chatID)
	if err != nil {
		if !store.IsNotFound(err) {
			m.log.Error("could not read a chat to decide a permission request", "chat_id", chatID, "error", err)
		}
		return harness.Config{}, false
	}
	if row.PermissionMode == "" {
		return harness.Config{}, false
	}
	kind := kindOfChat(row.System, row.TargetKind, row.TargetID)
	cfg := harness.Config{
		Mode:    chatDecisionMode(kind, protocol.PermissionMode(row.PermissionMode)),
		Profile: m.profile(), Blocklist: m.blocklist,
	}
	if kind != chatKindIntegrator {
		return cfg, true
	}
	cfg.ProtectRefs = true
	if guard, ok := m.chatGuardOf(chatID); ok {
		c := gitx.NewContainment(projects.WorktreesDir(m.cfg.DataDir, row.ProjectID),
			guard.worktree, protocol.IntegrationBranchName, guard.main)
		cfg.Containment = &c
	}
	return cfg, true
}

// chatToolRules are the tool rules a chat's agent is started with, in Claude Code's syntax (an
// adapter without such rules ignores them). Every chat with Marshal's server may call it. The
// Orchestrator may not edit files or run commands at all. The Integrator may edit in its own
// workspace and run the Git commands a merge needs, and is refused the ones that move refs: a deny
// rule wins over any allow rule the owner's own settings add.
func chatToolRules(kind chatKind, hasServer bool) (allowed, denied []string) {
	if hasServer {
		allowed = append(allowed, internalToolRule)
	}
	switch kind {
	case chatKindOrchestrator:
		denied = []string{"Edit", "Write", "MultiEdit", "NotebookEdit", "Bash"}
	case chatKindIntegrator:
		allowed = append(allowed, integratorGitAllowed()...)
		denied = integratorGitDenied()
	}
	return allowed, denied
}

// integratorGitAllowed are the Git commands the Integrator needs to inspect a merge, resolve it, and
// finish it in its own workspace. Anything else it is not told it may run is refused, because
// Marshal runs the agent with its permission prompts off. Claude Code matches a rule by how the
// command starts, so a rule must not be one that a dangerous flag can follow: "reset HEAD" and
// "checkout --" are left out, and the agent unstages and restores with "git restore" instead.
func integratorGitAllowed() []string {
	return gitRules("status", "diff", "log", "show", "ls-files", "ls-tree", "merge-tree", "merge-base",
		"rev-parse", "rev-list", "blame", "grep", "add", "rm", "mv", "restore", "commit", "merge",
		"cherry-pick", "revert", "apply", "am", "stash push", "stash list", "stash show", "stash apply",
		"checkout --ours", "checkout --theirs", "branch --show-current")
}

// integratorGitDenied are the Git commands the Integrator may never run: the ones that move or delete
// a ref, change a worktree, rewrite history, or reach a remote. Claude Code matches a rule by the
// start of the command, so these name the forms that are unambiguous; the harness's own check
// (harness.RefMovingBreach) reads the rest for an agent that asks first.
func integratorGitDenied() []string {
	return gitRules("push", "pull", "fetch", "rebase", "update-ref", "symbolic-ref", "switch",
		"checkout -b", "checkout -B", "checkout --orphan", "checkout --detach",
		"branch -f", "branch -d", "branch -D", "branch -m", "branch -M", "branch -c", "branch -C",
		"branch --force", "branch --delete", "branch --move", "branch --copy",
		"reset --hard", "reset --merge", "reset --keep", "reset --soft",
		"worktree remove", "worktree prune", "worktree add", "worktree move", "worktree lock",
		"stash drop", "stash clear", "stash pop", "stash branch",
		"tag -d", "tag -f", "tag --delete", "tag --force",
		"reflog expire", "reflog delete", "gc", "prune", "filter-branch", "replace",
		"remote add", "remote remove", "remote rm", "remote rename", "remote set-url", "config")
}

// gitRules writes each Git command prefix as a Claude Code Bash rule.
func gitRules(prefixes ...string) []string {
	rules := make([]string, len(prefixes))
	for i, prefix := range prefixes {
		rules[i] = "Bash(git " + prefix + ":*)"
	}
	return rules
}
