// Package harness makes the permission decision for one request an agent makes
// (docs/architecture.md section 13: permissions are checked before every tool call, file write, and
// command, and the agent's own claims are never trusted).
//
// It is deliberately small. A decision is a pure function of what the daemon already knows - the
// session's permission mode, the profile, the command blocklist, the deploy rule, and the card's
// worktree - so the same request always gets the same answer, and the answer can be tested without
// running an agent.
//
// It is not the only place a mode is enforced. An agent that carries out a mode itself (Claude Code
// through its own flags, an ACP agent through its session modes) is still driven that way, because
// the agent is the one that knows what it is about to do. The harness is the second gate: it applies
// the rules that do not depend on the agent's cooperation at all, and it applies the mode too when an
// agent asks anyway. Two gates that disagree refuse, they do not allow.
package harness

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
)

// The names a decision is recorded under when it is not one of security's own rules.
const (
	// RuleMode is a decision the session's permission mode made on its own.
	RuleMode = "mode"
	// RuleBypass is a request answered by bypass, the one mode that skips the guards.
	RuleBypass = "bypass"
	// RuleUnreadable is a command the daemon could not read, because it substitutes a command or
	// names one through a variable. It is refused rather than guessed at.
	RuleUnreadable = security.RuleUnreadable
)

// Decision is what the harness answers for one request.
type Decision string

const (
	// DecisionAllow answers the agent itself, with yes.
	DecisionAllow Decision = "allow"
	// DecisionAsk leaves the request to a person.
	DecisionAsk Decision = "ask"
	// DecisionDeny answers the agent itself, with no.
	DecisionDeny Decision = "deny"
)

// Outcome is a decision and the rule that produced it. The rule is what the audit row and the
// card's own history name, so a person can see why a request was answered without them.
type Outcome struct {
	Decision Decision
	Rule     string
}

// Request is one thing an agent wants to do, as the daemon can read it from a permission request.
// All three fields are what the agent said about the request, which is why every rule is a check on
// the words themselves rather than on a description of them.
type Request struct {
	// Kind is the agent's own kind word for the tool call, such as "edit" or "execute".
	Kind string
	// Path is the file the request is about, if there is one.
	Path string
	// Command is the command line the request would run, if there is one.
	Command string
}

// Config is what a decision is made from. Mode is required; the rest may be left out, and a decision
// is then made with less to go on rather than with a guess.
type Config struct {
	// Mode is the session's permission mode.
	Mode protocol.PermissionMode
	// Profile says what is allowed at all. The zero value refuses everything it covers, so a caller
	// that wants the shipped profile passes security.DefaultProfile.
	Profile security.Profile
	// Blocklist is the command blocklist. A nil list has nothing to say.
	Blocklist *security.Blocklist
	// Containment is the card's worktree rule, or nil for a session with no worktree (a chat). It is
	// the one rule that is kept in every mode, bypass included.
	Containment *gitx.Containment
}

// Decide answers one request. The rules are applied from the ones that depend on nothing to the ones
// that depend on the mode:
//
//  1. The worktree rule, in every mode. Bypass keeps it, so nothing here may skip it.
//  2. Bypass itself, which allows what is left.
//  3. The permission profile, which refuses what is not allowed at all.
//  4. The command blocklist, which refuses or hands to a person what is on the list.
//  5. The deploy rule, which hands a deploy to a person.
//  6. The mode, which decides what is left: allow, ask, or refuse.
//
// The first rule with something to say decides. A rule that refuses is never overridden by a mode
// that would have allowed the same request.
func (c Config) Decide(req Request) Outcome {
	if rule, breach := c.breach(req); breach {
		return Outcome{Decision: DecisionDeny, Rule: rule}
	}
	if c.Mode == protocol.PermissionModeBypass {
		return Outcome{Decision: DecisionAllow, Rule: RuleBypass}
	}
	action := security.Classify(req.Kind, req.Command)
	if finding, refuses := c.Profile.Check(action); refuses {
		return Outcome{Decision: DecisionDeny, Rule: finding.Rule}
	}
	if finding, found := c.Blocklist.Check(req.Command); found {
		return Outcome{Decision: decisionOf(finding.Verdict), Rule: finding.Rule}
	}
	if security.DeployWorkflow(req.Command) {
		return Outcome{Decision: DecisionAsk, Rule: security.RuleDeploy}
	}
	return c.byMode(action)
}

// byMode applies the mode's own policy to an action that no rule has spoken for
// (docs/marshal-product-scope.md section 14.1):
//
//   - Ask asks before every edit and command; reading is not one of those, so reading is allowed.
//   - Auto-accept edits allows edits and asks about everything else.
//   - Plan only reads, and refuses every other action the daemon can see.
//   - Full auto allows what the rules have not refused.
//   - Bypass was answered before this is reached.
//
// A mode the daemon does not know is treated as the careful one: the request is the person's.
func (c Config) byMode(action security.Action) Outcome {
	switch c.Mode {
	case protocol.PermissionModeFullAuto:
		return Outcome{Decision: DecisionAllow, Rule: RuleMode}
	case protocol.PermissionModeAutoEdits:
		if action == security.ActionRead || action == security.ActionEdit {
			return Outcome{Decision: DecisionAllow, Rule: RuleMode}
		}
	case protocol.PermissionModePlan:
		if action == security.ActionRead {
			return Outcome{Decision: DecisionAllow, Rule: RuleMode}
		}
		return Outcome{Decision: DecisionDeny, Rule: RuleMode}
	case protocol.PermissionModeAsk:
		if action == security.ActionRead {
			return Outcome{Decision: DecisionAllow, Rule: RuleMode}
		}
	}
	return Outcome{Decision: DecisionAsk, Rule: RuleMode}
}

// breach asks the card's worktree rule about a request, when the session has one.
func (c Config) breach(req Request) (string, bool) {
	if c.Containment == nil {
		return "", false
	}
	return ContainmentBreach(*c.Containment, req.Path, req.Command)
}

// decisionOf turns a rule's verdict into a decision.
func decisionOf(v security.Verdict) Decision {
	switch v {
	case security.VerdictAllow:
		return DecisionAllow
	case security.VerdictBlock:
		return DecisionDeny
	}
	return DecisionAsk
}

// ContainmentBreach says whether a request breaks the card's worktree rule, and which rule it
// breaks. It reads what the agent named: the file it would change, and the command it would run.
func ContainmentBreach(c gitx.Containment, path, command string) (string, bool) {
	if p := strings.TrimSpace(path); p != "" && !InsideWorktree(c, p) {
		return gitx.RuleOutsideWorktree, true
	}
	return GitCommandBreach(c, command)
}

// InsideWorktree says whether a path the agent named is inside the card's worktree. A relative path
// is read against the worktree, which is where the agent runs. A containment with no worktree knows
// nothing, and says so by calling everything outside.
func InsideWorktree(c gitx.Containment, path string) bool {
	if c.Worktree == "" {
		return false
	}
	if !filepath.IsAbs(path) {
		clean := filepath.Clean(path)
		return clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
	}
	clean := filepath.Clean(path)
	return clean == c.Worktree || strings.HasPrefix(clean, c.Worktree+string(filepath.Separator))
}

// GitCommandBreach says whether a command an agent asked to run is a Git command that would leave
// the worktree or touch the main branch. It reads every command in the line and finds the Git
// program wherever the line puts it - past the wrappers, their options, and anything else in front of
// it - so "ls && git push origin main" and "sudo --chdir /tmp git push origin main" are both seen as
// a push. Each command is read as a shell's argument list, so a commit message that mentions the main
// branch ("git commit -m 'work on main'") is one argument and is left alone.
//
// A Git command that cannot be read is refused rather than guessed at: a line that substitutes a
// command, or names a ref through a variable, could move the main branch without ever saying so.
//
// A line that holds no Git command is left alone here: the blocklist and the mode answer for those.
func GitCommandBreach(c gitx.Containment, command string) (string, bool) {
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
			return RuleUnreadable, true
		}
		var v gitx.Violation
		if errors.As(c.CheckCommand(args[at+1:]), &v) {
			return v.Rule, true
		}
	}
	return "", false
}
