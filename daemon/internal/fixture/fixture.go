// Package fixture loads sample data into a dev daemon, so the screens and the end-to-end specs
// have something real to point at (docs/development.md section 3.5).
//
// Only one fixture exists so far, the prototype: the three projects the prototype app shows.
// Each one is a real Git repository made from a folder under daemon/testdata/repos. The
// repositories are made inside the data folder, so the fixture never touches the checkout it
// came from, and the daemon manages them like any repository a person added.
//
// Loading is safe on every start: a project that already exists is left alone, and a repository
// that is already made is reused.
package fixture

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// PrototypeName is the value of --fixture (or MARSHAL_FIXTURE) that loads the prototype fixture.
const PrototypeName = "prototype"

const (
	// fixturesFolder is where the fixture repositories are made, inside the data folder.
	fixturesFolder = "fixtures"
	// smallRepo and monorepo are the folders under daemon/testdata/repos the seeds are made from.
	smallRepo = "small-repo"
	monorepo  = "monorepo"
)

// seed is one project of the fixture.
type seed struct {
	// id is the project id, the same one the prototype uses.
	id string
	// name is the display name, and the name of the folder made under <data>/fixtures.
	name string
	// source is the folder under daemon/testdata/repos the repository is made from.
	source string
}

// prototypeSeeds are the three projects of the prototype (apps/web/src/mock/seed/projects.ts).
// The two that share a source are made as independent copies, because a project's folder is
// unique and each one needs its own Git history. The language and package labels are whatever
// detection finds in the repository, not the words the mock hand-picked.
func prototypeSeeds() []seed {
	return []seed{
		{id: projectAPI, name: "api-gateway", source: smallRepo},
		{id: projectWeb, name: "web-dashboard", source: smallRepo},
		{id: projectMobile, name: "mobile-app", source: monorepo},
	}
}

// Option changes how LoadPrototype works.
type Option func(*loader)

// WithSourceDir sets the folder that holds the fixture repositories (small-repo and monorepo).
// Without it they are looked for next to this source file and next to the running program (see
// findSource). Tests pass daemon/testdata/repos.
func WithSourceDir(dir string) Option {
	return func(l *loader) { l.sourceDir = dir }
}

// WithLogger sets the logger. The default logs nothing.
func WithLogger(log *slog.Logger) Option {
	return func(l *loader) {
		if log != nil {
			l.log = log
		}
	}
}

// loader holds what one LoadPrototype call works with.
// The prototype's three project ids, named once for the fixture's own code. The card table below
// spells them as data.
const (
	projectAPI    = "api"
	projectWeb    = "web"
	projectMobile = "mobile"
)

type loader struct {
	sourceDir string
	log       *slog.Logger
	git       *gitx.Git
	dataDir   string
	projects  Cards
	// sessions seeds a session row for a card that should look awake. It may be nil.
	sessions Sessions
	// now is the clock the cards' dates are worked out from, so a test can fix them.
	now func() time.Time
}

// Cards is what the fixture needs beyond the projects interface: the card and label calls that
// write the prototype's 29 cards. The projects service implements it.
type Cards interface {
	projects.Projects
	// Board is how the fixture knows whether a project already has its cards.
	Board(ctx context.Context, projectID string) (protocol.BoardSnapshot, error)
	// Labels and CreateLabel make the labels the prototype's cards carry.
	Labels(ctx context.Context, projectID string) (protocol.LabelSnapshot, error)
	CreateLabel(ctx context.Context, projectID string, in protocol.CreateLabelRequest) (protocol.Label, error)
	// CreateCard writes one card, with the fields the prototype's seed has.
	CreateCard(ctx context.Context, projectID string, in projects.CardInput, opts ...projects.CardOption) (protocol.Card, error)
	// SavedViews and CreateSavedView make the saved views the prototype's menu shows.
	SavedViews(ctx context.Context, projectID string) (protocol.SavedViewListSnapshot, error)
	CreateSavedView(ctx context.Context, projectID string, in protocol.CreateSavedViewRequest) (protocol.SavedView, bool, error)
}

// Sessions seeds a session row for a fixture card, without starting an agent. The session manager
// implements it. The fixture uses it so the screens that draw sessions (Home's awake list, the
// Agents view) look as they do in the prototype, without a process per card.
type Sessions interface {
	SeedSession(ctx context.Context, cardID string, state protocol.SessionState) error
}

// WithSessions sets who seeds a card's session. Without it, the fixture writes cards and no
// sessions, which is what a test that does not care about sessions uses.
func WithSessions(s Sessions) Option {
	return func(l *loader) { l.sessions = s }
}

// LoadPrototype makes the three prototype projects (api, web, and mobile) in dataDir, adds them to
// the projects module, and writes the prototype's 29 cards. It is safe to call on every start: a
// project whose id already exists is skipped without looking at its folder, a project that already
// has its cards is left alone, and a fixture repository that is already made is reused. It stops at
// the first problem and returns it: the caller decides whether that is fatal, and a later call
// carries on from where this one stopped.
//
// dataDir must be a full path.
func LoadPrototype(ctx context.Context, dataDir string, svc Cards, opts ...Option) error {
	if !filepath.IsAbs(dataDir) {
		return errors.New("the fixture needs the data folder as a full path")
	}
	if svc == nil {
		return errors.New("the fixture needs the projects module")
	}
	l := &loader{
		log:      slog.New(slog.DiscardHandler),
		git:      fixtureGit(),
		dataDir:  filepath.Clean(dataDir),
		projects: svc,
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(l)
	}
	created, existing := 0, 0
	cards, savedViews := 0, 0
	for _, s := range prototypeSeeds() {
		made, err := l.load(ctx, s)
		if err != nil {
			return fmt.Errorf("load the fixture project %s: %w", s.id, err)
		}
		if made {
			created++
		} else {
			existing++
		}
		written, err := l.loadCards(ctx, s.id)
		if err != nil {
			return fmt.Errorf("load the fixture cards of %s: %w", s.id, err)
		}
		cards += written
		views, err := l.loadSavedViews(ctx, s.id)
		if err != nil {
			return fmt.Errorf("load the fixture saved views of %s: %w", s.id, err)
		}
		savedViews += views
	}
	l.log.Info("prototype fixture ready", "created", created, "existing", existing, "cards", cards,
		"saved_views", savedViews)
	return nil
}

// load makes one project unless it already exists, and says whether it made it.
func (l *loader) load(ctx context.Context, s seed) (bool, error) {
	exists, err := l.exists(ctx, s.id)
	if err != nil {
		return false, err
	}
	if exists {
		l.log.Info("fixture project already exists, leaving it as it is", "project_id", s.id)
		return false, nil
	}
	path, err := l.ensureRepo(ctx, s)
	if err != nil {
		return false, err
	}
	in := protocol.CreateProjectRequest{Source: protocol.ProjectSourceFolder, Path: path, Name: s.name}
	if _, err := l.projects.Create(ctx, in, projects.WithID(s.id)); err != nil {
		return false, fmt.Errorf("add the project: %w", err)
	}
	l.log.Info("fixture project created", "project_id", s.id)
	return true, nil
}

// exists reports whether a project with this id is already known. Only the "not found" answer
// means it is not: any other error (a closed database, a cancelled request) is returned, because
// treating it as "missing" would try to create a project that may be there.
func (l *loader) exists(ctx context.Context, id string) (bool, error) {
	_, err := l.projects.Get(ctx, id)
	if err == nil {
		return true, nil
	}
	var perr *protocol.Error
	if errors.As(err, &perr) && perr.Code == protocol.ErrorCodeNotFound {
		return false, nil
	}
	return false, fmt.Errorf("look for the project: %w", err)
}
