// Package memory owns Marshal's memory of a project: the one note each card keeps in the vault, the
// files and packages a card has claimed, and the search over the notes. It is the module
// docs/architecture.md section 3 calls `memory` ("knowledge base, lessons, card notes, vault sync,
// session search") and the groundwork of docs/backend-checklist.md B7.2, B7.4, and B7.5.
//
// # Where memory lives
//
// In the vault, on disk, as markdown a person can read and edit: section 12 lays it out as
// `<vault>/<project>/memory`, `lessons`, `cards/<card>.md`, and `briefs/`. The vault root is
// `<data>/vault` unless the person has pointed it somewhere else, and the service is handed it once
// when it is built. A note's file is what a person edits in Obsidian, so the file - not a row - is
// the truth about what a note says.
//
// The `notes` row (migration 0019) is the *index* of that file and not a second copy of it: it is
// where the author and the save time live, it is what search matches against, and it is what lets
// the daemon answer "which card is this file" without walking the tree. A save writes the file and
// then the row; the vault watcher (task 7.8) is what brings an edit made in Obsidian back into the
// row.
//
// # The project folder is the project's id
//
// `<project>` in that path is the project's id, not its name. The id is made from the name when the
// project is added (protocol.SlugFromName), so it reads the way the name does - "small-repo",
// "web-dashboard-2" - and it has the two properties a folder needs and a name does not: it is unique
// (two projects may not share one, and a numbered one is chosen until it is free) and it never
// changes, so renaming a project never moves the vault out from under the person's links.
//
// # The module boundary
//
// A card and its project are read through the Cards interface below and not out of the projects
// module's tables, so this module never reaches into another one's data (section 3) and a change to
// how a card is found changes one service. The three readers are what a note's path is built from
// and what the vault watcher turns a file name back into a card with: a card for its number and its
// title, a project's cards for the number in a file's name, and the project for nothing today but
// kept so the placeholder note can name it.
package memory

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
)

// Cards is what memory needs from the projects service: one card, the cards of a project, and the
// project a card is in.
type Cards interface {
	// Card returns one card by its id. A card that is not there comes back as the projects
	// service's own not-found error, which this module passes on rather than inventing its own.
	Card(ctx context.Context, id string) (protocol.Card, error)
	// Cards returns every card of one project. The vault watcher (task 7.8) needs them: a note file
	// in the vault names the card it belongs to only by the number in its file name, so the watcher
	// reads the project's cards to turn that number back into a card.
	Cards(ctx context.Context, projectID string) ([]protocol.Card, error)
	// Get returns one project by its id. It is named Get and not Project because that is the
	// method the projects service has, and an interface whose methods do not match teaches whoever
	// wires this up to write an adapter for no reason.
	Get(ctx context.Context, id string) (protocol.Project, error)
}

// Deps are the parts the service is built from.
type Deps struct {
	// Store holds the note rows, their search index, and the file claims. Required.
	Store *store.Store
	// Cards resolves a card and its project, which is what a note's path is made from. Required.
	Cards Cards
	// Root is the vault root on disk. Required, and never empty on purpose: a service with no root
	// would build paths under the current directory, which is the one place a note must never be
	// written by accident.
	Root string
}

// Service is Marshal's memory. It is safe for use by many goroutines: it keeps nothing between
// calls beyond its own clock and its id counter.
type Service struct {
	store   *store.Store
	cards   Cards
	root    string
	now     func() time.Time
	entropy io.Reader

	idMu sync.Mutex
}

// Option changes how New builds a Service.
type Option func(*Service)

// WithClock sets the clock that stamps a save. The default is time.Now.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// WithEntropy sets where a note's own id takes its random bytes from. The default is
// crypto/rand.Reader, and tests set it so an id is the same on every run.
func WithEntropy(entropy io.Reader) Option {
	return func(s *Service) {
		if entropy != nil {
			s.entropy = entropy
		}
	}
}

// New builds the service. Every part is required: a store to read and write the rows, the cards to
// build a note's path from, and the vault root to write under.
func New(deps Deps, opts ...Option) (*Service, error) {
	switch {
	case deps.Store == nil:
		return nil, errors.New("memory: a store is required")
	case deps.Cards == nil:
		return nil, errors.New("memory: the cards are required")
	case deps.Root == "":
		return nil, errors.New("memory: a vault root is required")
	}
	s := &Service{
		store: deps.Store, cards: deps.Cards, root: deps.Root,
		now: time.Now, entropy: rand.Reader,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// Root is the vault root the service writes under. The command that starts the daemon uses it to
// say where the vault is.
func (s *Service) Root() string { return s.root }

// newID makes an id for a new note, in the same shape every other opaque id has (section 11.5).
func (s *Service) newID() (string, error) {
	s.idMu.Lock()
	defer s.idMu.Unlock()
	return protocol.NewID(s.now(), s.entropy)
}

// cardAndProject reads the card and its project in one place, because every note call needs both and
// a note's path is built from them together.
func (s *Service) cardAndProject(ctx context.Context, cardID string) (protocol.Card, protocol.Project, error) {
	card, err := s.cards.Card(ctx, cardID)
	if err != nil {
		return protocol.Card{}, protocol.Project{}, fmt.Errorf("read the card: %w", err)
	}
	project, err := s.cards.Get(ctx, card.ProjectID)
	if err != nil {
		return protocol.Card{}, protocol.Project{}, fmt.Errorf("read the card's project: %w", err)
	}
	return card, project, nil
}
