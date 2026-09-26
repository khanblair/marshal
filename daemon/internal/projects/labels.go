package projects

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// A project's labels (decision D3): a managed list, each with a name and a color from the fixed
// set. Phase 2 stores them and puts them on cards; the screens keep showing the names exactly as
// they do today, because the design has no label picker, no label management, and no label colors
// yet (Q20). Every change publishes label.updated with the project's labels as they now are.

// maxLabelChars is the longest label name.
const maxLabelChars = 40

// Labels returns a project's labels, by name.
func (s *Service) Labels(ctx context.Context, projectID string) (protocol.LabelSnapshot, error) {
	var labels []protocol.Label
	err := s.store.Read(ctx, func(q *db.Queries) error {
		if _, err := q.GetProject(ctx, projectID); err != nil {
			return notFound(fmt.Errorf("read project %s: %w", projectID, err), notFoundProject(projectID))
		}
		rows, err := q.ListLabelsByProject(ctx, projectID)
		if err != nil {
			return fmt.Errorf("list the labels of project %s: %w", projectID, err)
		}
		labels = toLabels(rows)
		return nil
	})
	if err != nil {
		return protocol.LabelSnapshot{}, err
	}
	return protocol.LabelSnapshot{
		ProjectID:  projectID,
		Labels:     labels,
		ServerTime: protocol.NewTimestamp(s.now()),
	}, nil
}

// CreateLabel adds a label to a project. A name that the project already uses is a conflict: the
// list is what people pick from, so two labels with one name would be unusable.
func (s *Service) CreateLabel(ctx context.Context, projectID string, in protocol.CreateLabelRequest) (protocol.Label, error) {
	name := strings.TrimSpace(in.Name)
	if err := checkLabelName(name); err != nil {
		return protocol.Label{}, err
	}
	color := in.Color
	if color == "" {
		color = protocol.LabelColorSlate
	}
	if !color.Valid() {
		return protocol.Label{}, protocol.InvalidArgument("That is not a label color Marshal knows.").With("color", string(color))
	}
	id, err := s.newID()
	if err != nil {
		return protocol.Label{}, fmt.Errorf("make a label id: %w", err)
	}
	var row db.Label
	err = s.store.Write(ctx, func(q *db.Queries) error {
		if _, err := q.GetProject(ctx, projectID); err != nil {
			return notFound(fmt.Errorf("read project %s: %w", projectID, err), notFoundProject(projectID))
		}
		taken, err := labelNameTaken(ctx, q, projectID, name, "")
		if err != nil {
			return err
		}
		if taken {
			return protocol.Conflict("This project already has a label with that name.").With("label", name)
		}
		row = db.Label{ID: id, ProjectID: projectID, Name: name, Color: string(color), CreatedAt: s.now().UnixMilli()}
		if err := q.CreateLabel(ctx, db.CreateLabelParams(row)); err != nil {
			return fmt.Errorf("insert the label: %w", err)
		}
		return nil
	})
	if err != nil {
		return protocol.Label{}, err
	}
	s.log.Info("added a label", "project_id", projectID, "label_id", row.ID)
	s.publishLabels(ctx, projectID)
	return toLabel(row), nil
}

// UpdateLabel renames a label, recolors it, or both.
func (s *Service) UpdateLabel(ctx context.Context, id string, in protocol.UpdateLabelRequest) (protocol.Label, error) {
	if in.Name == nil && in.Color == nil {
		// Nothing to change: answer with the label as it is, so a client that sent an empty body
		// still gets what it asked about.
		return s.label(ctx, id)
	}
	if err := checkLabelUpdate(in); err != nil {
		return protocol.Label{}, err
	}
	var row db.Label
	err := s.store.Write(ctx, func(q *db.Queries) error {
		current, err := q.GetLabel(ctx, id)
		if err != nil {
			return notFound(fmt.Errorf("read label %s: %w", id, err), notFoundLabel(id))
		}
		row = current
		if in.Name != nil {
			name := strings.TrimSpace(*in.Name)
			taken, err := labelNameTaken(ctx, q, current.ProjectID, name, id)
			if err != nil {
				return err
			}
			if taken {
				return protocol.Conflict("This project already has a label with that name.").With("label", name)
			}
			row.Name = name
		}
		if in.Color != nil {
			row.Color = string(*in.Color)
		}
		changed, err := q.UpdateLabel(ctx, db.UpdateLabelParams{Name: row.Name, Color: row.Color, ID: row.ID})
		if err != nil {
			return fmt.Errorf("update label %s: %w", id, err)
		}
		if changed == 0 {
			return notFoundLabel(id)
		}
		return nil
	})
	if err != nil {
		return protocol.Label{}, err
	}
	s.log.Info("edited a label", "project_id", row.ProjectID, "label_id", row.ID)
	s.publishLabels(ctx, row.ProjectID)
	return toLabel(row), nil
}

// DeleteLabel removes a label from its project and from every card that carried it. Each card that
// carried it is published as changed too, so a client that draws cards never shows a label that is
// gone.
func (s *Service) DeleteLabel(ctx context.Context, id string) error {
	var carried []string
	var projectID string
	err := s.store.Write(ctx, func(q *db.Queries) error {
		row, err := q.GetLabel(ctx, id)
		if err != nil {
			return notFound(fmt.Errorf("read label %s: %w", id, err), notFoundLabel(id))
		}
		projectID = row.ProjectID
		// The cards that carry the label, before the delete removes the rows that say so.
		cards, err := q.ListCardsWithLabel(ctx, id)
		if err != nil {
			return fmt.Errorf("list the cards with label %s: %w", id, err)
		}
		carried = cards
		if _, err := q.DeleteLabel(ctx, id); err != nil {
			return fmt.Errorf("delete label %s: %w", id, err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.log.Info("deleted a label", "project_id", projectID, "label_id", id, "cards", len(carried))
	s.publishLabels(ctx, projectID)
	for _, cardID := range carried {
		s.publishCardUpdated(ctx, cardID)
	}
	return nil
}

// label reads one label by its id.
func (s *Service) label(ctx context.Context, id string) (protocol.Label, error) {
	row, err := s.store.Queries().GetLabel(ctx, id)
	if err != nil {
		return protocol.Label{}, notFound(fmt.Errorf("read label %s: %w", id, err), notFoundLabel(id))
	}
	return toLabel(row), nil
}

// publishLabels publishes label.updated with the project's labels as they now are. It is critical:
// a client that missed it would keep offering a label that is gone.
func (s *Service) publishLabels(ctx context.Context, projectID string) {
	snapshot, err := s.Labels(ctx, projectID)
	if err != nil {
		// The change is committed; the event says what the list is now. A read that fails here is
		// logged and the clients resync on their next connection.
		s.log.Error("could not read the labels to publish them", "project_id", projectID, "error", err)
		return
	}
	s.publish(protocol.ProjectTopic(projectID), protocol.EventTypeLabelUpdated,
		protocol.LabelUpdatedEventData{ProjectID: projectID, Labels: snapshot.Labels}, true)
}

// publishCardUpdated publishes one card's new state, for a change that came from somewhere other
// than the card itself, such as a label being deleted.
func (s *Service) publishCardUpdated(ctx context.Context, cardID string) {
	card, err := s.Card(ctx, cardID)
	if err != nil {
		s.log.Error("could not read a card to publish it", "card_id", cardID, "error", err)
		return
	}
	s.publish(protocol.ProjectTopic(card.ProjectID), protocol.EventTypeCardUpdated, protocol.CardEventData{Card: card}, false)
}

// checkLabelUpdate refuses an edit that is not allowed, before the store is touched.
func checkLabelUpdate(in protocol.UpdateLabelRequest) error {
	if in.Name != nil {
		if err := checkLabelName(strings.TrimSpace(*in.Name)); err != nil {
			return err
		}
	}
	if in.Color != nil && !in.Color.Valid() {
		return protocol.InvalidArgument("That is not a label color Marshal knows.").With("color", string(*in.Color))
	}
	return nil
}

// labelNameTaken reports whether a project already uses a name, ignoring case and spaces. exceptID
// is the label being renamed, which may keep its own name.
func labelNameTaken(ctx context.Context, q *db.Queries, projectID, name, exceptID string) (bool, error) {
	existing, err := q.ListLabelsByProject(ctx, projectID)
	if err != nil {
		return false, fmt.Errorf("list the labels of project %s: %w", projectID, err)
	}
	for _, label := range existing {
		if label.ID != exceptID && strings.EqualFold(label.Name, name) {
			return true, nil
		}
	}
	return false, nil
}

// checkLabelName refuses a label name that cannot be used.
func checkLabelName(name string) error {
	switch {
	case name == "":
		return protocol.InvalidArgument("Give the label a name.")
	case utf8.RuneCountInString(name) > maxLabelChars:
		return protocol.InvalidArgument(fmt.Sprintf("Label names can have at most %d characters.", maxLabelChars))
	}
	return nil
}

func notFoundLabel(id string) *protocol.Error {
	return protocol.NotFound("label").With("id", id)
}
