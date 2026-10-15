package session

import (
	"context"
	"sync"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// PushPolicy says whether a project may publish: whether its agents may push, and Marshal may open
// pull requests on GitHub. A project that never chose is not allowed, so by default nothing a card's
// agent does leaves the machine.
type PushPolicy interface {
	AllowPush(ctx context.Context, projectID string) bool
}

// pushPolicy is the manager's policy and the lock that guards it.
type pushPolicy struct {
	mu     sync.RWMutex
	policy PushPolicy
}

// SetPushPolicy gives the manager the module that says whether a project may publish. It is set after
// the manager is built, like SetAttacher, and a session started later reads it.
func (m *Manager) SetPushPolicy(p PushPolicy) {
	m.push.mu.Lock()
	m.push.policy = p
	m.push.mu.Unlock()
}

// allowPush reads the policy for a project. With no policy set, nothing may be pushed.
func (m *Manager) allowPush(ctx context.Context, projectID string) bool {
	m.push.mu.RLock()
	policy := m.push.policy
	m.push.mu.RUnlock()
	return policy != nil && policy.AllowPush(ctx, projectID)
}

// cardToolRules are the tool rules a card's agent is started with, in Claude Code's syntax (an
// adapter without such rules ignores them). The agent may stage and commit its own work, which is
// what lets a finished card have commits to merge; Marshal runs the agent with its permission
// prompts off, so a command that is not allowed is refused instead of asked about. Pushing and
// publishing a pull request are denied unless the project allows them, and a deny rule wins over any
// allow rule and over every permission mode.
func cardToolRules(allowPush bool) (allowed, denied []string) {
	allowed = gitRules("status", "diff", "log", "show", "add", "commit")
	if !allowPush {
		denied = append(gitRules("push"), "Bash(gh pr create:*)", "Bash(gh pr merge:*)")
	}
	return allowed, denied
}

// cardRulesFor are the rules for one card's session.
func (m *Manager) cardRulesFor(ctx context.Context, card protocol.Card) (allowed, denied []string) {
	return cardToolRules(m.allowPush(ctx, card.ProjectID))
}
