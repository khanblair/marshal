package integrations

// What Marshal would change in Trello (docs/marshal-product-scope.md section 19.2). The list is not
// kept: it is worked out each time from the difference between a Marshal card and its Trello
// snapshot, so it cannot drift from what the cards say. Only what the person dismissed is stored.

import (
	"context"
	"crypto/sha1" //nolint:gosec // names a change; it is not a security hash.
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// detailRunes is how much of a change's text the list shows. Apply writes all of it.
const detailRunes = 600

// trelloOp is one pending change with what Apply needs to carry it out.
type trelloOp struct {
	change protocol.TrelloChange
	card   protocol.Card
	// ListID, ItemID, and CommentID are Marshal's ids for the part the change is about.
	listID, itemID, commentID string
	// name and desc are the words a createCard or updateCard writes. A nil one is left alone.
	name, desc *string
	text       string
	done       bool
	items      []trelloOpItem
	// conflictKey is the snapshot field a conflict on this change is recorded under.
	conflictKey string
}

type trelloOpItem struct {
	ID   string
	Text string
	Done bool
}

func changeID(parts ...string) string {
	sum := sha1.Sum([]byte(strings.Join(parts, "\x00"))) //nolint:gosec // see the import.
	return hex.EncodeToString(sum[:])[:16]
}

func clip(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= detailRunes {
		return string(runes)
	}
	return string(runes[:detailRunes]) + "..."
}

func (op *trelloOp) set(kind protocol.TrelloChangeKind, summary, detail, byKind string, keyParts ...string) {
	parts := append([]string{string(kind), op.card.ID}, keyParts...)
	op.change = protocol.TrelloChange{
		ID: changeID(parts...), CardID: op.card.ID, CardTitle: op.card.Title, Kind: kind,
		Summary: summary, Detail: clip(detail), ByKind: byKind,
	}
}

// PendingTrello answers what is pending for a project's Trello board. A project the board is not
// linked to answers Linked false and nothing else, which is a supported state and not an error.
func (s *Service) PendingTrello(ctx context.Context, projectID string) (protocol.TrelloPending, error) {
	empty := protocol.TrelloPending{
		Changes: []protocol.TrelloChange{}, Cards: []protocol.TrelloLinkedCard{},
		ServerTime: protocol.NewTimestamp(s.now()),
	}
	env, err := s.trelloEnvFor(ctx, projectID)
	if errors.Is(err, ErrTrelloNotLinked) || errors.Is(err, ErrNotConnected) {
		return empty, nil
	}
	if err != nil {
		return empty, err
	}
	ops, snaps, err := s.trelloOps(ctx, env, projectID)
	if err != nil {
		return empty, err
	}
	out := empty
	out.Linked = true
	for _, op := range ops {
		out.Changes = append(out.Changes, op.change)
	}
	out.Cards = linkedCards(snaps)
	state, found, err := s.syncState(ctx, projectID)
	if err != nil {
		return empty, err
	}
	if found {
		out.BoardName = state.BoardName
		at := protocol.NewTimestamp(time.UnixMilli(state.FetchedAt))
		out.FetchedAt = &at
	}
	return out, nil
}

func linkedCards(snaps map[string]storedSnap) []protocol.TrelloLinkedCard {
	out := make([]protocol.TrelloLinkedCard, 0, len(snaps))
	for _, snap := range snaps {
		out = append(out, protocol.TrelloLinkedCard{
			CardID: snap.CardID, URL: snap.URL, ListName: snap.Snap.ListName, FromTrello: snap.FromTrello,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CardID < out[j].CardID })
	return out
}

func (s *Service) syncState(ctx context.Context, projectID string) (db.TrelloSyncState, bool, error) {
	var out db.TrelloSyncState
	var found bool
	err := s.store.Read(ctx, func(q *db.Queries) error {
		row, err := q.GetTrelloSyncState(ctx, projectID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		out, found = row, true
		return nil
	})
	return out, found, err
}

// trelloOps works out every pending change of a project that is not dismissed, card by card.
func (s *Service) trelloOps(ctx context.Context, env trelloEnv, projectID string) ([]trelloOp, map[string]storedSnap, error) {
	cards, err := env.board.Cards(ctx, projectID)
	if err != nil {
		return nil, nil, fmt.Errorf("list the cards for Trello: %w", err)
	}
	sort.SliceStable(cards, func(i, j int) bool { return cards[i].Number < cards[j].Number })
	snaps, err := s.projectSnaps(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}
	dismissed, err := s.dismissedSet(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}
	var ops []trelloOp
	for _, card := range cards {
		snap, linked := snaps[card.ID]
		var found []trelloOp
		if linked {
			found, err = s.cardOps(ctx, env, card, snap)
		} else {
			found, err = s.newCardOps(ctx, card)
		}
		if err != nil {
			return nil, nil, err
		}
		for _, op := range found {
			if !dismissed[op.change.ID] {
				ops = append(ops, op)
			}
		}
	}
	return ops, snaps, nil
}

func (s *Service) dismissedSet(ctx context.Context, projectID string) (map[string]bool, error) {
	out := map[string]bool{}
	err := s.store.Read(ctx, func(q *db.Queries) error {
		ids, err := q.ListDismissedTrelloChanges(ctx, projectID)
		for _, id := range ids {
			out[id] = true
		}
		return err
	})
	return out, err
}

// newCardOps is the one change a Marshal card with no Trello card has: to make one.
func (s *Service) newCardOps(ctx context.Context, card protocol.Card) ([]trelloOp, error) {
	var linked bool
	err := s.store.Read(ctx, func(q *db.Queries) error {
		_, err := q.CardExternalID(ctx, db.CardExternalIDParams{CardID: card.ID, Kind: KindTrello})
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		linked = err == nil
		return err
	})
	if err != nil || linked {
		return nil, err
	}
	title, body := card.Title, card.Body
	op := trelloOp{card: card, name: &title, desc: &body}
	op.set(protocol.TrelloChangeKindCreateCard, "Create this card in Trello", body, "", title, body)
	return []trelloOp{op}, nil
}

// cardOps is every change of one card that is linked to a Trello card.
func (s *Service) cardOps(ctx context.Context, env trelloEnv, card protocol.Card, snap storedSnap) ([]trelloOp, error) {
	var ops []trelloOp
	if op, ok := wordsOp(card, snap.Snap); ok {
		ops = append(ops, op)
	}
	if card.State == protocol.CardStateDone && !snap.Snap.Done {
		op := trelloOp{card: card, conflictKey: "list"}
		op.set(protocol.TrelloChangeKindMoveDone, "Move to Done", "", "person")
		ops = append(ops, op)
	}
	links, err := s.loadLinks(ctx, card.ID)
	if err != nil {
		return nil, err
	}
	lists, err := env.panel.Checklists(ctx, card.ID)
	if err != nil {
		return nil, fmt.Errorf("read the checklists of card %s: %w", card.ID, err)
	}
	ops = append(ops, checklistOps(card, snap.Snap, links, lists)...)
	comments, err := env.panel.Comments(ctx, card.ID)
	if err != nil {
		return nil, fmt.Errorf("read the comments of card %s: %w", card.ID, err)
	}
	ops = append(ops, commentOps(card, links, comments)...)
	for i := range ops {
		for _, key := range conflictKeysOf(ops[i]) {
			if note, ok := snap.Snap.Conflicts[key]; ok && key != "" {
				ops[i].change.Conflict, ops[i].change.ConflictNote = true, note
				break
			}
		}
	}
	return ops, nil
}

// wordsOp is the change that writes a new title or description.
// conflictKeysOf lists the snapshot keys a change's conflict is recorded under. A words change covers
// the title, the description, or both, so each field it writes counts.
func conflictKeysOf(op trelloOp) []string {
	if op.name == nil && op.desc == nil {
		return []string{op.conflictKey}
	}
	var keys []string
	if op.name != nil {
		keys = append(keys, "name")
	}
	if op.desc != nil {
		keys = append(keys, "desc")
	}
	return keys
}

func wordsOp(card protocol.Card, snap trelloSnap) (trelloOp, bool) {
	titleChanged, bodyChanged := card.Title != snap.Name, card.Body != snap.Desc
	if !titleChanged && !bodyChanged {
		return trelloOp{}, false
	}
	op := trelloOp{card: card}
	var summary, detail string
	switch {
	case titleChanged && bodyChanged:
		summary, detail = "Change the title and description", card.Title+"\n\n"+card.Body
	case titleChanged:
		summary, detail = "Rename the card", card.Title
	default:
		summary, detail = "Change the description", card.Body
	}
	if titleChanged {
		title := card.Title
		op.name = &title
		op.conflictKey = "name"
	}
	if bodyChanged {
		body := card.Body
		op.desc = &body
	}
	op.set(protocol.TrelloChangeKindUpdateCard, summary, detail, "person", card.Title, card.Body)
	return op, true
}

// checklistOps are the new checklists, the new items, and the ticks of one card.
func checklistOps(card protocol.Card, snap trelloSnap, links itemLinks, lists protocol.ChecklistList) []trelloOp {
	var ops []trelloOp
	for _, list := range lists.Checklists {
		trelloListID, linked := links.trelloID(linkChecklist, list.ID)
		if !linked {
			if len(list.Items) == 0 {
				continue
			}
			op := trelloOp{card: card, listID: list.ID, text: list.Name}
			var lines []string
			for _, it := range list.Items {
				op.items = append(op.items, trelloOpItem{ID: it.ID, Text: it.Text, Done: it.Done})
				lines = append(lines, "- "+it.Text)
			}
			op.set(protocol.TrelloChangeKindAddChecklist,
				fmt.Sprintf("Add the checklist %q (%d items)", list.Name, len(list.Items)),
				strings.Join(lines, "\n"), "", list.ID, fmt.Sprint(len(list.Items)))
			ops = append(ops, op)
			continue
		}
		base := snap.list(trelloListID)
		if base == nil {
			continue
		}
		for _, it := range list.Items {
			if op, ok := itemOp(card, list, it, base, links); ok {
				ops = append(ops, op)
			}
		}
	}
	return ops
}

func itemOp(card protocol.Card, list protocol.Checklist, it protocol.ChecklistItem, base *snapList, links itemLinks) (trelloOp, bool) {
	trelloItemID, linked := links.trelloID(linkItem, it.ID)
	op := trelloOp{card: card, listID: list.ID, itemID: it.ID, text: it.Text, done: it.Done}
	if !linked {
		op.set(protocol.TrelloChangeKindAddItem, fmt.Sprintf("Add %q to the checklist %q", it.Text, list.Name),
			"", it.DoneByKind, it.ID)
		return op, true
	}
	known := base.item(trelloItemID)
	if known == nil || known.Done == it.Done {
		return trelloOp{}, false
	}
	verb := "Reopen"
	if it.Done {
		verb = "Tick"
	}
	op.conflictKey = "item:" + trelloItemID
	op.set(protocol.TrelloChangeKindSetItem, fmt.Sprintf("%s %q", verb, it.Text), "", it.DoneByKind,
		it.ID, fmt.Sprint(it.Done))
	return op, true
}

// commentOps are the comments Marshal holds that Trello does not. A comment that came from Trello
// is linked when it is copied, so it is never sent back.
func commentOps(card protocol.Card, links itemLinks, comments protocol.CommentList) []trelloOp {
	var ops []trelloOp
	for _, c := range comments.Comments {
		if _, linked := links.trelloID(linkComment, c.ID); linked || strings.TrimSpace(c.Body) == "" {
			continue
		}
		op := trelloOp{card: card, commentID: c.ID, text: c.Body}
		op.set(protocol.TrelloChangeKindAddComment, "Add a comment", c.Body, string(c.AuthorKind), c.ID)
		ops = append(ops, op)
	}
	return ops
}

// DismissTrello stops showing changes the person chose not to send. They stay in Marshal and are
// never written to Trello, until the change itself changes.
func (s *Service) DismissTrello(ctx context.Context, projectID string, ids []string) error {
	now := s.now().UnixMilli()
	err := s.store.Write(ctx, func(q *db.Queries) error {
		for _, id := range ids {
			if err := q.DismissTrelloChange(ctx, db.DismissTrelloChangeParams{
				ProjectID: projectID, ChangeID: id, DismissedAt: now,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("dismiss Trello changes: %w", err)
	}
	s.notifyTrello(projectID)
	return nil
}
