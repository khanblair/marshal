// Package integrator runs Marshal's merge queue and its own branch (docs/architecture.md section 8,
// docs/backend-checklist.md B5.5, build-plan 5.8 and 5.9). A card in Ready to merge is merged, one
// at a time per project, into the Integrator's own branch in the Integrator's own workspace, tested
// there, and then delivered to the project's integration branch and to the owner's folder.
//
// A merge goes through four phases, each announced as a merge.progress event:
//
//  1. queued, then resolving: the card's branch is merged into the integrator branch. A conflict
//     goes to the ConflictResolver; without one, or when it is not confident, the card goes to
//     Needs you;
//  2. testing: the tests a merge must pass run in the workspace;
//  3. landing: the integration branch moves to the merged commit. When the owner's folder has that
//     branch checked out, the folder follows: by Git's own fast-forward when that loses nothing,
//     otherwise by merging the owner's uncommitted changes into the result (wip.go);
//  4. done: the card moves to Done and the delivery is written to the history, where it can be
//     undone while the branch is still where the merge left it.
//
// A merge that stops sends the card to Needs you with the merge phase "stopped" and the reason as
// its merge note, which is what makes the card retryable. A stop before landing resets the
// integrator branch to where it was. A delivery that stops (the folder is busy, or the owner's
// changes clash) keeps the tested merge on the integrator branch, so a retry only delivers.
//
// Nothing here pushes anywhere, no branch moves except forward (an undo is the one move back, by
// compare-and-swap), and the owner's folder is never reset, checked out over, or cleaned. Every test
// runs against a throwaway repository.
package integrator

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/keyedlock"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Cards is the part of the projects module the queue needs.
type Cards interface {
	// Card reads one card.
	Card(ctx context.Context, id string) (protocol.Card, error)
	// SetState moves a card to a state with no manual-move rules: the daemon's own moves.
	SetState(ctx context.Context, id string, state protocol.CardState) (protocol.Card, error)
	// SetNeeds moves a card to Needs you with the reason a person reads.
	SetNeeds(ctx context.Context, id string, reason protocol.NeedsReason) (protocol.Card, error)
}

// Projects is the part of the projects module the queue needs to read a project.
type Projects interface {
	// Get reads one project.
	Get(ctx context.Context, id string) (protocol.Project, error)
}

// Publisher sends an event to the clients that follow a topic. The event bus implements it.
type Publisher interface {
	Publish(topic, eventType string, data any, critical bool) uint64
}

// Brief is what the cards module knows about a card's work, for the resolver.
type Brief struct {
	// Plan is the card's approved plan, empty when it has none.
	Plan string
	// Handoff is the card's handoff note, empty when it has none.
	Handoff string
}

// Briefs reads the plan and the handoff note of a card. Nil means the resolver is told only the
// card's title and description.
type Briefs interface {
	Brief(ctx context.Context, cardID string) (Brief, error)
}

// Outcome is what a test run over a merged branch found. It is deliberately small: the queue only
// needs to know whether the merge may move the target forward, and what to say if it may not.
type Outcome struct {
	// Passed is true when every test passed.
	Passed bool
	// Summary is one line a person reads, shown on the card when the tests fail.
	Summary string
}

// Tester runs the tests a merge must pass, in the merged worktree and over only the files the merge
// changed. Phase 6's local CI is one implementation; a project with no tests can leave it nil, and
// the queue then moves the target forward on a clean merge alone.
type Tester interface {
	Run(ctx context.Context, worktree string, changed []string) (Outcome, error)
}

// ChecklistGate says whether a card's required checklists are done. The card panel implements it.
type ChecklistGate interface {
	// OpenRequiredItems says how many lines of the card's required checklists are still open.
	OpenRequiredItems(ctx context.Context, cardID string) (int, error)
}

// FolderRetry bounds how long a delivery waits for the owner's folder to be free, and how often it
// reads the folder again when an editor keeps writing to it.
type FolderRetry struct {
	// Attempts is how many times it tries. Zero means 4.
	Attempts int
	// Wait is how long it waits between tries. Zero means 2 seconds.
	Wait time.Duration
}

// Deps are the parts the queue is built from.
type Deps struct {
	Cards    Cards
	Projects Projects
	// Git runs every Git command.
	Git *gitx.Git
	// Tests runs the merge's tests. Nil means a clean merge is enough.
	Tests Tester
	// Checklists holds a card back while a required checklist is open (B10.5). Nil means none is.
	Checklists ChecklistGate
	// Resolver resolves conflicts by intent. Nil sends a conflict to Needs you.
	Resolver ConflictResolver
	// ResolveTimeout bounds one call to the resolver. Zero means 15 minutes.
	ResolveTimeout time.Duration
	// Briefs reads a card's plan and handoff note for the resolver. Nil leaves them empty.
	Briefs Briefs
	// Ledger keeps the history, the settings, and the merge progress of cards. Nil keeps the
	// history and the settings in memory, and writes no progress to the cards.
	Ledger Ledger
	// Events receives merge.progress. Nil sends nothing.
	Events Publisher
	// Workspace makes the Integrator's workspace. Nil builds the Git one under DataDir.
	Workspace Workspace
	// Folder bounds the waits of a delivery into the owner's folder.
	Folder FolderRetry
	// DataDir is where the Integrator's workspaces go: <DataDir>/integrator/<project>.
	DataDir string
	// Now is the clock. Nil means time.Now.
	Now func() time.Time
	// Log is where problems are written. Nil discards.
	Log *slog.Logger
}

const (
	defaultResolveTimeout = 15 * time.Minute
	defaultFolderAttempts = 4
	defaultFolderWait     = 2 * time.Second
	// maxAttempts is how many times a merge starts over when the target branch moves under it.
	maxAttempts = 3
)

// Service is the merge queue. It is safe for use by many goroutines; one card at a time per project
// is held by a per-project lock.
type Service struct {
	cards    Cards
	projects Projects
	git      *gitx.Git
	tests    Tester
	lists    ChecklistGate
	resolver ConflictResolver
	briefs   Briefs
	ledger   Ledger
	events   Publisher
	ws       Workspace
	dataDir  string
	log      *slog.Logger
	now      func() time.Time
	timeout  time.Duration
	folder   FolderRetry

	locks keyedlock.Locks
	idMu  sync.Mutex
	// qmu guards the cards Enqueue has accepted: waiting by project, and which projects are being
	// drained by a goroutine.
	qmu      sync.Mutex
	waiting  map[string][]string
	draining map[string]bool
	// retrying holds the cards whose retry has started and not yet ended.
	retrying map[string]bool
	// base is the context the merges that Enqueue starts run in; Close cancels it.
	base    context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	closing atomic.Bool
}

var _ Reader = (*Service)(nil)

// New builds a Service. Everything but the tests, the resolver, the ledger, the events, and the
// briefs is required.
func New(deps Deps) (*Service, error) {
	if deps.Cards == nil || deps.Projects == nil || deps.Git == nil {
		return nil, errors.New("the merge queue needs the cards, the projects, and Git")
	}
	if !filepath.IsAbs(deps.DataDir) {
		return nil, errors.New("the merge queue needs the data folder as a full path")
	}
	s := &Service{
		cards: deps.Cards, projects: deps.Projects, git: deps.Git, tests: deps.Tests, lists: deps.Checklists,
		resolver: deps.Resolver, briefs: deps.Briefs, ledger: deps.Ledger, events: deps.Events,
		ws: deps.Workspace, dataDir: filepath.Clean(deps.DataDir), log: deps.Log, now: deps.Now,
		timeout: deps.ResolveTimeout, folder: deps.Folder,
		waiting: map[string][]string{}, draining: map[string]bool{}, retrying: map[string]bool{},
	}
	s.fillDefaults()
	if s.ws == nil {
		ws, err := NewWorkspace(WorkspaceDeps{Projects: s.projects, Git: s.git, DataDir: s.dataDir})
		if err != nil {
			return nil, err
		}
		s.ws = ws
	}
	s.base, s.cancel = context.WithCancel(context.Background())
	return s, nil
}

// fillDefaults gives every optional part of a Service its default.
func (s *Service) fillDefaults() {
	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.ledger == nil {
		s.ledger = newMemoryLedger()
	}
	if s.timeout <= 0 {
		s.timeout = defaultResolveTimeout
	}
	if s.folder.Attempts <= 0 {
		s.folder.Attempts = defaultFolderAttempts
	}
	if s.folder.Wait <= 0 {
		s.folder.Wait = defaultFolderWait
	}
}

// Result says what the queue did.
type Result struct {
	// Merged is true when the card's work is in the target branch.
	Merged bool
	// Commit is the commit the target moved to, when it moved.
	Commit string
	// Note is a sentence about the delivery that is not a failure, such as that the owner's folder
	// is on another branch and so did not change.
	Note string
	// Reason is a plain sentence for a person when the merge did not finish.
	Reason string
}

// Merge takes one card from Ready to merge to Done, or leaves it in Needs you with a reason, and
// the project's integration branch untouched. One card at a time per project: a second card waits
// for the first project's merge to finish.
//
// A card that is not in Ready to merge is refused, because the queue is not a way to merge a card
// the board has not sent to it.
func (s *Service) Merge(ctx context.Context, cardID string) (Result, error) {
	card, err := s.cards.Card(ctx, cardID)
	if err != nil {
		return Result{}, err
	}
	if card.State != protocol.CardStateReady {
		return Result{}, refusedNotReady(cardID)
	}
	if s.lists != nil {
		open, err := s.lists.OpenRequiredItems(ctx, cardID)
		if err != nil {
			return Result{}, err
		}
		if open > 0 {
			return Result{}, protocol.Refused("A required checklist on this card still has open items. Finish it before the merge.").
				With("cardId", cardID).With("reason", "merge_checklist_open")
		}
	}
	return s.queue(ctx, card, false)
}

// refusedNotReady is the refusal for a card that the board has not sent to the queue.
func refusedNotReady(cardID string) *protocol.Error {
	return protocol.Refused("Only a card in Ready to merge goes to the merge queue.").
		With("cardId", cardID).With("reason", "merge_not_ready")
}

// queue waits for the project's turn and runs the card's merge. The card is read again once the lock
// is held, because it may have left Ready while it waited. A retry is a card a merge stopped.
func (s *Service) queue(ctx context.Context, card protocol.Card, retry bool) (Result, error) {
	project, err := s.projects.Get(ctx, card.ProjectID)
	if err != nil {
		return Result{}, err
	}
	if !retry {
		s.markQueued(ctx, card)
	}
	unlock := s.locks.Lock(project.ID)
	defer unlock()
	fresh, err := s.cards.Card(ctx, card.ID)
	if err != nil {
		return Result{}, err
	}
	if !mergeable(fresh, retry) {
		if !retry {
			s.clearProgress(ctx, fresh, "")
		}
		return Result{}, refusedNotReady(card.ID)
	}
	return s.run(ctx, fresh, project)
}

// mergeable says whether a card may be merged now: a card in Ready to merge, or, for a retry, one that
// is waiting in Needs you because a merge stopped it.
func mergeable(card protocol.Card, retry bool) bool {
	if retry {
		return card.State == protocol.CardStateNeeds
	}
	return card.State == protocol.CardStateReady
}

// stalled says whether a merge stopped this card and left it for a person: it is in Needs you with
// the merge phase "stopped".
func (s *Service) stalled(ctx context.Context, card protocol.Card) bool {
	if card.State != protocol.CardStateNeeds {
		return false
	}
	rows, err := s.ledger.Stalled(ctx, card.ProjectID)
	if err != nil {
		s.log.Warn("could not read the cards a merge stopped", "project_id", card.ProjectID, "error", err)
		return false
	}
	for _, row := range rows {
		if row.ID == card.ID {
			return true
		}
	}
	return false
}

// isRetrying says whether a retry of the card is under way.
func (s *Service) isRetrying(cardID string) bool {
	s.qmu.Lock()
	defer s.qmu.Unlock()
	return s.retrying[cardID]
}

// startRetry marks a card as being retried. It answers false when a retry is already under way.
func (s *Service) startRetry(cardID string) bool {
	s.qmu.Lock()
	defer s.qmu.Unlock()
	if s.retrying[cardID] {
		return false
	}
	s.retrying[cardID] = true
	return true
}

func (s *Service) endRetry(cardID string) {
	s.qmu.Lock()
	defer s.qmu.Unlock()
	delete(s.retrying, cardID)
}

// MergeRoot is the folder the old queue's temporary worktrees lived under, for one project. The
// Integrator's own workspace is WorkspaceDir.
func MergeRoot(dataDir, projectID string) string {
	return filepath.Join(dataDir, "merge", projectID)
}

// conflictReason is the sentence a person reads when a merge conflicts and nothing resolves it.
func conflictReason(target string, conflicts []string) string {
	files := "a file"
	if len(conflicts) > 0 {
		files = strings.Join(conflicts, ", ")
	}
	return fmt.Sprintf("The merge into %s conflicted in %s. The target branch was not changed. "+
		"Resolve it on the card's branch, then send the card to ready again.", target, files)
}

// mergeMessage is the merge commit's message, naming the card so a person reading the branch's log
// sees which card each merge was.
func mergeMessage(card protocol.Card) string {
	title := strings.TrimSpace(card.Title)
	if title == "" {
		return "Marshal: merge card " + card.ID
	}
	return "Marshal: merge " + card.Key + " " + title
}

// shortID is the first eight characters of an opaque id, for a backup branch name.
func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// newID makes an opaque id from the clock and the system's entropy.
func (s *Service) newID() (string, error) {
	s.idMu.Lock()
	defer s.idMu.Unlock()
	return protocol.NewID(s.now(), entropy())
}

// entropy is where ids get their random part.
func entropy() io.Reader { return rand.Reader }
