package integrations

// What Marshal remembers of a Trello card between reads (docs/architecture.md section 19.4). The
// snapshot is the Trello side as Marshal last agreed with it: a change Marshal makes is the
// difference between the Marshal card and this, and a change Trello makes is the difference between
// the live card and this.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrations/trello"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The kinds of id a Marshal card's parts are matched to Trello's under.
const (
	linkChecklist = "checklist"
	linkItem      = "item"
	linkComment   = "comment"
)

// trelloSnap is the Trello side of one card as Marshal last agreed with it.
type trelloSnap struct {
	ListID     string     `json:"listId"`
	ListName   string     `json:"listName"`
	Done       bool       `json:"done"`
	Name       string     `json:"name"`
	Desc       string     `json:"desc"`
	Checklists []snapList `json:"checklists"`
	Comments   []string   `json:"comments"`
	// Conflicts says where Trello changed something that Marshal changed too. The key names the
	// thing ("name", "desc", "list", or "item:<trello id>") and the value says what Trello holds.
	Conflicts map[string]string `json:"conflicts,omitempty"`
}

type snapList struct {
	ID    string     `json:"id"`
	Name  string     `json:"name"`
	Items []snapItem `json:"items"`
}

type snapItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Done bool   `json:"done"`
}

// storedSnap is a snapshot with the row it came from.
type storedSnap struct {
	CardID     string
	TrelloID   string
	URL        string
	FromTrello bool
	FetchedAt  time.Time
	Snap       trelloSnap
}

func (s *trelloSnap) list(id string) *snapList {
	for i := range s.Checklists {
		if s.Checklists[i].ID == id {
			return &s.Checklists[i]
		}
	}
	return nil
}

func (l *snapList) item(id string) *snapItem {
	for i := range l.Items {
		if l.Items[i].ID == id {
			return &l.Items[i]
		}
	}
	return nil
}

func (s *trelloSnap) knowsComment(id string) bool {
	for _, c := range s.Comments {
		if c == id {
			return true
		}
	}
	return false
}

// snapOf reads a live Trello card into a snapshot. lists names the board's lists, so the snapshot
// can say which one the card is on and whether it reads as done.
func snapOf(card trello.Card, lists []trello.List) trelloSnap {
	snap := trelloSnap{
		ListID: card.IDList, Name: card.Name, Desc: card.Desc, Comments: []string{},
		Checklists: []snapList{},
	}
	for _, l := range lists {
		if l.ID == card.IDList {
			snap.ListName = l.Name
			snap.Done = looksDone(l.Name)
		}
	}
	for _, cl := range card.Checklists {
		out := snapList{ID: cl.ID, Name: cl.Name, Items: []snapItem{}}
		for _, it := range cl.Items {
			out.Items = append(out.Items, snapItem{ID: it.ID, Name: it.Name, Done: it.Done()})
		}
		snap.Checklists = append(snap.Checklists, out)
	}
	return snap
}

func (s *Service) saveSnap(ctx context.Context, cardID, trelloID, url string, fromTrello bool, snap trelloSnap) error {
	raw, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("write the Trello snapshot of card %s: %w", cardID, err)
	}
	from := int64(0)
	if fromTrello {
		from = 1
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		return q.UpsertTrelloSnapshot(ctx, db.UpsertTrelloSnapshotParams{
			CardID: cardID, TrelloCardID: trelloID, Url: url, FromTrello: from,
			SnapshotJSON: string(raw), FetchedAt: s.now().UnixMilli(),
		})
	})
	if err != nil {
		return fmt.Errorf("save the Trello snapshot of card %s: %w", cardID, err)
	}
	return nil
}

func decodeSnap(row db.TrelloSnapshot) (storedSnap, error) {
	out := storedSnap{
		CardID: row.CardID, TrelloID: row.TrelloCardID, URL: row.Url, FromTrello: row.FromTrello != 0,
		FetchedAt: time.UnixMilli(row.FetchedAt),
	}
	if err := json.Unmarshal([]byte(row.SnapshotJSON), &out.Snap); err != nil {
		return storedSnap{}, fmt.Errorf("read the Trello snapshot of card %s: %w", row.CardID, err)
	}
	return out, nil
}

func (s *Service) loadSnap(ctx context.Context, cardID string) (storedSnap, bool, error) {
	var out storedSnap
	var found bool
	err := s.store.Read(ctx, func(q *db.Queries) error {
		row, err := q.GetTrelloSnapshot(ctx, cardID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		snap, err := decodeSnap(row)
		if err != nil {
			return err
		}
		out, found = snap, true
		return nil
	})
	return out, found, err
}

func (s *Service) projectSnaps(ctx context.Context, projectID string) (map[string]storedSnap, error) {
	out := map[string]storedSnap{}
	err := s.store.Read(ctx, func(q *db.Queries) error {
		rows, err := q.ListProjectTrelloSnapshots(ctx, projectID)
		if err != nil {
			return err
		}
		for _, row := range rows {
			snap, err := decodeSnap(db.TrelloSnapshot(row))
			if err != nil {
				return err
			}
			out[snap.CardID] = snap
		}
		return nil
	})
	return out, err
}

// itemLinks are a card's Marshal ids matched to Trello's, both ways, by kind.
type itemLinks struct {
	toTrello  map[string]string // kind + "\x00" + marshal id -> trello id
	toMarshal map[string]string // kind + "\x00" + trello id -> marshal id
}

func linkKey(kind, id string) string { return kind + "\x00" + id }

func (l itemLinks) trelloID(kind, marshalID string) (string, bool) {
	id, ok := l.toTrello[linkKey(kind, marshalID)]
	return id, ok
}

func (l itemLinks) marshalID(kind, trelloID string) (string, bool) {
	id, ok := l.toMarshal[linkKey(kind, trelloID)]
	return id, ok
}

func (s *Service) loadLinks(ctx context.Context, cardID string) (itemLinks, error) {
	out := itemLinks{toTrello: map[string]string{}, toMarshal: map[string]string{}}
	err := s.store.Read(ctx, func(q *db.Queries) error {
		rows, err := q.ListTrelloItemLinks(ctx, cardID)
		if err != nil {
			return err
		}
		for _, r := range rows {
			out.toTrello[linkKey(r.Kind, r.MarshalID)] = r.TrelloID
			out.toMarshal[linkKey(r.Kind, r.TrelloID)] = r.MarshalID
		}
		return nil
	})
	return out, err
}

func (s *Service) saveLink(ctx context.Context, cardID, kind, marshalID, trelloID string) error {
	err := s.store.Write(ctx, func(q *db.Queries) error {
		return q.UpsertTrelloItemLink(ctx, db.UpsertTrelloItemLinkParams{
			CardID: cardID, Kind: kind, MarshalID: marshalID, TrelloID: trelloID,
		})
	})
	if err != nil {
		return fmt.Errorf("link the %s %s to Trello's %s: %w", kind, marshalID, trelloID, err)
	}
	return nil
}
