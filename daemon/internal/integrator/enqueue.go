package integrator

import (
	"context"
	"slices"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Enqueue is what runs when a card becomes ready to merge. The projects module calls it through its
// OnReadyToMerge hook, in the middle of a drag or of a review, so it never waits: it decides, and a
// merge that is wanted runs in a goroutine of its own, one card at a time per project, in the order
// the cards became ready.
//
// A card is merged on its own only when the project's auto-merge is on, the project is not paused,
// and no earlier delivery waits for the owner. Otherwise it stays in Ready to merge, where Resume or
// a manual merge picks it up.
func (s *Service) Enqueue(ctx context.Context, cardID string) {
	if s.closing.Load() {
		return
	}
	card, err := s.cards.Card(ctx, cardID)
	if err != nil {
		s.log.Warn("could not read a card that became ready to merge", "card_id", cardID, "error", err)
		return
	}
	if card.State != protocol.CardStateReady {
		return
	}
	if card.MergePhase == protocol.MergePhaseStopped {
		// The card is ready again, so the stop that sent it away is over.
		s.clearProgress(ctx, card, "")
	}
	if !s.autoAllowed(ctx, card.ProjectID) {
		return
	}
	s.qmu.Lock()
	if slices.Contains(s.waiting[card.ProjectID], cardID) {
		s.qmu.Unlock()
		return
	}
	s.waiting[card.ProjectID] = append(s.waiting[card.ProjectID], cardID)
	start := !s.draining[card.ProjectID]
	s.draining[card.ProjectID] = true
	s.qmu.Unlock()
	if start {
		s.spawn(func(ctx context.Context) { s.drain(ctx, card.ProjectID) })
	}
}

// autoAllowed says whether a project's ready cards are merged without being asked.
func (s *Service) autoAllowed(ctx context.Context, projectID string) bool {
	settings, err := s.ledger.Settings(ctx, projectID)
	if err != nil {
		s.log.Warn("could not read the merge settings of a project", "project_id", projectID, "error", err)
		return false
	}
	return settings.AutoMerge && !settings.Paused && settings.PendingTip == ""
}

// drain merges a project's waiting cards one after another. It stops, leaving the rest in Ready to
// merge, when the settings no longer allow it: a pause, or a delivery that waits for the owner.
func (s *Service) drain(ctx context.Context, projectID string) {
	for {
		id, ok := s.nextWaiting(projectID)
		if !ok {
			return
		}
		if ctx.Err() != nil || !s.autoAllowed(ctx, projectID) {
			s.dropWaiting(projectID)
			return
		}
		if _, err := s.Merge(ctx, id); err != nil {
			s.log.Info("a queued merge did not run", "card_id", id, "error", err)
		}
	}
}

// nextWaiting takes the first waiting card of a project. When there is none the project stops
// draining, in the same step, so a card enqueued just after starts a new drain.
func (s *Service) nextWaiting(projectID string) (string, bool) {
	s.qmu.Lock()
	defer s.qmu.Unlock()
	ids := s.waiting[projectID]
	if len(ids) == 0 {
		delete(s.waiting, projectID)
		delete(s.draining, projectID)
		return "", false
	}
	s.waiting[projectID] = ids[1:]
	return ids[0], true
}

// dropWaiting forgets a project's waiting cards and stops its drain.
func (s *Service) dropWaiting(projectID string) {
	s.qmu.Lock()
	defer s.qmu.Unlock()
	delete(s.waiting, projectID)
	delete(s.draining, projectID)
}

// spawn runs fn in a goroutine that Wait and Close know about, with the service's own context, which
// the request that caused it does not cancel.
func (s *Service) spawn(fn func(ctx context.Context)) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		fn(s.base)
	}()
}

// Wait blocks until every merge that Enqueue or Retry started has finished.
func (s *Service) Wait() { s.wg.Wait() }

// Close stops new merges from starting, cancels the ones under way, and waits for them. A merge that
// was cancelled leaves its card marked as merging; Recover puts it back at the next start.
func (s *Service) Close() {
	s.closing.Store(true)
	s.cancel()
	s.wg.Wait()
}

// Recover puts the cards that a crash left marked as merging back to Ready to merge. It is for the
// start of the daemon, before any merge runs, and clears the cards' merge progress. A card that
// moves to Ready this way is announced to Enqueue like any other.
func (s *Service) Recover(ctx context.Context) error {
	rows, err := s.ledger.Merging(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		card := protocol.Card{ID: row.ID, ProjectID: row.ProjectID}
		s.clearProgress(ctx, card, "")
		if _, err := s.cards.SetState(ctx, row.ID, protocol.CardStateReady); err != nil {
			s.log.Warn("could not put a card back to ready after a restart", "card_id", row.ID, "error", err)
		}
	}
	return nil
}

// SetAutoMerge turns a project's automatic merging on or off. Turning it on merges the cards that
// are already waiting.
func (s *Service) SetAutoMerge(ctx context.Context, projectID string, on bool) (protocol.IntegrationState, error) {
	if _, err := s.projects.Get(ctx, projectID); err != nil {
		return protocol.IntegrationState{}, err
	}
	if err := s.ledger.SetAutoMerge(ctx, projectID, on); err != nil {
		return protocol.IntegrationState{}, err
	}
	if on {
		s.kick(ctx, projectID)
	}
	return s.State(ctx, projectID)
}

// kick enqueues the cards of a project that are already waiting in Ready to merge, in the order they
// became ready. Each one is checked against the settings again by Enqueue.
func (s *Service) kick(ctx context.Context, projectID string) {
	rows, err := s.ledger.Queue(ctx, projectID)
	if err != nil {
		s.log.Warn("could not read the merge queue", "project_id", projectID, "error", err)
		return
	}
	for _, row := range rows {
		if row.State == protocol.CardStateReady {
			s.Enqueue(ctx, row.ID)
		}
	}
}
