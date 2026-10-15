package integrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The way a finished card reaches Ready to merge on this machine, with nothing sent anywhere: its
// leftover work is committed, the Reviewer reads its changes, the project's own checks run on them,
// and then the card moves to Ready to merge, where the queue takes it. Each stage can be turned off
// per project (Settings), and a stage that has nothing to do it with (no reviewer wired, no workflow
// files) is passed over rather than failed.
//
// A review that asks for changes, or checks that fail, send the card back to Working and say so in
// the card's own session, so the agent that wrote the changes fixes them and finishes again. Nothing
// here waits for a person unless something could not run.

// ChangeReview is what a Reviewer is given: the card, and the folder and branch its changes are in.
type ChangeReview struct {
	// Card is the card whose changes are read.
	Card protocol.Card
	// Project is the card's project.
	Project protocol.Project
	// Worktree is the folder the card's changes are in, committed.
	Worktree string
	// Base is the branch the card's changes are measured against: the project's integration branch.
	Base string
}

// ChangeComment is one thing the Reviewer found. An empty Path is a comment on the changes as a whole.
type ChangeComment struct {
	Path string
	Line int
	Body string
}

// ChangeVerdict is what the Reviewer answered.
type ChangeVerdict struct {
	// Approved is true when the Reviewer read the changes and has no changes to ask for.
	Approved bool
	// Summary is one plain sentence about the changes.
	Summary string
	// Comments are what was found, one by one.
	Comments []ChangeComment
}

// ChangeReviewer reads a card's committed changes and answers with a verdict.
type ChangeReviewer interface {
	ReviewChanges(ctx context.Context, req ChangeReview) (ChangeVerdict, error)
}

// CheckOutcome is what running a project's checks found.
type CheckOutcome struct {
	// Ran is false when the project has nothing that can run here, which is not a failure.
	Ran bool
	// Passed is true when everything that ran passed. It is meaningful only when Ran is true.
	Passed bool
	// Summary is a few plain lines for a person: what failed and what it printed.
	Summary string
}

// ChangeChecker runs the project's own checks on a card's changes, on this machine.
type ChangeChecker interface {
	CheckChanges(ctx context.Context, cardID string) (CheckOutcome, error)
}

// Notes posts a note on a card, where a person reads it with the card's comments.
type Notes interface {
	Note(ctx context.Context, cardID, body string) error
}

// Worker sends a message into a card's own session, waking it when it sleeps.
type Worker interface {
	Send(ctx context.Context, cardID, text string) error
}

// startShip claims a card's review and checks, and says false when they are already running.
func (s *Service) startShip(cardID string) bool {
	s.qmu.Lock()
	defer s.qmu.Unlock()
	if s.shipping[cardID] {
		return false
	}
	s.shipping[cardID] = true
	return true
}

// endShip releases a card's review and checks.
func (s *Service) endShip(cardID string) {
	s.qmu.Lock()
	delete(s.shipping, cardID)
	s.qmu.Unlock()
}

// commitLeftovers commits what a card's worktree holds that its branch does not, so the review and
// the merge read what is there. A commit that cannot be made is logged; the review then reads what
// was committed.
func (s *Service) commitLeftovers(ctx context.Context, card protocol.Card) {
	if strings.TrimSpace(card.Worktree) == "" {
		return
	}
	if _, err := s.git.CommitAll(ctx, card.Worktree, "Marshal: "+card.Title); err != nil {
		s.log.Warn("could not commit what a card left in its worktree", "card_id", card.ID, "error", err)
	}
}

// ship runs a card's review and checks, then moves it to Ready to merge, or sends it back, or stops
// it with the reason. It runs in the background: a review takes a model's time.
func (s *Service) ship(ctx context.Context, card protocol.Card, review, checks bool) {
	defer s.endShip(card.ID)
	project, err := s.projects.Get(ctx, card.ProjectID)
	if err != nil {
		s.stopShip(ctx, card, "The card's project could not be read: "+err.Error())
		return
	}
	if review && !s.review(ctx, card, project) {
		return
	}
	if checks && !s.check(ctx, card) {
		return
	}
	s.clearProgress(ctx, card, "")
	if _, err := s.cards.SetState(ctx, card.ID, protocol.CardStateReady); err != nil {
		s.stopShip(ctx, card, "The card could not be moved to Ready to merge: "+err.Error())
	}
}

// review has the Reviewer read the card's changes. It answers whether the card goes on.
func (s *Service) review(ctx context.Context, card protocol.Card, project protocol.Project) bool {
	s.setProgress(ctx, card, Progress{Phase: protocol.MergePhaseReviewing, Note: "Reading the changes", DoingNow: "Being reviewed"})
	verdict, err := s.reviewer.ReviewChanges(ctx, ChangeReview{
		Card: card, Project: project, Worktree: card.Worktree, Base: project.Target(),
	})
	if err != nil {
		s.stopShip(ctx, card, "The review could not run: "+err.Error())
		return false
	}
	s.note(ctx, card.ID, reviewNote(verdict))
	if verdict.Approved {
		return true
	}
	s.sendBack(ctx, card, workerReviewText(verdict), "The Reviewer asked for changes: "+oneLine(verdict.Summary))
	return false
}

// check runs the project's checks on the card's changes. It answers whether the card goes on.
func (s *Service) check(ctx context.Context, card protocol.Card) bool {
	s.setProgress(ctx, card, Progress{Phase: protocol.MergePhaseChecking, Note: "Running the checks", DoingNow: "Running checks"})
	outcome, err := s.checker.CheckChanges(ctx, card.ID)
	if err != nil {
		s.stopShip(ctx, card, "The checks could not run: "+err.Error())
		return false
	}
	if !outcome.Ran {
		return true
	}
	if !outcome.Passed {
		s.note(ctx, card.ID, "**Checks failed.**\n\n"+outcome.Summary)
		s.sendBack(ctx, card,
			"The project's checks failed on your changes.\n\n"+outcome.Summary+"\n\nFix them, commit, and finish the card again.",
			"The checks failed: "+oneLine(outcome.Summary))
		return false
	}
	passed := protocol.CIStatePassed
	if _, err := s.cards.SetCI(ctx, card.ID, &passed); err != nil {
		s.log.Warn("could not record that a card's checks passed", "card_id", card.ID, "error", err)
	}
	return true
}

// sendBack returns a card to Working and tells its agent what to fix. An agent that cannot be reached
// leaves the card in Needs you with the same words.
func (s *Service) sendBack(ctx context.Context, card protocol.Card, toWorker, reason string) {
	s.clearProgress(ctx, card, reason)
	if _, err := s.cards.SetState(ctx, card.ID, protocol.CardStateWorking); err != nil {
		s.log.Warn("could not send a card back to working", "card_id", card.ID, "error", err)
		return
	}
	if s.worker != nil {
		err := s.worker.Send(ctx, card.ID, toWorker)
		if err == nil {
			return
		}
		s.log.Warn("could not tell a card's agent what to fix", "card_id", card.ID, "error", err)
	}
	s.needs(ctx, card.ID, reason+" Its agent could not be reached, so it needs you.")
}

// stopShip ends a card's review or checks with the reason, in Needs you. Pressing Send to merge again
// runs them again.
func (s *Service) stopShip(ctx context.Context, card protocol.Card, reason string) {
	s.clearProgress(ctx, card, reason)
	s.needs(ctx, card.ID, reason)
}

// needs moves a card to Needs you, and logs when it cannot.
func (s *Service) needs(ctx context.Context, cardID, reason string) {
	if _, err := s.cards.SetNeeds(ctx, cardID, protocol.NeedsReason{Kind: protocol.NeedsReasonKindStuck, Text: reason}); err != nil {
		s.log.Warn("could not move a card to Needs you", "card_id", cardID, "error", err)
	}
}

// note posts a note on the card, and logs when it cannot.
func (s *Service) note(ctx context.Context, cardID, body string) {
	if s.notes == nil || strings.TrimSpace(body) == "" {
		return
	}
	if err := s.notes.Note(ctx, cardID, body); err != nil {
		s.log.Warn("could not post a review note on a card", "card_id", cardID, "error", err)
	}
}

// reviewNote is what the Reviewer said, as one note a person reads on the card.
func reviewNote(v ChangeVerdict) string {
	var b strings.Builder
	if v.Approved {
		b.WriteString("**Reviewed and approved.**")
	} else {
		b.WriteString("**Reviewed. Changes are needed.**")
	}
	if summary := strings.TrimSpace(v.Summary); summary != "" {
		b.WriteString("\n\n" + summary)
	}
	for _, c := range v.Comments {
		if strings.TrimSpace(c.Body) == "" {
			continue
		}
		b.WriteString("\n\n- ")
		if c.Path != "" {
			fmt.Fprintf(&b, "`%s`", c.Path)
			if c.Line > 0 {
				fmt.Fprintf(&b, ":%d", c.Line)
			}
			b.WriteString(": ")
		}
		b.WriteString(strings.TrimSpace(c.Body))
	}
	return b.String()
}

// workerReviewText is what the agent that wrote the changes is told when the Reviewer asked for more.
func workerReviewText(v ChangeVerdict) string {
	var b strings.Builder
	b.WriteString("The Reviewer read your changes and asked for changes.")
	if summary := strings.TrimSpace(v.Summary); summary != "" {
		b.WriteString("\n\n" + summary)
	}
	for _, c := range v.Comments {
		if strings.TrimSpace(c.Body) == "" {
			continue
		}
		b.WriteString("\n\n- ")
		if c.Path != "" {
			fmt.Fprintf(&b, "%s:%d: ", c.Path, c.Line)
		}
		b.WriteString(strings.TrimSpace(c.Body))
	}
	b.WriteString("\n\nFix these, commit, and finish the card again.")
	return b.String()
}

// oneLine is a text cut to its first line and a length a card shows in one place.
func oneLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	const most = 200
	if r := []rune(line); len(r) > most {
		return string(r[:most]) + "..."
	}
	return line
}

// CardFinished is what happens when a card's agent has said it is finished and its turn has ended. With
// the project's auto-ship on, the card goes the whole way (SendToMerge); with it off, the card waits in
// In review with the agent's summary, for a person to send. A card that cannot go (it has no commits,
// or a required checklist is open) is left in Needs you with the reason, so the agent's claim is never
// silently ignored.
func (s *Service) CardFinished(ctx context.Context, cardID, summary string) {
	card, err := s.cards.Card(ctx, cardID)
	if err != nil {
		s.log.Warn("could not read a card its agent finished", "card_id", cardID, "error", err)
		return
	}
	switch card.State {
	case protocol.CardStateDone, protocol.CardStateMerging, protocol.CardStateReady,
		protocol.CardStateBacklog, protocol.CardStatePlanning:
		return
	}
	s.note(ctx, cardID, "**The agent says it is finished.** "+summary)
	settings, err := s.ledger.Settings(ctx, card.ProjectID)
	if err != nil {
		s.log.Warn("could not read a project's settings for a finished card", "card_id", cardID, "error", err)
		return
	}
	if !settings.AutoShip {
		if card.State != protocol.CardStateReview {
			if _, err := s.cards.SetState(ctx, cardID, protocol.CardStateReview); err != nil {
				s.log.Warn("could not move a finished card to review", "card_id", cardID, "error", err)
			}
		}
		return
	}
	if _, err := s.SendToMerge(ctx, cardID); err != nil {
		var refusal *protocol.Error
		reason := err.Error()
		if errors.As(err, &refusal) {
			reason = refusal.Message
		}
		s.needs(ctx, cardID, "The agent says it is finished, but the card cannot be merged yet. "+reason)
	}
}
