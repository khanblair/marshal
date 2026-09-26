// Package accounts owns the person using Marshal (docs/architecture.md section 10, docs/backend-
// checklist.md B2.2, B2.5, and B2.13, inventory N27 and N30, decision D2): their profile and
// avatar, the users list that member pickers read, their onboarding and tour progress, and the
// screen preferences that follow them between devices.
//
// Everything here is per user from the start, and solo use has one user, the owner (decision D10).
// The routes give the service the user the request's token belongs to, so Phase 9 adds more people
// without changing a signature. Devices are not here: they stay on the app's mock until Phase 9
// adds pairing.
//
// Every change to the profile, the preferences, or the progress is published as me.updated on the
// `me` topic with all three as they now are, so a second device that applies the event lands where
// the first one is. The service reaches projects and their saved views only through the small
// Projects interface, to check that a preference names something that exists.
package accounts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	// The time zone database is compiled in, so a time zone name is checked the same way on every
	// machine, including one that has no zone files.
	_ "time/tzdata"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// avatarsFolder is the folder under the data folder that holds avatar images.
const avatarsFolder = "avatars"

// Events is the part of the event bus this module uses. cmd/marshald gives it the real bus.
type Events interface {
	Publish(topic, eventType string, data any, critical bool) uint64
}

// Projects is what the preferences need from the projects module: that a project is there, and
// which project a saved view belongs to. The projects service implements it.
type Projects interface {
	// Get returns one project, or the not found answer.
	Get(ctx context.Context, id string) (protocol.Project, error)
	// SavedView returns one saved view by its id, or the not found answer.
	SavedView(ctx context.Context, id string) (protocol.SavedView, error)
}

// Deps are the parts the service is built from.
type Deps struct {
	// Store is the open database.
	Store *store.Store
	// Bus publishes me.updated.
	Bus Events
	// Projects checks that the projects and saved views a preference names exist.
	Projects Projects
	// DataDir is the folder that holds Marshal's own files: avatars go in <DataDir>/avatars. It
	// must be a full path.
	DataDir string
}

// Service owns the person's profile, progress, and preferences. It is safe for use by many
// goroutines.
type Service struct {
	store    *store.Store
	bus      Events
	projects Projects
	avatars  string
	log      *slog.Logger
	now      func() time.Time

	// changeMu lets one change at a time write and publish. The event carries all of the person's
	// state, so two changes that published out of order could leave a device on the older one.
	changeMu sync.Mutex
}

// Option changes how New builds a Service.
type Option func(*Service)

// WithClock sets the clock that stamps a change. The default is time.Now.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// WithLogger sets the logger. The default logs nothing.
func WithLogger(log *slog.Logger) Option {
	return func(s *Service) {
		if log != nil {
			s.log = log
		}
	}
}

// New builds the service. The store, the bus, and the projects are all required: every change is
// published, and a preference that named a project the service could not check would be a guess.
func New(deps Deps, opts ...Option) (*Service, error) {
	if deps.Store == nil || deps.Bus == nil || deps.Projects == nil {
		return nil, errors.New("accounts: a store, the event bus, and the projects module are all required")
	}
	if !filepath.IsAbs(deps.DataDir) {
		return nil, errors.New("accounts: the data folder must be a full path")
	}
	s := &Service{
		store: deps.Store, bus: deps.Bus, projects: deps.Projects,
		avatars: filepath.Join(filepath.Clean(deps.DataDir), avatarsFolder),
		log:     slog.New(slog.DiscardHandler), now: time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// Me returns the profile, the preferences, and the progress of a person: what me.updated carries.
// They are read in one transaction, so the three agree.
func (s *Service) Me(ctx context.Context, userID string) (protocol.MeUpdatedEventData, error) {
	var me protocol.MeUpdatedEventData
	err := s.store.Read(ctx, func(q *db.Queries) error {
		var err error
		me, err = readMe(ctx, q, userID)
		return err
	})
	return me, err
}

// readMe reads the three parts of a person's state with the queries it is given.
func readMe(ctx context.Context, q *db.Queries, userID string) (protocol.MeUpdatedEventData, error) {
	user, err := readUser(ctx, q, userID)
	if err != nil {
		return protocol.MeUpdatedEventData{}, err
	}
	preferences, err := readPreferences(ctx, q, userID)
	if err != nil {
		return protocol.MeUpdatedEventData{}, err
	}
	progress, err := readProgress(ctx, q, userID)
	if err != nil {
		return protocol.MeUpdatedEventData{}, err
	}
	return protocol.MeUpdatedEventData{Profile: toProfile(user), Preferences: preferences, Progress: progress}, nil
}

// readUser reads a user, as the not found answer when there is none.
func readUser(ctx context.Context, q *db.Queries, userID string) (db.User, error) {
	user, err := q.GetUser(ctx, userID)
	if err != nil {
		if store.IsNotFound(err) {
			return db.User{}, notFoundUser(userID)
		}
		return db.User{}, fmt.Errorf("read user %s: %w", userID, err)
	}
	return user, nil
}

// publishMe publishes me.updated with a person's state as it is now. It is critical: a device that
// missed it would keep showing the old theme or the old name. The change is already committed, so
// a read that fails here is logged and the devices reload on their next connection.
func (s *Service) publishMe(ctx context.Context, userID string) {
	me, err := s.Me(ctx, userID)
	if err != nil {
		s.log.Error("could not read the account to publish it", "user_id", userID, "error", err)
		return
	}
	s.bus.Publish(string(protocol.MeTopic), string(protocol.EventTypeMeUpdated), me, true)
}

func notFoundUser(id string) *protocol.Error {
	return protocol.NotFound("user").With("id", id)
}
