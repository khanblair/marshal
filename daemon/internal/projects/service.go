// Package projects owns projects, their boards, and their cards: the rows in the database, the
// rules for adding and removing a project, and the events that tell clients about a change.
//
// It changes a project's card state and nothing else does (docs/architecture.md section 6). Other
// modules reach it through the small interfaces below and through the event bus, never through
// its tables.
//
// The module never deletes or edits a file in a user's repository folder. The only things it
// changes in a repository's Git data are the branches it makes for cards (they start with
// "marshal/"), the worktree records for the folders it makes under <data>/worktrees, and, the
// first time a sparse worktree is made, the setting extensions.worktreeConfig in .git/config.
//
// Slow work (asking Git, cloning, reading files) always runs before a database write starts,
// because a write holds the only writer connection. Events are published after the write commits.
package projects

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
)

// The names below are the words of the wire types, so the service and the API speak one language.
type (
	// CreateInput says which repository to add, and what to call it.
	CreateInput = protocol.CreateProjectRequest
	// UpdateInput says which project fields to change. A nil field is left as it is.
	UpdateInput = protocol.UpdateProjectRequest
	// RemoveOptions says what to keep when a project is removed.
	RemoveOptions = protocol.RemoveProjectRequest
	// CardInput describes a new card.
	CardInput = protocol.CreateCardRequest
)

// Projects is what the API layer needs to manage projects.
type Projects interface {
	// Create adds a repository as a project. Options such as WithID are for fixtures and tests.
	Create(ctx context.Context, in CreateInput, opts ...CreateOption) (protocol.Project, error)
	// List returns every project with its badges.
	List(ctx context.Context) (protocol.ProjectListSnapshot, error)
	// Get returns one project.
	Get(ctx context.Context, id string) (protocol.Project, error)
	// Update changes the fields that are set.
	Update(ctx context.Context, id string, in UpdateInput) (protocol.Project, error)
	// Remove stops managing a project. It never deletes the repository folder.
	Remove(ctx context.Context, id string, opts RemoveOptions) error
}

// Cards is what the API layer needs to manage cards and boards.
type Cards interface {
	// CreateCard adds a card to the backlog of a project.
	CreateCard(ctx context.Context, projectID string, in CardInput, opts ...CardOption) (protocol.Card, error)
	// Card returns one card by its opaque id.
	Card(ctx context.Context, id string) (protocol.Card, error)
	// CardByKey returns the card with this project and number.
	CardByKey(ctx context.Context, key protocol.CardKey) (protocol.Card, error)
	// Cards returns all cards of a project, in number order.
	Cards(ctx context.Context, projectID string) ([]protocol.Card, error)
	// Board returns a project's columns and cards.
	Board(ctx context.Context, projectID string) (protocol.BoardSnapshot, error)
	// SetState moves a card to a state. Which moves are allowed is decided by the caller for now.
	SetState(ctx context.Context, id string, state protocol.CardState) (protocol.Card, error)
	// SetWorktree records the worktree folder and branch that were made for a card.
	SetWorktree(ctx context.Context, id, path, branch string) (protocol.Card, error)
}

var (
	_ Projects = (*Service)(nil)
	_ Cards    = (*Service)(nil)
)

// SessionStopper stops the agent sessions of a project. The session manager implements it. The
// service calls it first when a project is removed, so nothing runs in a worktree that is about to
// go.
type SessionStopper interface {
	StopProjectSessions(ctx context.Context, projectID string) error
}

// MemoryRemover deletes a project's memory folder. The memory module implements it. It is called
// only when a project is removed and the person did not ask to keep the memory.
type MemoryRemover interface {
	RemoveProjectMemory(ctx context.Context, projectID string) error
}

// AwakeCounter says how many cards of a project have a running agent. The session manager
// implements it, and the number becomes the "awake" badge. Until it exists the badge is 0.
type AwakeCounter interface {
	AwakeCards(ctx context.Context, projectID string) (int, error)
}

// Deps are the parts the service is built from.
type Deps struct {
	// Store is the open database.
	Store *store.Store
	// Bus receives the events the service publishes.
	Bus *events.Bus
	// Git runs every Git command.
	Git *gitx.Git
	// DataDir is the folder that holds Marshal's own files: worktrees go in <DataDir>/worktrees.
	// It must be a full path.
	DataDir string
}

// Service implements Projects and Cards. It is safe for use by many goroutines.
type Service struct {
	store    *store.Store
	bus      *events.Bus
	git      *gitx.Git
	dataDir  string
	log      *slog.Logger
	now      func() time.Time
	entropy  io.Reader
	sessions SessionStopper
	memory   MemoryRemover
	awake    AwakeCounter
	// allowLocalClone lets a clone come from a folder on this machine. Only dev mode sets it.
	allowLocalClone bool

	locks repoLocks
	// idMu makes ids come from the entropy reader one at a time, so a test reader needs no lock.
	idMu sync.Mutex
}

// Option changes how New builds a Service.
type Option func(*Service)

// WithClock sets the clock. The default is time.Now.
func WithClock(now func() time.Time) Option {
	return func(s *Service) { s.now = now }
}

// WithEntropy sets where ids get their random part. The default is crypto/rand.
func WithEntropy(r io.Reader) Option {
	return func(s *Service) { s.entropy = r }
}

// WithLogger sets the logger. The default logs nothing.
func WithLogger(log *slog.Logger) Option {
	return func(s *Service) {
		if log != nil {
			s.log = log
		}
	}
}

// WithSessionStopper sets who stops a project's sessions on removal. The default does nothing.
func WithSessionStopper(stopper SessionStopper) Option {
	return func(s *Service) { s.sessions = stopper }
}

// WithMemoryRemover sets who deletes a project's memory folder on removal. The default does nothing.
func WithMemoryRemover(remover MemoryRemover) Option {
	return func(s *Service) { s.memory = remover }
}

// WithAwakeCounter sets where the "awake" badge comes from. The default always says 0.
func WithAwakeCounter(counter AwakeCounter) Option {
	return func(s *Service) { s.awake = counter }
}

// WithLocalClones lets a project be cloned from a folder or a file:// address on this machine.
// Only dev mode and tests turn it on, because it reads any repository on the machine.
func WithLocalClones(allow bool) Option {
	return func(s *Service) { s.allowLocalClone = allow }
}

// New builds a Service. Every dependency is required.
func New(deps Deps, opts ...Option) (*Service, error) {
	if deps.Store == nil || deps.Bus == nil || deps.Git == nil {
		return nil, errors.New("the projects module needs the store, the event bus, and Git")
	}
	if !filepath.IsAbs(deps.DataDir) {
		return nil, errors.New("the projects module needs the data folder as a full path")
	}
	s := &Service{
		store:    deps.Store,
		bus:      deps.Bus,
		git:      deps.Git,
		dataDir:  filepath.Clean(deps.DataDir),
		log:      slog.New(slog.DiscardHandler),
		now:      time.Now,
		entropy:  rand.Reader,
		sessions: noSessions{},
		memory:   noMemory{},
		awake:    noAwake{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// WorktreesDir is the folder that holds every card worktree of a project. The projects module and
// the tests share this one rule for where worktrees go.
func WorktreesDir(dataDir, projectID string) string {
	return filepath.Join(dataDir, "worktrees", projectID)
}

// publish sends an event after a change has been committed.
func (s *Service) publish(topic protocol.Topic, typ protocol.EventType, data any, critical bool) {
	s.bus.Publish(string(topic), string(typ), data, critical)
}

// newID makes an opaque id from the clock and the entropy reader.
func (s *Service) newID() (string, error) {
	s.idMu.Lock()
	defer s.idMu.Unlock()
	return protocol.NewID(s.now(), s.entropy)
}
