package accounts

import (
	"context"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// maxOnboardingStep is the last onboarding screen a person can be saved at, counted from 0. The
// design has four screens; the number leaves room for a screen to be added without a migration,
// and only stops nonsense.
const maxOnboardingStep = 9

// Progress returns how far the person is with onboarding and with the tour. A person who has not
// started either has both pending, at step 0.
func (s *Service) Progress(ctx context.Context, userID string) (protocol.Progress, error) {
	if _, err := readUser(ctx, s.store.Queries(), userID); err != nil {
		return protocol.Progress{}, err
	}
	return readProgress(ctx, s.store.Queries(), userID)
}

// UpdateProgress saves the parts of the progress the request sets. Skipping and finishing both
// stamp the time and differ only in the status, so a skipped onboarding never shows again by
// itself and can still be told from a finished one. A request that changes nothing answers with
// the progress as it is and publishes nothing.
func (s *Service) UpdateProgress(ctx context.Context, userID string, in protocol.UpdateProgressRequest) (protocol.Progress, error) {
	if err := checkProgressUpdate(in); err != nil {
		return protocol.Progress{}, err
	}
	now := store.Millis(s.now())
	return s.changeProgress(ctx, userID, func(row db.UserProgress) db.UserProgress {
		return applyProgressUpdate(row, in, now)
	})
}

// ResetFirstLaunch puts onboarding back to its first screen and the tour back to pending, as they
// are for a person who has just installed Marshal. Only a dev daemon offers it (architecture.md
// 11.1), so a developer can walk through first launch again.
func (s *Service) ResetFirstLaunch(ctx context.Context, userID string) (protocol.Progress, error) {
	return s.changeProgress(ctx, userID, func(db.UserProgress) db.UserProgress {
		return db.UserProgress{UserID: userID}
	})
}

// changeProgress applies a change to the person's progress row, writes it when it differs, and
// publishes me.updated when it did.
func (s *Service) changeProgress(ctx context.Context, userID string, change func(db.UserProgress) db.UserProgress) (protocol.Progress, error) {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	var (
		next    db.UserProgress
		changed bool
	)
	err := s.store.Write(ctx, func(q *db.Queries) error {
		if _, err := readUser(ctx, q, userID); err != nil {
			return err
		}
		current, err := readProgressRow(ctx, q, userID)
		if err != nil {
			return err
		}
		next = change(current)
		if next == current {
			return nil
		}
		changed = true
		err = q.UpsertUserProgress(ctx, db.UpsertUserProgressParams{
			UserID: userID, OnboardingStep: next.OnboardingStep, OnboardingDoneAt: next.OnboardingDoneAt,
			OnboardingSkipped: next.OnboardingSkipped, TutorialDoneAt: next.TutorialDoneAt,
			TutorialSkipped: next.TutorialSkipped,
		})
		if err != nil {
			return fmt.Errorf("save the progress of user %s: %w", userID, err)
		}
		return nil
	})
	if err != nil {
		return protocol.Progress{}, err
	}
	if changed {
		s.log.Info("saved the progress", "user_id", userID)
		s.publishMe(ctx, userID)
	}
	return toProgress(next), nil
}

// applyProgressUpdate sets what the request names on a copy of the row. A status the row already
// has changes nothing, so finishing twice keeps the first time.
func applyProgressUpdate(row db.UserProgress, in protocol.UpdateProgressRequest, now int64) db.UserProgress {
	if o := in.Onboarding; o != nil {
		if o.Step != nil {
			row.OnboardingStep = int64(*o.Step)
		}
		if o.Status != nil {
			row.OnboardingDoneAt, row.OnboardingSkipped = withStatus(
				row.OnboardingDoneAt, row.OnboardingSkipped, *o.Status, now)
		}
	}
	if t := in.Tutorial; t != nil && t.Status != nil {
		row.TutorialDoneAt, row.TutorialSkipped = withStatus(row.TutorialDoneAt, row.TutorialSkipped, *t.Status, now)
	}
	return row
}

// withStatus gives the finished time and the skipped flag that stand for a status. A status the
// pair already stands for is returned as it is.
func withStatus(doneAt *int64, skipped int64, status protocol.ProgressStatus, now int64) (*int64, int64) {
	if statusOf(doneAt, skipped) == status {
		return doneAt, skipped
	}
	switch status {
	case protocol.ProgressStatusDone:
		return &now, 0
	case protocol.ProgressStatusSkipped:
		return &now, 1
	default:
		return nil, 0
	}
}

// statusOf is the status a finished time and a skipped flag stand for.
func statusOf(doneAt *int64, skipped int64) protocol.ProgressStatus {
	switch {
	case doneAt == nil:
		return protocol.ProgressStatusPending
	case skipped == 1:
		return protocol.ProgressStatusSkipped
	default:
		return protocol.ProgressStatusDone
	}
}

// checkProgressUpdate refuses a progress change that is not allowed, before the store is touched.
func checkProgressUpdate(in protocol.UpdateProgressRequest) error {
	if in.Onboarding != nil {
		if err := checkOnboardingUpdate(*in.Onboarding); err != nil {
			return err
		}
	}
	if in.Tutorial != nil && in.Tutorial.Status != nil {
		return checkProgressStatus(*in.Tutorial.Status)
	}
	return nil
}

func checkOnboardingUpdate(in protocol.UpdateOnboardingProgress) error {
	if in.Step != nil && (*in.Step < 0 || *in.Step > maxOnboardingStep) {
		return protocol.InvalidArgument(fmt.Sprintf(
			"Onboarding has no screen %d. Use a screen from 0 to %d.", *in.Step, maxOnboardingStep))
	}
	if in.Status != nil {
		return checkProgressStatus(*in.Status)
	}
	return nil
}

func checkProgressStatus(status protocol.ProgressStatus) error {
	if !status.Valid() {
		return protocol.InvalidArgument("That is not a status Marshal knows. Use pending, done, or skipped.").
			With("status", string(status))
	}
	return nil
}

// readProgress reads a person's progress. A person with no row has both parts pending.
func readProgress(ctx context.Context, q *db.Queries, userID string) (protocol.Progress, error) {
	row, err := readProgressRow(ctx, q, userID)
	if err != nil {
		return protocol.Progress{}, err
	}
	return toProgress(row), nil
}

// readProgressRow reads the progress row, and answers the row of a person who has not started
// anything when there is none.
func readProgressRow(ctx context.Context, q *db.Queries, userID string) (db.UserProgress, error) {
	row, err := q.GetUserProgress(ctx, userID)
	switch {
	case err == nil:
		return row, nil
	case store.IsNotFound(err):
		return db.UserProgress{UserID: userID}, nil
	}
	return db.UserProgress{}, fmt.Errorf("read the progress of user %s: %w", userID, err)
}

// toProgress builds the wire progress from a row.
func toProgress(row db.UserProgress) protocol.Progress {
	return protocol.Progress{
		Onboarding: protocol.OnboardingProgress{
			Status:     statusOf(row.OnboardingDoneAt, row.OnboardingSkipped),
			Step:       int(row.OnboardingStep),
			FinishedAt: store.OptionalTimestamp(row.OnboardingDoneAt),
		},
		Tutorial: protocol.TutorialProgress{
			Status:     statusOf(row.TutorialDoneAt, row.TutorialSkipped),
			FinishedAt: store.OptionalTimestamp(row.TutorialDoneAt),
		},
	}
}
