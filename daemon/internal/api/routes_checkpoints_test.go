package api_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The checkpoint routes (docs/backend-checklist.md B5.3, build-plan 5.21, docs/architecture.md
// 11.3): GET a card's restore points, and POST a restore of one. The restore route is the one the
// architecture doc names verbatim. There is no mock for either, so these tests are the contract.

// seedCheckpoint stores a restore point for a card the way the session manager does when it makes
// one, so a route test does not need an agent to have run.
func seedCheckpoint(t *testing.T, st *stack, cardID, id, sha, label string) {
	t.Helper()
	now := time.Now().UTC()
	err := st.store.Write(context.Background(), func(q *db.Queries) error {
		return q.InsertCheckpoint(context.Background(), db.InsertCheckpointParams{
			ID: id, CardID: cardID, GitRef: sha, Label: label, CreatedAt: now.UnixMilli(),
		})
	})
	if err != nil {
		t.Fatalf("store a checkpoint for card %s: %v", cardID, err)
	}
}

// The list route answers a card's restore points newest first, with the commit and the label, and an
// empty list for a card that has none, so a panel can always draw it.
func TestCheckpointListAnswersNewestFirst(t *testing.T) {
	st := newStack(t)
	project, repo := st.addProject("small-repo")
	card := st.addCard(project.ID, "Give it restore points")
	dir := st.startWorktree(t, project, repo, card)
	head := headOf(t, st, dir)

	seedCheckpoint(t, st, card.ID, "01M3C107JB041061050R3GG28B", head, "before turn 1")
	time.Sleep(2 * time.Millisecond) // distinct created_at, so newest-first is unambiguous
	seedCheckpoint(t, st, card.ID, "01M3C107JB041061050R3GG28C", head, "before turn 2")

	list := decode[protocol.CheckpointList](t,
		st.do(http.MethodGet, "/v1/cards/"+card.ID+"/checkpoints", nil).want(t, http.StatusOK))
	if list.CardID != card.ID {
		t.Errorf("the list is for card %q, want %s", list.CardID, card.ID)
	}
	if len(list.Checkpoints) != 2 {
		t.Fatalf("the list = %+v, want two restore points", list.Checkpoints)
	}
	if list.Checkpoints[0].Label != "before turn 2" || list.Checkpoints[1].Label != "before turn 1" {
		t.Errorf("the list is %+v, want the newest first", labelsOf(list.Checkpoints))
	}
	for _, cp := range list.Checkpoints {
		if cp.CardID != card.ID || cp.SHA == "" || cp.CreatedAt.Time().IsZero() {
			t.Errorf("restore point %+v is missing a field a screen needs", cp)
		}
	}
	if list.ServerTime.Time().IsZero() {
		t.Error("the list carries no server time")
	}

	// A card with nothing to restore answers an empty list, never null.
	other := st.addCard(project.ID, "Never started")
	empty := decode[protocol.CheckpointList](t,
		st.do(http.MethodGet, "/v1/cards/"+other.ID+"/checkpoints", nil).want(t, http.StatusOK))
	if empty.Checkpoints == nil {
		t.Error("a card with no restore points answers null, want an empty list")
	}
	if len(empty.Checkpoints) != 0 {
		t.Errorf("a card with no restore points answers %+v", empty.Checkpoints)
	}
}

// The list of a card that is not there, or an id that cannot be one, is not found.
func TestCheckpointListRefusesWhatItCannotAnswer(t *testing.T) {
	st := newStack(t)
	st.do(http.MethodGet, "/v1/cards/"+sampleCardID+"/checkpoints", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodGet, "/v1/cards/not-an-id/checkpoints", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}

// Restoring a card to one of its restore points puts the worktree back and answers the card: the
// work of a later turn is gone, and the answer is the card as it now stands.
func TestRestorePutsTheCardBack(t *testing.T) {
	st := newStack(t)
	project, repo := st.addProject("small-repo")
	card := st.addCard(project.ID, "Put this one back")
	dir := st.startWorktree(t, project, repo, card)
	head := headOf(t, st, dir)
	const checkpointID = "01M3C107JB041061050R3GG28B"
	seedCheckpoint(t, st, card.ID, checkpointID, head, "before turn 1")

	// Work that came after the restore point, which a restore must remove.
	stray := filepath.Join(dir, "left-over.txt")
	if err := os.WriteFile(stray, []byte("work that came later\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	after := decode[protocol.Card](t,
		st.do(http.MethodPost, "/v1/cards/"+card.ID+"/checkpoints/"+checkpointID+"/restore", nil).
			want(t, http.StatusOK))
	if after.ID != card.ID {
		t.Errorf("the restore answered card %s, want %s", after.ID, card.ID)
	}
	if _, err := os.Stat(stray); !os.IsNotExist(err) {
		t.Errorf("the file the restore point does not hold is still there (err = %v)", err)
	}

	// The body is optional: the same route with a body that asks for the conversation too is also
	// accepted, and records the restore point as the boundary the conversation is restored from.
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/checkpoints/"+checkpointID+"/restore",
		protocol.RestoreCheckpointRequest{Conversation: true}).want(t, http.StatusOK)
}

// A restore is refused for a card that has no worktree, and a body that is not JSON is a bad request
// rather than a restore of the whole worktree by accident.
func TestRestoreRefusesWhatItCannotDo(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "No worktree at all")
	const checkpointID = "01M3C107JB041061050R3GG28B"
	seedCheckpoint(t, st, card.ID, checkpointID, "0123456789012345678901234567890123456789", "before turn 1")

	// A card with no worktree cannot be restored, with or without a body.
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/checkpoints/"+checkpointID+"/restore", nil).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/checkpoints/"+checkpointID+"/restore",
		protocol.RestoreCheckpointRequest{Conversation: true}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)

	// A body that is not JSON is refused by name.
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/checkpoints/"+checkpointID+"/restore", "not an object").
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/checkpoints/"+checkpointID+"/restore",
		map[string]any{"nonsense": true}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
}

// A restore point that is not on the card, an id that cannot be one, and a card that is not there
// are all not found, and the two id cases are not told apart.
func TestRestoreRefusesIdsItCannotFind(t *testing.T) {
	st := newStack(t)
	project, repo := st.addProject("small-repo")
	card := st.addCard(project.ID, "The card with the point")
	dir := st.startWorktree(t, project, repo, card)
	const checkpointID = "01M3C107JB041061050R3GG28B"
	seedCheckpoint(t, st, card.ID, checkpointID, headOf(t, st, dir), "before turn 1")
	other := st.addCard(project.ID, "Another card")

	// The restore point belongs to the first card, so the second cannot be restored to it.
	st.do(http.MethodPost, "/v1/cards/"+other.ID+"/checkpoints/"+checkpointID+"/restore", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	// An id that cannot be a restore point's is not found without asking the service.
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/checkpoints/not-an-id/restore", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	// A card that is not there is not found.
	st.do(http.MethodPost, "/v1/cards/"+sampleCardID+"/checkpoints/"+checkpointID+"/restore", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodPost, "/v1/cards/not-an-id/checkpoints/"+checkpointID+"/restore", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}

// headOf is the commit a worktree's branch is on, which is what a seeded restore point names.
func headOf(t *testing.T, st *stack, dir string) string {
	t.Helper()
	sha, err := st.git.Run(context.Background(), dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("read the head of %s: %v", dir, err)
	}
	return sha
}

// labelsOf are the labels of a list of restore points, for an error message.
func labelsOf(checkpoints []protocol.Checkpoint) []string {
	out := make([]string, 0, len(checkpoints))
	for _, cp := range checkpoints {
		out = append(out, cp.Label)
	}
	return out
}
