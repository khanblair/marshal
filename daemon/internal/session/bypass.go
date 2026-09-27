package session

import (
	"context"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// What a card's own history says when bypass is turned on or off. The sentences are the app's,
// moved with the call that writes them (apps/web/src/mock/actions/cards.ts), because the person
// reads them in the card's chat whatever the view.
const (
	bypassOnNote      = "Bypass permissions is on for this card. The agent can run any command in its worktree without asking."
	bypassOffNote     = "Bypass permissions is off. Permission mode set to Full auto."
	bypassOnActivity  = "Bypass permissions turned on by you"
	bypassOffActivity = "Bypass permissions turned off by you"
)

// SetBypass turns bypass permissions on for a card. It is the only way a card reaches that mode:
// the request carries the acknowledgement a person gave, the project's bypass lock is checked, the
// change is audited, and the card's own history says what happened.
//
// A request without the acknowledgement is refused, so no client, script, or stale tab can grant
// bypass as a side effect of setting a field.
func (m *Manager) SetBypass(ctx context.Context, cardID string, in protocol.BypassRequest) (protocol.Card, error) {
	if !in.Acknowledged {
		return protocol.Card{}, protocol.Refused("Turning on bypass permissions takes an acknowledgement: the agent will run every command and edit in this card's worktree without asking.").
			With("reason", string(protocol.BypassRefusalReasonUnacknowledged)).With("cardId", cardID)
	}
	return m.setBypass(ctx, cardID, true)
}

// ClearBypass turns bypass permissions off for a card, leaving it in full auto.
func (m *Manager) ClearBypass(ctx context.Context, cardID string) (protocol.Card, error) {
	return m.setBypass(ctx, cardID, false)
}

// setBypass is the shared tail of SetBypass and ClearBypass. Every explicit call is audited, even
// one that changes nothing (a second press, or a retry after a failed audit write), because the
// audit log records what people asked for. Only a real change is written into the card's history,
// so a double press does not say the same thing twice.
func (m *Manager) setBypass(ctx context.Context, cardID string, on bool) (protocol.Card, error) {
	before, err := m.projects.Card(ctx, cardID)
	if err != nil {
		return protocol.Card{}, err
	}
	want := protocol.PermissionModeBypass
	if !on {
		want = projects.BypassOffMode
	}
	card := before
	changed := before.PermissionMode != want
	if changed {
		if card, err = m.projects.SetBypassMode(ctx, cardID, on); err != nil {
			return protocol.Card{}, err
		}
	}
	if err := m.auditBypass(ctx, card, on, changed); err != nil {
		return protocol.Card{}, err
	}
	if changed {
		m.noteBypass(ctx, card, on)
		m.log.Info("changed a card's bypass", "card_id", cardID, "on", on)
	}
	return card, nil
}

// auditBypass writes the audit row for an explicit bypass call. It is written whether or not the
// mode changed, and a row that cannot be written fails the call: an audit trail with a hole in it
// is worse than a call a person has to repeat.
func (m *Manager) auditBypass(ctx context.Context, card protocol.Card, on, changed bool) error {
	action := audit.ActionBypassOff
	if on {
		action = audit.ActionBypassOn
	}
	err := m.audit.Record(ctx, audit.Entry{
		Actor: audit.ActorPerson, Action: action, Target: card.ID,
		Detail: map[string]any{
			"cardId": card.ID, "projectId": card.ProjectID,
			"permissionMode": string(card.PermissionMode), "changed": changed,
		},
	})
	if err != nil {
		return fmt.Errorf("audit the bypass change of card %s: %w", card.ID, err)
	}
	return nil
}

// noteBypass writes the system message and the activity row into the card's own history, so the
// chat says what happened whichever view opened it. A card that has never started has no session
// row to write into and therefore no chat yet: the message appears from its first start, and the
// audit row above is what accounts for the grant.
func (m *Manager) noteBypass(ctx context.Context, card protocol.Card, on bool) {
	if m.cfg.History == nil {
		return
	}
	sessionID, ok := m.sessionRowIDOf(ctx, card.ID)
	if !ok {
		return
	}
	note, activity := bypassOnNote, bypassOnActivity
	if !on {
		note, activity = bypassOffNote, bypassOffActivity
	}
	records := []history.Record{
		{Kind: history.KindSystem, Summary: note},
		{Kind: history.KindApproval, Summary: activity},
	}
	if err := m.cfg.History.Append(m.ctx, card.ID, sessionID, records); err != nil {
		m.log.Error("could not store the bypass note", "card_id", card.ID, "error", err)
	}
}

// sessionRowIDOf is the session row of a card, and whether it has one.
func (m *Manager) sessionRowIDOf(ctx context.Context, cardID string) (string, bool) {
	row, err := m.store.Queries().GetSessionByCard(ctx, cardID)
	if err != nil {
		if !store.IsNotFound(err) {
			m.log.Error("could not read a card's session row", "card_id", cardID, "error", err)
		}
		return "", false
	}
	return row.ID, true
}

// containmentOf builds the containment rule for a card, from the card's own worktree and branch and
// its project's main branch. It reports false for a card that has no worktree yet, and an error when
// the card has a worktree but the rest of the rule cannot be built - its project cannot be read, or
// its row names no project or branch: the caller must not decide a request without the rule the
// card's worktree is meant to carry.
func (m *Manager) containmentOf(row db.Card) (gitx.Containment, bool, error) {
	if row.WorktreePath == "" {
		return gitx.Containment{}, false, nil
	}
	if row.ProjectID == "" || row.Branch == "" {
		return gitx.Containment{}, false, fmt.Errorf(
			"card %s has the worktree %s but no project or branch to contain it with", row.ID, row.WorktreePath)
	}
	project, err := m.store.Queries().GetProject(m.ctx, row.ProjectID)
	if err != nil {
		m.log.Error("could not read a project to contain a card", "card_id", row.ID, "error", err)
		return gitx.Containment{}, false, err
	}
	return gitx.NewContainment(
		projects.WorktreesDir(m.cfg.DataDir, row.ProjectID),
		row.WorktreePath, row.Branch, project.DefaultBranch,
	), true, nil
}
