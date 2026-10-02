// Package quality is the code-smell module (docs/architecture.md section 17, docs/backend-checklist.md
// B5.8, build-plan 5.20, docs/marshal-product-scope.md section 15.4). It checks a card's own changes
// for code smells and keeps what it found: what a check found in a card's diff, and one project's
// profile of which checks run at what thresholds.
//
// It is the layer between the HTTP routes and the work a check does. The routes read the request and
// write the answer; this package finds the card's worktree, reads the card's diff, runs the checks
// the project's profile turns on, keeps only the smells the card itself introduced, and saves the
// findings. Nothing here publishes a card's state: a blocking finding is handed to the agent through
// the same Session the rest of the daemon uses, and the card's own move to review consults this
// module rather than being changed by it.
//
// Git is never run from here except through the Git interface, which is gitx.Git in the daemon and a
// fake in a test. A project's own linter is a child process run through internal/proc, behind the
// Linter interface, so a test drives the whole pipeline without starting a program.
package quality

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

const (
	// DefaultMaxFiles is the most files one check reads. A card that changed more than this has the
	// rest left unchecked, because a check that reads ten thousand files is not fast feedback.
	DefaultMaxFiles = 300
	// DefaultMaxLines is the most lines of one file's hunks that are read, which is what bounds the
	// added-line set of an enormous file.
	DefaultMaxLines = 5000
	// DefaultMaxBytes caps how much of one Git command's output is read.
	DefaultMaxBytes = 2 << 20
	// maxDismissReasonChars is the longest a dismissal's reason may be.
	maxDismissReasonChars = 500
	// maxFixTextChars bounds the message sent to the agent, so a finding with an enormous message
	// cannot make one.
	maxFixTextChars = 2000
)

// Store is the part of the store the module uses.
type Store interface {
	// Read runs a read-only function against the queries.
	Read(ctx context.Context, fn func(*db.Queries) error) error
	// Write runs a function that writes, in one transaction.
	Write(ctx context.Context, fn func(*db.Queries) error) error
}

// Cards is the part of the projects module this module needs: reading one card.
type Cards interface {
	// Card reads one card.
	Card(ctx context.Context, id string) (protocol.Card, error)
}

// Projects is the part of the projects module this module needs: the project a card belongs to, and
// the worktree its agent worked in.
type Projects interface {
	// Get reads one project.
	Get(ctx context.Context, id string) (protocol.Project, error)
	// Worktree returns the folder and branch recorded for a card's worktree. Both are empty until
	// the card starts.
	Worktree(ctx context.Context, cardID string) (path, branch string, err error)
}

// Git is the part of gitx this module needs: the card's changed files, one file's added lines, a
// file as the target branch has it, and one plain command for the merge base and the head commit.
type Git interface {
	// DiffFiles lists what the card changed, with each file's status.
	DiffFiles(ctx context.Context, dir, base string, maxBytes int) ([]gitx.DiffFile, bool, error)
	// DiffHunks returns one file's hunks, which is where its added lines are.
	DiffHunks(ctx context.Context, dir, base, path string, maxLines, maxBytes int) (gitx.DiffFileHunks, bool, error)
	// FileAtCommit returns one file as one commit has it.
	FileAtCommit(ctx context.Context, dir, sha, path string) (string, error)
	// Run runs a read-only Git command and answers its standard output.
	Run(ctx context.Context, dir string, args ...string) (string, error)
}

// Worker is how a finding reaches the agent that wrote the code: a message into the card's own
// session, which wakes it if it is asleep. The session manager implements it.
type Worker interface {
	// Send delivers a message into a card's session.
	Send(ctx context.Context, cardID, text string) error
}

// Publisher publishes an event, so the card's checks panel redraws without asking again. The event
// bus implements it.
type Publisher interface {
	// Publish sends an event on a topic.
	Publish(topic, eventType string, data any, critical bool) uint64
}

// Limits bound what one check reads. A zero field takes its default.
type Limits struct {
	// MaxFiles is the most files one check reads.
	MaxFiles int
	// MaxLines is the most lines of one file's hunks that are read.
	MaxLines int
	// MaxBytes caps how much of one Git command's output is read.
	MaxBytes int
}

// withDefaults fills in every limit left at zero.
func (l Limits) withDefaults() Limits {
	if l.MaxFiles <= 0 {
		l.MaxFiles = DefaultMaxFiles
	}
	if l.MaxLines <= 0 {
		l.MaxLines = DefaultMaxLines
	}
	if l.MaxBytes <= 0 {
		l.MaxBytes = DefaultMaxBytes
	}
	return l
}

// Deps are the parts the service is built from. Store, Cards, Projects, and Git are required.
type Deps struct {
	// Store holds the findings and the profiles.
	Store Store
	// Cards reads the card.
	Cards Cards
	// Projects reads the project and the card's worktree.
	Projects Projects
	// Git reads the card's diff and the target branch's copy of a file.
	Git Git
	// Worker delivers a finding to the agent that wrote the code. Nil means a finding is recorded
	// and shown but no agent is ever asked to fix it.
	Worker Worker
	// Bus publishes the quality.checked event. Nil publishes nothing.
	Bus Publisher
	// NewLinter builds a linter from a profile's entry. Nil uses NewCommandLinter, which is what the
	// daemon uses; a test gives a fake that never starts a process.
	NewLinter func(protocol.SmellLinter) (Linter, error)
	// Log is where problems are written. Nil discards.
	Log *slog.Logger
}

// Service checks a card's changes for code smells. It is safe for use by many goroutines.
type Service struct {
	store    Store
	cards    Cards
	projects Projects
	git      Git
	worker   Worker
	bus      Publisher
	linters  func(protocol.SmellLinter) (Linter, error)
	log      *slog.Logger
	now      func() time.Time
	entropy  io.Reader
	limits   Limits
}

// Option changes how New builds a Service.
type Option func(*Service)

// WithClock sets the clock the answers' serverTime and the findings' ids come from. The default is
// time.Now.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// WithEntropy sets the reader the findings' opaque ids are made from. The default is crypto/rand.
func WithEntropy(r io.Reader) Option {
	return func(s *Service) {
		if r != nil {
			s.entropy = r
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

// WithLimits sets how much one check reads. It is how a test reaches a bounded check.
func WithLimits(limits Limits) Option {
	return func(s *Service) { s.limits = limits.withDefaults() }
}

// New builds the service.
func New(deps Deps, opts ...Option) (*Service, error) {
	if deps.Store == nil || deps.Cards == nil || deps.Projects == nil || deps.Git == nil {
		return nil, errors.New("quality: the store, the cards module, the projects module, and Git are all required")
	}
	s := &Service{
		store: deps.Store, cards: deps.Cards, projects: deps.Projects, git: deps.Git,
		worker: deps.Worker, bus: deps.Bus, linters: deps.NewLinter,
		log: slog.New(slog.DiscardHandler), now: time.Now, entropy: rand.Reader,
		limits: Limits{}.withDefaults(),
	}
	if s.linters == nil {
		s.linters = func(entry protocol.SmellLinter) (Linter, error) { return NewCommandLinter(entry) }
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// Check runs the checks for a card and answers its findings. It reads the card's worktree, reads the
// diff against the project's integration branch, runs the checks the project's profile turns on, keeps
// only the smells the card itself introduced, and saves them.
//
// A commit is checked once: a second call for the same commit answers what the first one saved,
// whether or not it found anything. A card that never started has no worktree and answers an empty
// list, which is the empty state the card's checks panel draws.
func (s *Service) Check(ctx context.Context, cardID string) (protocol.SmellFindingList, error) {
	card, err := s.cards.Card(ctx, cardID)
	if err != nil {
		return protocol.SmellFindingList{}, err
	}
	project, err := s.projects.Get(ctx, card.ProjectID)
	if err != nil {
		return protocol.SmellFindingList{}, fmt.Errorf("read the project of card %s: %w", cardID, err)
	}
	profile, err := s.profileFor(ctx, project.ID)
	if err != nil {
		return protocol.SmellFindingList{}, err
	}
	path, _, err := s.projects.Worktree(ctx, cardID)
	if err != nil {
		return protocol.SmellFindingList{}, err
	}
	if strings.TrimSpace(path) == "" || !folderExists(path) {
		return protocol.NewSmellFindingList(cardID, "", nil, time.Time{}, s.now()), nil
	}
	commit := s.head(ctx, path)
	if checkedAt, ok := s.checkedAt(ctx, cardID, commit); ok {
		findings, err := s.findings(ctx, cardID, commit)
		if err != nil {
			return protocol.SmellFindingList{}, err
		}
		return protocol.NewSmellFindingList(cardID, commit, findings, checkedAt, s.now()), nil
	}
	files, err := s.changedFiles(ctx, path, project.Target())
	if err != nil {
		return protocol.SmellFindingList{}, err
	}
	raw := checkBuiltins(profile, files)
	raw = append(raw, s.linted(ctx, profile, path, files)...)
	findings, checkedAt, err := s.save(ctx, cardID, commit, profile, raw)
	if err != nil {
		return protocol.SmellFindingList{}, err
	}
	answer := protocol.NewSmellFindingList(cardID, commit, findings, checkedAt, s.now())
	s.publish(cardID, answer)
	return answer, nil
}

// Findings answers a card's findings without running anything: what the last check saved for the
// card's current commit. A card whose commit has never been checked answers an empty list.
func (s *Service) Findings(ctx context.Context, cardID string) (protocol.SmellFindingList, error) {
	card, err := s.cards.Card(ctx, cardID)
	if err != nil {
		return protocol.SmellFindingList{}, err
	}
	path, _, err := s.projects.Worktree(ctx, card.ID)
	if err != nil {
		return protocol.SmellFindingList{}, err
	}
	commit := ""
	if strings.TrimSpace(path) != "" && folderExists(path) {
		commit = s.head(ctx, path)
	}
	checkedAt, _ := s.checkedAt(ctx, cardID, commit)
	findings, err := s.findings(ctx, cardID, commit)
	if err != nil {
		return protocol.SmellFindingList{}, err
	}
	return protocol.NewSmellFindingList(cardID, commit, findings, checkedAt, s.now()), nil
}

// BeforeReview runs the checks for a card that is about to move to review, and answers its findings.
// When any finding blocks, they are handed to the agent that wrote the code and the caller refuses
// the move: blocking findings keep the card where it is and go back to the agent. When nothing
// blocks, the caller moves the card.
func (s *Service) BeforeReview(ctx context.Context, cardID string) (protocol.SmellFindingList, error) {
	list, err := s.Check(ctx, cardID)
	if err != nil {
		return list, err
	}
	if list.Blocking == 0 {
		return list, nil
	}
	if s.worker == nil {
		return list, nil
	}
	if err := s.worker.Send(ctx, cardID, blockingText(list.Findings)); err != nil {
		// The card is refused either way; a delivery that could not be made is logged and the
		// person still sees the findings on the card.
		s.log.Warn("could not send blocking findings back to the agent", "card_id", cardID, "error", err)
		return list, nil
	}
	return list, nil
}

// BlockingFindings runs the checks for a card that is about to move to In review and answers how
// many of its findings block that move, after any of them have gone back to the card's agent. It is
// the shape the projects module's review gate needs (architecture.md section 17.1): the move asks
// the question, and the sentence a person reads on a refusal belongs to the move rules, not here.
//
// An error is a check that could not run at all; it is the caller's to decide what that means.
func (s *Service) BlockingFindings(ctx context.Context, cardID string) (int, error) {
	list, err := s.BeforeReview(ctx, cardID)
	return list.Blocking, err
}

// Fix asks the card's agent to fix one finding: the finding is described in a message into the
// card's own session, and its status becomes fixed, meaning it was handed over. The next check of a
// newer commit is what says whether the smell is really gone.
func (s *Service) Fix(ctx context.Context, cardID, findingID string) (protocol.SmellFinding, error) {
	finding, err := s.getFinding(ctx, cardID, findingID)
	if err != nil {
		return protocol.SmellFinding{}, err
	}
	if s.worker == nil {
		return protocol.SmellFinding{}, protocol.Refused(
			"This card has no agent to fix the finding.").With("cardId", cardID).With("reason", "quality_no_agent")
	}
	if err := s.worker.Send(ctx, cardID, fixText(finding)); err != nil {
		return protocol.SmellFinding{}, err
	}
	if err := s.setStatus(ctx, cardID, findingID, protocol.SmellStatusFixed, ""); err != nil {
		return protocol.SmellFinding{}, err
	}
	finding.Status = protocol.SmellStatusFixed
	return finding, nil
}

// Dismiss waves one finding away with a reason. The reason is required: a dismissed finding keeps
// why, and a reason is what the auto lessons of a later phase learn from.
func (s *Service) Dismiss(ctx context.Context, cardID, findingID, reason string) (protocol.SmellFinding, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return protocol.SmellFinding{}, protocol.InvalidArgument("A dismissed finding needs a reason.").
			With("field", "reason")
	}
	if len(reason) > maxDismissReasonChars {
		return protocol.SmellFinding{}, protocol.InvalidArgument(
			fmt.Sprintf("A reason may be up to %d characters.", maxDismissReasonChars)).With("field", "reason")
	}
	finding, err := s.getFinding(ctx, cardID, findingID)
	if err != nil {
		return protocol.SmellFinding{}, err
	}
	if err := s.setStatus(ctx, cardID, findingID, protocol.SmellStatusDismissed, reason); err != nil {
		return protocol.SmellFinding{}, err
	}
	finding.Status = protocol.SmellStatusDismissed
	finding.DismissReason = reason
	return finding, nil
}

// Profile answers one project's smell profile, with every threshold and check filled in.
func (s *Service) Profile(ctx context.Context, projectID string) (protocol.SmellProfile, error) {
	if _, err := s.projects.Get(ctx, projectID); err != nil {
		return protocol.SmellProfile{}, err
	}
	stored, err := s.storedProfile(ctx, projectID)
	if err != nil {
		return protocol.SmellProfile{}, err
	}
	return resolveProfile(projectID, stored), nil
}

// SetProfile saves one project's smell profile and answers it resolved. It replaces whatever was
// there: the profile is one document, and a person edits it as a whole.
func (s *Service) SetProfile(ctx context.Context, projectID string, in protocol.SmellProfile) (protocol.SmellProfile, error) {
	if _, err := s.projects.Get(ctx, projectID); err != nil {
		return protocol.SmellProfile{}, err
	}
	checked, err := checkProfile(projectID, in)
	if err != nil {
		return protocol.SmellProfile{}, err
	}
	encoded, err := encodeProfile(checked)
	if err != nil {
		return protocol.SmellProfile{}, err
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		return q.UpsertSmellProfile(ctx, db.UpsertSmellProfileParams{ProjectID: projectID, SpecJSON: encoded})
	})
	if err != nil {
		return protocol.SmellProfile{}, fmt.Errorf("save the smell profile of project %s: %w", projectID, err)
	}
	return resolveProfile(projectID, &checked), nil
}

// profileFor reads and resolves a project's profile.
func (s *Service) profileFor(ctx context.Context, projectID string) (protocol.SmellProfile, error) {
	stored, err := s.storedProfile(ctx, projectID)
	if err != nil {
		return protocol.SmellProfile{}, err
	}
	return resolveProfile(projectID, stored), nil
}

// storedProfile reads a project's saved profile, or nil when it has none.
func (s *Service) storedProfile(ctx context.Context, projectID string) (*protocol.SmellProfile, error) {
	var row db.SmellProfile
	err := s.store.Read(ctx, func(q *db.Queries) error {
		var err error
		row, err = q.GetSmellProfile(ctx, projectID)
		return err
	})
	if err != nil {
		if store.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read the smell profile of project %s: %w", projectID, err)
	}
	stored, err := decodeProfile(projectID, row.SpecJSON)
	if err != nil {
		return nil, err
	}
	return &stored, nil
}

// getFinding reads one finding of one card, or answers not found.
func (s *Service) getFinding(ctx context.Context, cardID, findingID string) (protocol.SmellFinding, error) {
	if !protocol.ValidID(findingID) {
		return protocol.SmellFinding{}, notFoundFinding(findingID)
	}
	var row db.SmellFinding
	err := s.store.Read(ctx, func(q *db.Queries) error {
		var err error
		row, err = q.GetSmellFinding(ctx, db.GetSmellFindingParams{CardID: cardID, ID: findingID})
		return err
	})
	if err != nil {
		if store.IsNotFound(err) {
			return protocol.SmellFinding{}, notFoundFinding(findingID)
		}
		return protocol.SmellFinding{}, err
	}
	return toWire(row), nil
}

// setStatus writes a finding's new status, and the reason when it was dismissed.
func (s *Service) setStatus(ctx context.Context, cardID, findingID string, status protocol.SmellStatus, reason string) error {
	err := s.store.Write(ctx, func(q *db.Queries) error {
		return q.SetSmellFindingStatus(ctx, db.SetSmellFindingStatusParams{
			Status: string(status), DismissReason: reason, CardID: cardID, ID: findingID,
		})
	})
	if err != nil {
		return fmt.Errorf("change the status of finding %s: %w", findingID, err)
	}
	return nil
}

// findings reads a card's findings for one commit.
func (s *Service) findings(ctx context.Context, cardID, commit string) ([]protocol.SmellFinding, error) {
	var rows []db.SmellFinding
	err := s.store.Read(ctx, func(q *db.Queries) error {
		var err error
		rows, err = q.ListSmellFindingsForCardCommit(ctx, db.ListSmellFindingsForCardCommitParams{
			CardID: cardID, CommitSha: commit,
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("read the findings of card %s: %w", cardID, err)
	}
	out := make([]protocol.SmellFinding, 0, len(rows))
	for _, row := range rows {
		out = append(out, toWire(row))
	}
	return out, nil
}

// checkedAt answers when the checks last ran for one card and commit, and whether they ever have.
func (s *Service) checkedAt(ctx context.Context, cardID, commit string) (time.Time, bool) {
	var millis int64
	err := s.store.Read(ctx, func(q *db.Queries) error {
		var err error
		millis, err = q.GetSmellCheck(ctx, db.GetSmellCheckParams{CardID: cardID, CommitSha: commit})
		return err
	})
	if err != nil {
		if !store.IsNotFound(err) {
			s.log.Warn("could not read when the checks last ran", "card_id", cardID, "error", err)
		}
		return time.Time{}, false
	}
	return time.UnixMilli(millis).UTC(), true
}

// save writes a check's findings and records that the commit was checked, in one transaction, and
// answers the findings as they were read back.
func (s *Service) save(ctx context.Context, cardID, commit string, profile protocol.SmellProfile, raw []rawFinding) ([]protocol.SmellFinding, time.Time, error) {
	now := s.now()
	err := s.store.Write(ctx, func(q *db.Queries) error {
		if err := q.RecordSmellCheck(ctx, db.RecordSmellCheckParams{
			CardID: cardID, CommitSha: commit, CheckedAt: now.UnixMilli(),
		}); err != nil {
			return err
		}
		for _, finding := range raw {
			id, err := protocol.NewID(now, s.entropy)
			if err != nil {
				return err
			}
			severity := finding.Severity
			if severity == "" {
				severity = settingOf(profile, finding.Check).Severity
			}
			if err := q.InsertSmellFinding(ctx, db.InsertSmellFindingParams{
				ID: id, CardID: cardID, CommitSha: commit,
				Family: string(finding.family()), Smell: finding.smellName(),
				File: finding.File, Line: int64(finding.Line),
				Severity: string(severity), Message: finding.Message, Suggestion: finding.Suggestion,
				Status: string(protocol.SmellStatusOpen), DismissReason: "",
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("save the findings of card %s: %w", cardID, err)
	}
	findings, err := s.findings(ctx, cardID, commit)
	if err != nil {
		return nil, time.Time{}, err
	}
	return findings, now, nil
}

// publish sends the quality.checked event, so the card's checks panel redraws from one event.
func (s *Service) publish(cardID string, list protocol.SmellFindingList) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(string(protocol.CardTopic(cardID)), string(protocol.EventTypeQualityChecked),
		protocol.SmellCheckedEventData{
			CardID: list.CardID, Commit: list.Commit, Findings: list.Findings, Blocking: list.Blocking,
		}, false)
}

// lints runs every linter a project named, over the files the card changed, and answers what they
// found, kept to the card's own new lines. A linter that could not be run is logged and skipped: one
// linter that is not installed must not fail a card's whole check.
func (s *Service) linted(ctx context.Context, profile protocol.SmellProfile, dir string, files []sourceFile) []rawFinding {
	if len(profile.Linters) == 0 || len(files) == 0 {
		return nil
	}
	paths := make([]string, 0, len(files))
	byPath := make(map[string]sourceFile, len(files))
	for _, file := range files {
		if file.Now == "" {
			continue
		}
		paths = append(paths, file.Path)
		byPath[file.Path] = file
	}
	if len(paths) == 0 {
		return nil
	}
	sort.Strings(paths)
	request := LintRequest{Dir: dir, Files: paths, Language: commonLanguage(files)}
	var found []rawFinding
	for _, entry := range profile.Linters {
		linter, err := s.linters(entry)
		if err != nil {
			s.log.Warn("a project's linter could not be built", "linter", entry.Name, "error", err)
			continue
		}
		reported, err := linter.Lint(ctx, request)
		if err != nil {
			s.log.Warn("a project's linter could not be run", "linter", entry.Name, "error", err)
			continue
		}
		for _, one := range reported {
			file, known := byPath[one.File]
			if !known {
				continue
			}
			// A linter reads a whole file, so its findings are kept only where the card added a
			// line (or the file is new): the same new-or-worse rule the built-in checks follow.
			if !touchesAdded(file, one.Line, one.Line) && !file.New {
				continue
			}
			family := one.Family
			if family == "" {
				family = entry.Family
			}
			found = append(found, rawFinding{
				Family: family, Smell: entry.Name, File: one.File, Line: one.Line,
				Severity: protocol.SmellSeverityWarning, Message: one.Message,
				Suggestion: "Fix what the project's own linter reported.",
			})
		}
	}
	return found
}

// head reads the worktree's current commit. A worktree that cannot answer is logged and treated as
// uncommitted work, whose commit is the empty string.
func (s *Service) head(ctx context.Context, dir string) string {
	out, err := s.git.Run(ctx, dir, "rev-parse", "--verify", "HEAD")
	if err != nil {
		s.log.Warn("could not read the head commit of a card's worktree", "worktree", dir, "error", err)
		return ""
	}
	return strings.TrimSpace(out)
}

// notFoundFinding is the answer for a finding that cannot be found. A malformed id reads the same as
// an unknown one, because it can never exist and the two must not be told apart.
func notFoundFinding(id string) *protocol.Error {
	if len(id) > maxEchoedFindingID {
		id = id[:maxEchoedFindingID]
	}
	return protocol.NotFound("finding").With("id", id)
}

// maxEchoedFindingID cuts an id before it is sent back in an error.
const maxEchoedFindingID = 64

// folderExists reports whether path is a folder that can be looked at.
func folderExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// readWorktreeFile reads one file of a card's worktree. A path that would leave the worktree, and a
// file that cannot be read, answer the empty string: a file that is gone is simply not checked.
func readWorktreeFile(dir, path string) string {
	joined := filepath.Join(dir, filepath.FromSlash(path))
	cleanRoot := filepath.Clean(dir)
	if joined != cleanRoot && !strings.HasPrefix(joined, cleanRoot+string(filepath.Separator)) {
		return ""
	}
	data, err := os.ReadFile(joined)
	if err != nil {
		return ""
	}
	return string(data)
}
