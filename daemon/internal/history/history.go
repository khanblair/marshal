package history

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

const (
	// DefaultPageSize is how many events one page holds when the caller does not say.
	DefaultPageSize = 50
	// MaxPageSize is the most events one page returns, so one client cannot ask for a whole card's
	// history at once.
	MaxPageSize = 200
)

// Record is one event a caller wants stored. The store gives each record an id of its own and the
// card's next sequence number, so a caller never invents either.
type Record struct {
	// Kind is what happened.
	Kind Kind
	// State is how an activity entry ended. It is empty for a chat message.
	State State
	// Summary is the one line the history and the activity list show.
	Summary string
	// Detail is the record's structured payload as JSON, and is empty when the summary says
	// everything a reader needs.
	Detail string
}

// Event is one stored event, as a reader gets it back.
type Event struct {
	// ID is the event's own opaque id.
	ID string
	// CardID is the card whose history the event belongs to. It is empty for an event of a chat.
	CardID string
	// ChatID is the chat whose history the event belongs to. It is empty for an event of a card.
	ChatID string
	// SessionID is the session that emitted it.
	SessionID string
	// Seq is the event's place in its owner's history (a card's or a chat's), starting at 1.
	Seq int64
	// Kind is what happened.
	Kind Kind
	// State is how an activity entry ended, and is empty when the event is not one.
	State State
	// Summary is the one line the history and the activity list show.
	Summary string
	// Detail is the event's structured payload as JSON, and is empty when there is none.
	Detail string
	// At is when it happened, in UTC.
	At time.Time
}

// ownerLabel names the card or chat the event belongs to, for an error.
func (e Event) ownerLabel() string {
	if e.ChatID != "" {
		return "chat " + e.ChatID
	}
	return "card " + e.CardID
}

// Page is one page of a card's history, newest first.
type Page struct {
	// Events are the stored events, newest first.
	Events []Event
	// Cursor is the sequence number to pass for the page that follows this one. It is the oldest
	// sequence on this page, or the cursor the caller gave when the page is empty.
	Cursor int64
	// More reports whether another page follows this one.
	More bool
}

// Store writes and reads a card's history. It is safe for use by many goroutines.
type Store struct {
	store   *store.Store
	now     func() time.Time
	entropy io.Reader
	idMu    sync.Mutex
}

// Option changes how New builds a Store.
type Option func(*Store)

// WithClock sets the clock an event is stamped with. The default is time.Now.
func WithClock(now func() time.Time) Option {
	return func(s *Store) {
		if now != nil {
			s.now = now
		}
	}
}

// WithEntropy sets where an event's id comes from. The default is crypto/rand.
func WithEntropy(entropy io.Reader) Option {
	return func(s *Store) {
		if entropy != nil {
			s.entropy = entropy
		}
	}
}

// New builds a Store on an open database.
func New(st *store.Store, opts ...Option) (*Store, error) {
	if st == nil {
		return nil, errors.New("the history store needs the store")
	}
	s := &Store{store: st, now: time.Now, entropy: rand.Reader}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// owner is whose history an event belongs to: a card's or a chat's (docs/architecture.md section
// 10, and migration 0009). Exactly one of the two is set. Each owner numbers its own events from 1.
type owner struct {
	// id is the card's or the chat's opaque id.
	id string
	// chat says which of the two the id is.
	chat bool
}

func cardOwner(id string) owner { return owner{id: id} }
func chatOwner(id string) owner { return owner{id: id, chat: true} }

// kind is the word for the owner in a sentence.
func (o owner) kind() string {
	if o.chat {
		return "chat"
	}
	return "card"
}

// label names the owner for an error, so a message says which kind it was about.
func (o owner) label() string { return o.kind() + " " + o.id }

// Append stores records as the card's next events, in the order given. One write transaction holds
// them all: either every record is stored or none is, and no two appends can take the same
// sequence number. Every record of one call is stamped with the same time, which is the moment the
// events it came from arrived.
func (s *Store) Append(ctx context.Context, cardID, sessionID string, records []Record) error {
	return s.append(ctx, cardOwner(cardID), sessionID, records)
}

// AppendChat is Append for a chat's history: the records become the chat's next events, numbered
// from the chat's own sequence, in the same table a card's history is in.
func (s *Store) AppendChat(ctx context.Context, chatID, sessionID string, records []Record) error {
	return s.append(ctx, chatOwner(chatID), sessionID, records)
}

// append stores records as an owner's next events.
func (s *Store) append(ctx context.Context, o owner, sessionID string, records []Record) error {
	if len(records) == 0 {
		return nil
	}
	if o.id == "" || sessionID == "" {
		return fmt.Errorf("store history: a %s and a session are both needed", o.kind())
	}
	if err := checkRecords(records); err != nil {
		return err
	}
	at := s.now().UTC()
	return s.store.Write(ctx, func(q *db.Queries) error {
		return s.insertAll(ctx, batch{q: q, owner: o, sessionID: sessionID, at: at}, records)
	})
}

// batch is one append's fixed values, so the call that writes a single record stays short.
type batch struct {
	q         *db.Queries
	owner     owner
	sessionID string
	at        time.Time
}

// insertAll writes one append's records in the transaction Append opened, taking the owner's next
// sequence number once and moving it on for each record.
func (s *Store) insertAll(ctx context.Context, b batch, records []Record) error {
	seq, err := nextSeq(ctx, b)
	if err != nil {
		return fmt.Errorf("read the next history sequence of %s: %w", b.owner.label(), err)
	}
	for _, record := range records {
		if err := s.insert(ctx, b, seq, record); err != nil {
			return err
		}
		seq++
	}
	return nil
}

// nextSeq reads the sequence number the owner's next event takes.
func nextSeq(ctx context.Context, b batch) (int64, error) {
	if b.owner.chat {
		return b.q.NextChatEventSeq(ctx, &b.owner.id)
	}
	return b.q.NextSessionEventSeq(ctx, b.owner.id)
}

// insert writes one record at the sequence number given.
func (s *Store) insert(ctx context.Context, b batch, seq int64, record Record) error {
	id, err := s.newID(b.at)
	if err != nil {
		return err
	}
	if b.owner.chat {
		err = b.q.InsertChatEvent(ctx, db.InsertChatEventParams{
			ID: id, ChatID: &b.owner.id, SessionID: b.sessionID, Seq: seq,
			Kind: string(record.Kind), State: string(record.State),
			Summary: record.Summary, DetailJSON: record.Detail, CreatedAt: b.at.UnixMilli(),
		})
	} else {
		err = b.q.InsertSessionEvent(ctx, db.InsertSessionEventParams{
			ID: id, CardID: b.owner.id, SessionID: b.sessionID, Seq: seq,
			Kind: string(record.Kind), State: string(record.State),
			Summary: record.Summary, DetailJSON: record.Detail, CreatedAt: b.at.UnixMilli(),
		})
	}
	if err != nil {
		return fmt.Errorf("store a %s event of %s: %w", record.Kind, b.owner.label(), err)
	}
	return nil
}

// checkRecords refuses a record the table would take but a reader could not use, so the two words
// the migration leaves to Go (kind and state) are checked before anything is written.
func checkRecords(records []Record) error {
	for i, record := range records {
		if !record.Kind.Valid() {
			return fmt.Errorf("store history: record %d has the kind %q, which is not one this package stores", i, record.Kind)
		}
		if !record.State.Valid() {
			return fmt.Errorf("store history: record %d has the state %q, which is not one this package stores", i, record.State)
		}
	}
	return nil
}

// Page returns up to limit events of a card, newest first. A cursor of zero or less starts at the
// newest event; otherwise the page holds the events before that sequence number, which is what
// Page.Cursor carries on from the page before.
func (s *Store) Page(ctx context.Context, cardID string, cursor int64, limit int) (Page, error) {
	return s.page(ctx, pageQuery{owner: cardOwner(cardID), cursor: cursor, limit: limit})
}

// PageChat is Page for a chat's history.
func (s *Store) PageChat(ctx context.Context, chatID string, cursor int64, limit int) (Page, error) {
	return s.page(ctx, pageQuery{owner: chatOwner(chatID), cursor: cursor, limit: limit})
}

// PageByKind is Page restricted to one kind of event, for a card's activity list (docs/
// backend-inventory.md 4.4, N14). A chat has no activity list, so this is a card's alone.
func (s *Store) PageByKind(ctx context.Context, cardID string, kind Kind, cursor int64, limit int) (Page, error) {
	if !kind.Valid() {
		return Page{}, fmt.Errorf("read the history of card %s: %q is not an event kind", cardID, kind)
	}
	return s.page(ctx, pageQuery{owner: cardOwner(cardID), kind: string(kind), cursor: cursor, limit: limit})
}

// pageQuery is what one page read asks for: whose history, which kind of event (empty means every
// kind), where the page starts, and how many events it holds.
type pageQuery struct {
	owner  owner
	kind   string
	cursor int64
	limit  int
}

// page reads one page of an owner's history, newest first. Both the query and the read transaction
// come from the store, so a page is one consistent view.
func (s *Store) page(ctx context.Context, pq pageQuery) (Page, error) {
	if pq.owner.id == "" {
		return Page{}, fmt.Errorf("read history: a %s is needed", pq.owner.kind())
	}
	size := pageSize(pq.limit)
	var rows []db.SessionEvent
	err := s.store.Read(ctx, func(q *db.Queries) error {
		var err error
		rows, err = listRows(ctx, q, pq, size)
		return err
	})
	if err != nil {
		return Page{}, fmt.Errorf("read the history of %s: %w", pq.owner.label(), err)
	}
	return pageOf(rows, size, pq.cursor), nil
}

// listRows runs the query that fits a page read: a chat's, a card's, or a card's of one kind.
func listRows(ctx context.Context, q *db.Queries, pq pageQuery, size int) ([]db.SessionEvent, error) {
	limit := int64(size) + 1
	switch {
	case pq.owner.chat:
		return q.ListChatEvents(ctx, db.ListChatEventsParams{ChatID: &pq.owner.id, Seq: before(pq.cursor), Limit: limit})
	case pq.kind == "":
		return q.ListSessionEvents(ctx, db.ListSessionEventsParams{CardID: pq.owner.id, Seq: before(pq.cursor), Limit: limit})
	default:
		return q.ListSessionEventsByKind(ctx, db.ListSessionEventsByKindParams{
			CardID: pq.owner.id, Kind: pq.kind, Seq: before(pq.cursor), Limit: limit,
		})
	}
}

// Event returns one event in full, for the detail a history row opens on demand. An event that is
// not there is reported as store.IsNotFound.
func (s *Store) Event(ctx context.Context, cardID, id string) (Event, error) {
	return s.event(ctx, cardOwner(cardID), id)
}

// ChatEvent is Event for a chat's history.
func (s *Store) ChatEvent(ctx context.Context, chatID, id string) (Event, error) {
	return s.event(ctx, chatOwner(chatID), id)
}

// event reads one event of an owner.
func (s *Store) event(ctx context.Context, o owner, id string) (Event, error) {
	if o.id == "" || id == "" {
		return Event{}, fmt.Errorf("read one history event: a %s and an event id are both needed", o.kind())
	}
	var row db.SessionEvent
	err := s.store.Read(ctx, func(q *db.Queries) error {
		var err error
		if o.chat {
			row, err = q.GetChatEvent(ctx, db.GetChatEventParams{ChatID: &o.id, ID: id})
		} else {
			row, err = q.GetSessionEvent(ctx, db.GetSessionEventParams{CardID: o.id, ID: id})
		}
		return err
	})
	if err != nil {
		return Event{}, fmt.Errorf("read event %s of %s: %w", id, o.label(), err)
	}
	return eventOf(row), nil
}

// before turns a page cursor into the "sequence less than" bound its query takes. No cursor means
// the newest page, and no stored sequence is ever this large, since a card's sequence starts at 1.
func before(cursor int64) int64 {
	if cursor <= 0 {
		return math.MaxInt64
	}
	return cursor
}

// pageSize is how many events a page returns: the caller's count, the default when it says
// nothing, and no more than MaxPageSize.
func pageSize(limit int) int {
	switch {
	case limit <= 0:
		return DefaultPageSize
	case limit > MaxPageSize:
		return MaxPageSize
	default:
		return limit
	}
}

// pageOf turns the rows read (one more than the page holds, when there is more) into a page.
func pageOf(rows []db.SessionEvent, size int, cursor int64) Page {
	page := Page{Cursor: cursor}
	if len(rows) > size {
		page.More = true
		rows = rows[:size]
	}
	page.Events = make([]Event, len(rows))
	for i, row := range rows {
		page.Events[i] = eventOf(row)
	}
	if len(page.Events) > 0 {
		page.Cursor = page.Events[len(page.Events)-1].Seq
	}
	return page
}

// eventOf turns a stored row into the event a reader gets.
func eventOf(row db.SessionEvent) Event {
	chatID := ""
	if row.ChatID != nil {
		chatID = *row.ChatID
	}
	return Event{
		ID: row.ID, CardID: row.CardID, ChatID: chatID, SessionID: row.SessionID, Seq: row.Seq,
		Kind: Kind(row.Kind), State: State(row.State), Summary: row.Summary,
		Detail: row.DetailJSON, At: time.UnixMilli(row.CreatedAt).UTC(),
	}
}

// newID makes an opaque event id from the store's clock and its entropy reader. The lock makes the
// entropy reader safe for concurrent appends, and lets a test reader need no lock of its own.
func (s *Store) newID(now time.Time) (string, error) {
	s.idMu.Lock()
	defer s.idMu.Unlock()
	id, err := protocol.NewID(now, s.entropy)
	if err != nil {
		return "", fmt.Errorf("make a history event id: %w", err)
	}
	return id, nil
}
