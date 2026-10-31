package integrations

// Reading a linked Trello board into Marshal (docs/marshal-product-scope.md section 19.2). It is
// the only direction Trello data travels on its own: cards Marshal has not seen become idle Marshal
// cards, and a field Trello changed is taken over only where Marshal did not change it too. Where
// both did, Marshal keeps its own and the snapshot remembers what Trello holds, so Apply can say so.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrations/trello"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// trelloDefaultPace is the wait between two writes to Trello. Trello allows about 100 requests per
// ten seconds for a token, so one every 110 milliseconds stays under it.
const trelloDefaultPace = 110 * time.Millisecond

// ErrTrelloNotLinked is returned for a project that is not the one the Trello board is linked to.
var ErrTrelloNotLinked = errors.New("integrations: this project is not linked to a Trello board")

// TrelloBoard is the board work the sync needs beyond TrelloCards: listing a project's cards and
// changing a card's words. The attached TrelloCards must also satisfy it for a refresh to run.
type TrelloBoard interface {
	// Cards lists every card of a project.
	Cards(ctx context.Context, projectID string) ([]protocol.Card, error)
	// UpdateCard changes a card's title or description.
	UpdateCard(ctx context.Context, id string, in protocol.UpdateCardRequest) (protocol.Card, error)
}

// TrelloPanel is how the sync reads and writes what a card's panel holds: checklists and comments.
// ImportComment is not Post: it never puts the text in front of the card's agent, because text that
// came from Trello must not wake an agent.
type TrelloPanel interface {
	Checklists(ctx context.Context, cardID string) (protocol.ChecklistList, error)
	CreateChecklist(ctx context.Context, cardID, name string) (protocol.ChecklistList, error)
	AddItem(ctx context.Context, cardID, listID, text string) (protocol.ChecklistList, error)
	SetItem(ctx context.Context, cardID, listID, itemID string, done bool, userID string) (protocol.ChecklistList, error)
	Comments(ctx context.Context, cardID string) (protocol.CommentList, error)
	ImportComment(ctx context.Context, cardID, body, userID string, at time.Time) (string, error)
}

// SetTrelloPanel attaches the card panel the sync reads and writes checklists and comments through.
func (s *Service) SetTrelloPanel(panel TrelloPanel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trelloPanel = panel
}

// SetTrelloNotifier attaches what tells the event stream that a project's Trello state changed.
func (s *Service) SetTrelloNotifier(notify func(projectID string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trelloNotify = notify
}

func (s *Service) notifyTrello(projectID string) {
	s.mu.Lock()
	notify := s.trelloNotify
	s.mu.Unlock()
	if notify != nil {
		notify(projectID)
	}
}

// trelloEnv is what one refresh or apply works with.
type trelloEnv struct {
	link   TrelloLinkInfo
	client *trello.Client
	cards  TrelloCards
	board  TrelloBoard
	panel  TrelloPanel
	owner  string
}

// trelloEnvFor gathers what a refresh or an apply needs, or says why it cannot run.
func (s *Service) trelloEnvFor(ctx context.Context, projectID string) (trelloEnv, error) {
	link, ok, err := s.TrelloLink(ctx)
	if err != nil {
		return trelloEnv{}, err
	}
	if !ok || link.ProjectID != projectID {
		return trelloEnv{}, ErrTrelloNotLinked
	}
	client, _, err := s.TrelloClient(ctx)
	if err != nil {
		return trelloEnv{}, err
	}
	cards := s.attachedTrelloCards()
	board, _ := cards.(TrelloBoard)
	s.mu.Lock()
	panel := s.trelloPanel
	s.mu.Unlock()
	if cards == nil || board == nil || panel == nil {
		return trelloEnv{}, errors.New("integrations: the board module is not attached to the Trello sync")
	}
	owner, err := s.ownerID(ctx)
	if err != nil {
		return trelloEnv{}, err
	}
	return trelloEnv{link: link, client: client, cards: cards, board: board, panel: panel, owner: owner}, nil
}

// ownerID is the person Trello content is written as in Marshal and Marshal content is written as in
// Trello: the owner of this daemon, who is also the person whose token is linked.
func (s *Service) ownerID(ctx context.Context) (string, error) {
	var id string
	err := s.store.Read(ctx, func(q *db.Queries) error {
		owner, err := q.GetOwner(ctx)
		if err != nil {
			return err
		}
		id = owner.ID
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("read the owner for the Trello sync: %w", err)
	}
	return id, nil
}

// TrelloRefreshCounts says what one refresh did.
type TrelloRefreshCounts struct {
	// Imported is how many Trello cards were new to Marshal.
	Imported int
	// Updated is how many Marshal cards took a change from Trello.
	Updated int
}

// RefreshTrello reads the linked board and brings Marshal up to date with it. keep names Marshal
// cards whose title, description, and ticks are set back to Trello's even where Marshal changed them.
func (s *Service) RefreshTrello(ctx context.Context, projectID string, keep []string) (TrelloRefreshCounts, error) {
	s.trelloMu.Lock()
	defer s.trelloMu.Unlock()
	env, err := s.trelloEnvFor(ctx, projectID)
	if err != nil {
		return TrelloRefreshCounts{}, err
	}
	board, err := env.client.GetBoard(ctx, env.link.BoardID)
	if err != nil {
		return TrelloRefreshCounts{}, fmt.Errorf("read the Trello board: %w", err)
	}
	lists, err := env.client.ListLists(ctx, env.link.BoardID)
	if err != nil {
		return TrelloRefreshCounts{}, fmt.Errorf("read the Trello lists: %w", err)
	}
	live, err := env.client.BoardCards(ctx, env.link.BoardID)
	if err != nil {
		return TrelloRefreshCounts{}, fmt.Errorf("read the Trello cards: %w", err)
	}
	comments, err := env.client.BoardComments(ctx, env.link.BoardID)
	if err != nil {
		return TrelloRefreshCounts{}, fmt.Errorf("read the Trello comments: %w", err)
	}
	byCard := commentsByCard(comments)
	keepSet := map[string]bool{}
	for _, id := range keep {
		keepSet[id] = true
	}
	var counts TrelloRefreshCounts
	for _, tc := range live {
		read := trelloRead{card: tc, lists: lists, comments: byCard[tc.ID]}
		cardID, known, err := s.ExternalCard(ctx, trello.ID, tc.ID)
		if err != nil {
			return counts, err
		}
		if !known {
			if err := s.importTrelloCard(ctx, env, read); err != nil {
				s.log.Warn("could not import a Trello card", "trello_card", tc.ID, "error", err)
				continue
			}
			counts.Imported++
			continue
		}
		changed, err := s.refreshTrelloCard(ctx, env, cardID, read, keepSet[cardID])
		if err != nil {
			s.log.Warn("could not refresh a Trello card", "trello_card", tc.ID, "card", cardID, "error", err)
			continue
		}
		if changed {
			counts.Updated++
		}
	}
	if err := s.saveSyncState(ctx, projectID, board.Name); err != nil {
		return counts, err
	}
	s.notifyTrello(projectID)
	return counts, nil
}

// trelloRead is one live Trello card with the board's lists and the card's comments, oldest first.
type trelloRead struct {
	card     trello.Card
	lists    []trello.List
	comments []trello.Comment
}

func commentsByCard(all []trello.Comment) map[string][]trello.Comment {
	out := map[string][]trello.Comment{}
	for _, c := range all {
		out[c.CardID] = append(out[c.CardID], c)
	}
	for _, list := range out {
		sort.SliceStable(list, func(i, j int) bool { return list[i].Date < list[j].Date })
	}
	return out
}

func (s *Service) saveSyncState(ctx context.Context, projectID, boardName string) error {
	err := s.store.Write(ctx, func(q *db.Queries) error {
		return q.UpsertTrelloSyncState(ctx, db.UpsertTrelloSyncStateParams{
			ProjectID: projectID, BoardName: boardName, FetchedAt: s.now().UnixMilli(),
		})
	})
	if err != nil {
		return fmt.Errorf("save when the Trello board was read: %w", err)
	}
	return nil
}

// importTrelloCard makes an idle Marshal card from a Trello card Marshal has not seen, with its
// checklists and comments. The card is never started: it waits for the person.
func (s *Service) importTrelloCard(ctx context.Context, env trelloEnv, read trelloRead) error {
	tc := read.card
	title := tc.Name
	if title == "" {
		title = "Untitled Trello card"
	}
	created, err := env.cards.CreateCard(ctx, env.link.ProjectID, protocol.CreateCardRequest{Title: title, Body: tc.Desc})
	if err != nil {
		return fmt.Errorf("make a card from the Trello card %s: %w", tc.ID, err)
	}
	if err := s.LinkExternal(ctx, created.ID, trello.ID, tc.ID); err != nil {
		return err
	}
	snap := snapOf(tc, read.lists)
	snap.Checklists = []snapList{}
	if snap.Done {
		if _, err := env.cards.MoveCard(ctx, created.ID, protocol.MoveCardRequest{State: protocol.CardStateDone}); err != nil {
			s.log.Warn("could not put an imported card in done", "card", created.ID, "error", err)
		}
	}
	for _, cl := range tc.Checklists {
		if imported := s.importChecklist(ctx, env, created.ID, cl); imported.ID != "" {
			snap.Checklists = append(snap.Checklists, imported)
		}
	}
	s.importComments(ctx, env, created.ID, read.comments, &snap)
	return s.saveSnap(ctx, created.ID, tc.ID, tc.URL, true, snap)
}

// importChecklist copies one Trello checklist onto a card and returns how it reads in the snapshot.
// A checklist or item the card's panel refuses (a limit, say) is left out of the snapshot, so the
// next refresh tries it again.
func (s *Service) importChecklist(ctx context.Context, env trelloEnv, cardID string, cl trello.Checklist) snapList {
	out := snapList{ID: cl.ID, Name: cl.Name, Items: []snapItem{}}
	lists, err := env.panel.CreateChecklist(ctx, cardID, cl.Name)
	if err != nil || len(lists.Checklists) == 0 {
		s.log.Warn("could not copy a Trello checklist", "card", cardID, "checklist", cl.ID, "error", err)
		return snapList{ID: "", Name: cl.Name}
	}
	listID := lists.Checklists[len(lists.Checklists)-1].ID
	if err := s.saveLink(ctx, cardID, linkChecklist, listID, cl.ID); err != nil {
		s.log.Warn("could not remember a Trello checklist", "error", err)
	}
	for _, it := range cl.Items {
		if err := s.importItem(ctx, env, cardID, listID, it); err != nil {
			s.log.Warn("could not copy a Trello checklist item", "card", cardID, "item", it.ID, "error", err)
			continue
		}
		out.Items = append(out.Items, snapItem{ID: it.ID, Name: it.Name, Done: it.Done()})
	}
	return out
}

// importItem copies one Trello checklist item onto a Marshal checklist and ticks it if Trello did.
func (s *Service) importItem(ctx context.Context, env trelloEnv, cardID, listID string, it trello.CheckItem) error {
	lists, err := env.panel.AddItem(ctx, cardID, listID, it.Name)
	if err != nil {
		return err
	}
	itemID := lastItemID(lists, listID)
	if itemID == "" {
		return errors.New("the new item was not in the answer")
	}
	if it.Done() {
		if _, err := env.panel.SetItem(ctx, cardID, listID, itemID, true, env.owner); err != nil {
			return err
		}
	}
	return s.saveLink(ctx, cardID, linkItem, itemID, it.ID)
}

func lastItemID(lists protocol.ChecklistList, listID string) string {
	for _, l := range lists.Checklists {
		if l.ID == listID && len(l.Items) > 0 {
			return l.Items[len(l.Items)-1].ID
		}
	}
	return ""
}

// importComments copies Trello comments the card does not have yet. Each one reads as a comment by
// the owner that says who wrote it in Trello, and none of them reaches the card's agent.
func (s *Service) importComments(ctx context.Context, env trelloEnv, cardID string, comments []trello.Comment, snap *trelloSnap) {
	for _, c := range comments {
		if snap.knowsComment(c.ID) {
			continue
		}
		at, _ := time.Parse(time.RFC3339, c.Date)
		body := c.Text
		if c.Author != "" {
			body = c.Author + " on Trello: " + c.Text
		}
		id, err := env.panel.ImportComment(ctx, cardID, body, env.owner, at)
		if err != nil {
			s.log.Warn("could not copy a Trello comment", "card", cardID, "comment", c.ID, "error", err)
			continue
		}
		if err := s.saveLink(ctx, cardID, linkComment, id, c.ID); err != nil {
			s.log.Warn("could not remember a Trello comment", "error", err)
		}
		snap.Comments = append(snap.Comments, c.ID)
	}
}
