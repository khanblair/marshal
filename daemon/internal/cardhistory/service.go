// Package cardhistory serves a card's chat and its activity over the API (docs/backend-checklist.md
// B2.6 and B2.8, N13 and N14), and a project chat's messages (B2.10), which are the same stored
// events under the same wire types. It is the layer between the HTTP routes and the stored history:
// the routes read the request and write the answer, and this package decides the page size, walks
// the cursor, maps every stored event to a wire message or activity item, and turns a missing card
// or message into the answer a client reads.
//
// It reads only. Nothing here writes the history, and nothing here publishes an event: the write
// path is the session manager's (internal/session), through history.Store.
package cardhistory

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

const (
	// DefaultPageSize is how many items one page holds when the caller does not say. It matches
	// the API's own default (api.DefaultPageLimit) and the history store's.
	DefaultPageSize = 50
	// MaxPageSize is the most items one page returns, so one client cannot ask for a whole card's
	// history at once. It matches the API's own cap.
	MaxPageSize = 200
	// maxStoreChunk is the most stored events one read asks the history store for. Asking for more
	// than the store's own cap would only be cut there.
	maxStoreChunk = 200
	// scanBudgetFactor is how many stored events one page may read to fill itself. An activity
	// page skips the chat messages in between, and a page filtered by one activity kind skips
	// nearly everything, so one read of `limit` rows is not enough: a page reads at most this many
	// times its size, hands back what it found, and lets the next page pick up where it stopped.
	scanBudgetFactor = 4
	// minScanBudget is the smallest number of stored events one page reads, so a small page still
	// fills instead of walking the history four events at a time.
	minScanBudget = 200
)

// Deps are the parts the service is built from. Both are required.
type Deps struct {
	// Store is the open database. It is read to check that the card or chat exists, so an unknown
	// one is not found rather than an empty history.
	Store *store.Store
	// History is the card's stored history, which is what a page is read from.
	History *history.Store
}

// Service serves a card's chat and activity. It is safe for use by many goroutines.
type Service struct {
	store   *store.Store
	history *history.Store
	log     *slog.Logger
	now     func() time.Time
}

// Option changes how New builds a Service.
type Option func(*Service)

// WithClock sets the clock the answer's serverTime comes from. The default is time.Now.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// WithLogger sets the logger.
func WithLogger(log *slog.Logger) Option {
	return func(s *Service) {
		if log != nil {
			s.log = log
		}
	}
}

// New builds the service. The store and the history are both required: the store says whether the
// card is there, and the history is what a page comes from.
func New(deps Deps, opts ...Option) (*Service, error) {
	if deps.Store == nil || deps.History == nil {
		return nil, fmt.Errorf("cardhistory: a store and the history are both required")
	}
	s := &Service{
		store: deps.Store, history: deps.History,
		log: slog.New(slog.DiscardHandler), now: time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// Page is one page of items, with the position the page after it starts from.
type Page[T any] struct {
	// Items are the items of this page, newest first. Never nil, so a page with nothing on it
	// sends an empty list rather than null.
	Items []T
	// Cursor is the sequence of the oldest stored event this page read. It is where the next page
	// starts, and it is only meaningful when More is true.
	Cursor int64
	// More reports whether another page follows this one.
	More bool
}

// Messages returns one page of a card's chat, newest first. The cursor is the sequence the
// previous page ended at, or zero for the newest page. An unknown card is not found.
func (s *Service) Messages(ctx context.Context, cardID string, cursor int64, limit int) (Page[protocol.ChatMessage], error) {
	if err := s.cardExists(ctx, cardID); err != nil {
		return Page[protocol.ChatMessage]{}, err
	}
	return fill(ctx, s.cardPager(cardID), cursor, pageSize(limit), messageOf)
}

// ChatMessages is Messages for a project chat: the chat's own history, newest first, paged the same
// way. An unknown chat is not found.
func (s *Service) ChatMessages(ctx context.Context, chatID string, cursor int64, limit int) (Page[protocol.ChatMessage], error) {
	if err := s.chatExists(ctx, chatID); err != nil {
		return Page[protocol.ChatMessage]{}, err
	}
	return fill(ctx, s.chatPager(chatID), cursor, pageSize(limit), messageOf)
}

// messageOf maps one stored event to a chat message. Every event is one.
func messageOf(ev history.Event) (protocol.ChatMessage, bool, error) {
	message, err := history.ChatMessageOf(ev)
	return message, true, err
}

// Activity returns one page of a card's activity, newest first. A kind that is not empty keeps
// only the entries of that kind. The cursor works as it does for Messages, and an unknown card is
// not found.
func (s *Service) Activity(ctx context.Context, cardID string, kind protocol.ActivityKind, cursor int64, limit int) (Page[protocol.ActivityItem], error) {
	if kind != "" && !kind.Valid() {
		return Page[protocol.ActivityItem]{}, fmt.Errorf("cardhistory: %q is not an activity kind", kind)
	}
	if err := s.cardExists(ctx, cardID); err != nil {
		return Page[protocol.ActivityItem]{}, err
	}
	return fill(ctx, s.cardPager(cardID), cursor, pageSize(limit),
		func(ev history.Event) (protocol.ActivityItem, bool, error) {
			item, ok, err := history.ActivityItemOf(ev)
			return item, ok && (kind == "" || item.Kind == kind), err
		})
}

// MessageDetail returns one message in full, for the tool block a person opens on demand: the tool
// call's output and the files it changed. A message that carries no such detail answers with the
// message alone. An unknown card or an id that is not this card's is not found.
func (s *Service) MessageDetail(ctx context.Context, cardID, messageID string) (protocol.ChatMessageDetail, error) {
	if messageID == "" {
		return protocol.ChatMessageDetail{}, notFoundMessage(messageID)
	}
	if err := s.cardExists(ctx, cardID); err != nil {
		return protocol.ChatMessageDetail{}, err
	}
	return s.detailOf(messageID, func() (history.Event, error) {
		return s.history.Event(ctx, cardID, messageID)
	})
}

// ChatMessageDetail is MessageDetail for a project chat. An unknown chat or an id that is not this
// chat's is not found.
func (s *Service) ChatMessageDetail(ctx context.Context, chatID, messageID string) (protocol.ChatMessageDetail, error) {
	if messageID == "" {
		return protocol.ChatMessageDetail{}, notFoundMessage(messageID)
	}
	if err := s.chatExists(ctx, chatID); err != nil {
		return protocol.ChatMessageDetail{}, err
	}
	return s.detailOf(messageID, func() (history.Event, error) {
		return s.history.ChatEvent(ctx, chatID, messageID)
	})
}

// detailOf builds the answer of a message read with the reader given, which is what makes one
// builder serve a card and a chat.
func (s *Service) detailOf(messageID string, read func() (history.Event, error)) (protocol.ChatMessageDetail, error) {
	ev, err := read()
	if err != nil {
		if store.IsNotFound(err) {
			return protocol.ChatMessageDetail{}, notFoundMessage(messageID)
		}
		return protocol.ChatMessageDetail{}, fmt.Errorf("read message %s: %w", messageID, err)
	}
	message, err := history.ChatMessageOf(ev)
	if err != nil {
		return protocol.ChatMessageDetail{}, err
	}
	detail := protocol.ChatMessageDetail{
		Message: message, ServerTime: protocol.NewTimestamp(s.now()),
	}
	if message.Kind != protocol.ChatMessageKindTool {
		return detail, nil
	}
	tool, err := history.ToolDetailOf(ev)
	if err != nil {
		return protocol.ChatMessageDetail{}, err
	}
	detail.Tool = &tool
	return detail, nil
}

// pager reads one page of stored events, newest first, for one card or one chat.
type pager func(ctx context.Context, cursor int64, limit int) (history.Page, error)

// cardPager pages one card's history.
func (s *Service) cardPager(cardID string) pager {
	return func(ctx context.Context, cursor int64, limit int) (history.Page, error) {
		return s.history.Page(ctx, cardID, cursor, limit)
	}
}

// chatPager pages one chat's history.
func (s *Service) chatPager(chatID string) pager {
	return func(ctx context.Context, cursor int64, limit int) (history.Page, error) {
		return s.history.PageChat(ctx, chatID, cursor, limit)
	}
}

// fill reads stored events from the history, newest first, and hands each one to convert until it
// has limit items. convert says whether an event belongs on this page, which is what makes one
// reader serve both the chat (every event is a message) and the activity list (only the events
// that ended somehow are entries).
//
// The walk is bounded: a page reads at most scanBudgetFactor times its size in stored events, so a
// card whose history is mostly chat still answers promptly when it is asked for its activity. When
// the budget runs out the page ends where it stopped and More says another page follows, so no
// item is ever skipped.
func fill[T any](
	ctx context.Context, src pager, cursor int64, limit int,
	convert func(history.Event) (T, bool, error),
) (Page[T], error) {
	page := Page[T]{Items: []T{}, Cursor: cursor}
	scan := cursor
	budget := max(limit*scanBudgetFactor, minScanBudget)
	for {
		chunk, err := src(ctx, scan, min(limit-len(page.Items), maxStoreChunk))
		if err != nil {
			return Page[T]{}, err
		}
		budget -= len(chunk.Events)
		for i, ev := range chunk.Events {
			item, keep, err := convert(ev)
			if err != nil {
				return Page[T]{}, err
			}
			if !keep {
				continue
			}
			page.Items = append(page.Items, item)
			page.Cursor = ev.Seq
			if len(page.Items) == limit {
				page.More = i < len(chunk.Events)-1 || chunk.More
				return page, nil
			}
		}
		if !chunk.More {
			// The card's history ends here, so this is the last page. A page with nothing on it
			// keeps the cursor it was given, the way the history store does.
			if len(page.Items) == 0 {
				page.Cursor = cursor
			}
			return page, nil
		}
		if budget <= 0 {
			// Out of budget with the whole chunk read: the next page starts after the oldest
			// event read, so nothing is skipped and nothing is repeated.
			page.Cursor, page.More = chunk.Cursor, true
			return page, nil
		}
		scan = chunk.Cursor
	}
}

// pageSize is how many items a page returns: the caller's count, the default when it says nothing,
// and no more than MaxPageSize.
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

// cardExists reports whether the card is there, as the not found answer when it is not. The
// history tables point at the card, so a card that does not exist has no history to page: an
// unknown card must read as not found rather than as an empty list.
func (s *Service) cardExists(ctx context.Context, cardID string) error {
	err := s.store.Read(ctx, func(q *db.Queries) error {
		_, err := q.GetCard(ctx, cardID)
		return err
	})
	switch {
	case err == nil:
		return nil
	case store.IsNotFound(err):
		return protocol.NotFound("card").With("id", cardID)
	default:
		return fmt.Errorf("read card %s: %w", cardID, err)
	}
}

// chatExists reports whether the chat is there, as the not found answer when it is not, for the
// same reason cardExists does.
func (s *Service) chatExists(ctx context.Context, chatID string) error {
	err := s.store.Read(ctx, func(q *db.Queries) error {
		_, err := q.GetChat(ctx, chatID)
		return err
	})
	switch {
	case err == nil:
		return nil
	case store.IsNotFound(err):
		return protocol.NotFound("chat").With("id", chatID)
	default:
		return fmt.Errorf("read chat %s: %w", chatID, err)
	}
}

// notFoundMessage is the answer for a message that is not on this card or chat. It is built the
// way the services build theirs, so an id that cannot exist and one that belongs to another card or
// chat read the same.
func notFoundMessage(id string) *protocol.Error {
	if len(id) > maxEchoedIDBytes {
		id = id[:maxEchoedIDBytes]
	}
	return protocol.NotFound("message").With("id", id)
}

// maxEchoedIDBytes cuts an id before it is sent back in an error, so a very long one is not
// repeated in full.
const maxEchoedIDBytes = 64
