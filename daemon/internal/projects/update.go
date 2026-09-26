package projects

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Editing a card by hand (inventory N3 and N1). Every field a person can change is here; the
// settings a session uses (agent, model, thinking, permission mode) are stored now and enforced by
// the harness when a session starts, which is Phase 3.

// maxRoleChars is the longest role name a card can carry.
const maxRoleChars = 60

// maxPackageChars is the longest package name a card can carry.
const maxPackageChars = 200

// UpdateCard changes the fields of a card that the request sets, and leaves the rest as they are.
// A card that is not there is not found; a card the daemon is already working on can still be
// edited, because nothing here changes the session.
func (s *Service) UpdateCard(ctx context.Context, id string, in protocol.UpdateCardRequest) (protocol.Card, error) {
	if err := checkCardUpdate(in); err != nil {
		return protocol.Card{}, err
	}
	var (
		after  db.Card
		labels []protocol.Label
		repeat bool
	)
	err := s.store.Write(ctx, func(q *db.Queries) error {
		row, err := q.GetCard(ctx, id)
		if err != nil {
			return notFound(fmt.Errorf("read card %s: %w", id, err), notFoundCard(id))
		}
		project, err := q.GetProject(ctx, row.ProjectID)
		if err != nil {
			return notFound(fmt.Errorf("read project %s: %w", row.ProjectID, err), notFoundProject(row.ProjectID))
		}
		// A new title, and only a new title, is marked as a change in the log line below.
		repeat = in.Title != nil && strings.TrimSpace(*in.Title) != row.Title
		now := s.now().UnixMilli()
		applyCardUpdate(&row, in, now)
		if in.Labels != nil {
			labels, err = replaceCardLabels(ctx, q, project.ID, row.ID, *in.Labels)
			if err != nil {
				return err
			}
		} else if labels, err = labelsForCard(ctx, q, row.ID); err != nil {
			return err
		}
		row.UpdatedAt = now
		if err := updateCardRow(ctx, q, row); err != nil {
			return err
		}
		after = row
		return nil
	})
	if err != nil {
		return protocol.Card{}, err
	}
	card, err := s.withSession(ctx, toCard(after, labels))
	if err != nil {
		return protocol.Card{}, err
	}
	what := "edited a card"
	if repeat {
		what = "renamed a card"
	}
	s.log.Info(what, "project_id", card.ProjectID, "card_id", card.ID)
	s.publish(protocol.ProjectTopic(card.ProjectID), protocol.EventTypeCardUpdated, protocol.CardEventData{Card: card}, false)
	return card, nil
}

// checkCardUpdate refuses an edit that is not allowed. A field that is not set is not checked.
func checkCardUpdate(in protocol.UpdateCardRequest) error {
	switch {
	case in.Title != nil && (strings.TrimSpace(*in.Title) == "" || utf8.RuneCountInString(strings.TrimSpace(*in.Title)) > maxTitleChars):
		return protocol.InvalidArgument(fmt.Sprintf("Card titles can have at most %d characters, and cannot be empty.", maxTitleChars))
	case in.Body != nil && len(*in.Body) > maxBodyBytes:
		return protocol.InvalidArgument("That description is too long. Shorten it, or attach a file instead.")
	case in.Agent != nil && !in.Agent.Valid():
		return protocol.InvalidArgument("Marshal does not know that agent.").With("agent", string(*in.Agent))
	case in.Thinking != nil && *in.Thinking != "" && !in.Thinking.Valid():
		return protocol.InvalidArgument("That is not a thinking setting Marshal knows.").With("thinking", string(*in.Thinking))
	case in.PermissionMode != nil && !in.PermissionMode.Valid():
		return protocol.InvalidArgument("That is not a permission mode Marshal knows.").With("permissionMode", string(*in.PermissionMode))
	case in.Model != nil && utf8.RuneCountInString(*in.Model) > maxModelChars:
		return protocol.InvalidArgument("That model name is too long.")
	case in.Role != nil && utf8.RuneCountInString(*in.Role) > maxRoleChars:
		return protocol.InvalidArgument(fmt.Sprintf("Role names can have at most %d characters.", maxRoleChars))
	case in.Package != nil && utf8.RuneCountInString(*in.Package) > maxPackageChars:
		return protocol.InvalidArgument(fmt.Sprintf("Package names can have at most %d characters.", maxPackageChars))
	case in.NeedsReason != nil && !in.NeedsReason.Kind.Valid():
		return protocol.InvalidArgument("Marshal does not know that reason.").With("kind", string(in.NeedsReason.Kind))
	}
	return nil
}

// applyCardUpdate writes the fields the request sets onto the row. The caller has checked them.
// now is the daemon's clock, for the moment a card starts waiting on a person.
func applyCardUpdate(row *db.Card, in protocol.UpdateCardRequest, now int64) {
	if in.Title != nil {
		row.Title = strings.TrimSpace(*in.Title)
	}
	if in.Body != nil {
		row.Body = *in.Body
	}
	if in.Agent != nil {
		row.AgentKind = string(*in.Agent)
	}
	if in.Model != nil {
		row.Model = strings.TrimSpace(*in.Model)
	}
	if in.Thinking != nil {
		row.Thinking = string(*in.Thinking)
	}
	if in.PermissionMode != nil {
		row.PermissionMode = string(*in.PermissionMode)
	}
	if in.Role != nil {
		row.Role = strings.TrimSpace(*in.Role)
	}
	if in.Package != nil {
		row.Package = strings.TrimSpace(*in.Package)
	}
	if in.DoingNow != nil {
		row.DoingNow = strings.TrimSpace(*in.DoingNow)
	}
	row.PlannedStart = applyDate(row.PlannedStart, in.PlannedStart)
	row.PlannedEnd = applyDate(row.PlannedEnd, in.PlannedEnd)
	row.Due = applyDate(row.Due, in.Due)
	row.ActualStart = applyDate(row.ActualStart, in.ActualStart)
	row.ActualEnd = applyDate(row.ActualEnd, in.ActualEnd)
	if in.NeedsReason != nil {
		row.NeedsReasonKind = string(in.NeedsReason.Kind)
		row.NeedsReasonText = in.NeedsReason.Text
		// The moment a card starts waiting is the moment it is given a reason.
		row.NeedsSince = &now
	}
}

// applyDate returns the new value of a date column: nil to clear it, the new moment otherwise, and
// the value it already had when the request did not mention it.
func applyDate(current *int64, change *protocol.DateChange) *int64 {
	if change == nil {
		return current
	}
	if change.Clear || change.At == nil {
		return nil
	}
	ms := change.At.Time().UnixMilli()
	return &ms
}

// replaceCardLabels sets a card's labels to exactly these ids, and returns them. Every id must be
// a label of the card's own project: a card can only carry its project's labels (decision D3), and
// a repeated id is applied once.
func replaceCardLabels(ctx context.Context, q *db.Queries, projectID, cardID string, ids []string) ([]protocol.Label, error) {
	if err := q.ClearCardLabels(ctx, cardID); err != nil {
		return nil, fmt.Errorf("clear the labels of card %s: %w", cardID, err)
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, done := seen[id]; done {
			continue
		}
		seen[id] = struct{}{}
		label, err := q.GetLabel(ctx, id)
		if err != nil || label.ProjectID != projectID {
			return nil, protocol.InvalidArgument("That label is not one of this project's labels.").With("label", id)
		}
		if err := q.AddCardLabel(ctx, db.AddCardLabelParams{CardID: cardID, LabelID: id}); err != nil {
			return nil, fmt.Errorf("add label %s to card %s: %w", id, cardID, err)
		}
	}
	rows, err := q.ListLabelsForCard(ctx, cardID)
	if err != nil {
		return nil, fmt.Errorf("read the labels of card %s: %w", cardID, err)
	}
	return toLabels(rows), nil
}

// labelsForCard reads a card's labels inside a write closure the caller already holds.
func labelsForCard(ctx context.Context, q *db.Queries, cardID string) ([]protocol.Label, error) {
	rows, err := q.ListLabelsForCard(ctx, cardID)
	if err != nil {
		return nil, fmt.Errorf("read the labels of card %s: %w", cardID, err)
	}
	return toLabels(rows), nil
}
