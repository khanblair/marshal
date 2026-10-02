package integrator

import (
	"context"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
)

// historyLimit is how many deliveries the Integration view shows.
const historyLimit = 20

// State answers what the project's Integrator is doing: the cards waiting or being merged, what it
// delivered, and how far its branch is ahead of the integration branch. It reads only, and never
// waits for a merge that is under way.
func (s *Service) State(ctx context.Context, projectID string) (protocol.IntegrationState, error) {
	project, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return protocol.IntegrationState{}, err
	}
	target := project.Target()
	settings, err := s.ledger.Settings(ctx, projectID)
	if err != nil {
		return protocol.IntegrationState{}, err
	}
	rows, err := s.ledger.Queue(ctx, projectID)
	if err != nil {
		return protocol.IntegrationState{}, err
	}
	stalled, err := s.ledger.Stalled(ctx, projectID)
	if err != nil {
		return protocol.IntegrationState{}, err
	}
	history, err := s.history(ctx, project)
	if err != nil {
		return protocol.IntegrationState{}, err
	}
	state := protocol.IntegrationState{
		ProjectID: projectID, Target: target, IntegratorBranch: protocol.IntegrationBranchName,
		AheadBy: s.aheadBy(ctx, project, target), Queue: queueItems(rows), History: history,
		ServerTime: store.Timestamp(s.now().UnixMilli()),
	}
	state.CurrentCardID = currentCard(rows)
	state.State, state.Message = integratorState(settings, state, stalled)
	return state, nil
}

// aheadBy counts the commits on the integrator branch that the integration branch does not have. A
// project with no integrator branch yet, or a branch that cannot be read, is 0.
func (s *Service) aheadBy(ctx context.Context, project protocol.Project, target string) int {
	exists, err := s.git.BranchExists(ctx, project.Path, protocol.IntegrationBranchName)
	if err != nil || !exists || target == "" {
		return 0
	}
	n, err := s.git.CountAhead(ctx, project.Path, target, protocol.IntegrationBranchName)
	if err != nil {
		s.log.Debug("could not count the Integrator branch's commits", "project_id", project.ID, "error", err)
		return 0
	}
	return n
}

// queueItems turns the cards in the queue into the wire items, numbered from 1. A ready card that
// no merge has touched yet is queued.
func queueItems(rows []CardRow) []protocol.IntegrationQueueItem {
	items := make([]protocol.IntegrationQueueItem, 0, len(rows))
	for i, row := range rows {
		phase := row.Phase
		if phase == "" || phase == protocol.MergePhaseStopped {
			phase = protocol.MergePhaseQueued
		}
		items = append(items, protocol.IntegrationQueueItem{
			CardID: row.ID, Key: row.Key, Title: row.Title, Phase: phase, Position: i + 1,
		})
	}
	return items
}

// currentCard is the card being merged, or "".
func currentCard(rows []CardRow) string {
	for _, row := range rows {
		if row.State == protocol.CardStateMerging {
			return row.ID
		}
	}
	return ""
}

// integratorState works out what the Integrator is doing and, when it is waiting, why. A merge under
// way comes first, then a pause, then a merge that stopped; cards that wait with auto-merge off are
// waiting for the owner too.
func integratorState(settings Settings, state protocol.IntegrationState, stalled []CardRow) (protocol.IntegratorState, string) {
	switch {
	case state.CurrentCardID != "":
		return protocol.IntegratorStateMerging, ""
	case settings.Paused:
		return protocol.IntegratorStatePaused, "Merging is paused."
	case len(stalled) > 0:
		return protocol.IntegratorStateWaiting, stalledMessage(stalled[0])
	case len(state.Queue) > 0 && !settings.AutoMerge:
		return protocol.IntegratorStateWaiting, "Auto-merge is off. Merge the waiting cards when you are ready."
	case len(state.Queue) > 0:
		return protocol.IntegratorStateMerging, ""
	}
	return protocol.IntegratorStateIdle, ""
}

// stalledMessage is the sentence for the card a merge stopped.
func stalledMessage(row CardRow) string {
	text := row.NeedsText
	if text == "" {
		text = row.Note
	}
	return fmt.Sprintf("%s is waiting for you. %s", row.Key, text)
}

// history reads the newest deliveries, each with whether it can still be undone.
func (s *Service) history(ctx context.Context, project protocol.Project) ([]protocol.IntegrationHistoryItem, error) {
	rows, err := s.ledger.Deliveries(ctx, project.ID, historyLimit)
	if err != nil {
		return nil, err
	}
	items := make([]protocol.IntegrationHistoryItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, protocol.IntegrationHistoryItem{
			CardID: row.CardID, Key: row.CardKey, Title: row.CardTitle,
			MergedAt: protocol.NewTimestamp(row.MergedAt), Commit: row.Commit, Resolved: row.Resolved,
			Summary: row.Summary, CanUndo: s.canUndo(ctx, project, row),
		})
	}
	return items, nil
}

// Pause stops the project's merging from starting on its own. A merge that is under way finishes.
func (s *Service) Pause(ctx context.Context, projectID string) (protocol.IntegrationState, error) {
	if _, err := s.projects.Get(ctx, projectID); err != nil {
		return protocol.IntegrationState{}, err
	}
	if err := s.ledger.SetPaused(ctx, projectID, true); err != nil {
		return protocol.IntegrationState{}, err
	}
	return s.State(ctx, projectID)
}

// Resume lets the project's merging start again, and merges the cards that waited in Ready to merge
// while it was paused.
func (s *Service) Resume(ctx context.Context, projectID string) (protocol.IntegrationState, error) {
	if _, err := s.projects.Get(ctx, projectID); err != nil {
		return protocol.IntegrationState{}, err
	}
	if err := s.ledger.SetPaused(ctx, projectID, false); err != nil {
		return protocol.IntegrationState{}, err
	}
	s.kick(ctx, projectID)
	return s.State(ctx, projectID)
}

// Retry runs the merge, or only the delivery, of a card that a merge stopped. The card's "stopped"
// phase is cleared at once. It is not moved to Ready first, because that would send it to Enqueue a
// second time. The merge runs in the background, and the card is answered as it is now.
func (s *Service) Retry(ctx context.Context, cardID string) (protocol.Card, error) {
	card, err := s.cards.Card(ctx, cardID)
	if err != nil {
		return protocol.Card{}, err
	}
	if !s.stalled(ctx, card) {
		return protocol.Card{}, protocol.Refused("Only a card that a merge stopped can be retried.").
			With("cardId", cardID).With("reason", "merge_not_stalled")
	}
	if s.closing.Load() || !s.startRetry(cardID) {
		return card, nil
	}
	s.clearProgress(ctx, card, "")
	s.spawn(func(ctx context.Context) {
		defer s.endRetry(cardID)
		if _, err := s.queue(ctx, card, true); err != nil {
			s.log.Info("a retried merge did not run", "card_id", cardID, "error", err)
		}
	})
	return s.cards.Card(ctx, cardID)
}
