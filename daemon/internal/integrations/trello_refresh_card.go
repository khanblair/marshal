package integrations

// Refreshing one Marshal card that is already linked to a Trello card: a field Trello changed is
// taken over where Marshal did not change it too, and a field both changed is left alone and noted
// in the snapshot's conflicts, which is what lets Apply warn before it overwrites.

import (
	"context"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/integrations/trello"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// refreshTrelloCard reports whether the Marshal card took a change from Trello. keep sets the
// words, the list, and the ticks back to Trello's even where Marshal changed them.
func (s *Service) refreshTrelloCard(ctx context.Context, env trelloEnv, cardID string, read trelloRead, keep bool) (bool, error) {
	stored, found, err := s.loadSnap(ctx, cardID)
	if err != nil {
		return false, err
	}
	tc := read.card
	if !found {
		// A link made before the snapshots existed: Trello's side becomes the base, and what it holds
		// that Marshal lacks is copied the way an import does.
		snap := snapOf(tc, read.lists)
		snap.Checklists = []snapList{}
		for _, cl := range tc.Checklists {
			if imported := s.importChecklist(ctx, env, cardID, cl); imported.ID != "" {
				snap.Checklists = append(snap.Checklists, imported)
			}
		}
		s.importComments(ctx, env, cardID, read.comments, &snap)
		return true, s.saveSnap(ctx, cardID, tc.ID, tc.URL, true, snap)
	}
	card, err := env.cards.Card(ctx, cardID)
	if err != nil {
		return false, fmt.Errorf("read card %s: %w", cardID, err)
	}
	snap := stored.Snap
	snap.Conflicts = map[string]string{}
	changed, err := s.refreshWords(ctx, env, card, tc, &snap, keep)
	if err != nil {
		return changed, err
	}
	moved, err := s.refreshList(ctx, env, card, read, &snap, keep)
	if err != nil {
		return changed, err
	}
	changed = changed || moved
	ticked := s.refreshChecklists(ctx, env, cardID, read, &snap, keep)
	known := len(snap.Comments)
	s.importComments(ctx, env, cardID, read.comments, &snap)
	changed = changed || ticked || len(snap.Comments) > known
	return changed, s.saveSnap(ctx, cardID, tc.ID, tc.URL, stored.FromTrello, snap)
}

// refreshWords takes Trello's title and description over Marshal's where Marshal did not change them.
func (s *Service) refreshWords(ctx context.Context, env trelloEnv, card protocol.Card, tc trello.Card, snap *trelloSnap, keep bool) (bool, error) {
	name, desc := tc.Name, tc.Desc
	var in protocol.UpdateCardRequest
	if name != snap.Name {
		switch {
		case card.Title == name:
			snap.Name = name
		case keep || card.Title == snap.Name:
			in.Title = &name
			snap.Name = name
		default:
			snap.Conflicts["name"] = fmt.Sprintf("Trello renamed this card to %q.", name)
		}
	}
	if desc != snap.Desc {
		switch {
		case card.Body == desc:
			snap.Desc = desc
		case keep || card.Body == snap.Desc:
			in.Body = &desc
			snap.Desc = desc
		default:
			snap.Conflicts["desc"] = "Trello changed this card's description."
		}
	}
	if in.Title == nil && in.Body == nil {
		return false, nil
	}
	if _, err := env.board.UpdateCard(ctx, card.ID, in); err != nil {
		return false, fmt.Errorf("take Trello's words onto card %s: %w", card.ID, err)
	}
	return true, nil
}

// refreshList follows a card Trello moved. Only the done list means something in Marshal: a card that
// reaches it is done, and a card taken out of it is put back in the backlog. A Marshal card the person
// moved to done, which Trello has not been told yet, keeps that and notes where Trello put the card.
func (s *Service) refreshList(ctx context.Context, env trelloEnv, card protocol.Card, read trelloRead, snap *trelloSnap, keep bool) (bool, error) {
	tc := read.card
	if tc.IDList == snap.ListID {
		return false, nil
	}
	after := snapOf(tc, read.lists)
	pendingMove := card.State == protocol.CardStateDone && !snap.Done
	// Trello in done too is agreement, not a conflict: both sides moved the card to done.
	if pendingMove && !keep && !after.Done {
		snap.Conflicts["list"] = fmt.Sprintf("Trello moved this card to %q.", after.ListName)
		return false, nil
	}
	target := protocol.CardState("")
	switch {
	case after.Done && card.State != protocol.CardStateDone:
		target = protocol.CardStateDone
	case !after.Done && card.State == protocol.CardStateDone && (snap.Done || keep):
		target = protocol.CardStateBacklog
	}
	snap.ListID, snap.ListName, snap.Done = after.ListID, after.ListName, after.Done
	if target == "" {
		return false, nil
	}
	if _, err := env.cards.MoveCard(ctx, card.ID, protocol.MoveCardRequest{State: target}); err != nil {
		return false, fmt.Errorf("follow Trello's move of card %s: %w", card.ID, err)
	}
	return true, nil
}

// refreshChecklists copies new checklists and items, and follows ticks Marshal did not change.
func (s *Service) refreshChecklists(ctx context.Context, env trelloEnv, cardID string, read trelloRead, snap *trelloSnap, keep bool) bool {
	links, err := s.loadLinks(ctx, cardID)
	if err != nil {
		s.log.Warn("could not read the Trello links of a card", "card", cardID, "error", err)
		return false
	}
	panel, err := env.panel.Checklists(ctx, cardID)
	if err != nil {
		s.log.Warn("could not read a card's checklists", "card", cardID, "error", err)
		return false
	}
	changed := false
	for _, cl := range read.card.Checklists {
		marshalListID, linked := links.marshalID(linkChecklist, cl.ID)
		if !linked {
			if imported := s.importChecklist(ctx, env, cardID, cl); imported.ID != "" {
				snap.Checklists = append(snap.Checklists, imported)
				changed = true
			}
			continue
		}
		if snap.list(cl.ID) == nil {
			snap.Checklists = append(snap.Checklists, snapList{ID: cl.ID, Name: cl.Name, Items: []snapItem{}})
		}
		mine := findChecklist(panel, marshalListID)
		for _, it := range cl.Items {
			ref := itemRefresh{cardID: cardID, marshalListID: marshalListID, sl: snap.list(cl.ID), mine: mine, links: links, snap: snap, keep: keep}
			if s.refreshItem(ctx, env, ref, it) {
				changed = true
			}
		}
	}
	return changed
}

// itemRefresh is what refreshing one checklist item needs to know about its card.
type itemRefresh struct {
	cardID, marshalListID string
	sl                    *snapList
	mine                  *protocol.Checklist
	links                 itemLinks
	snap                  *trelloSnap
	keep                  bool
}

// refreshItem brings one Trello checklist item into line with Marshal's, and reports whether
// Marshal's changed.
func (s *Service) refreshItem(ctx context.Context, env trelloEnv, ref itemRefresh, it trello.CheckItem) bool {
	cardID, marshalListID, sl, mine, links, snap, keep := ref.cardID, ref.marshalListID, ref.sl, ref.mine, ref.links, ref.snap, ref.keep
	marshalItemID, linked := links.marshalID(linkItem, it.ID)
	if !linked {
		if err := s.importItem(ctx, env, cardID, marshalListID, it); err != nil {
			s.log.Warn("could not copy a Trello checklist item", "card", cardID, "item", it.ID, "error", err)
			return false
		}
		sl.Items = append(sl.Items, snapItem{ID: it.ID, Name: it.Name, Done: it.Done()})
		return true
	}
	base := sl.item(it.ID)
	if base == nil {
		sl.Items = append(sl.Items, snapItem{ID: it.ID, Name: it.Name, Done: it.Done()})
		return false
	}
	if base.Done == it.Done() {
		return false
	}
	mineDone := itemDone(mine, marshalItemID)
	if mineDone == it.Done() || keep || mineDone == base.Done {
		base.Done = it.Done()
		if mineDone == it.Done() {
			return false
		}
		if _, err := env.panel.SetItem(ctx, cardID, marshalListID, marshalItemID, it.Done(), env.owner); err != nil {
			s.log.Warn("could not take Trello's tick onto a card", "card", cardID, "item", it.ID, "error", err)
			base.Done = !it.Done()
			return false
		}
		return true
	}
	verb := "reopened"
	if it.Done() {
		verb = "ticked"
	}
	snap.Conflicts["item:"+it.ID] = fmt.Sprintf("Trello %s %q.", verb, it.Name)
	return false
}

func findChecklist(lists protocol.ChecklistList, id string) *protocol.Checklist {
	for i := range lists.Checklists {
		if lists.Checklists[i].ID == id {
			return &lists.Checklists[i]
		}
	}
	return nil
}

func itemDone(list *protocol.Checklist, itemID string) bool {
	if list == nil {
		return false
	}
	for _, it := range list.Items {
		if it.ID == itemID {
			return it.Done
		}
	}
	return false
}
