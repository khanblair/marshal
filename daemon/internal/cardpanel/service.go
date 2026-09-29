// Package cardpanel is what a card's panel holds beyond the card: its acceptance checks, its named
// checklists, its comments with their attachments, and the people on it (docs/backend-checklist.md
// B10.2, B10.5, B10.6). Every list is read whole and every change answers the whole list again.
//
// The rows are the store's. This package decides who may change them (the agent may not tick a
// people-only list), what a comment reaches the agent, and which event a change publishes.
package cardpanel

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/localci"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Worktrees says where a card's agent works, which is where a check's command runs.
type Worktrees interface {
	// Worktree returns the folder and branch recorded for a card. Both are empty until it starts.
	Worktree(ctx context.Context, cardID string) (path, branch string, err error)
}

// Agent reaches a card's agent. Send wakes a sleeping session first.
type Agent interface {
	// Send puts a message in front of the card's agent.
	Send(ctx context.Context, cardID, text string) error
}

// Deps are the parts the service is built from. Store is required.
type Deps struct {
	// Store is the open database.
	Store *store.Store
	// Bus carries the change events. Nil publishes nothing.
	Bus *events.Bus
	// Worktrees finds where a check runs. Nil leaves the checks unable to run.
	Worktrees Worktrees
	// Runner runs a check's command. Nil uses the real one.
	Runner localci.Runner
	// Agent gets a comment that mentions it or asks a question. Nil leaves comments with people.
	Agent Agent
	// AttachmentsDir is where attached files are kept. Empty refuses file attachments.
	AttachmentsDir string
	// Log is where what could not be delivered is written. Nil discards.
	Log *slog.Logger
	// Now is the clock. Nil uses the real one.
	Now func() time.Time
	// Entropy makes ids. Nil uses crypto/rand.
	Entropy io.Reader
}

// Service serves the card panel. It is safe for use by many goroutines.
type Service struct {
	store     *store.Store
	bus       *events.Bus
	worktrees Worktrees
	runner    localci.Runner
	agent     Agent
	files     string
	log       *slog.Logger
	now       func() time.Time
	entropy   io.Reader
}

// New builds the service.
func New(deps Deps) (*Service, error) {
	if deps.Store == nil {
		return nil, fmt.Errorf("cardpanel: a store is required")
	}
	s := &Service{
		store: deps.Store, bus: deps.Bus, worktrees: deps.Worktrees, runner: deps.Runner,
		agent: deps.Agent, files: deps.AttachmentsDir, log: deps.Log, now: deps.Now, entropy: deps.Entropy,
	}
	if s.runner == nil {
		s.runner = localci.NewCommandRunner(0, 0, 0)
	}
	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.entropy == nil {
		s.entropy = rand.Reader
	}
	return s, nil
}

func (s *Service) newID() (string, error) { return protocol.NewID(s.now(), s.entropy) }

func (s *Service) serverTime() protocol.Timestamp { return protocol.NewTimestamp(s.now()) }

func ms(t time.Time) int64 { return t.UnixMilli() }

func stamp(v int64) protocol.Timestamp { return protocol.NewTimestamp(time.UnixMilli(v)) }

func stampPtr(v *int64) *protocol.Timestamp {
	if v == nil {
		return nil
	}
	t := stamp(*v)
	return &t
}

// card reads a card, as the not found answer when it is not there.
func (s *Service) card(ctx context.Context, cardID string) (db.Card, error) {
	var row db.Card
	err := s.store.Read(ctx, func(q *db.Queries) (err error) {
		row, err = q.GetCard(ctx, cardID)
		return err
	})
	switch {
	case err == nil:
		return row, nil
	case store.IsNotFound(err):
		return db.Card{}, protocol.NotFound("card").With("id", cardID)
	default:
		return db.Card{}, fmt.Errorf("read card %s: %w", cardID, err)
	}
}

// publish tells everyone following the card that part of its panel changed. The payload only names
// the card: a client reads the list again.
func (s *Service) publish(cardID string, eventType protocol.EventType) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(string(protocol.CardTopic(cardID)), string(eventType), map[string]string{"cardId": cardID}, false)
}
