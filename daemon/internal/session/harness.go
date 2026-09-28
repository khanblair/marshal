package session

import (
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// harnessConfig builds the harness's view of one live session: the permission mode it runs under,
// the profile and blocklist Marshal ships with, and, for a card, its worktree rule.
//
// It reports false when the session runs under no mode at all - a chat that has not been given one -
// and then every request is a person's, exactly as it was before the harness existed. It also
// reports false when the row cannot be read, or when a card has a worktree the project cannot be
// read for, for the same reason: a decision made without knowing the mode or the worktree would be a
// guess, and the careful direction is to ask.
func (m *Manager) harnessConfig(ls *liveSession) (harness.Config, bool) {
	if ls.isChat() {
		chat, err := m.store.Queries().GetChat(m.ctx, ls.chatID)
		if err != nil {
			if !store.IsNotFound(err) {
				m.log.Error("could not read a chat to decide a permission request", "chat_id", ls.chatID, "error", err)
			}
			return harness.Config{}, false
		}
		if chat.PermissionMode == "" {
			return harness.Config{}, false
		}
		return harness.Config{
			Mode: protocol.PermissionMode(chat.PermissionMode), Profile: m.profile(), Blocklist: m.blocklist,
		}, true
	}
	row, err := m.store.Queries().GetCard(m.ctx, ls.cardID)
	if err != nil {
		if !store.IsNotFound(err) {
			m.log.Error("could not read a card to decide a permission request", "card_id", ls.cardID, "error", err)
		}
		return harness.Config{}, false
	}
	return m.configForCardRow(row)
}

// HarnessConfigFor answers the harness's view of a card as it is now, for the internal MCP server's
// permission checks (docs/architecture.md section 13): the card's mode, Marshal's profile and
// blocklist, and the card's worktree rule. It is the card half of harnessConfig, reachable without a
// live session, because the server checks a call the same way whether or not the session's own
// permission requests are what raised it.
//
// It answers false in the same cases harnessConfig does for a card: no mode, an unreadable row, or a
// worktree whose project cannot be read. A decision made without those would be a guess.
func (m *Manager) HarnessConfigFor(cardID string) (harness.Config, bool) {
	row, err := m.store.Queries().GetCard(m.ctx, cardID)
	if err != nil {
		if !store.IsNotFound(err) {
			m.log.Error("could not read a card to decide a tool call", "card_id", cardID, "error", err)
		}
		return harness.Config{}, false
	}
	return m.configForCardRow(row)
}

// configForCardRow builds a card's harness rules from its row, or reports false when there is nothing
// careful to decide with.
func (m *Manager) configForCardRow(row db.Card) (harness.Config, bool) {
	if row.PermissionMode == "" {
		return harness.Config{}, false
	}
	cfg := harness.Config{
		Mode: protocol.PermissionMode(row.PermissionMode), Profile: m.profile(), Blocklist: m.blocklist,
	}
	containment, hasWorktree, err := m.containmentOf(row)
	if err != nil {
		// A card with a worktree whose project cannot be read would otherwise be decided without the
		// one rule every mode keeps; the careful direction is the person's, not a guess.
		m.log.Error("could not build a card's worktree rule", "card_id", row.ID, "error", err)
		return harness.Config{}, false
	}
	if hasWorktree {
		cfg.Containment = &containment
	}
	return cfg, true
}

// profile is the profile the manager decides with: the one it was configured with, else the one
// Marshal ships with. The config keeps it a pointer, so "nothing allowed" is not read as "not set".
func (m *Manager) profile() security.Profile {
	if m.cfg.Profile == nil {
		return security.DefaultProfile()
	}
	return *m.cfg.Profile
}

// answerWithoutAsking answers the agent itself, audits it, tells the card's own history when the
// request was refused, and puts the session back to working. It is how a request the harness has
// already decided is answered, whether the decision is the daemon's (bypass, B3.2) or a rule's
// (the blocklist, the deploy rule, a mode, B3.1, B3.3, B3.7).
//
// It reports whether the agent was answered. When it was not - the request offers no option for the
// decision, or the agent refused the answer - the caller asks the person instead, which is always
// safe: a decision the daemon could not deliver is one a person makes.
func (m *Manager) answerWithoutAsking(ls *liveSession, e agents.PermissionRequested, out harness.Outcome, mode protocol.PermissionMode) bool {
	decision := protocol.ApprovalDecisionApproved
	if out.Decision == harness.DecisionDeny {
		decision = protocol.ApprovalDecisionDenied
	}
	option, err := chooseOption(e.Options, decision, "")
	if err != nil {
		m.log.Warn("a request the daemon would answer offered nothing to answer with",
			ls.noun()+"_id", ls.key(), "reason", out.Rule)
		return false
	}
	if err := ls.agent.Respond(m.ctx, ls.handle, agents.ApprovalResponse{RequestID: e.RequestID, OptionID: option}); err != nil {
		m.log.Error("could not answer a request on the person's behalf", ls.noun()+"_id", ls.key(), "error", err)
		return false
	}
	action := audit.ActionApprove
	if decision == protocol.ApprovalDecisionDenied {
		action = audit.ActionDeny
	}
	m.audit.LogAndForget(m.ctx, audit.Entry{
		Actor: audit.ActorDaemon, Action: action, Target: e.ToolCallID, SessionID: ls.sessionRowID,
		Detail: map[string]any{
			cardIDDetailKey: ls.cardID, "chatId": ls.chatID, "mode": string(mode), "reason": out.Rule,
			"title": e.Title, "path": e.Path, "command": e.Command,
		},
	})
	// This request never goes through holdPermission, so it is never given an approval id or a row
	// in the approvals table (S8b) - there is nothing for a person to answer, since the daemon just
	// did. It still gets its own history row, already resolved, the way every PermissionRequested
	// did before this change; a card is never left with an unrecorded permission decision just
	// because nobody was asked.
	resolved := history.StateOK
	if decision == protocol.ApprovalDecisionDenied {
		resolved = history.StateFailed
	}
	m.appendApprovalHistory(ls, "", resolved, e)
	if decision == protocol.ApprovalDecisionDenied {
		m.noteRefusal(ls, e, out, mode)
	}
	// An agent that asks is mid-turn, so once the daemon has answered, the session reads as working
	// again: it is never left waiting for an answer nobody is going to give. This is the same place
	// Respond puts a session back to after a person answers.
	if err := m.setSessionState(m.ctx, ls, protocol.SessionStateWorking); err != nil {
		m.log.Warn("could not record that a session went on working", ls.noun()+"_id", ls.key(), "error", err)
	}
	m.publishState(ls, protocol.SessionStateWorking, "")
	return true
}

// noteRefusal tells the person, in the card's own history, that a request was refused without them:
// what was refused and which rule refused it. In bypass the sentence names bypass, because that is
// the mode a person chose and the worktree rule is the only one it keeps; in every other mode the
// sentence names Marshal, because the rule is Marshal's own.
func (m *Manager) noteRefusal(ls *liveSession, e agents.PermissionRequested, out harness.Outcome, mode protocol.PermissionMode) {
	subject := refusalSubject(e)
	summary := fmt.Sprintf("Marshal refused to run %s: the %s rule.", subject, out.Rule)
	if mode == protocol.PermissionModeBypass {
		summary = fmt.Sprintf(
			"Bypass permissions refused to run %s: the %s rule. The agent stays inside this card's worktree.",
			subject, out.Rule)
	}
	m.appendRecords(ls, []history.Record{{Kind: history.KindSystem, Summary: summary}})
}

// refusalSubject is the shortest true name for what a request wanted to do, taken from what the
// agent said about it: its title, else the command, else the path.
func refusalSubject(e agents.PermissionRequested) string {
	for _, candidate := range []string{e.Title, e.Command, e.Path} {
		if subject := strings.TrimSpace(candidate); subject != "" {
			return subject
		}
	}
	return "a request"
}
