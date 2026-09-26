// Package diff serves a card's diff over the API (docs/backend-checklist.md B2.9,
// docs/backend-inventory.md N15). It is the layer between the HTTP routes and the card's worktree:
// the routes read the request and write the answer, and this package finds the worktree, asks
// internal/gitx for the changed files or one file's hunks, bounds the answer, and turns a card
// that has nothing to show into the answer a client reads.
//
// It reads only. Nothing here changes a repository, and nothing here publishes an event: a diff is
// a view of a worktree the session manager is already changing.
//
// Git is never run from here. Every call goes through gitx, which is the one place the Git program
// is started (docs/backend-checklist.md section 4).
package diff

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const (
	// DefaultMaxFiles is the most files one list carries. A card that changed more than this has
	// its list cut and its Truncated flag set, so one enormous diff cannot be answered whole.
	DefaultMaxFiles = 2000
	// DefaultMaxLines is the most lines one file's hunks carry. A file with more is cut and says
	// so, which is what keeps a generated file from stalling the screen that opened it (N15).
	DefaultMaxLines = 2000
	// DefaultMaxBytes is how much of one Git command's output is read before the rest is dropped.
	DefaultMaxBytes = 2 << 20
	// DefaultLargeLines is the changed-line count at or above which a file is marked Large. The
	// screen keeps such a file collapsed until a person asks for it.
	DefaultLargeLines = 1000
)

// Projects is what the service needs from the projects module: the card, its project, and the
// worktree its agent worked in. It is the module's own interface, so the two modules still talk
// through it rather than through each other's tables.
type Projects interface {
	// Card returns one card, or a not found error when there is no such card.
	Card(ctx context.Context, id string) (protocol.Card, error)
	// Get returns the project a card belongs to, for its default branch.
	Get(ctx context.Context, id string) (protocol.Project, error)
	// Worktree returns the folder and branch recorded for a card's worktree. Both are empty until
	// the card starts.
	Worktree(ctx context.Context, cardID string) (path, branch string, err error)
}

// Limits bound what one answer carries. A zero field takes its default.
type Limits struct {
	// MaxFiles is the most files one list carries.
	MaxFiles int
	// MaxLines is the most lines one file's hunks carry.
	MaxLines int
	// MaxBytes caps how much of one Git command's output is read.
	MaxBytes int
	// LargeLines is the changed-line count at or above which a file is marked Large.
	LargeLines int
}

// withDefaults fills in every limit that was left at zero.
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
	if l.LargeLines <= 0 {
		l.LargeLines = DefaultLargeLines
	}
	return l
}

// Deps are the parts the service is built from. Both are required.
type Deps struct {
	// Projects says which worktree a card's work is in, and which branch it started from.
	Projects Projects
	// Git reads the diff of that worktree.
	Git *gitx.Git
}

// Service serves a card's diff. It is safe for use by many goroutines.
type Service struct {
	projects Projects
	git      *gitx.Git
	log      *slog.Logger
	now      func() time.Time
	limits   Limits
}

// Option changes how New builds a Service.
type Option func(*Service)

// WithClock sets the clock the answers' serverTime comes from. The default is time.Now.
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

// WithLimits sets how much one answer may carry. It is how a test reaches a cut answer with a diff
// it can count, and how a smaller machine could be given smaller bounds.
func WithLimits(limits Limits) Option {
	return func(s *Service) { s.limits = limits.withDefaults() }
}

// New builds the service.
func New(deps Deps, opts ...Option) (*Service, error) {
	if deps.Projects == nil || deps.Git == nil {
		return nil, errors.New("diff: the projects module and Git are both required")
	}
	s := &Service{
		projects: deps.Projects, git: deps.Git,
		log: slog.New(slog.DiscardHandler), now: time.Now, limits: Limits{}.withDefaults(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// worktree is everything the service found for one card: the card, its project, and the folder its
// agent worked in, if it ever started.
type worktree struct {
	project protocol.Project
	path    string
	branch  string
}

// readable reports whether there is a worktree to read. A card that never started has none, and a
// card whose recorded folder was deleted by hand is treated the same way: an empty diff, never an
// error.
func (w worktree) readable() bool { return w.path != "" && w.project.DefaultBranch != "" }

// Files returns the card's whole diff: every file it changed, with the counts of each, and no
// hunks. An unknown card is not found. A card that never started, or whose worktree is gone, is an
// empty diff and not an error: the Diff tab draws its empty state for it.
func (s *Service) Files(ctx context.Context, cardID string) (protocol.CardDiff, error) {
	work, err := s.worktreeOf(ctx, cardID)
	if err != nil {
		return protocol.CardDiff{}, err
	}
	answer := protocol.CardDiff{
		CardID: cardID, Base: work.project.DefaultBranch, Branch: work.branch,
		Files: []protocol.ChangedFile{}, ServerTime: protocol.NewTimestamp(s.now()),
	}
	if !work.readable() {
		return answer, nil
	}
	files, truncated, err := s.git.DiffFiles(ctx, work.path, work.project.DefaultBranch, s.limits.MaxBytes)
	if err != nil {
		return protocol.CardDiff{}, fmt.Errorf("read the diff of card %s: %w", cardID, err)
	}
	for _, file := range files {
		answer.Additions += file.Additions
		answer.Deletions += file.Deletions
	}
	answer.FileCount = len(files)
	if len(files) > s.limits.MaxFiles {
		files, truncated = files[:s.limits.MaxFiles], true
	}
	for _, file := range files {
		answer.Files = append(answer.Files, changedFile(file, s.limits.LargeLines))
	}
	answer.Truncated = truncated
	return answer, nil
}

// Hunks returns one file's hunks, for the screen that opened that file. The file must be one the
// card changed: a path that is not part of the diff, an unknown card, and a card with no worktree
// are all not found, so a client can never ask the daemon to read a file outside the worktree.
func (s *Service) Hunks(ctx context.Context, cardID, path string) (protocol.FileHunks, error) {
	work, err := s.worktreeOf(ctx, cardID)
	if err != nil {
		return protocol.FileHunks{}, err
	}
	if !work.readable() {
		return protocol.FileHunks{}, notFoundFile(path)
	}
	file, found, err := s.git.DiffHunks(ctx, work.path, work.project.DefaultBranch, path, s.limits.MaxLines, s.limits.MaxBytes)
	if err != nil {
		if errors.Is(err, gitx.ErrBadPath) {
			// A path that cannot name a file inside the worktree reads the same as a path that
			// names nothing at all.
			return protocol.FileHunks{}, notFoundFile(path)
		}
		return protocol.FileHunks{}, fmt.Errorf("read the diff of %s on card %s: %w", path, cardID, err)
	}
	if !found {
		return protocol.FileHunks{}, notFoundFile(path)
	}
	answer := protocol.FileHunks{
		Path: path, Status: protocol.DiffFileStatus(file.Status), Hunks: []protocol.DiffHunk{},
		Truncated: file.Truncated, ServerTime: protocol.NewTimestamp(s.now()),
	}
	for _, hunk := range file.Hunks {
		answer.Hunks = append(answer.Hunks, wireHunk(hunk))
	}
	return answer, nil
}

// worktreeOf finds the card, its project, and the folder its agent worked in. A recorded folder
// that is no longer there is logged and cleared: a worktree a person deleted by hand leaves the
// card with a path that names nothing, and the diff of such a card is empty rather than broken.
func (s *Service) worktreeOf(ctx context.Context, cardID string) (worktree, error) {
	card, err := s.projects.Card(ctx, cardID)
	if err != nil {
		return worktree{}, err
	}
	project, err := s.projects.Get(ctx, card.ProjectID)
	if err != nil {
		return worktree{}, fmt.Errorf("read the project of card %s: %w", cardID, err)
	}
	path, branch, err := s.projects.Worktree(ctx, cardID)
	if err != nil {
		return worktree{}, err
	}
	if path != "" && !folderExists(path) {
		s.log.Warn("a card's worktree folder is gone, so its diff is empty",
			"card_id", cardID, "project_id", card.ProjectID, "worktree", path)
		path = ""
	}
	return worktree{project: project, path: path, branch: branch}, nil
}

// changedFile maps one file of a Git diff to the wire type, and marks the files a screen keeps
// collapsed until they are asked for.
func changedFile(file gitx.DiffFile, largeLines int) protocol.ChangedFile {
	return protocol.ChangedFile{
		Path:      file.Path,
		OldPath:   file.OldPath,
		Status:    protocol.DiffFileStatus(file.Status),
		Additions: file.Additions,
		Deletions: file.Deletions,
		Binary:    file.Binary,
		Large:     !file.Binary && file.Additions+file.Deletions >= largeLines,
	}
}

// wireHunk maps one hunk of a Git diff to the wire type.
func wireHunk(hunk gitx.DiffHunk) protocol.DiffHunk {
	lines := make([]protocol.DiffLine, 0, len(hunk.Lines))
	for _, line := range hunk.Lines {
		lines = append(lines, protocol.DiffLine{
			Kind: protocol.DiffLineKind(line.Kind), OldLine: line.OldLine, NewLine: line.NewLine, Text: line.Text,
		})
	}
	return protocol.DiffHunk{Header: hunk.Header, Lines: lines}
}

// notFoundFile is the answer for a path that is not part of a card's diff. It is built the way the
// other services build theirs, so a path that cannot exist and one that belongs to another card
// read the same.
func notFoundFile(path string) *protocol.Error {
	if len(path) > maxEchoedPathBytes {
		path = path[:maxEchoedPathBytes]
	}
	return protocol.NotFound("file").With("path", path)
}

// maxEchoedPathBytes cuts a path before it is sent back in an error, so a very long address is not
// repeated in full.
const maxEchoedPathBytes = 256

// folderExists reports whether path is a folder that can be looked at.
func folderExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
