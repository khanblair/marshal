package projects

import (
	"context"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// BypassOffMode is the permission mode a card is left in when bypass is turned off. Full auto is
// what the app's own bypass switch does, and it is the strictest mode that still asks nothing about
// work the person was already letting through. The session manager reads it, so that turning bypass
// off twice is recognized as nothing to do.
const BypassOffMode = protocol.PermissionModeFullAuto

// SetBypassMode turns bypass permissions on or off for one card and publishes card.updated when the
// mode really changed. It is the only write that sets a card's mode to bypass.
//
// The decision to grant bypass is not made here: the acknowledgement, the audit row, and the note
// in the card's own history belong to the call that grants it (internal/session, docs/backend-
// checklist.md B3.2). What belongs here is the write, and the one rule about it that is a project's:
// a project whose settings lock bypass refuses it for every card in it (N16).
//
// Turning it on when it is already on, and off when it is already off, changes nothing and
// publishes nothing, the way setting a card's pause flag twice does.
func (s *Service) SetBypassMode(ctx context.Context, id string, on bool) (protocol.Card, error) {
	mode := protocol.PermissionModeBypass
	if !on {
		mode = BypassOffMode
	}
	var after db.Card
	changed := false
	err := s.store.Write(ctx, func(q *db.Queries) error {
		row, err := q.GetCard(ctx, id)
		if err != nil {
			return notFound(fmt.Errorf("read card %s: %w", id, err), notFoundCard(id))
		}
		if on {
			if err := checkBypassLock(ctx, q, row.ProjectID, id); err != nil {
				return err
			}
		}
		after = row
		if protocol.PermissionMode(row.PermissionMode) == mode {
			return nil
		}
		after.PermissionMode, changed = string(mode), true
		after.UpdatedAt = s.now().UnixMilli()
		return updateCardRow(ctx, q, after)
	})
	if err != nil {
		return protocol.Card{}, err
	}
	card, err := s.cardWithLabels(ctx, after)
	if err != nil {
		return protocol.Card{}, err
	}
	if !changed {
		return card, nil
	}
	s.log.Info("changed a card's bypass", "project_id", card.ProjectID, "card_id", id,
		"permission_mode", string(mode))
	s.publish(protocol.ProjectTopic(card.ProjectID), protocol.EventTypeCardUpdated,
		protocol.CardEventData{Card: card}, true)
	return card, nil
}

// checkBypassLock refuses bypass for a card of a project that locks it. The reasoning is the same
// for every caller, so it is stated once: the lock is a project setting, and the person who wants
// bypass has to change that setting first (apps/web/src/views/settings/ProjectSection.tsx).
func checkBypassLock(ctx context.Context, q *db.Queries, projectID, cardID string) error {
	project, err := q.GetProject(ctx, projectID)
	if err != nil {
		return notFound(fmt.Errorf("read project %s: %w", projectID, err), notFoundProject(projectID))
	}
	if !boolFromInt(project.BypassLocked) {
		return nil
	}
	return protocol.Refused("Bypass permissions is locked for this project. Nobody can turn it on for its cards.").
		With("reason", string(protocol.BypassRefusalReasonLocked)).With("cardId", cardID).
		With("projectId", projectID)
}

// refusedBypass is the refusal for a request that tried to grant bypass without the call that
// carries the acknowledgement. The reason travels in the details, so the app shows its own words
// for it rather than the daemon's sentence.
func refusedBypass(message string) *protocol.Error {
	return protocol.Refused(message).
		With("reason", string(protocol.BypassRefusalReasonUnacknowledged)).
		With("permissionMode", string(protocol.PermissionModeBypass))
}
