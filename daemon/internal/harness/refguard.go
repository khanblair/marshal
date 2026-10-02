package harness

import (
	"path/filepath"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/security"
)

// RuleRefMoving is the rule a Git command is refused under when it would move or delete a branch, a
// tag, or a worktree, or rewrite what a branch holds. It is for an agent that must change only its
// own working folder, such as the Integrator, and it is kept in every mode.
const RuleRefMoving = "ref-moving-command"

// RefMovingBreach says whether a command line holds a Git command that moves refs: a push, a pull,
// a rebase, a hard reset, a branch or tag delete or force, a checkout or switch to another branch,
// update-ref, a worktree change, or a stash drop. It reads every command in the line and finds the
// Git program wherever the line puts it, as GitCommandBreach does.
//
// A Git command that cannot be read (a substitution, a variable where a word should be) is refused
// under RuleUnreadable. A line with no Git command is left alone.
func RefMovingBreach(command string) (rule, detail string, breach bool) {
	for _, segment := range security.Segments(command) {
		args := security.Argv(segment)
		at := -1
		for i, arg := range args {
			if filepath.Base(arg) == "git" {
				at = i
				break
			}
		}
		if at < 0 {
			continue
		}
		if !security.Readable(segment) {
			return RuleUnreadable, "a Git command that cannot be read", true
		}
		sub, rest := gitSubcommandOf(args[at+1:])
		if detail, bad := refMovingSubcommand(sub, rest); bad {
			return RuleRefMoving, detail, true
		}
	}
	return "", "", false
}

// gitSubcommandOf separates the global options of a Git command from its subcommand and the
// subcommand's own arguments.
func gitSubcommandOf(args []string) (sub string, rest []string) {
	takesValue := map[string]bool{
		"-C": true, "-c": true, "--git-dir": true, "--work-tree": true, "--namespace": true,
		"--exec-path": true, "--config-env": true, "--attr-source": true, "--super-prefix": true,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return "", nil
		}
		if !strings.HasPrefix(arg, "-") {
			return arg, args[i+1:]
		}
		if takesValue[arg] {
			i++
		}
	}
	return "", nil
}

// refMovingSubcommand reads one Git subcommand and says what in it moves a ref.
func refMovingSubcommand(sub string, rest []string) (string, bool) {
	switch sub {
	case "push", "pull", "rebase", "update-ref", "switch", "filter-branch", "filter-repo", "gc",
		"prune", "replace":
		return sub, true
	case "fetch":
		return fetchMoves(rest)
	case "branch":
		return branchMoves(rest)
	case "tag":
		return tagMoves(rest)
	case "checkout":
		return checkoutMoves(rest)
	case "reset":
		return resetMoves(rest)
	case "worktree":
		return onlyActions(sub, rest, "list")
	case "stash":
		return stashMoves(rest)
	case "reflog":
		return reflogMoves(rest)
	case "symbolic-ref":
		return symbolicRefMoves(rest)
	}
	return "", false
}

// positional returns the arguments that are not flags, before any "--".
func positional(rest []string) []string {
	var out []string
	for _, arg := range rest {
		if arg == "--" {
			break
		}
		if !strings.HasPrefix(arg, "-") {
			out = append(out, arg)
		}
	}
	return out
}

// hasFlag says whether any argument is one of the long flags (with or without "=value") or carries
// one of the short flag letters, so "-fd" is both "-f" and "-d".
func hasFlag(rest []string, long []string, short string) bool {
	for _, arg := range rest {
		if arg == "--" {
			return false
		}
		if strings.HasPrefix(arg, "--") {
			name, _, _ := strings.Cut(arg, "=")
			for _, want := range long {
				if name == want {
					return true
				}
			}
			continue
		}
		if strings.HasPrefix(arg, "-") && len(arg) > 1 && strings.ContainsAny(arg[1:], short) && short != "" {
			return true
		}
	}
	return false
}

// hasSeparator says whether the arguments hold a "--", which makes what follows a path.
func hasSeparator(rest []string) bool {
	for _, arg := range rest {
		if arg == "--" {
			return true
		}
	}
	return false
}

// onlyActions refuses a subcommand whose own action is not one of the read-only ones given.
func onlyActions(sub string, rest []string, allowed ...string) (string, bool) {
	action := ""
	for _, arg := range rest {
		if !strings.HasPrefix(arg, "-") {
			action = arg
			break
		}
	}
	for _, ok := range allowed {
		if action == ok {
			return "", false
		}
	}
	return strings.TrimSpace(sub + " " + action), true
}

// fetchMoves refuses a fetch that names a destination, which can move a local branch, and one that
// forces or updates the current head.
func fetchMoves(rest []string) (string, bool) {
	for _, arg := range rest {
		if strings.Contains(arg, ":") && !strings.HasPrefix(arg, "-") {
			return "fetch " + arg, true
		}
	}
	if hasFlag(rest, []string{"--force", "--update-head-ok", "--prune"}, "fup") {
		return "fetch with a force or update flag", true
	}
	return "", false
}

// branchMoves refuses every form of "git branch" except listing: a create, a delete, a move, a copy,
// a force, and a change of upstream all change a ref.
func branchMoves(rest []string) (string, bool) {
	if hasFlag(rest, []string{"--delete", "--force", "--move", "--copy", "--set-upstream-to",
		"--unset-upstream", "--edit-description"}, "dDfmMcCu") {
		return "branch with a delete, move, copy, or force flag", true
	}
	listing := hasFlag(rest, []string{"--list", "--contains", "--no-contains", "--merged", "--no-merged",
		"--points-at", "--show-current", "--all", "--remotes", "--verbose"}, "lavr")
	if len(positional(rest)) > 0 && !listing {
		return "branch " + positional(rest)[0], true
	}
	return "", false
}

// tagMoves refuses a tag that is made, deleted, or forced; listing is left alone.
func tagMoves(rest []string) (string, bool) {
	if hasFlag(rest, []string{"--delete", "--force"}, "df") {
		return "tag with a delete or force flag", true
	}
	listing := hasFlag(rest, []string{"--list", "--contains", "--no-contains", "--merged", "--no-merged",
		"--points-at"}, "l")
	if len(positional(rest)) > 0 && !listing {
		return "tag " + positional(rest)[0], true
	}
	return "", false
}

// checkoutMoves refuses a checkout that could change which branch is checked out. A checkout of
// files is left alone: one with a "--" before the paths, or with --ours, --theirs, or --patch, which
// only ever name files.
func checkoutMoves(rest []string) (string, bool) {
	if hasFlag(rest, []string{"--orphan", "--detach", "--track", "--no-track"}, "bBd") {
		return "checkout that makes or leaves a branch", true
	}
	if hasSeparator(rest) || hasFlag(rest, []string{"--ours", "--theirs", "--patch", "--conflict"}, "p") {
		return "", false
	}
	if len(positional(rest)) == 0 {
		return "", false
	}
	return "checkout " + positional(rest)[0], true
}

// resetMoves refuses a reset that moves the current branch or throws working changes away. A reset
// that only unstages paths ("reset HEAD file", "reset -- file") is left alone.
func resetMoves(rest []string) (string, bool) {
	if hasFlag(rest, []string{"--hard", "--merge", "--keep", "--soft"}, "") {
		return "reset with a flag that moves the branch", true
	}
	if hasSeparator(rest) {
		return "", false
	}
	pos := positional(rest)
	if len(pos) == 0 || pos[0] == "HEAD" {
		return "", false
	}
	return "reset " + pos[0], true
}

// stashMoves refuses the stash actions that drop an entry or make a branch.
func stashMoves(rest []string) (string, bool) {
	switch action := firstPositional(rest); action {
	case "drop", "clear", "pop", "branch":
		return "stash " + action, true
	}
	return "", false
}

// reflogMoves refuses the reflog actions that delete entries.
func reflogMoves(rest []string) (string, bool) {
	switch action := firstPositional(rest); action {
	case "expire", "delete":
		return "reflog " + action, true
	}
	return "", false
}

// symbolicRefMoves refuses a symbolic-ref that writes: one with a value to set, or a delete.
func symbolicRefMoves(rest []string) (string, bool) {
	if hasFlag(rest, []string{"--delete"}, "d") || len(positional(rest)) > 1 {
		return "symbolic-ref that writes", true
	}
	return "", false
}

// firstPositional is the first argument that is not a flag.
func firstPositional(rest []string) string {
	for _, arg := range rest {
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
	}
	return ""
}
