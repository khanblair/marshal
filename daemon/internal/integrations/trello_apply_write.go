package integrations

// The writes of Apply, one per kind of change. Each one writes to Trello, then records what it wrote
// in the snapshot and the links, so the change stops being pending and a later refresh agrees.

import (
	"context"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/integrations/trello"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// writeOp writes one change to an existing Trello card and returns the snapshot it leaves behind.
func (s *Service) writeOp(ctx context.Context, run *applyRun, op trelloOp, stored storedSnap) (storedSnap, error) {
	snap := stored.Snap
	var err error
	switch op.change.Kind {
	case protocol.TrelloChangeKindUpdateCard:
		err = writeWords(ctx, run, op, stored, &snap)
	case protocol.TrelloChangeKindMoveDone:
		err = writeMoveDone(ctx, run, stored, &snap)
	case protocol.TrelloChangeKindAddComment:
		err = s.writeComment(ctx, run, op, stored, &snap)
	case protocol.TrelloChangeKindSetItem:
		err = s.writeTick(ctx, run, op, stored, &snap)
	case protocol.TrelloChangeKindAddItem:
		err = s.writeAddItem(ctx, run.env.client, op, op.card.ID, &snap)
	case protocol.TrelloChangeKindAddChecklist:
		err = s.writeAddChecklist(ctx, run.env.client, op, op.card.ID, stored.TrelloID, &snap)
	default:
		err = fmt.Errorf("no way to write a %q change", op.change.Kind)
	}
	// What did reach Trello is recorded even when a later part of the change failed, so it does not
	// stay pending and get written twice. The bookkeeping ignores a cancel for the same reason.
	if err == nil {
		for _, key := range conflictKeysOf(op) {
			delete(snap.Conflicts, key)
		}
	}
	stored.Snap = snap
	if saveErr := s.saveSnap(context.WithoutCancel(ctx), op.card.ID, stored.TrelloID, stored.URL, stored.FromTrello, snap); saveErr != nil {
		return stored, saveErr
	}
	return stored, err
}

func writeWords(ctx context.Context, run *applyRun, op trelloOp, stored storedSnap, snap *trelloSnap) error {
	fields := trello.UpdateCardFields{Name: op.name, Desc: op.desc}
	if err := run.env.client.UpdateCard(ctx, stored.TrelloID, fields); err != nil {
		return err
	}
	if op.name != nil {
		snap.Name = *op.name
	}
	if op.desc != nil {
		snap.Desc = *op.desc
	}
	return nil
}

func writeMoveDone(ctx context.Context, run *applyRun, stored storedSnap, snap *trelloSnap) error {
	lists, err := run.boardLists(ctx)
	if err != nil {
		return err
	}
	done, ok := doneListOf(lists)
	if !ok {
		return fmt.Errorf("the board has no list that reads as done")
	}
	if err := run.env.client.MoveCard(ctx, stored.TrelloID, done.ID); err != nil {
		return err
	}
	snap.ListID, snap.ListName, snap.Done = done.ID, done.Name, true
	return nil
}

func (s *Service) writeComment(ctx context.Context, run *applyRun, op trelloOp, stored storedSnap, snap *trelloSnap) error {
	id, err := run.env.client.AddComment(ctx, stored.TrelloID, op.text)
	if err != nil {
		return err
	}
	if err := s.saveLink(context.WithoutCancel(ctx), op.card.ID, linkComment, op.commentID, id); err != nil {
		return err
	}
	snap.Comments = append(snap.Comments, id)
	return nil
}

func (s *Service) writeTick(ctx context.Context, run *applyRun, op trelloOp, stored storedSnap, snap *trelloSnap) error {
	links, err := s.loadLinks(ctx, op.card.ID)
	if err != nil {
		return err
	}
	itemID, _ := links.trelloID(linkItem, op.itemID)
	if err := run.env.client.SetCheckItem(ctx, stored.TrelloID, itemID, op.done); err != nil {
		return err
	}
	for i := range snap.Checklists {
		if it := snap.Checklists[i].item(itemID); it != nil {
			it.Done = op.done
		}
	}
	return nil
}

func (s *Service) writeAddItem(ctx context.Context, client *trello.Client, op trelloOp, cardID string, snap *trelloSnap) error {
	links, err := s.loadLinks(ctx, cardID)
	if err != nil {
		return err
	}
	trelloListID, ok := links.trelloID(linkChecklist, op.listID)
	if !ok {
		return fmt.Errorf("the checklist is not on the Trello card")
	}
	item, err := client.AddCheckItem(ctx, trelloListID, op.text, op.done)
	if err != nil {
		return err
	}
	if err := s.saveLink(context.WithoutCancel(ctx), cardID, linkItem, op.itemID, item.ID); err != nil {
		return err
	}
	if base := snap.list(trelloListID); base != nil {
		base.Items = append(base.Items, snapItem{ID: item.ID, Name: item.Name, Done: item.Done()})
	}
	return nil
}

func (s *Service) writeAddChecklist(ctx context.Context, client *trello.Client, op trelloOp, cardID, trelloID string, snap *trelloSnap) error {
	list, err := client.CreateChecklist(ctx, trelloID, op.text)
	if err != nil {
		return err
	}
	if err := s.saveLink(context.WithoutCancel(ctx), cardID, linkChecklist, op.listID, list.ID); err != nil {
		return err
	}
	written := snapList{ID: list.ID, Name: list.Name, Items: []snapItem{}}
	// The checklist is recorded whatever happens to its items, so the items already on Trello are
	// known to the next Apply and refresh.
	snap.Checklists = append(snap.Checklists, written)
	idx := len(snap.Checklists) - 1
	var stepErr error
	for _, it := range op.items {
		if stepErr = s.pause(ctx); stepErr != nil {
			break
		}
		item, err := client.AddCheckItem(ctx, list.ID, it.Text, it.Done)
		if err != nil {
			stepErr = err
			break
		}
		if err := s.saveLink(context.WithoutCancel(ctx), cardID, linkItem, it.ID, item.ID); err != nil {
			stepErr = err
			break
		}
		written.Items = append(written.Items, snapItem{ID: item.ID, Name: item.Name, Done: item.Done()})
		snap.Checklists[idx] = written
	}
	snap.Checklists[idx] = written
	return stepErr
}

// applyCreate makes a Trello card for a Marshal card that has none, on the list new cards go to.
func (s *Service) applyCreate(ctx context.Context, run *applyRun, op trelloOp) error {
	lists, err := run.boardLists(ctx)
	if err != nil {
		return err
	}
	listID := run.env.link.NewCardListID
	if listID == "" {
		listID = firstOpenList(lists)
	}
	if listID == "" {
		return fmt.Errorf("the board has no list to put a card on")
	}
	if err := s.pause(ctx); err != nil {
		return err
	}
	name, desc := "", ""
	if op.name != nil {
		name = *op.name
	}
	if op.desc != nil {
		desc = *op.desc
	}
	tc, err := run.env.client.CreateCard(ctx, listID, name, desc)
	if err != nil {
		return err
	}
	if err := s.LinkExternal(context.WithoutCancel(ctx), op.card.ID, trello.ID, tc.ID); err != nil {
		return err
	}
	snap := snapOf(tc, lists)
	snap.Checklists = []snapList{}
	if err := s.saveSnap(ctx, op.card.ID, tc.ID, tc.URL, false, snap); err != nil {
		return err
	}
	cur, _, err := s.loadSnap(ctx, op.card.ID)
	if err == nil {
		run.snaps[op.card.ID] = cur
	}
	return err
}

// firstOpenList is the first list of the board that does not read as done, or the first list.
func firstOpenList(lists []trello.List) string {
	for _, l := range lists {
		if !looksDone(l.Name) {
			return l.ID
		}
	}
	if len(lists) > 0 {
		return lists[0].ID
	}
	return ""
}
