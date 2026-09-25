package session_test

import (
	"context"
	"errors"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
)

func TestAwakeCardsCountsOnlyTheProjectsOwnLiveSessions(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	ctx := context.Background()
	first := e.project(t, "small-repo")
	second := e.project(t, "small-repo")
	for _, p := range []protocol.Project{first, first, second} {
		card := e.card(t, p.ID, "A card")
		if _, err := e.mgr.Start(ctx, card.ID); err != nil {
			t.Fatalf("Start: %v", err)
		}
	}
	for project, want := range map[string]int{first.ID: 2, second.ID: 1, "nobody": 0} {
		got, err := e.mgr.AwakeCards(ctx, project)
		if err != nil || got != want {
			t.Errorf("AwakeCards(%q) = %d, %v, want %d", project, got, err, want)
		}
	}
}

func TestStopProjectSessionsStopsThatProjectAndNoOther(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	ctx := context.Background()
	doomed := e.project(t, "small-repo")
	kept := e.project(t, "small-repo")
	doomedCard := e.card(t, doomed.ID, "Doomed")
	keptCard := e.card(t, kept.ID, "Kept")
	for _, id := range []string{doomedCard.ID, keptCard.ID} {
		if _, err := e.mgr.Start(ctx, id); err != nil {
			t.Fatalf("Start: %v", err)
		}
	}

	if err := e.mgr.StopProjectSessions(ctx, doomed.ID); err != nil {
		t.Fatalf("StopProjectSessions: %v", err)
	}

	err := e.mgr.Send(ctx, doomedCard.ID, "anyone there?")
	if !errors.Is(err, session.ErrNoLiveSession) {
		t.Errorf("Send to a stopped project's card = %v, want no live session", err)
	}
	if err := e.mgr.Send(ctx, keptCard.ID, "still there?"); err != nil {
		t.Errorf("Send to the other project's card: %v", err)
	}
	if got, _ := e.mgr.AwakeCards(ctx, doomed.ID); got != 0 {
		t.Errorf("AwakeCards after stopping = %d, want 0", got)
	}
	// Stopping a project with nothing running is not an error.
	if err := e.mgr.StopProjectSessions(ctx, doomed.ID); err != nil {
		t.Errorf("StopProjectSessions with nothing running: %v", err)
	}
}

func TestACardCanBeStartedAgainAfterItsAgentFailedToStart(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	ctx := context.Background()
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	e.agent.startErr = errors.New("the agent program is missing")
	if _, err := e.mgr.Start(ctx, card.ID); err == nil {
		t.Fatal("Start with a failing agent should return an error")
	}
	branches, err := e.git.Branches(ctx, project.Path, "marshal/")
	if err != nil || len(branches) != 0 {
		t.Fatalf("branches after the failed start = %v, %v, want none: the branch is undone with the worktree", branches, err)
	}

	e.agent.startErr = nil
	if _, err := e.mgr.Start(ctx, card.ID); err != nil {
		t.Fatalf("Start again once the agent works: %v", err)
	}
}
