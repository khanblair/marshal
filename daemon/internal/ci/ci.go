// Package ci is Marshal's CI monitor: it turns a workflow run on a card's branch into a card's CI
// state, and, when a run fails, runs the fix loop of docs/architecture.md section 9
// (docs/backend-checklist.md B6.2 and B6.3, build-plan 6.2 and 6.3).
//
// It is the only writer of the `ci_runs` table. It writes from three places, all of them the same
// path: a verified webhook delivery (the sink the connections service hands events to), the
// conditional polling backup that covers a delivery GitHub did not send, and a simulated failure
// (slice 3, which injects a run through this same entry point so the loop is exercised for real).
//
// Section 9's four steps live here and nowhere else: rerun the failed jobs once, and on a second
// failure fetch only the failed step's log, trim it, and send it to the card's session; count the
// rounds against the card's loop limits (internal/harness, through internal/roles); and when the
// limit is reached, move the card to Needs you with the reason `ci-failed`.
//
// Nothing here decides what a delivery means on its own: a delivery is parsed into a run, the run
// is resolved to a project and a card, and then the same rule runs whether the run arrived from
// GitHub or from a poll. That is what makes the polling backup a backup rather than a second
// implementation.
package ci

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/khanblair/marshal/daemon/internal/audit"
	gh "github.com/khanblair/marshal/daemon/internal/github"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Store is the part of the store this module needs: reads and writes against the queries.
type Store interface {
	// Read runs a read-only function against the queries.
	Read(ctx context.Context, fn func(*db.Queries) error) error
	// Write runs a function that writes, in one transaction.
	Write(ctx context.Context, fn func(*db.Queries) error) error
}

// Cards is the part of the projects module this monitor needs: the card a branch belongs to, the
// cards of a project, the move to Needs you when the loop limit is reached, and the write of a
// card's own CI badge.
type Cards interface {
	// Card reads one card.
	Card(ctx context.Context, id string) (protocol.Card, error)
	// Cards lists a project's cards, which is how a run's branch is resolved to a card.
	Cards(ctx context.Context, projectID string) ([]protocol.Card, error)
	// SetNeeds moves a card to Needs you with the reason a person reads.
	SetNeeds(ctx context.Context, id string, reason protocol.NeedsReason) (protocol.Card, error)
	// SetCI records the state of the runs on a card's branch, so the card's badge and the CI column
	// of a list follow the forge without a person asking. A nil state clears it.
	SetCI(ctx context.Context, id string, state *protocol.CIState) (protocol.Card, error)
}

// Projects is the part of the projects module this monitor needs: the projects, so a delivery's
// repository can be resolved to the project whose remote it is, and a card's worktree, which is
// where the real mode of a simulated failure makes its marked commit.
type Projects interface {
	// List reads every project.
	List(ctx context.Context) (protocol.ProjectListSnapshot, error)
	// Get reads one project.
	Get(ctx context.Context, id string) (protocol.Project, error)
	// Worktree answers the folder a card's work is checked out in and the branch it is on. It is
	// read only by the real mode of a simulated failure.
	Worktree(ctx context.Context, cardID string) (path, branch string, err error)
}

// Git is the part of gitx this monitor needs: reading a project's remotes to learn which repository
// a project is, the same way the pull-request and review services do, and - for the real mode of a
// simulated failure only - making one marked commit and pushing it.
type Git interface {
	// Inspect reads a repository's facts, including its remotes.
	Inspect(ctx context.Context, path string) (gitx.RepoInfo, error)
	// CommitFile writes one file inside a working folder and commits it, answering the commit's id.
	CommitFile(ctx context.Context, dir, path, content, message string) (string, error)
	// Push pushes one branch to a named remote, refusing a force-push and any push of the main
	// branch. It is the one push Marshal runs by itself.
	Push(ctx context.Context, dir, remote, branch, main string) error
}

// Forge is the part of the forge client this monitor needs: the three Actions calls of section 9.
// The App client and the personal-token client both implement it.
type Forge interface {
	// ListWorkflowRuns lists the runs of a branch, newest first. The polling backup reads it.
	ListWorkflowRuns(ctx context.Context, repo gh.Repository, branch string) ([]gh.WorkflowRun, error)
	// RerunFailedJobs asks the forge to run a run's failed jobs again.
	RerunFailedJobs(ctx context.Context, repo gh.Repository, runID int64) error
	// FailedLog reads the log of a run's first failed job, named and cut to maxBytes.
	FailedLog(ctx context.Context, repo gh.Repository, runID int64, maxBytes int) (string, error)
}

// Worker is how a failure reaches the agent that wrote the branch: a message into the card's own
// session, which wakes it if it is asleep. The session manager implements it.
type Worker interface {
	// Send delivers a message into a card's session.
	Send(ctx context.Context, cardID, text string) error
}

// Roles reads the ceilings of the role a card runs as (docs/architecture.md section 3, Phase 5's
// harness). The roles service implements it.
type Roles interface {
	// LimitsFor reads one role's limits as a project has them. The bool is false when the project
	// has no such role, which is not an error.
	LimitsFor(ctx context.Context, projectID, name string) (harness.Limits, bool, error)
}

// Publisher publishes an event. The event bus implements it.
type Publisher interface {
	// Publish sends an event on a topic.
	Publish(topic, eventType string, data any, critical bool) uint64
}

// Options are the knobs a test turns and the daemon leaves alone. Every one of them has a default
// that is what the daemon runs with.
type Options struct {
	// Logger is where problems are written. A nil logger discards.
	Logger *slog.Logger
	// Now is the clock. A nil clock is time.Now.
	Now func() time.Time
	// Forge is the forge client. Nil means Marshal is not connected to a forge, and only simulated
	// failures write runs; the daemon builds it from the stored GitHub App.
	Forge Forge
	// Repo resolves a project to the repository its remote names. A nil value uses gitx, which is
	// what the daemon does; a test gives its own so no test reads a real remote.
	Repo func(ctx context.Context, project protocol.Project) (gh.Repository, bool)
	// LogBytes caps the trimmed log sent to a card's session.
	LogBytes int
	// LogLines caps how many trailing lines of a log are kept.
	LogLines int
	// PollEvery is how often the polling backup looks. Zero disables the backup loop.
	PollEvery time.Duration
}

// Defaults for a log that is sent to a card's session. A step's log can be megabytes; what a person
// reads in a card's chat is the end of it, where the failure is.
const (
	defaultLogBytes = 8 << 10
	defaultLogLines = 80
	// pollEvery is how often the backup looks for a run whose outcome Marshal is still waiting on.
	// It is deliberately slow: the webhook is the fast path, and the backup only covers a delivery
	// GitHub did not send.
	defaultPollEvery = 5 * time.Minute
	// pollLookback bounds what the backup asks the forge for: only runs Marshal already knows are
	// queued or running are checked, so the backup's work is a function of that small set.
	repoCacheTTL = time.Minute
)

// ErrNoProject is answered when a delivery names a repository no project has.
var ErrNoProject = errors.New("no project has that repository")

// ErrNoForge is answered when an operation needs the forge and Marshal is not connected to one.
var ErrNoForge = errors.New("marshal is not connected to a forge")

// Service is the CI monitor. It is safe for use by many goroutines.
type Service struct {
	store    Store
	cards    Cards
	projects Projects
	git      Git
	roles    Roles
	worker   Worker
	bus      Publisher
	forge    Forge
	audit    *audit.Recorder
	repo     func(ctx context.Context, project protocol.Project) (gh.Repository, bool)
	log      *slog.Logger
	now      func() time.Time

	logBytes int
	logLines int
	// pollEvery is how often the backup looks. Zero disables it.
	pollEvery time.Duration

	// syntheticSeq counts the runs a simulated failure makes up, so two simulations on one daemon
	// never share a run id and the second is a new run rather than a rerun of the first.
	syntheticSeq atomic.Int64

	// resolved caches which repository a project is, so a burst of deliveries from one repository
	// does not read the same remote again for each one. It holds at most one entry per project.
	mu       sync.Mutex
	resolved map[string]cachedRepo

	// The polling backup's own state. tickerMu guards the ticker and its stop channel; missMu
	// guards the branches remembered as having no runs.
	tickerMu     sync.Mutex
	ticker       *time.Ticker
	tickerDone   chan struct{}
	missMu       sync.Mutex
	branchMisses map[string]time.Time
}

// cachedRepo is one project's repository, with the moment it was read.
type cachedRepo struct {
	repo gh.Repository
	ok   bool
	at   time.Time
}

// New builds the monitor. The store, the cards, the projects, and the roles are required: without
// any of them there is nothing to record a run against, nothing to move to Needs you, or no
// ceilings to count rounds against. The forge is optional (a daemon nobody has connected GitHub to
// still records simulated failures) and so are the worker, the publisher, and the log.
func New(deps Deps) (*Service, error) {
	if deps.Store == nil || deps.Cards == nil || deps.Projects == nil || deps.Roles == nil {
		return nil, errors.New("the CI monitor needs the store, the cards, the projects, and the roles")
	}
	if deps.Git == nil && deps.Options.Repo == nil {
		return nil, errors.New("the CI monitor needs Git, or a way to resolve a project's repository")
	}
	opts := deps.Options
	log := opts.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	repoFn := opts.Repo
	if repoFn == nil {
		git := deps.Git
		repoFn = func(ctx context.Context, project protocol.Project) (gh.Repository, bool) {
			return remoteRepository(ctx, git, project)
		}
	}
	logBytes := opts.LogBytes
	if logBytes <= 0 {
		logBytes = defaultLogBytes
	}
	logLines := opts.LogLines
	if logLines <= 0 {
		logLines = defaultLogLines
	}
	pollEvery := opts.PollEvery
	if pollEvery == 0 {
		pollEvery = defaultPollEvery
	}
	return &Service{
		store: deps.Store, cards: deps.Cards, projects: deps.Projects, git: deps.Git,
		roles: deps.Roles, worker: deps.Worker, bus: deps.Bus, forge: opts.Forge, audit: deps.Audit,
		repo: repoFn, log: log, now: now,
		logBytes: logBytes, logLines: logLines, pollEvery: pollEvery,
		resolved:     map[string]cachedRepo{},
		branchMisses: map[string]time.Time{},
	}, nil
}

// Deps are the parts the monitor is built from.
type Deps struct {
	// Store reads and writes the runs.
	Store Store
	// Cards resolves a branch to a card and moves one to Needs you.
	Cards Cards
	// Projects resolves a repository to a project.
	Projects Projects
	// Git reads a project's remotes, when Repo is not given, and makes the marked commit the real
	// mode of a simulated failure pushes.
	Git Git
	// Roles reads the ceilings rounds are counted against.
	Roles Roles
	// Worker delivers a failure's log into a card's session.
	Worker Worker
	// Bus publishes the ci.updated event.
	Bus Publisher
	// Audit records the row a simulated failure writes (B6.4). A nil recorder writes none, which
	// only a test asks for: the daemon always passes the real one.
	Audit *audit.Recorder
	// Options are the knobs a test turns.
	Options Options
}

// Snapshot answers every project Marshal holds runs for, in project order, which is what
// GET /v1/ci answers with (section 11.1).
func (s *Service) Snapshot(ctx context.Context) (protocol.CISnapshot, error) {
	var rows []db.CiRun
	if err := s.store.Read(ctx, func(q *db.Queries) error {
		got, err := q.ListCiRuns(ctx)
		if err != nil {
			return err
		}
		rows = got
		return nil
	}); err != nil {
		return protocol.CISnapshot{}, err
	}
	projects, err := s.projectOrder(ctx)
	if err != nil {
		return protocol.CISnapshot{}, err
	}
	return protocol.NewCISnapshot(groupRuns(rows, projects), s.now()), nil
}

// RunsForProject answers one project's CI health, which a project's board and its CI panel read.
// A project Marshal holds no run for has an entry with no runs and a queued state, which is the
// honest answer and not an error.
func (s *Service) RunsForProject(ctx context.Context, projectID string) (protocol.ProjectCI, error) {
	var rows []db.CiRun
	if err := s.store.Read(ctx, func(q *db.Queries) error {
		got, err := q.ListCiRunsForProject(ctx, projectID)
		if err != nil {
			return err
		}
		rows = got
		return nil
	}); err != nil {
		return protocol.ProjectCI{}, err
	}
	return projectCI(projectID, rows), nil
}

// projectOrder reads the project ids in the order the projects service lists them, so the CI health
// page draws the same order as every other page. A daemon whose projects cannot be read still
// answers its runs, in the order the store holds them.
func (s *Service) projectOrder(ctx context.Context) ([]string, error) {
	list, err := s.projects.List(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(list.Projects))
	for _, p := range list.Projects {
		ids = append(ids, p.ID)
	}
	return ids, nil
}

// groupRuns turns the run rows into one ProjectCI per project that has a run, in project order. A
// project with no run is left out, which is what makes the CI health page say GitHub is not
// connected when nothing has any.
func groupRuns(rows []db.CiRun, order []string) []protocol.ProjectCI {
	byProject := map[string][]db.CiRun{}
	for _, row := range rows {
		byProject[row.ProjectID] = append(byProject[row.ProjectID], row)
	}
	out := make([]protocol.ProjectCI, 0, len(byProject))
	seen := map[string]bool{}
	for _, id := range order {
		group, ok := byProject[id]
		if !ok {
			continue
		}
		seen[id] = true
		out = append(out, projectCI(id, group))
	}
	// A run for a project that is not in the list (a project removed while a delivery was in
	// flight) is still reported, so a screen never silently loses a run Marshal has.
	for _, id := range sortedKeys(byProject) {
		if !seen[id] {
			out = append(out, projectCI(id, byProject[id]))
		}
	}
	return out
}

// projectCI builds one project's CI health from its rows, which arrive newest first.
func projectCI(projectID string, rows []db.CiRun) protocol.ProjectCI {
	runs := make([]protocol.CiRun, 0, len(rows))
	status := protocol.CIStateQueued
	for i, row := range rows {
		runs = append(runs, runToWire(row))
		if i == 0 {
			status = protocol.CIState(row.Status)
		}
	}
	return protocol.ProjectCI{ProjectID: projectID, Status: status, Runs: runs}
}

// runToWire turns a stored row into the wire shape. A zero start time is a null on the wire: a run
// that is still queued has not started, and saying otherwise would be a time that never happened.
func runToWire(row db.CiRun) protocol.CiRun {
	cardID := ""
	if row.CardID != nil {
		cardID = *row.CardID
	}
	run := protocol.CiRun{
		ID:        row.ID,
		ProjectID: row.ProjectID,
		CardID:    cardID,
		Branch:    row.Branch,
		Workflow:  row.Workflow,
		Status:    protocol.CIState(row.Status),
		URL:       row.Url,
		UpdatedAt: protocol.NewTimestamp(time.UnixMilli(row.UpdatedAt).UTC()),
	}
	if row.StartedAt > 0 {
		started := protocol.NewTimestamp(time.UnixMilli(row.StartedAt).UTC())
		run.StartedAt = &started
	}
	return run
}

// repository returns the repository a project is, cached for a short while. The second answer is
// false for a project whose remote is not a GitHub repository.
func (s *Service) repository(ctx context.Context, project protocol.Project) (gh.Repository, bool) {
	s.mu.Lock()
	cached, ok := s.resolved[project.ID]
	s.mu.Unlock()
	if ok && s.now().Sub(cached.at) < repoCacheTTL {
		return cached.repo, cached.ok
	}
	repo, found := s.repo(ctx, project)
	s.mu.Lock()
	s.resolved[project.ID] = cachedRepo{repo: repo, ok: found, at: s.now()}
	s.mu.Unlock()
	return repo, found
}

// remoteRepository reads a project's origin remote and answers the repository it names, if any.
func remoteRepository(ctx context.Context, git Git, project protocol.Project) (gh.Repository, bool) {
	info, err := git.Inspect(ctx, project.Path)
	if err != nil {
		return gh.Repository{}, false
	}
	for _, remote := range info.Remotes {
		if remote.Name != "origin" {
			continue
		}
		if repo, ok := gh.RepositoryFromURL(remote.URL); ok {
			return repo, true
		}
	}
	return gh.Repository{}, false
}

// publishProject tells a project's screens that one of its runs changed, and tells Home, because CI
// health is a Home section: one event per topic, carrying the shape that topic's screens read.
func (s *Service) publishProject(ctx context.Context, projectID string) {
	if s.bus == nil {
		return
	}
	project, err := s.RunsForProject(ctx, projectID)
	if err != nil {
		s.log.Warn("could not read a project's CI health to publish", "project_id", projectID, "error", err)
		return
	}
	s.bus.Publish(string(protocol.ProjectTopic(projectID)), string(protocol.EventTypeCIUpdated),
		protocol.CIEventData{Project: &project}, true)
	snapshot, err := s.Snapshot(ctx)
	if err != nil {
		s.log.Warn("could not read the CI snapshot to publish", "error", err)
		return
	}
	s.bus.Publish(string(protocol.HomeTopic), string(protocol.EventTypeCIUpdated),
		protocol.CIEventData{Snapshot: &snapshot}, true)
}

// sortedKeys lists a map's keys in order, so a list built from a map is stable.
func sortedKeys[V any](m map[string]V) []string { return slices.Sorted(maps.Keys(m)) }

// describe names a run the way a person reads it, for a log line and a message.
func describe(row db.CiRun) string {
	if strings.TrimSpace(row.Workflow) == "" {
		return "a workflow"
	}
	return fmt.Sprintf("the %q workflow", row.Workflow)
}
