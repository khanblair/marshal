package accounts_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A person who has not started anything has onboarding on its first screen and the tour to come, and
// nothing is stored for them yet.
func TestProgressOfANewPerson(t *testing.T) {
	e := newEnv(t)
	got, err := e.svc.Progress(context.Background(), e.userID)
	if err != nil {
		t.Fatalf("Progress: %v", err)
	}
	want := protocol.Progress{
		Onboarding: protocol.OnboardingProgress{Status: protocol.ProgressStatusPending},
		Tutorial:   protocol.TutorialProgress{Status: protocol.ProgressStatusPending},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("progress = %+v, want %+v", got, want)
	}
}

// Onboarding can be left and resumed: the screen is saved as it changes, finishing stamps the time,
// and every change is published with the progress as it now is.
func TestOnboardingIsSavedAndResumed(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	got, err := e.svc.UpdateProgress(ctx, e.userID, protocol.UpdateProgressRequest{
		Onboarding: &protocol.UpdateOnboardingProgress{Step: ptr(2)},
	})
	if err != nil || got.Onboarding.Step != 2 || got.Onboarding.Status != protocol.ProgressStatusPending || got.Onboarding.FinishedAt != nil {
		t.Fatalf("saving the step = %+v, %v", got, err)
	}
	if me := e.nextMe(t); me.Progress.Onboarding.Step != 2 {
		t.Errorf("the event's step = %d, want 2", me.Progress.Onboarding.Step)
	}

	// The daemon starts again, and onboarding resumes where it was.
	e.restart(t)
	if resumed, err := e.svc.Progress(ctx, e.userID); err != nil || resumed.Onboarding.Step != 2 {
		t.Fatalf("after a restart the progress = %+v, %v; want step 2", resumed, err)
	}

	done, err := e.svc.UpdateProgress(ctx, e.userID, protocol.UpdateProgressRequest{
		Onboarding: &protocol.UpdateOnboardingProgress{Step: ptr(3), Status: ptr(protocol.ProgressStatusDone)},
	})
	if err != nil {
		t.Fatalf("finishing: %v", err)
	}
	if done.Onboarding.Status != protocol.ProgressStatusDone || done.Onboarding.Step != 3 || done.Onboarding.FinishedAt == nil {
		t.Fatalf("progress = %+v, want done at step 3 with a time", done)
	}
	e.nextMe(t)

	// Finishing again changes nothing, and keeps the first time.
	again, err := e.svc.UpdateProgress(ctx, e.userID, protocol.UpdateProgressRequest{
		Onboarding: &protocol.UpdateOnboardingProgress{Status: ptr(protocol.ProgressStatusDone)},
	})
	if err != nil || !reflect.DeepEqual(again, done) {
		t.Errorf("finishing twice = %+v, %v; want %+v", again, err, done)
	}
	e.noEvent(t)
}

// Skipping is told apart from finishing, and both stamp the time. Skipping the tour and replaying
// it sets it back to pending with no time.
func TestSkippingAndReplaying(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	skipped, err := e.svc.UpdateProgress(ctx, e.userID, protocol.UpdateProgressRequest{
		Onboarding: &protocol.UpdateOnboardingProgress{Status: ptr(protocol.ProgressStatusSkipped)},
		Tutorial:   &protocol.UpdateTutorialProgress{Status: ptr(protocol.ProgressStatusSkipped)},
	})
	if err != nil {
		t.Fatalf("skipping: %v", err)
	}
	if skipped.Onboarding.Status != protocol.ProgressStatusSkipped || skipped.Onboarding.FinishedAt == nil ||
		skipped.Tutorial.Status != protocol.ProgressStatusSkipped || skipped.Tutorial.FinishedAt == nil {
		t.Fatalf("progress = %+v, want both skipped, with times", skipped)
	}
	e.nextMe(t)

	// A skipped tour that is finished on a later replay is done, with a new time.
	replayed, err := e.svc.UpdateProgress(ctx, e.userID, protocol.UpdateProgressRequest{
		Tutorial: &protocol.UpdateTutorialProgress{Status: ptr(protocol.ProgressStatusPending)},
	})
	if err != nil || replayed.Tutorial.Status != protocol.ProgressStatusPending || replayed.Tutorial.FinishedAt != nil {
		t.Fatalf("replaying = %+v, %v; want the tour pending with no time", replayed, err)
	}
	if !reflect.DeepEqual(replayed.Onboarding, skipped.Onboarding) {
		t.Errorf("replaying the tour changed onboarding: %+v, want %+v", replayed.Onboarding, skipped.Onboarding)
	}
	e.nextMe(t)
	finished, err := e.svc.UpdateProgress(ctx, e.userID, protocol.UpdateProgressRequest{
		Tutorial: &protocol.UpdateTutorialProgress{Status: ptr(protocol.ProgressStatusDone)},
	})
	if err != nil || finished.Tutorial.Status != protocol.ProgressStatusDone || finished.Tutorial.FinishedAt == nil {
		t.Fatalf("finishing the tour = %+v, %v", finished, err)
	}
	if !finished.Tutorial.FinishedAt.Time().After(skipped.Tutorial.FinishedAt.Time()) {
		t.Errorf("the tour's time did not move: %v, then %v", skipped.Tutorial.FinishedAt.Time(), finished.Tutorial.FinishedAt.Time())
	}
	// Skipping what was finished is a change, and it is remembered as a skip.
	if now, err := e.svc.UpdateProgress(ctx, e.userID, protocol.UpdateProgressRequest{
		Tutorial: &protocol.UpdateTutorialProgress{Status: ptr(protocol.ProgressStatusSkipped)},
	}); err != nil || now.Tutorial.Status != protocol.ProgressStatusSkipped {
		t.Errorf("skipping a finished tour = %+v, %v", now, err)
	}
}

// An empty request changes nothing and publishes nothing.
func TestAnEmptyProgressChange(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	before, _ := e.svc.Progress(ctx, e.userID)
	for name, in := range map[string]protocol.UpdateProgressRequest{
		"an empty body":               {},
		"an empty onboarding":         {Onboarding: &protocol.UpdateOnboardingProgress{}},
		"an empty tutorial":           {Tutorial: &protocol.UpdateTutorialProgress{}},
		"step 0 on a fresh person":    {Onboarding: &protocol.UpdateOnboardingProgress{Step: ptr(0)}},
		"pending on a fresh person":   {Onboarding: &protocol.UpdateOnboardingProgress{Status: ptr(protocol.ProgressStatusPending)}},
		"pending on a fresh tutorial": {Tutorial: &protocol.UpdateTutorialProgress{Status: ptr(protocol.ProgressStatusPending)}},
	} {
		got, err := e.svc.UpdateProgress(ctx, e.userID, in)
		if err != nil || !reflect.DeepEqual(got, before) {
			t.Errorf("%s: %+v, %v; want the progress unchanged", name, got, err)
		}
	}
	e.noEvent(t)
}

// Each refusal is a plain sentence and changes nothing.
func TestProgressRefusals(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	const badStatus = "That is not a status Marshal knows. Use pending, done, or skipped."
	tests := []struct {
		name string
		in   protocol.UpdateProgressRequest
		text string
	}{
		{"a negative screen", protocol.UpdateProgressRequest{Onboarding: &protocol.UpdateOnboardingProgress{Step: ptr(-1)}},
			"Onboarding has no screen -1. Use a screen from 0 to 9."},
		{"a screen past the last", protocol.UpdateProgressRequest{Onboarding: &protocol.UpdateOnboardingProgress{Step: ptr(10)}},
			"Onboarding has no screen 10. Use a screen from 0 to 9."},
		{"an onboarding status that is not one", protocol.UpdateProgressRequest{
			Onboarding: &protocol.UpdateOnboardingProgress{Status: ptr(protocol.ProgressStatus("started"))}}, badStatus},
		{"a tutorial status that is not one", protocol.UpdateProgressRequest{
			Tutorial: &protocol.UpdateTutorialProgress{Status: ptr(protocol.ProgressStatus(""))}}, badStatus},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.svc.UpdateProgress(ctx, e.userID, tc.in)
			wantCode(t, err, protocol.ErrorCodeInvalidArgument, tc.text)
		})
	}
	if got, _ := e.svc.Progress(ctx, e.userID); got.Onboarding.Step != 0 {
		t.Errorf("a refusal changed the progress: %+v", got)
	}
	e.noEvent(t)
}

// Progress belongs to a person: one person finishing onboarding does not finish it for another.
func TestProgressIsPerPerson(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	other := e.secondUser(t, "Sam Reyes")
	if _, err := e.svc.UpdateProgress(ctx, e.userID, protocol.UpdateProgressRequest{
		Onboarding: &protocol.UpdateOnboardingProgress{Step: ptr(2), Status: ptr(protocol.ProgressStatusDone)},
	}); err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}
	got, err := e.svc.Progress(ctx, other)
	if err != nil || got.Onboarding.Status != protocol.ProgressStatusPending || got.Onboarding.Step != 0 {
		t.Errorf("the other person's progress = %+v, %v; want a fresh one", got, err)
	}
}

// The dev reset puts both parts back as they are on first launch, and publishes it. Resetting what is
// already reset changes nothing.
func TestResetFirstLaunch(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.svc.UpdateProgress(ctx, e.userID, protocol.UpdateProgressRequest{
		Onboarding: &protocol.UpdateOnboardingProgress{Step: ptr(3), Status: ptr(protocol.ProgressStatusDone)},
		Tutorial:   &protocol.UpdateTutorialProgress{Status: ptr(protocol.ProgressStatusSkipped)},
	}); err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}
	e.nextMe(t)

	reset, err := e.svc.ResetFirstLaunch(ctx, e.userID)
	if err != nil {
		t.Fatalf("ResetFirstLaunch: %v", err)
	}
	fresh := protocol.Progress{
		Onboarding: protocol.OnboardingProgress{Status: protocol.ProgressStatusPending},
		Tutorial:   protocol.TutorialProgress{Status: protocol.ProgressStatusPending},
	}
	if !reflect.DeepEqual(reset, fresh) {
		t.Errorf("progress = %+v, want %+v", reset, fresh)
	}
	if me := e.nextMe(t); !reflect.DeepEqual(me.Progress, fresh) {
		t.Errorf("the event's progress = %+v, want %+v", me.Progress, fresh)
	}
	if _, err := e.svc.ResetFirstLaunch(ctx, e.userID); err != nil {
		t.Fatalf("ResetFirstLaunch again: %v", err)
	}
	e.noEvent(t)
}
