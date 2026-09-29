// Package integrator runs Marshal's merge queue (docs/architecture.md section 8,
// docs/backend-checklist.md B5.5, build-plan 5.8 and 5.9). A card in Ready to merge is merged into
// the project's default branch one at a time per project, the target branch moving forward only
// after the merge has been made in a throwaway worktree and its tests have passed.
//
// The order is the one the architecture doc draws:
//
//  1. a dry-run merge, which touches nothing and names the conflicts;
//  2. a backup branch at the target's tip;
//  3. a merge inside a temporary worktree;
//  4. the tests a merge must pass;
//  5. the target branch moved forward, or aborted with the target untouched.
//
// When the merge conflicts, the queue does not guess at a resolution: it names the conflicted files
// on the card and moves the card to Needs you, which is "the Integrator says so when it is not
// confident". Resolving a conflict by intent needs the context of every card involved and is a
// later slice; this queue is the mechanism around it.
//
// Nothing here pushes anywhere, and the only method that moves a branch moves it forward
// (gitx.FastForwardRef). Every test runs against a throwaway repository.
package integrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

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

// Git is the part of gitx the queue uses. It is the merge-queue primitives (internal/gitx/merge.go)
// and nothing else, so a test can drive the whole flow with a fake.
type Git interface {
	DryRunMerge(ctx context.Context, repo, into, branch string) (gitx.MergePreview, error)
	BackupBranch(ctx context.Context, repo, name, from string) error
	AddMergeWorktree(ctx context.Context, repo, path, base string) error
	MergeInto(ctx context.Context, worktree, branch, message string) (string, error)
	FastForwardRef(ctx context.Context, repo, target, commit string) error
	AbortMerge(ctx context.Context, worktree string) error
	RemoveWorktree(ctx context.Context, repo, path, root string, force bool) error
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

// Deps are the parts the queue is built from.
type Deps struct {
	Cards    Cards
	Projects Projects
	Git      Git
	// Tests runs the merge's tests. Nil means a clean merge is enough.
	Tests Tester
	// Checklists holds a card back while a required checklist is open (B10.5). Nil means none is.
	Checklists ChecklistGate
	// DataDir is where the queue's temporary worktrees go: <DataDir>/merge/<project>/<card>.
	DataDir string
	// Log is where problems are written. Nil discards.
	Log *slog.Logger
}

// Service is the merge queue. It is safe for use by many goroutines; one card at a time per project
// is held by a per-project lock.
type Service struct {
	cards    Cards
	projects Projects
	git      Git
	tests    Tester
	lists    ChecklistGate
	dataDir  string
	log      *slog.Logger
	locks    keyedlock.Locks
}

// New builds a Service. Everything but Tests is required.
func New(deps Deps) (*Service, error) {
	if deps.Cards == nil || deps.Projects == nil || deps.Git == nil {
		return nil, errors.New("the merge queue needs the cards, the projects, and Git")
	}
	if !filepath.IsAbs(deps.DataDir) {
		return nil, errors.New("the merge queue needs the data folder as a full path")
	}
	log := deps.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Service{
		cards: deps.Cards, projects: deps.Projects, git: deps.Git,
		tests: deps.Tests, lists: deps.Checklists, dataDir: filepath.Clean(deps.DataDir), log: log,
	}, nil
}

// ChecklistGate says whether a card's required checklists are done. The card panel implements it.
type ChecklistGate interface {
	// OpenRequiredItems says how many lines of the card's required checklists are still open.
	OpenRequiredItems(ctx context.Context, cardID string) (int, error)
}

// Result says what the queue did.
type Result struct {
	// Merged is true when the target branch moved forward.
	Merged bool
	// Commit is the merge commit, when there is one.
	Commit string
	// Reason is a plain sentence for a person when the merge did not finish.
	Reason string
}

// Merge takes one card from Ready to merge to Done, or leaves it in Needs you with a reason, and
// the project's default branch untouched. One card at a time per project: a second card waits for
// the first project's merge to finish.
//
// A card that is not in Ready to merge is refused, because the queue is not a way to merge a card
// the board has not sent to it.
func (s *Service) Merge(ctx context.Context, cardID string) (Result, error) {
	card, err := s.cards.Card(ctx, cardID)
	if err != nil {
		return Result{}, err
	}
	if card.State != protocol.CardStateReady {
		return Result{}, protocol.Refused("Only a card in Ready to merge goes to the merge queue.").
			With("cardId", cardID).With("reason", "merge_not_ready")
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
	project, err := s.projects.Get(ctx, card.ProjectID)
	if err != nil {
		return Result{}, err
	}
	unlock := s.locks.Lock(card.ProjectID)
	defer unlock()
	return s.merge(ctx, card, project)
}

// merge is Merge's body, run while the project's lock is held.
func (s *Service) merge(ctx context.Context, card protocol.Card, project protocol.Project) (Result, error) {
	if strings.TrimSpace(card.Branch) == "" {
		return s.fail(ctx, card.ID, protocol.NeedsReasonKindConflict,
			"This card has no branch to merge.")
	}
	target := project.DefaultBranch
	if strings.TrimSpace(target) == "" {
		return s.fail(ctx, card.ID, protocol.NeedsReasonKindConflict,
			"This project has no default branch to merge into.")
	}
	// The board shows the card as merging from here until it is done or sent back.
	if _, err := s.cards.SetState(ctx, card.ID, protocol.CardStateMerging); err != nil {
		return Result{}, err
	}

	preview, err := s.git.DryRunMerge(ctx, project.Path, target, card.Branch)
	if err != nil {
		return s.fail(ctx, card.ID, protocol.NeedsReasonKindConflict,
			"The merge could not be tested: "+err.Error())
	}
	if !preview.Clean {
		return s.fail(ctx, card.ID, protocol.NeedsReasonKindConflict, conflictReason(target, preview.Conflicts))
	}

	// A backup branch keeps the target's tip, so a mistaken move is recoverable. Its name carries
	// the card's own id, so two merges never share a backup.
	backup := "marshal/backup/" + target + "-" + shortID(card.ID)
	if err := s.git.BackupBranch(ctx, project.Path, backup, target); err != nil {
		// The backup is a safety net, not a requirement: a backup that cannot be made (for
		// example, one with that name already there) must not stop a merge whose dry run was clean.
		s.log.Warn("could not make a merge backup branch", "card_id", card.ID, "branch", backup, "error", err)
	}

	worktree := s.worktreePath(project.ID, card.ID)
	defer s.cleanWorktree(ctx, project.Path, worktree, project.ID)
	if err := s.git.AddMergeWorktree(ctx, project.Path, worktree, target); err != nil {
		return s.fail(ctx, card.ID, protocol.NeedsReasonKindConflict,
			"The merge worktree could not be made: "+err.Error())
	}
	commit, err := s.git.MergeInto(ctx, worktree, card.Branch, mergeMessage(card))
	if err != nil {
		if errors.Is(err, gitx.ErrMergeConflict) {
			_ = s.git.AbortMerge(ctx, worktree)
			return s.fail(ctx, card.ID, protocol.NeedsReasonKindConflict, conflictReason(target, preview.Conflicts))
		}
		_ = s.git.AbortMerge(ctx, worktree)
		return s.fail(ctx, card.ID, protocol.NeedsReasonKindConflict, "The merge failed: "+err.Error())
	}

	if s.tests != nil {
		outcome, err := s.tests.Run(ctx, worktree, preview.Changed)
		if err != nil {
			return s.fail(ctx, card.ID, protocol.NeedsReasonKindCIFailed,
				"The merge queue's tests could not run: "+err.Error()+" The target branch was not changed.")
		}
		if !outcome.Passed {
			summary := outcome.Summary
			if strings.TrimSpace(summary) == "" {
				summary = "the tests failed"
			}
			return s.fail(ctx, card.ID, protocol.NeedsReasonKindCIFailed,
				"The merge queue's tests did not pass: "+summary+" The target branch was not changed.")
		}
	}

	// Everything passed. The target moves forward now, and nowhere else.
	if err := s.git.FastForwardRef(ctx, project.Path, target, commit); err != nil {
		return s.fail(ctx, card.ID, protocol.NeedsReasonKindConflict,
			"The target branch could not be moved forward: "+err.Error())
	}
	if _, err := s.cards.SetState(ctx, card.ID, protocol.CardStateDone); err != nil {
		return Result{Merged: true, Commit: commit}, err
	}
	s.log.Info("merged a card", "project_id", project.ID, "card_id", card.ID, "branch", card.Branch, "commit", commit)
	return Result{Merged: true, Commit: commit}, nil
}

// fail moves a card to Needs you with a reason the merge queue found, and returns a Result naming
// the same sentence. The target branch is untouched in every case that calls it.
func (s *Service) fail(ctx context.Context, cardID string, kind protocol.NeedsReasonKind, text string) (Result, error) {
	if _, err := s.cards.SetNeeds(ctx, cardID, protocol.NeedsReason{Kind: kind, Text: text}); err != nil {
		return Result{}, err
	}
	s.log.Warn("the merge queue stopped a card", "card_id", cardID, "reason", text)
	return Result{Merged: false, Reason: text}, nil
}

// cleanWorktree removes the queue's temporary worktree. It never fails the merge: a leftover folder
// is a smaller problem than a merge reported as failed after the target moved.
func (s *Service) cleanWorktree(ctx context.Context, repo, worktree, projectID string) {
	root := filepath.Join(s.dataDir, "merge", projectID)
	if err := s.git.RemoveWorktree(ctx, repo, worktree, root, true); err != nil {
		s.log.Warn("could not remove the merge worktree", "worktree", worktree, "error", err)
	}
}

// worktreePath is where the queue puts the temporary worktree of one card's merge.
func (s *Service) worktreePath(projectID, cardID string) string {
	return filepath.Join(s.dataDir, "merge", projectID, cardID)
}

// MergeRoot is the folder the queue's temporary worktrees live under, for one project. The daemon
// and the tests share this one rule, the way they share projects.WorktreesDir.
func MergeRoot(dataDir, projectID string) string {
	return filepath.Join(dataDir, "merge", projectID)
}

// conflictReason is the sentence a person reads when a merge conflicts.
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
