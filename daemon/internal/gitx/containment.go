package gitx

import (
	"fmt"
	"path/filepath"
	"strings"
)

// The rules Containment enforces. They are stable names, so a refusal can be named in a log or an
// audit row and a test can say which rule it expected.
const (
	// RuleOutsideWorktree is a command that would work on a repository, a working folder, or a
	// repository setting that is not the card's own worktree.
	RuleOutsideWorktree = "outside-worktree"
	// RuleMainBranch is a command that names, moves, or fetches the project's main branch.
	RuleMainBranch = "main-branch"
	// RuleForcePush is a push that overrides what a remote already has.
	RuleForcePush = "force-push"
)

// Violation is why a command was refused: the rule that was broken, and a short phrase naming what
// broke it. It is a value, not a pointer, so a caller can compare or store it cheaply.
type Violation struct {
	// Rule is one of the Rule constants.
	Rule string
	// Detail names the subcommand and, where there is one, the argument that broke the rule.
	Detail string
}

func (v Violation) Error() string {
	return fmt.Sprintf("refused by the %s rule: %s", v.Rule, v.Detail)
}

// Containment is the one rule bypass mode keeps (docs/backend-checklist.md B3.2): bypass is the
// place a person lets an agent run commands without being asked, and even there the agent stays
// inside one card's worktree and nothing touches the project's main branch.
//
// It has two halves, and both are checks on something the daemon knows rather than a sandbox:
//
//   - CheckWorktrees reads what Git itself says about a repository's working folders. Bypass only
//     ever applies to a folder under the worktrees root, checked out on the card's own branch. A
//     card whose worktree has been pointed at the main branch - by a person, by another tool, or by
//     a mistake - is refused before its agent is started, and the session manager says so plainly.
//   - CheckCommand inspects a Git command line before it runs: an argument list, not a shell string,
//     so nothing has to be parsed or unquoted. It is the rule every Git command Marshal runs for a
//     card passes through, and the rule the harness uses to refuse one an agent asks to run in a
//     mode that is not bypass (B3.3).
//
// What it is not: an agent that brings its own shell can still type anything, and Marshal does not
// pretend otherwise. Containment is what the daemon can hold to, and it is what it holds to.
type Containment struct {
	// Worktree is the card's own worktree folder, as a full path. Nothing runs outside it, and
	// nothing it holds is touched from outside.
	Worktree string
	// Branch is the card's own branch, the only branch it may work on.
	Branch string
	// Main is the project's default branch. Reading it is fine; naming it where it could move is
	// not.
	Main string
	// Root is the folder Marshal's worktrees of a project are made under. A worktree that is not
	// inside it was not made by Marshal and is refused.
	Root string
}

// NewContainment builds the containment for one card. The folders are cleaned, so two spellings of
// the same folder compare equal. Symbolic links are not resolved: Marshal makes the worktree
// itself, under the data folder it was given.
func NewContainment(root, worktree, branch, main string) Containment {
	return Containment{
		Worktree: filepath.Clean(worktree),
		Branch:   strings.TrimSpace(branch),
		Main:     strings.TrimSpace(main),
		Root:     filepath.Clean(root),
	}
}

// CheckWorktrees finds the card's worktree in what Git reports for the repository and checks it
// against the rule. It is given the whole list (gitx.ListWorktrees) so the caller asks Git once.
//
// It refuses a worktree that is not there, is not under the worktrees root, is detached, is bare,
// or is checked out on any branch other than the card's own - including the project's main branch,
// which is the case this exists for.
func (c Containment) CheckWorktrees(list []Worktree) error {
	if c.Worktree == "" || c.Root == "" {
		return Violation{Rule: RuleOutsideWorktree, Detail: "containment has no worktree or no worktrees root"}
	}
	var found *Worktree
	for i := range list {
		if sameFolderAs(list[i].Path, c.Worktree) {
			found = &list[i]
			break
		}
	}
	if found == nil {
		return Violation{Rule: RuleOutsideWorktree, Detail: "the card's worktree is not one Git knows: " + c.Worktree}
	}
	if relationOf(c.Root, found.Path) == unrelated {
		return Violation{Rule: RuleOutsideWorktree, Detail: "the worktree is outside the worktrees root: " + found.Path}
	}
	if found.Bare {
		return Violation{Rule: RuleOutsideWorktree, Detail: "the worktree is a bare repository: " + found.Path}
	}
	if found.Detached || found.Branch == "" {
		return Violation{Rule: RuleOutsideWorktree, Detail: "the worktree has no branch checked out: " + found.Path}
	}
	if found.Branch != c.Branch {
		return Violation{Rule: RuleMainBranch, Detail: fmt.Sprintf("the worktree is on %s, not on the card's branch %s", found.Branch, c.Branch)}
	}
	if c.Main != "" && found.Branch == c.Main {
		return Violation{Rule: RuleMainBranch, Detail: "the worktree is on the main branch " + c.Main}
	}
	return nil
}

// CheckCommand refuses a Git command line that would leave the card's worktree or touch the main
// branch. It takes the arguments after "git" - for example {"push", "origin", "main"} - so what is
// checked is the argument list the caller is about to run, never a shell string.
//
// A command it does not recognize is allowed: this refuses the shapes that are known to be
// dangerous, and the modes that are not bypass refuse more than this (B3.3).
func (c Containment) CheckCommand(args []string) error {
	globals, sub, rest := splitGitArgs(args)
	if err := c.checkGlobals(globals); err != nil {
		return err
	}
	if sub == "" {
		return nil
	}
	if err := c.checkSubcommand(sub, rest); err != nil {
		return err
	}
	if !namesRefs(sub) {
		return nil
	}
	return c.checkRefs(sub, rest)
}

// checkGlobals refuses the options that point Git at another repository or another working folder.
// "-C" is allowed only when it names the card's own worktree, because running there is the point;
// "--git-dir" and "--work-tree" are never allowed, because a worktree's own git directory is not
// inside the worktree and Marshal always has a reason to run in one place.
func (c Containment) checkGlobals(globals []string) error {
	for i := 0; i < len(globals); i++ {
		name, value, hasValue := strings.Cut(globals[i], "=")
		switch name {
		case "-C", "--work-tree", "--git-dir", "--namespace":
			if !hasValue {
				i++
				if i >= len(globals) {
					return Violation{Rule: RuleOutsideWorktree, Detail: name + " has no value"}
				}
				value = globals[i]
			}
			if name == "-C" && c.inside(value) {
				continue
			}
			return Violation{Rule: RuleOutsideWorktree, Detail: name + " " + value}
		}
	}
	return nil
}

// checkSubcommand refuses the subcommands that build a repository somewhere else, reach into
// another one, or rewrite history wholesale. The read-only forms of them ("worktree list", "remote
// -v") are allowed, because they change nothing.
func (c Containment) checkSubcommand(sub string, rest []string) error {
	switch sub {
	case "clone", "init", "filter-branch", "daemon", "instaweb", "repack":
		return Violation{Rule: RuleOutsideWorktree, Detail: sub}
	case "worktree":
		if action := subAction(rest); action != "" && action != "list" {
			return Violation{Rule: RuleOutsideWorktree, Detail: "worktree " + action}
		}
	case "submodule":
		if action := subAction(rest); action != "" && action != "status" && action != "summary" {
			return Violation{Rule: RuleOutsideWorktree, Detail: "submodule " + action}
		}
	case "remote":
		switch subAction(rest) {
		case "add", "remove", "rm", "rename", "set-url", "set-branches", "set-head", "prune", "update":
			return Violation{Rule: RuleOutsideWorktree, Detail: "remote " + subAction(rest)}
		}
	case "config":
		// A repository setting belongs to the whole repository, so bypass changes none of them: the
		// hooks a commit runs, the folder a worktree fetches from, and the remote a push goes to are
		// all outside one card's worktree. Reading one is fine.
		if !configReads(rest) {
			return Violation{Rule: RuleOutsideWorktree, Detail: "config " + strings.Join(rest, " ")}
		}
	}
	return nil
}

// namesRefs says whether a subcommand takes a branch or a ref in a way that could move one. The
// subcommands that only read - log, show, diff, status, rev-parse, merge-base, cat-file - are not
// among them, so they never meet the main-branch rule.
func namesRefs(sub string) bool {
	switch sub {
	case "checkout", "switch", "merge", "rebase", "reset", "branch", "tag", "push", "fetch", "pull",
		"update-ref", "symbolic-ref", "cherry-pick", "revert", "stash", "am", "apply", "commit-tree",
		"reflog", "replace", "notes", "worktree":
		return true
	}
	return false
}

// checkRefs refuses an argument that names the main branch, and for a push it also refuses the
// flags that override a remote. Each argument is compared as a whole, so a commit message that
// mentions the main branch ("git commit -m main") is the only shape that can be caught by mistake;
// the value of -m and --message is skipped for exactly that reason.
func (c Containment) checkRefs(sub string, rest []string) error {
	if readsOnly(sub, rest) {
		return nil
	}
	if sub == "push" {
		if err := c.checkPushFlags(rest); err != nil {
			return err
		}
	}
	skipNext := false
	messageFlags := messageFlagSubcommands[sub]
	for _, arg := range rest {
		if skipNext {
			skipNext = false
			continue
		}
		if messageFlags && (arg == "-m" || arg == "--message") {
			skipNext = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			// A flag that takes a branch only to read it ("branch --contains main") names nothing
			// that could move.
			if refValueFlags[arg] {
				skipNext = true
			}
			continue
		}
		if sub == "push" {
			if c.pushTargetsMain(arg) {
				return Violation{Rule: RuleMainBranch, Detail: "push " + arg}
			}
			continue
		}
		if c.isMainRef(arg) {
			return Violation{Rule: RuleMainBranch, Detail: sub + " " + arg}
		}
	}
	return nil
}

// readsOnly says whether a subcommand's own action only reads, so none of its arguments can move the
// main branch. "stash show" and "reflog show" are the forms a person runs to look, and a "stash" or
// "reflog" with nothing after it lists.
func readsOnly(sub string, rest []string) bool {
	switch sub {
	case "stash":
		switch subAction(rest) {
		case "", "list", "show":
			return true
		}
	case "reflog":
		switch subAction(rest) {
		case "", "show":
			return true
		}
	}
	return false
}

// refValueFlags are the flags whose value is a ref only to match it against, not to move it.
var refValueFlags = map[string]bool{
	"--contains": true, "--no-contains": true, "--merged": true, "--no-merged": true,
	"--points-at": true, "--sort": true, "--format": true, "--list": true, "-l": true,
}

// checkPushFlags refuses a push that overrides a remote, and the forms that push every branch,
// because either can move the main branch of a repository nobody asked to change.
func (c Containment) checkPushFlags(rest []string) error {
	for _, arg := range rest {
		switch kind, value, hasValue := strings.Cut(arg, "="); {
		case kind == "--force", kind == "-f", kind == "--force-with-lease", kind == "--force-if-includes",
			kind == "--mirror", kind == "--all", kind == "--delete":
			detail := kind
			if hasValue {
				detail = kind + "=" + value
			}
			return Violation{Rule: RuleForcePush, Detail: "push " + detail}
		}
	}
	return nil
}

// pushTargetsMain says whether one argument of a push names the main branch. It reads the
// destination of a refspec ("HEAD:main", "+refs/heads/x:refs/heads/main") and the plain form
// ("main", "origin", "refs/heads/main"). The source side of a refspec is not inspected: pushing
// the card's own branch is the whole point, whatever it is called locally.
func (c Containment) pushTargetsMain(arg string) bool {
	if c.Main == "" {
		return false
	}
	if _, dest, isRefspec := strings.Cut(arg, ":"); isRefspec {
		return trimRef(dest) == c.Main
	}
	return trimRef(arg) == c.Main
}

// isMainRef says whether one argument names the main branch. The destination of a refspec is what
// counts, so "main:card" is left to the main-branch rule to refuse as the source, and ":main" is
// refused as the destination.
func (c Containment) isMainRef(arg string) bool {
	if c.Main == "" {
		return false
	}
	if src, dest, isRefspec := strings.Cut(arg, ":"); isRefspec {
		return trimRef(src) == c.Main || trimRef(dest) == c.Main
	}
	return trimRef(arg) == c.Main
}

// trimRef removes the decorations an argument may carry before it is compared with a branch name:
// the "+" and "^" of a refspec, the "refs/heads/", "refs/remotes/", and "refs/tags/" prefixes, and
// the remote name of a tracking ref.
//
// Only a name a tracking ref is really written with is taken off - "refs/remotes/" says so itself,
// and a bare prefix counts only for a remote Marshal knows. A branch whose own name holds a slash is
// therefore read as itself: "feature/main" is not the main branch.
func trimRef(arg string) string {
	arg = strings.TrimPrefix(arg, "+")
	arg = strings.TrimPrefix(arg, "^")
	arg = strings.TrimPrefix(arg, "refs/heads/")
	arg = strings.TrimPrefix(arg, "refs/tags/")
	arg = strings.TrimPrefix(arg, "heads/")
	if after, ok := strings.CutPrefix(arg, "refs/remotes/"); ok {
		if _, branch, found := strings.Cut(after, "/"); found {
			return branch
		}
		return after
	}
	if head, after, ok := strings.Cut(arg, "/"); ok && remotes[head] {
		return after
	}
	return arg
}

// remotes are the remote names a bare tracking ref is written with. A branch name that holds a slash
// is not one of them, so "feature/main" keeps its slash and is not read as the main branch.
var remotes = map[string]bool{"origin": true, "upstream": true}

// messageFlagSubcommands are the subcommands where "-m" carries a message rather than an action.
// It matters because the value after it is never a branch, while "branch -m main old-main" moves
// the main branch and must be refused.
var messageFlagSubcommands = map[string]bool{
	"commit": true, "merge": true, "tag": true, "revert": true, "cherry-pick": true,
	"am": true, "notes": true, "stash": true, "reflog": true,
}

// subAction is the subcommand's own verb: the first argument that is not a flag.
func subAction(rest []string) string {
	for _, arg := range rest {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return arg
	}
	return ""
}

// configReads says whether a config command only reads: "config --get x", "config --list",
// "config -l". Anything else would change a setting of the whole repository.
func configReads(rest []string) bool {
	for _, arg := range rest {
		switch arg {
		case "--get", "--get-all", "--get-regexp", "--get-urlmatch", "--list", "-l", "--show-origin":
			return true
		}
	}
	return false
}

// splitGitArgs separates the global options of a Git command line from the subcommand and its own
// arguments, so the flags of one are never read as the other.
func splitGitArgs(args []string) (globals []string, sub string, rest []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "" {
			continue
		}
		if arg == "--" {
			break
		}
		if !strings.HasPrefix(arg, "-") {
			return globals, arg, args[i+1:]
		}
		globals = append(globals, arg)
		if _, _, hasValue := strings.Cut(arg, "="); hasValue {
			continue
		}
		if globalTakesValue[arg] {
			i++
			if i < len(args) {
				globals = append(globals, args[i])
			}
		}
	}
	return globals, "", nil
}

// globalTakesValue names the options Git takes before its subcommand that are followed by a value.
var globalTakesValue = map[string]bool{
	"-C": true, "-c": true, "--git-dir": true, "--work-tree": true, "--namespace": true,
	"--exec-path": true, "--config-env": true, "--attr-source": true, "--super-prefix": true,
}

// inside says whether a folder is the worktree or is under it. A relative folder is read as
// relative to the worktree.
func (c Containment) inside(folder string) bool {
	if folder == "" || c.Worktree == "" {
		return false
	}
	if !filepath.IsAbs(folder) {
		folder = filepath.Join(c.Worktree, folder)
	}
	return relationOf(c.Worktree, folder) != unrelated
}
