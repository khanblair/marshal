package projects_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Deleting a card removes everything Marshal made for it: the worktree, the branch, the logs, and
// the row. It never touches the repository folder.
func TestDeleteCardRemovesEverything(t *testing.T) {
	log := &calls{}
	e := newEnv(t, projects.WithSessionStopper(fakeSessions{log: log}))
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Delete me")
	path, branch := e.startCard(t, project, card)
	e.drainEvents()

	if err := e.svc.DeleteCard(context.Background(), card.ID); err != nil {
		t.Fatalf("DeleteCard: %v", err)
	}
	if fi, err := os.Stat(path); err == nil {
		t.Errorf("the worktree is still there: %v", fi.Name())
	}
	if found, err := e.git.BranchExists(context.Background(), project.Path, branch); err != nil || found {
		t.Errorf("the branch %s is still there (%v)", branch, err)
	}
	if _, err := e.svc.Card(context.Background(), card.ID); err == nil {
		t.Error("the card is still there")
	}
	// The repository folder is untouched.
	for _, name := range []string{".git", "README.md"} {
		if _, err := os.Stat(filepath.Join(project.Path, name)); err != nil {
			t.Errorf("the repository lost %s: %v", name, err)
		}
	}
	// The session was stopped first, then its logs were removed, then the card went.
	want := []string{"stop the session of " + card.ID, "remove the logs of " + card.ID}
	if got := log.get(); !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
	ev := e.nextType(t, protocol.EventTypeCardDeleted, protocol.ProjectTopic(project.ID))
	data, ok := ev.Data.(protocol.CardDeletedEventData)
	if !ok || data.CardID != card.ID || data.ProjectID != project.ID {
		t.Errorf("event = %+v, want the card that is gone", ev.Data)
	}
	if !ev.Critical {
		t.Error("card.deleted must be critical: a client that missed it would draw a card that is gone")
	}
}

// A card that never started has no worktree and no branch, and is still deleted.
func TestDeleteCardThatNeverStarted(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Never started")
	e.drainEvents()

	if err := e.svc.DeleteCard(context.Background(), card.ID); err != nil {
		t.Fatalf("DeleteCard: %v", err)
	}
	if _, err := e.svc.Card(context.Background(), card.ID); err == nil {
		t.Error("the card is still there")
	}
	e.nextType(t, protocol.EventTypeCardDeleted, protocol.ProjectTopic(project.ID))
}

// Deleting a card that does not exist is not found, and nothing is stopped.
func TestDeleteCardOfAnUnknownCard(t *testing.T) {
	log := &calls{}
	e := newEnv(t, projects.WithSessionStopper(fakeSessions{log: log}))

	err := e.svc.DeleteCard(context.Background(), "01M3C107JB041061050R3GG28A")
	perr, ok := err.(*protocol.Error)
	if !ok || perr.Code != protocol.ErrorCodeNotFound {
		t.Fatalf("DeleteCard = %v, want not_found", err)
	}
	if got := log.get(); len(got) != 0 {
		t.Errorf("calls = %v, want none: nothing should be stopped for a card that is not there", got)
	}
}

// When the agent will not stop, the card stays: nothing is removed under a process that is still
// running, and the person can try again.
func TestDeleteCardStopsWhenTheSessionWillNotStop(t *testing.T) {
	log := &calls{}
	e := newEnv(t, projects.WithSessionStopper(fakeSessions{log: log, err: errors.New("agent will not stop")}))
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Stubborn")
	path, _ := e.startCard(t, project, card)

	err := e.svc.DeleteCard(context.Background(), card.ID)
	perr, ok := err.(*protocol.Error)
	if !ok || perr.Code != protocol.ErrorCodeUnavailable {
		t.Fatalf("DeleteCard = %v, want unavailable", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the worktree was removed even though the session did not stop: %v", err)
	}
	if _, err := e.svc.Card(context.Background(), card.ID); err != nil {
		t.Error("the card was removed even though its session did not stop")
	}
	if got := log.get(); !slices.Equal(got, []string{"stop the session of " + card.ID}) {
		t.Errorf("calls = %v, want only the stop attempt", got)
	}
}

// Deleting one card leaves the project's other cards, their worktrees, and their branches.
func TestDeleteCardLeavesTheOtherCards(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	doomed := e.card(t, project.ID, "Doomed")
	kept := e.card(t, project.ID, "Kept")
	doomedPath, doomedBranch := e.startCard(t, project, doomed)
	keptPath, keptBranch := e.startCard(t, project, kept)

	if err := e.svc.DeleteCard(context.Background(), doomed.ID); err != nil {
		t.Fatalf("DeleteCard: %v", err)
	}
	if _, err := e.svc.Card(context.Background(), kept.ID); err != nil {
		t.Errorf("the other card went with it: %v", err)
	}
	if _, err := os.Stat(keptPath); err != nil {
		t.Errorf("the other card's worktree went with it: %v", err)
	}
	if found, err := e.git.BranchExists(context.Background(), project.Path, keptBranch); err != nil || !found {
		t.Errorf("the other card's branch went with it (%v, %v)", found, err)
	}
	if _, err := os.Stat(doomedPath); err == nil {
		t.Error("the deleted card's worktree is still there")
	}
	if found, err := e.git.BranchExists(context.Background(), project.Path, doomedBranch); err != nil || found {
		t.Errorf("the deleted card's branch is still there (%v, %v)", found, err)
	}
	// The card numbers of the cards that are left do not change.
	if kept.Number != 2 {
		t.Errorf("the kept card is number %d, want 2", kept.Number)
	}
}

// A card is deleted even when the repository folder is gone: Marshal removes its own worktree
// folder without asking Git.
func TestDeleteCardWhenTheRepositoryIsGone(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Orphan")
	path, _ := e.startCard(t, project, card)
	if err := os.RemoveAll(project.Path); err != nil {
		t.Fatalf("remove the repository: %v", err)
	}

	if err := e.svc.DeleteCard(context.Background(), card.ID); err != nil {
		t.Fatalf("DeleteCard: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("the worktree folder is still there")
	}
	if _, err := e.svc.Card(context.Background(), card.ID); err == nil {
		t.Error("the card is still there")
	}
}
