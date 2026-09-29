package cardpanel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

const (
	maxChecklists     = 20
	maxItemsPerList   = 100
	maxChecklistName  = 100
	maxChecklistItem  = 500
	defaultListName   = "Checklist"
	messagePeopleOnly = "This checklist is for people only, so the agent cannot tick it."
	messageNoProof    = "That check has not passed, so it cannot be used to tick this item."
)

// Actor is who changes a checklist: a person, or the card's agent.
type Actor struct {
	// Agent is true for the card's agent.
	Agent bool
	// UserID is the person's user id. Empty for the agent.
	UserID string
}

type evidence struct {
	CheckID string `json:"checkId"`
}

func flag(v int64) bool { return v != 0 }

func toInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

// Checklists lists a card's checklists with their items.
func (s *Service) Checklists(ctx context.Context, cardID string) (protocol.ChecklistList, error) {
	if _, err := s.card(ctx, cardID); err != nil {
		return protocol.ChecklistList{}, err
	}
	return s.checklistList(ctx, cardID)
}

func (s *Service) checklistList(ctx context.Context, cardID string) (protocol.ChecklistList, error) {
	var lists []db.Checklist
	var items []db.ListCardChecklistItemsRow
	err := s.store.Read(ctx, func(q *db.Queries) (err error) {
		if lists, err = q.ListCardChecklists(ctx, cardID); err != nil {
			return err
		}
		items, err = q.ListCardChecklistItems(ctx, cardID)
		return err
	})
	if err != nil {
		return protocol.ChecklistList{}, fmt.Errorf("list the checklists of card %s: %w", cardID, err)
	}
	out := protocol.ChecklistList{Checklists: make([]protocol.Checklist, 0, len(lists)), ServerTime: s.serverTime()}
	for _, l := range lists {
		list := protocol.Checklist{
			ID: l.ID, Name: l.Name, Required: flag(l.Required), PeopleOnly: flag(l.PeopleOnly),
			HideChecked: flag(l.HideChecked), Items: []protocol.ChecklistItem{},
		}
		for _, it := range items {
			if it.ChecklistID != l.ID {
				continue
			}
			list.Items = append(list.Items, protocol.ChecklistItem{
				ID: it.ID, Text: it.Text, Done: flag(it.Done), DoneByKind: it.DoneByKind,
				DoneByID: it.DoneByID, DoneAt: stampPtr(it.DoneAt),
			})
		}
		out.Checklists = append(out.Checklists, list)
	}
	return out, nil
}

// checklistOf finds one checklist of the card, as not found when it is not the card's.
func (s *Service) checklistOf(ctx context.Context, cardID, listID string) (db.Checklist, error) {
	var row db.Checklist
	err := s.store.Read(ctx, func(q *db.Queries) (err error) {
		row, err = q.GetChecklist(ctx, listID)
		return err
	})
	if store.IsNotFound(err) || (err == nil && row.CardID != cardID) {
		return db.Checklist{}, protocol.NotFound("checklist").With("id", listID)
	}
	if err != nil {
		return db.Checklist{}, fmt.Errorf("read checklist %s: %w", listID, err)
	}
	return row, nil
}

// CreateChecklist adds an empty checklist to the card.
func (s *Service) CreateChecklist(ctx context.Context, cardID string, req protocol.CreateChecklistRequest) (protocol.ChecklistList, error) {
	if _, err := s.card(ctx, cardID); err != nil {
		return protocol.ChecklistList{}, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = defaultListName
	}
	if len(name) > maxChecklistName {
		return protocol.ChecklistList{}, protocol.InvalidArgument("That checklist's name is too long.")
	}
	id, err := s.newID()
	if err != nil {
		return protocol.ChecklistList{}, fmt.Errorf("make a checklist id: %w", err)
	}
	now := ms(s.now())
	err = s.store.Write(ctx, func(q *db.Queries) error {
		n, err := q.CountCardChecklists(ctx, cardID)
		if err != nil {
			return err
		}
		if n >= maxChecklists {
			return protocol.InvalidArgument("A card can have at most 20 checklists.")
		}
		return q.CreateChecklist(ctx, db.CreateChecklistParams{
			ID: id, CardID: cardID, Name: name, Position: n, CreatedAt: now, UpdatedAt: now,
		})
	})
	if err != nil {
		return protocol.ChecklistList{}, err
	}
	return s.changed(ctx, cardID)
}

func (s *Service) changed(ctx context.Context, cardID string) (protocol.ChecklistList, error) {
	s.publish(cardID, protocol.EventTypeChecklistUpdated)
	return s.checklistList(ctx, cardID)
}

// UpdateChecklist renames a checklist or changes its flags.
func (s *Service) UpdateChecklist(ctx context.Context, cardID, listID string, req protocol.UpdateChecklistRequest) (protocol.ChecklistList, error) {
	row, err := s.checklistOf(ctx, cardID, listID)
	if err != nil {
		return protocol.ChecklistList{}, err
	}
	p := db.UpdateChecklistParams{
		Name: row.Name, Required: row.Required, PeopleOnly: row.PeopleOnly,
		HideChecked: row.HideChecked, UpdatedAt: ms(s.now()), ID: listID,
	}
	if req.Name != nil {
		p.Name = strings.TrimSpace(*req.Name)
		if p.Name == "" || len(p.Name) > maxChecklistName {
			return protocol.ChecklistList{}, protocol.InvalidArgument("A checklist needs a name of at most 100 characters.")
		}
	}
	if req.Required != nil {
		p.Required = toInt(*req.Required)
	}
	if req.PeopleOnly != nil {
		p.PeopleOnly = toInt(*req.PeopleOnly)
	}
	if req.HideChecked != nil {
		p.HideChecked = toInt(*req.HideChecked)
	}
	if err := s.store.Write(ctx, func(q *db.Queries) error { return q.UpdateChecklist(ctx, p) }); err != nil {
		return protocol.ChecklistList{}, fmt.Errorf("update checklist %s: %w", listID, err)
	}
	return s.changed(ctx, cardID)
}

// DeleteChecklist removes a checklist and its items.
func (s *Service) DeleteChecklist(ctx context.Context, cardID, listID string) (protocol.ChecklistList, error) {
	if _, err := s.checklistOf(ctx, cardID, listID); err != nil {
		return protocol.ChecklistList{}, err
	}
	if err := s.store.Write(ctx, func(q *db.Queries) error { return q.DeleteChecklist(ctx, listID) }); err != nil {
		return protocol.ChecklistList{}, fmt.Errorf("delete checklist %s: %w", listID, err)
	}
	return s.changed(ctx, cardID)
}

// AddItem adds an open line to the end of a checklist.
func (s *Service) AddItem(ctx context.Context, cardID, listID string, req protocol.AddChecklistItemRequest) (protocol.ChecklistList, error) {
	if _, err := s.checklistOf(ctx, cardID, listID); err != nil {
		return protocol.ChecklistList{}, err
	}
	text := strings.TrimSpace(req.Text)
	if text == "" || len(text) > maxChecklistItem {
		return protocol.ChecklistList{}, protocol.InvalidArgument("A line needs some text, at most 500 characters.")
	}
	id, err := s.newID()
	if err != nil {
		return protocol.ChecklistList{}, fmt.Errorf("make an item id: %w", err)
	}
	now := ms(s.now())
	err = s.store.Write(ctx, func(q *db.Queries) error {
		existing, err := q.ListChecklistItems(ctx, listID)
		if err != nil {
			return err
		}
		if len(existing) >= maxItemsPerList {
			return protocol.InvalidArgument("A checklist can have at most 100 lines.")
		}
		return q.CreateChecklistItem(ctx, db.CreateChecklistItemParams{
			ID: id, ChecklistID: listID, Text: text, Position: int64(len(existing)), CreatedAt: now, UpdatedAt: now,
		})
	})
	if err != nil {
		return protocol.ChecklistList{}, err
	}
	return s.changed(ctx, cardID)
}

// itemOf finds one line of a checklist, as not found when it is not in it.
func (s *Service) itemOf(ctx context.Context, listID, itemID string) (db.ChecklistItem, error) {
	var row db.ChecklistItem
	err := s.store.Read(ctx, func(q *db.Queries) (err error) {
		row, err = q.GetChecklistItem(ctx, itemID)
		return err
	})
	if store.IsNotFound(err) || (err == nil && row.ChecklistID != listID) {
		return db.ChecklistItem{}, protocol.NotFound("checklist item").With("id", itemID)
	}
	if err != nil {
		return db.ChecklistItem{}, fmt.Errorf("read checklist item %s: %w", itemID, err)
	}
	return row, nil
}

// RemoveItem removes a line.
func (s *Service) RemoveItem(ctx context.Context, cardID, listID, itemID string) (protocol.ChecklistList, error) {
	if _, err := s.checklistOf(ctx, cardID, listID); err != nil {
		return protocol.ChecklistList{}, err
	}
	if _, err := s.itemOf(ctx, listID, itemID); err != nil {
		return protocol.ChecklistList{}, err
	}
	if err := s.store.Write(ctx, func(q *db.Queries) error { return q.DeleteChecklistItem(ctx, itemID) }); err != nil {
		return protocol.ChecklistList{}, fmt.Errorf("remove checklist item %s: %w", itemID, err)
	}
	return s.changed(ctx, cardID)
}

// Tick ticks or reopens a line for a person. The agent ticks through TickWithCheck.
func (s *Service) Tick(ctx context.Context, cardID, listID, itemID string, done bool, by Actor) (protocol.ChecklistList, error) {
	list, err := s.checklistOf(ctx, cardID, listID)
	if err != nil {
		return protocol.ChecklistList{}, err
	}
	if by.Agent && flag(list.PeopleOnly) {
		return protocol.ChecklistList{}, protocol.Forbidden(messagePeopleOnly)
	}
	if _, err := s.itemOf(ctx, listID, itemID); err != nil {
		return protocol.ChecklistList{}, err
	}
	return s.writeTick(ctx, cardID, itemID, done, by, "")
}

// TickWithCheck ticks a line for the agent, proven by a check that has passed. The evidence is kept,
// so the line is unticked again when that check fails on a later run.
func (s *Service) TickWithCheck(ctx context.Context, cardID, listID, itemID, checkID string) (protocol.ChecklistList, error) {
	list, err := s.checklistOf(ctx, cardID, listID)
	if err != nil {
		return protocol.ChecklistList{}, err
	}
	if flag(list.PeopleOnly) {
		return protocol.ChecklistList{}, protocol.Forbidden(messagePeopleOnly)
	}
	if _, err := s.itemOf(ctx, listID, itemID); err != nil {
		return protocol.ChecklistList{}, err
	}
	var check db.CardCheck
	err = s.store.Read(ctx, func(q *db.Queries) (err error) {
		check, err = q.GetCardCheck(ctx, checkID)
		return err
	})
	if err != nil || check.CardID != cardID || check.Status != string(protocol.CheckStatusPassed) {
		return protocol.ChecklistList{}, protocol.Refused(messageNoProof)
	}
	body, _ := json.Marshal(evidence{CheckID: checkID})
	return s.writeTick(ctx, cardID, itemID, true, Actor{Agent: true}, string(body))
}

func (s *Service) writeTick(ctx context.Context, cardID, itemID string, done bool, by Actor, proof string) (protocol.ChecklistList, error) {
	now := ms(s.now())
	p := db.TickChecklistItemParams{Done: toInt(done), UpdatedAt: now, ID: itemID, DoneAt: &now}
	if done {
		p.DoneByKind, p.DoneByID, p.EvidenceJSON = "person", by.UserID, proof
		if by.Agent {
			p.DoneByKind = "agent"
		}
	}
	if err := s.store.Write(ctx, func(q *db.Queries) error { return q.TickChecklistItem(ctx, p) }); err != nil {
		return protocol.ChecklistList{}, fmt.Errorf("tick checklist item %s: %w", itemID, err)
	}
	return s.changed(ctx, cardID)
}

// untickFailedEvidence reopens every ticked line whose proof is a check that no longer passes.
func (s *Service) untickFailedEvidence(ctx context.Context, cardID string) {
	var items []db.ListCardChecklistItemsRow
	var checks []db.CardCheck
	if err := s.store.Read(ctx, func(q *db.Queries) (err error) {
		if items, err = q.ListCardChecklistItems(ctx, cardID); err != nil {
			return err
		}
		checks, err = q.ListCardChecks(ctx, cardID)
		return err
	}); err != nil {
		s.log.Warn("read the checklist proof", "card", cardID, "error", err)
		return
	}
	failing := map[string]bool{}
	for _, c := range checks {
		failing[c.ID] = c.Status == string(protocol.CheckStatusFailed)
	}
	now := ms(s.now())
	for _, it := range items {
		var proof evidence
		if it.Done == 0 || it.EvidenceJSON == "" || json.Unmarshal([]byte(it.EvidenceJSON), &proof) != nil || !failing[proof.CheckID] {
			continue
		}
		err := s.store.Write(ctx, func(q *db.Queries) error {
			return q.TickChecklistItem(ctx, db.TickChecklistItemParams{UpdatedAt: now, ID: it.ID})
		})
		if err != nil {
			s.log.Warn("untick a line whose proof failed", "item", it.ID, "error", err)
		}
	}
}

// OpenRequiredItems says how many lines of the card's required checklists are still open. A card
// cannot go to Ready to merge until it is zero.
func (s *Service) OpenRequiredItems(ctx context.Context, cardID string) (int, error) {
	var n int64
	err := s.store.Read(ctx, func(q *db.Queries) (err error) {
		n, err = q.CountOpenRequiredItems(ctx, cardID)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("count the open required lines of card %s: %w", cardID, err)
	}
	return int(n), nil
}
