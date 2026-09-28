package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The harness's limits and stuck detector, wired to a card's live session (docs/architecture.md
// section 3, docs/backend-checklist.md B5.3, build-plan 5.3 and 5.4). The decision itself is pure
// and lives in internal/harness; this file is only what feeds it and what a card is stopped with.
//
// A card's ceilings come from the role it names, read when the card runs and not when the role was
// saved (internal/roles, whose doc comment says the same). A card with no role, a role that sets no
// ceilings, or a daemon with no roles module wired in is not limited at all: the harness only ever
// stops a card for a ceiling somebody actually set.

// RoleLimitsReader resolves the ceilings of the role a card runs under. The roles module implements
// it; the manager names only what it needs, the way it names HistoryRecorder and PlanStore.
type RoleLimitsReader interface {
	// LimitsFor returns the ceilings of a role by name, as the project sees it, so a project's own
	// override of a role is what its cards run under. It reports false when there is no such role,
	// and then the card is not limited.
	LimitsFor(ctx context.Context, projectID, roleName string) (harness.Limits, bool, error)
}

// SetRoleLimits gives the manager the reader that resolves a role's ceilings. It is set after the
// manager is built, because the roles module is built after it (the manager needs the projects
// service, which is built before both), and it is safe to call while sessions are running.
func (m *Manager) SetRoleLimits(r RoleLimitsReader) {
	m.limitsMu.Lock()
	m.limits = r
	m.limitsMu.Unlock()
}

// roleLimits returns the reader, or nil when none was set.
func (m *Manager) roleLimits() RoleLimitsReader {
	m.limitsMu.RLock()
	defer m.limitsMu.RUnlock()
	return m.limits
}

// limitsOf reads the ceilings of the role a card runs under. It reports false when the card has no
// role, when there is no reader, or when the role cannot be read, so a daemon that cannot answer is
// one that does not limit rather than one that stops a card for a guess.
func (m *Manager) limitsOf(card protocol.Card) (harness.Limits, bool) {
	reader := m.roleLimits()
	if reader == nil || strings.TrimSpace(card.Role) == "" {
		return harness.Limits{}, false
	}
	limits, found, err := reader.LimitsFor(m.ctx, card.ProjectID, card.Role)
	if err != nil {
		m.log.Warn("could not read a role's limits", "card_id", card.ID, "role", card.Role, "error", err)
		return harness.Limits{}, false
	}
	if !found {
		return harness.Limits{}, false
	}
	return limits, true
}

// usageOf reads what a card has used against its ceilings: how long the turn that just ended ran,
// what the card has cost altogether, and how many turns it has taken. The cost and the round count
// are read from stored rows, so they survive a restart and a resume; the turn's length is what the
// live session has been timing.
//
// Cost is read in whole dollars, which is the unit a role's ceiling is in, and is rounded down, so
// a card is never stopped early by a fraction of a cent.
func (m *Manager) usageOf(ls *liveSession) (harness.Usage, error) {
	rounds, err := m.store.Queries().CountSessionEventsOfKind(m.ctx, db.CountSessionEventsOfKindParams{
		CardID: ls.cardID, Kind: string(history.KindUser),
	})
	if err != nil {
		return harness.Usage{}, fmt.Errorf("count the turns of card %s: %w", ls.cardID, err)
	}
	micros, err := m.store.Queries().SumUsageForCard(m.ctx, ls.cardID)
	if err != nil {
		return harness.Usage{}, fmt.Errorf("read the spend of card %s: %w", ls.cardID, err)
	}
	return harness.Usage{
		TurnMinutes: ls.turnMinutes(m.cfg.Now()),
		CostDollars: int(micros / 1_000_000),
		Rounds:      int(rounds),
	}, nil
}

// checkAfterTurn decides whether a card must stop now that a turn has ended, and reports whether it
// did. It is asked once per turn, after the secret scan, so a card already stopped for a credential
// is not stopped a second time.
//
// A card is stopped for the stuck detector before its ceilings, because a loop is the more specific
// thing to tell a person: "the same error three times" is more useful than "13 turns".
func (m *Manager) checkAfterTurn(ls *liveSession) bool {
	if ls.isChat() {
		return false
	}
	if reason, stuck := ls.takeStuck(); stuck {
		m.stopCard(ls, reason, audit.ActionStuckPaused, map[string]any{
			cardIDDetailKey: ls.cardID, "reason": reason.Text,
		})
		return true
	}
	card, err := m.projects.Card(m.ctx, ls.cardID)
	if err != nil {
		m.log.Warn("could not read a card to check its limits", "card_id", ls.cardID, "error", err)
		return false
	}
	limits, ok := m.limitsOf(card)
	if !ok || limits.IsZero() {
		return false
	}
	usage, err := m.usageOf(ls)
	if err != nil {
		m.log.Warn("could not read what a card has used", "card_id", ls.cardID, "error", err)
		return false
	}
	reason, hit := limits.Check(usage)
	if !hit {
		return false
	}
	m.stopCard(ls, reason, audit.ActionLimitReached, map[string]any{
		cardIDDetailKey: ls.cardID, "role": card.Role, "reason": reason.Text,
		"turnMinutes": usage.TurnMinutes, "costDollars": usage.CostDollars, "rounds": usage.Rounds,
		"limits": limits,
	})
	return true
}

// stopCard moves a card to Needs you with a reason the harness found, tells the card's own history,
// and records it for the audit log. It does not stop the agent: the turn that just ended has ended,
// and the session is left awake and free so the person can read the card, resume it, or send it a
// message. Holding the agent alive is what makes the card's own question readable in its chat.
func (m *Manager) stopCard(ls *liveSession, reason harness.Reason, action string, detail map[string]any) {
	m.audit.LogAndForget(m.ctx, audit.Entry{
		Actor: audit.ActorDaemon, Action: action, Target: ls.cardID, SessionID: ls.sessionRowID, Detail: detail,
	})
	if _, err := m.projects.SetNeeds(m.ctx, ls.cardID, protocol.NeedsReason{Kind: reason.Kind, Text: reason.Text}); err != nil {
		m.log.Error("could not move a card to needs you after the harness stopped it",
			"card_id", ls.cardID, "action", action, "error", err)
	}
	m.appendRecords(ls, []history.Record{{Kind: history.KindSystem, Summary: reason.Text}})
	m.log.Warn("the harness stopped a card", "card_id", ls.cardID, "action", action, "reason", reason.Text)
}

// feedStuckDetector hands one agent event to a card's stuck detector, in the order the turn produced
// it. Only a card has a detector: a chat has no ceilings and no loop to catch.
//
// Two signals are watched. A failed tool call is a signal whose key is the failure's own text, so
// the same error repeated is the same key. An edit is a signal whose key is the file, so the same
// file rewritten again and again is caught even when every edit succeeds. Anything else is not a
// loop and is ignored, which leaves the detector's current run alone.
//
// The reason is kept, not acted on here: the agent holds the turn, and the card is moved when the
// turn ends, so the agent's own words about the loop reach the person too.
func (m *Manager) feedStuckDetector(ls *liveSession, ev agents.AgentEvent) {
	if ls.isChat() || ls.loop == nil {
		return
	}
	signal, ok := stuckSignal(ls, ev)
	if !ok {
		return
	}
	if reason, stuck := ls.loop.Observe(signal); stuck {
		ls.noteStuck(reason)
	}
}

// stuckSignal turns an agent event into the signal the detector watches, if it is one.
func stuckSignal(ls *liveSession, ev agents.AgentEvent) (harness.Signal, bool) {
	switch e := ev.(type) {
	case agents.ToolCall:
		ls.rememberToolPath(e.ID, e.Path)
		if e.Kind == toolKindEdit && strings.TrimSpace(e.Path) != "" {
			return harness.Signal{Kind: harness.SignalEdit, Key: e.Path, Where: e.Path}, true
		}
		if e.Status == agents.StatusFailed {
			return failedSignal(e.ID, e.Title, e.Content, ls), true
		}
	case agents.ToolCallUpdate:
		if e.Status == agents.StatusFailed {
			return failedSignal(e.ID, e.Title, e.Content, ls), true
		}
	}
	return harness.Signal{}, false
}

// toolKindEdit is the agent's own word for a tool that changes a file (agents.ToolCall.Kind).
const toolKindEdit = "edit"

// failedSignal is the signal for a failed tool call: the same failure text is the same key, and the
// file the call was about, when it is known, is where the sentence says it happened.
func failedSignal(id, title, content string, ls *liveSession) harness.Signal {
	key := failureFingerprint(title, content)
	if key == "" {
		return harness.Signal{}
	}
	return harness.Signal{Kind: harness.SignalError, Key: key, Where: ls.toolPath(id)}
}

// failureFingerprint is the shortest text that names a failure the same way twice: the failure's own
// output when there is any, and the tool's title when there is not. Whitespace is collapsed and the
// text is cut short, so two renderings of the same error that differ only in spacing or in a
// timestamp at the end still fingerprint the same.
func failureFingerprint(title, content string) string {
	text := strings.TrimSpace(content)
	if text == "" {
		text = strings.TrimSpace(title)
	}
	if text == "" {
		return ""
	}
	text = strings.Join(strings.Fields(text), " ")
	const maxFingerprint = 200
	if len(text) > maxFingerprint {
		text = text[:maxFingerprint]
	}
	return text
}

// turnMinutes is how long the current turn has been running, in whole minutes, or 0 when no turn is
// being timed. A session whose agent does not report turns is never timed, because it never says
// when a turn ended.
func (ls *liveSession) turnMinutes(now time.Time) int {
	ls.turnMu.Lock()
	started := ls.turnStartedAt
	ls.turnMu.Unlock()
	if started.IsZero() {
		return 0
	}
	return int(now.Sub(started).Minutes())
}

// startTurnClock records when the turn that is starting began, so its length can be measured against
// the role's time ceiling when it ends.
func (ls *liveSession) startTurnClock(now time.Time) {
	ls.turnMu.Lock()
	ls.turnStartedAt = now
	ls.turnMu.Unlock()
}

// noteStuck keeps the reason the stuck detector gave until the turn ends, when the card is stopped.
// Only the first reason is kept: a card caught in one loop is not caught again while the turn runs.
func (ls *liveSession) noteStuck(reason harness.Reason) {
	ls.turnMu.Lock()
	defer ls.turnMu.Unlock()
	if ls.stuck == nil {
		ls.stuck = &reason
	}
}

// takeStuck returns the stuck reason, if the detector found one during this turn, and clears it so
// the next turn starts with no reason of its own.
func (ls *liveSession) takeStuck() (harness.Reason, bool) {
	ls.turnMu.Lock()
	defer ls.turnMu.Unlock()
	if ls.stuck == nil {
		return harness.Reason{}, false
	}
	reason := *ls.stuck
	ls.stuck = nil
	return reason, true
}

// rememberToolPath keeps the file a tool call is about, so a later failed update on the same call
// can say where it happened: an update carries the failure but not the path. The map is bounded, and
// the oldest entries are dropped, so a long turn cannot grow it without end.
func (ls *liveSession) rememberToolPath(id, path string) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(path) == "" {
		return
	}
	ls.turnMu.Lock()
	defer ls.turnMu.Unlock()
	if ls.toolPaths == nil {
		ls.toolPaths = make(map[string]string)
	}
	if len(ls.toolPaths) >= maxRememberedToolPaths {
		ls.toolPaths = make(map[string]string)
	}
	ls.toolPaths[id] = path
}

// toolPath is the file a tool call was about, or empty when it was not one about a file.
func (ls *liveSession) toolPath(id string) string {
	ls.turnMu.Lock()
	defer ls.turnMu.Unlock()
	return ls.toolPaths[id]
}

// maxRememberedToolPaths bounds the tool calls one session remembers the file of.
const maxRememberedToolPaths = 256
