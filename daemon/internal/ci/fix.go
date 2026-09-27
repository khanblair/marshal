package ci

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	gh "github.com/khanblair/marshal/daemon/internal/github"
	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The fix loop of docs/architecture.md section 9, the second half of the CI monitor
// (docs/backend-checklist.md B6.3, build-plan 6.3). A failed run on a card's branch is answered the
// same way whether GitHub told Marshal about it or the polling backup found it, which is why the
// forge is a parameter here and not a field: a delivery uses the daemon's own client, and a
// simulated failure (slice 3) uses a synthetic one that never opens a socket.
//
// The four steps, in order:
//
//  1. Rerun the failed jobs once. The run keeps its id and lands on the same row, so `rerun_at` is
//     what says this has happened and a restart does not rerun a second time.
//  2. When it fails again - the same run, this time with `rerun_at` set - fetch only the failed
//     step's log, trim it, and send it to the card's session. `fix_sent_at` is what says this has
//     happened, so every later delivery of the same failed run records the state without sending
//     the same log again.
//  3. Count the rounds against the card's loop limits. The count is the harness's own: one message
//     into a card's session is one turn (internal/session's `usageOf`), so a round here is a
//     message Marshal would send, and the ceiling is the one the card's role sets.
//  4. When the next message would take the card past its ceiling, do not send it: move the card to
//     Needs you with the existing `ci-failed` reason, so a person looks instead of Marshal feeding
//     the agent forever.
//
// A card that was never given a role, or whose role sets no ceilings, is not limited at all: the
// loop sends its one log per run and stops there, which is already bounded by `fix_sent_at`.

// fix runs section 9's loop for one failed run of one card. Every step that could not be taken is
// logged and not returned: a run Marshal recorded is recorded, and a forge that could not be asked
// for a log must not lose the state that arrived from the delivery.
func (s *Service) fix(ctx context.Context, project protocol.Project, card protocol.Card, row db.CiRun, repo gh.Repository, forge Forge) error {
	if forge == nil {
		s.log.Debug("a run failed but Marshal is not connected to a forge",
			"card_id", card.ID, "run", row.ID)
		return nil
	}
	// Step 1: the first failure reruns the failed jobs, once.
	if row.RerunAt == 0 {
		return s.rerun(ctx, card, row, repo, forge)
	}
	// Step 2: it failed again, so its log goes to the agent that wrote the branch - unless Marshal
	// already sent it for this run.
	if row.FixSentAt > 0 {
		return nil
	}
	// Steps 3 and 4: the round the send would be is counted against the card's ceilings first, so a
	// card already at its limit is handed to a person instead of being fed one more log.
	stop, err := s.atLoopLimit(ctx, project, card)
	if err != nil {
		return err
	}
	if stop {
		return nil
	}
	return s.sendFailure(ctx, card, row, repo, forge)
}

// rerun asks the forge to run this run's failed jobs again and remembers that it did. The answer to
// the rerun is a later delivery, so nothing else happens here: the run's own state comes back
// through `apply` like every other state.
func (s *Service) rerun(ctx context.Context, card protocol.Card, row db.CiRun, repo gh.Repository, forge Forge) error {
	id, err := runID(row)
	if err != nil {
		s.log.Warn("a failed run has an id Marshal cannot ask the forge about",
			"card_id", card.ID, "run", row.ID, "error", err)
		return nil
	}
	if err := forge.RerunFailedJobs(ctx, repo, id); err != nil {
		if errors.Is(err, ErrNoForge) {
			s.log.Debug("a run failed but Marshal is not connected to a forge", "card_id", card.ID)
			return nil
		}
		// A rerun that could not be asked for is not a reason to lose the failure: the state is
		// recorded, `rerun_at` is left at 0, and the next delivery tries again.
		s.log.Warn("could not rerun a run's failed jobs", "card_id", card.ID, "run", row.ID, "error", err)
		return nil
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		return q.SetCiRunRerun(ctx, db.SetCiRunRerunParams{RerunAt: s.now().UnixMilli(), ID: row.ID})
	})
	if err != nil {
		return fmt.Errorf("remember rerunning run %s: %w", row.ID, err)
	}
	s.log.Info("reran a run's failed jobs", "card_id", card.ID, "run", row.ID, "branch", row.Branch)
	return nil
}

// sendFailure fetches only the failed step's log, trims it, and sends it into the card's session as
// a message. It then remembers that it did, so the same run's log is never sent twice.
func (s *Service) sendFailure(ctx context.Context, card protocol.Card, row db.CiRun, repo gh.Repository, forge Forge) error {
	id, err := runID(row)
	if err != nil {
		s.log.Warn("a failed run has an id Marshal cannot ask the forge about",
			"card_id", card.ID, "run", row.ID, "error", err)
		return nil
	}
	logText, err := forge.FailedLog(ctx, repo, id, s.logBytes)
	if err != nil {
		if errors.Is(err, ErrNoForge) {
			s.log.Debug("a run failed but Marshal is not connected to a forge", "card_id", card.ID)
			return nil
		}
		s.log.Warn("could not read a failed run's log", "card_id", card.ID, "run", row.ID, "error", err)
		return nil
	}
	trimmed := trimLog(logText, s.logLines)
	if trimmed == "" {
		// A run with no readable log is still a failure somebody should see, but there is nothing
		// to send, and sending an empty message would be worse than saying nothing.
		s.log.Info("a failed run has no log to send", "card_id", card.ID, "run", row.ID)
		return nil
	}
	if s.worker == nil {
		s.log.Warn("a CI failure has nowhere to go: Marshal has no session manager",
			"card_id", card.ID, "run", row.ID)
		return s.markFixSent(ctx, row)
	}
	if err := s.worker.Send(ctx, card.ID, failureMessage(card, row, trimmed)); err != nil {
		// The message could not be delivered; `fix_sent_at` is left alone so a later delivery of
		// the same run tries again rather than the failure being silently dropped.
		s.log.Warn("could not send a CI failure to a card's session", "card_id", card.ID, "error", err)
		return nil
	}
	s.log.Info("sent a CI failure to a card's session", "card_id", card.ID, "run", row.ID, "branch", row.Branch)
	return s.markFixSent(ctx, row)
}

// markFixSent remembers that this run's failure has been handed over.
func (s *Service) markFixSent(ctx context.Context, row db.CiRun) error {
	err := s.store.Write(ctx, func(q *db.Queries) error {
		return q.SetCiRunFixSent(ctx, db.SetCiRunFixSentParams{
			FixSentAt: s.now().UnixMilli(), ID: row.ID,
		})
	})
	if err != nil {
		return fmt.Errorf("remember sending the failure of run %s: %w", row.ID, err)
	}
	return nil
}

// atLoopLimit decides whether the next message into the card's session would take it past the
// ceiling its role sets, and moves the card to Needs you when it would. It reports whether the loop
// must stop.
//
// The count is read exactly the way internal/session reads it (CountSessionEventsOfKind of the
// card's user events), so "rounds" means the same thing here as it does to the harness: how many
// times the card has been told to do something. The message the loop would send counts as the next
// one, so a card already at its ceiling is stopped without being sent anything.
func (s *Service) atLoopLimit(ctx context.Context, project protocol.Project, card protocol.Card) (bool, error) {
	if s.roles == nil || strings.TrimSpace(card.Role) == "" {
		return false, nil
	}
	limits, found, err := s.roles.LimitsFor(ctx, project.ID, card.Role)
	if err != nil {
		// A ceiling that cannot be read is not a ceiling: the loop carries on, which is the same
		// choice internal/session makes.
		s.log.Warn("could not read a role's limits", "card_id", card.ID, "role", card.Role, "error", err)
		return false, nil
	}
	if !found || limits.IsZero() || limits.Rounds <= 0 {
		return false, nil
	}
	rounds, err := s.rounds(ctx, card.ID)
	if err != nil {
		s.log.Warn("could not count a card's rounds", "card_id", card.ID, "error", err)
		return false, nil
	}
	if _, hit := limits.Check(harness.Usage{Rounds: rounds + 1}); !hit {
		return false, nil
	}
	text := fmt.Sprintf(
		"CI is still failing on this card's branch and it has taken %d rounds, past the role's limit of %d. "+
			"Look at the run and decide what to do next.", rounds+1, limits.Rounds)
	if _, err := s.cards.SetNeeds(ctx, card.ID, protocol.NeedsReason{
		Kind: protocol.NeedsReasonKindCIFailed, Text: text,
	}); err != nil {
		return false, fmt.Errorf("move card %s to needs you after its CI loop limit: %w", card.ID, err)
	}
	s.log.Warn("a card's CI loop hit its round limit", "card_id", card.ID, "role", card.Role,
		"rounds", rounds+1, "limit", limits.Rounds)
	return true, nil
}

// rounds counts how many rounds a card has taken, by the harness's own definition: the number of
// messages that have been delivered into its session.
func (s *Service) rounds(ctx context.Context, cardID string) (int, error) {
	var count int64
	err := s.store.Read(ctx, func(q *db.Queries) error {
		got, err := q.CountSessionEventsOfKind(ctx, db.CountSessionEventsOfKindParams{
			CardID: cardID, Kind: string(history.KindUser),
		})
		if err != nil {
			return err
		}
		count = got
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("count the rounds of card %s: %w", cardID, err)
	}
	return int(count), nil
}

// runID reads a stored row's run id back as the forge's number. A row whose id is not a number was
// written by something other than a delivery or the backup, which Marshal has no way to ask about.
func runID(row db.CiRun) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(row.ID), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("run id %q is not a number: %w", row.ID, err)
	}
	return id, nil
}

// failureMessage is what the card's agent reads when its branch's CI fails: which run, where to look,
// and the end of the failed step's log.
func failureMessage(card protocol.Card, row db.CiRun, log string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "CI failed on this card's branch %s.\n\n", row.Branch)
	fmt.Fprintf(&b, "%s failed", capitalize(describe(row)))
	if row.Url != "" {
		fmt.Fprintf(&b, " (%s)", row.Url)
	}
	b.WriteString(". The failed step's log ends with:\n\n")
	b.WriteString(log)
	b.WriteString("\n\nFix the failure and push to the same branch.")
	return b.String()
}

// capitalize upper-cases the first letter of a sentence built from a name, so a message reads as
// prose rather than as a fragment.
func capitalize(text string) string {
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

// trimLog keeps the end of a log, which is where a failure is, and says how much was dropped. A log
// already short enough is handed back as it is, so the common case is not decorated.
func trimLog(text string, lines int) string {
	text = strings.TrimRight(text, "\n")
	if strings.TrimSpace(text) == "" {
		return ""
	}
	if lines <= 0 {
		return text
	}
	all := strings.Split(text, "\n")
	if len(all) <= lines {
		return text
	}
	kept := all[len(all)-lines:]
	var b strings.Builder
	fmt.Fprintf(&b, "(the first %d lines of the log are not shown)\n", len(all)-lines)
	b.WriteString(strings.Join(kept, "\n"))
	return b.String()
}
