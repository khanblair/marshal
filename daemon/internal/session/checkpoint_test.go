package session_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// A card's restore points (B5.3, build-plan 5.21, docs/architecture.md 10): Marshal makes one before
// every agent turn, the list is read newest first, and restoring one puts the card's worktree and
// branch back to it. There is no prototype for this - no mock screen makes or restores a checkpoint -
// so these tests are the specification of the semantics the report records as a ruling.

// worktreeOf is a card's worktree, which is where its restore points are made.
func worktreeOf(t *testing.T, e *env, cardID string) string {
	t.Helper()
	row, err := e.store.Queries().GetCard(context.Background(), cardID)
	if err != nil {
		t.Fatalf("read the session of card %s: %v", cardID, err)
	}
	if row.WorktreePath == "" {
		t.Fatalf("card %s has no worktree", cardID)
	}
	return row.WorktreePath
}

// checkpointRows are a card's stored restore points, newest first.
func checkpointRows(t *testing.T, e *env, cardID string) []db.Checkpoint {
	t.Helper()
	rows, err := e.store.Queries().ListCheckpointsForCard(context.Background(),
		db.ListCheckpointsForCardParams{CardID: cardID, Limit: -1})
	if err != nil {
		t.Fatalf("read the checkpoints of card %s: %v", cardID, err)
	}
	return rows
}

// waitForCheckpoints waits until a card has at least this many stored restore points.
func waitForCheckpoints(t *testing.T, e *env, cardID string, count int) {
	t.Helper()
	waitFor(t, fmt.Sprintf("card %s to have %d restore points", cardID, count), func() bool {
		return len(checkpointRows(t, e, cardID)) >= count
	})
}

// waitForIdle waits until a card's session is awake again after a turn. A turn's history is written
// before the pump lets the session go, so a restore asked for straight after a turn whose answer was
// stored is still refused as running: this waits for the moment the card is actually free.
func waitForIdle(t *testing.T, e *env, cardID string) {
	t.Helper()
	e.untilState(t, cardID, protocol.SessionStateAwake)
}

// A restore point is made before every turn, it captures whatever the worktree held, and the list
// reads newest first. The label says which turn it is the point before.
func TestACardGetsARestorePointBeforeEveryTurn(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := startCard(t, e, e.card(t, project.ID, "Make a restore point"))

	if err := e.mgr.Send(context.Background(), card.ID, "the first turn"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitForHistory(t, e, card.ID, 2)
	waitForIdle(t, e, card.ID)
	waitForCheckpoints(t, e, card.ID, 1)

	// The work the turn did, left in the worktree: the next restore point must capture it.
	worktree := worktreeOf(t, e, card.ID)
	writeFile(t, filepath.Join(worktree, "work-1.txt"), "the first turn's work\n")

	if err := e.mgr.Send(context.Background(), card.ID, "the second turn"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitForHistory(t, e, card.ID, 4)
	waitForIdle(t, e, card.ID)
	waitForCheckpoints(t, e, card.ID, 2)

	list, err := e.mgr.ListCheckpoints(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("ListCheckpoints: %v", err)
	}
	if list.CardID != card.ID || len(list.Checkpoints) != 2 {
		t.Fatalf("the list = %+v, want two restore points for card %s", list, card.ID)
	}
	newest, oldest := list.Checkpoints[0], list.Checkpoints[1]
	if newest.Label != "before turn 2" || oldest.Label != "before turn 1" {
		t.Errorf("labels = %q and %q, want the newest first", newest.Label, oldest.Label)
	}
	if newest.SHA == "" || oldest.SHA == "" {
		t.Error("a restore point has no commit for it")
	}
	if newest.SHA == oldest.SHA {
		t.Error("the second restore point did not capture the work the first turn left behind")
	}
	// The commit is real and belongs to this worktree, so a later restore can reset to it.
	for _, cp := range list.Checkpoints {
		if _, err := e.git.Run(context.Background(), worktree, "cat-file", "-e", cp.SHA+"^{commit}"); err != nil {
			t.Errorf("restore point %s names commit %s, which is not in the worktree: %v", cp.Label, cp.SHA, err)
		}
	}
	// The work the first turn left is committed by the second restore point, not left loose.
	if out, err := e.git.Run(context.Background(), worktree, "status", "--porcelain"); err != nil {
		t.Fatalf("git status: %v", err)
	} else if strings.TrimSpace(out) != "" {
		t.Errorf("the worktree is still dirty after a restore point: %q", out)
	}
}

// A card that never started has no worktree, so it has no restore points: the list is empty and not
// an error, because a card panel asks for it before anything has run.
func TestACardThatNeverStartedHasNoRestorePoints(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Never started")

	list, err := e.mgr.ListCheckpoints(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("ListCheckpoints: %v", err)
	}
	if len(list.Checkpoints) != 0 {
		t.Errorf("a card that never started has %d restore points, want none", len(list.Checkpoints))
	}
	if list.Checkpoints == nil {
		t.Error("the list is null, want an empty list a screen can draw")
	}
}

// The list of a card that is not there is not found, the same as asking for the card itself.
func TestTheRestorePointsOfACardThatIsNotThereAreNotFound(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })

	_, err := e.mgr.ListCheckpoints(context.Background(), "01M3C107JB041061050R3GG28A")
	var apiErr *protocol.Error
	if !errors.As(err, &apiErr) || apiErr.Code != protocol.ErrorCodeNotFound {
		t.Fatalf("err = %v, want not found", err)
	}
}

// Restoring puts the worktree back to the restore point: the files it holds come back, the work the
// restore point does not hold is removed, and the moment is written to the card's own history and
// the audit log.
func TestRestoringARestorePointPutsTheWorktreeBack(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := startCard(t, e, e.card(t, project.ID, "Put it back"))

	if err := e.mgr.Send(context.Background(), card.ID, "the first turn"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitForHistory(t, e, card.ID, 2)
	waitForIdle(t, e, card.ID)
	waitForCheckpoints(t, e, card.ID, 1)

	list, err := e.mgr.ListCheckpoints(context.Background(), card.ID)
	if err != nil || len(list.Checkpoints) == 0 {
		t.Fatalf("ListCheckpoints = %+v, %v", list, err)
	}
	restorePoint := list.Checkpoints[0]

	// Work the restore point does not hold: a new file the agent left behind.
	worktree := worktreeOf(t, e, card.ID)
	leftover := filepath.Join(worktree, "left-over.txt")
	writeFile(t, leftover, "work that came later\n")

	restored, err := e.mgr.RestoreCheckpoint(context.Background(), card.ID, restorePoint.ID, false)
	if err != nil {
		t.Fatalf("RestoreCheckpoint: %v", err)
	}
	if restored.ID != card.ID {
		t.Errorf("the restore answered card %s, want %s", restored.ID, card.ID)
	}
	if _, err := os.Stat(leftover); !os.IsNotExist(err) {
		t.Errorf("the file the restore point does not hold is still there (err = %v)", err)
	}
	if out, err := e.git.Run(context.Background(), worktree, "status", "--porcelain"); err != nil {
		t.Fatalf("git status: %v", err)
	} else if strings.TrimSpace(out) != "" {
		t.Errorf("the worktree is dirty after a restore: %q", out)
	}

	// The restore is accountable and readable from the card itself.
	if n := countAction(t, e, audit.ActionCheckpointRestored); n != 1 {
		t.Errorf("the audit log has %d restore rows, want one", n)
	}
	notes := systemNotes(t, e, card.ID)
	if len(notes) != 1 || !strings.Contains(notes[0].Summary, "Restored this card") {
		t.Errorf("the card's notes = %+v, want the restore recorded", summaries(notes))
	}
}

// A restore point belongs to the card it was made for: one card cannot be restored to another's.
func TestARestorePointOfAnotherCardIsNotFound(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := startCard(t, e, e.card(t, project.ID, "The card with the points"))
	other := startCard(t, e, e.card(t, project.ID, "Another card"))

	if err := e.mgr.Send(context.Background(), card.ID, "the first turn"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitForHistory(t, e, card.ID, 2)
	waitForIdle(t, e, card.ID)
	waitForCheckpoints(t, e, card.ID, 1)

	list, err := e.mgr.ListCheckpoints(context.Background(), card.ID)
	if err != nil || len(list.Checkpoints) == 0 {
		t.Fatalf("ListCheckpoints = %+v, %v", list, err)
	}
	_, err = e.mgr.RestoreCheckpoint(context.Background(), other.ID, list.Checkpoints[0].ID, false)
	var apiErr *protocol.Error
	if !errors.As(err, &apiErr) || apiErr.Code != protocol.ErrorCodeNotFound {
		t.Fatalf("err = %v, want not found for another card's restore point", err)
	}
}

// A restore while the agent is mid-turn is refused, because it would race the writes the turn is
// making. The refusal names why, so a client can tell the person to wait rather than retry blindly.
func TestARestoreIsRefusedWhileATurnIsRunning(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := startCard(t, e, e.card(t, project.ID, "Busy card"))

	// One turn to completion, so there is a restore point to ask for.
	if err := e.mgr.Send(context.Background(), card.ID, "the first turn"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitForHistory(t, e, card.ID, 2)
	waitForIdle(t, e, card.ID)
	waitForCheckpoints(t, e, card.ID, 1)
	list, err := e.mgr.ListCheckpoints(context.Background(), card.ID)
	if err != nil || len(list.Checkpoints) == 0 {
		t.Fatalf("ListCheckpoints = %+v, %v", list, err)
	}
	restorePoint := list.Checkpoints[0]

	// A turn held open in the agent, so the card's session is busy when the restore is asked for.
	e.agent.hold = make(chan struct{})
	t.Cleanup(func() { close(e.agent.hold) })
	if err := e.mgr.Send(context.Background(), card.ID, "a turn that will not end"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	e.untilState(t, card.ID, protocol.SessionStateWorking)

	_, err = e.mgr.RestoreCheckpoint(context.Background(), card.ID, restorePoint.ID, false)
	var apiErr *protocol.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want a refusal while the turn is running", err)
	}
	if apiErr.Code != protocol.ErrorCodeRefused {
		t.Errorf("code = %q, want refused", apiErr.Code)
	}
	if apiErr.Details["reason"] != "restore_turn_running" {
		t.Errorf("details = %+v, want reason restore_turn_running", apiErr.Details)
	}
}

// Restoring the conversation as well records the restore point as the boundary the conversation is
// restored from, and says plainly that the agent's own context is not rewound.
func TestRestoringTheConversationRecordsTheBoundary(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := startCard(t, e, e.card(t, project.ID, "Restore the chat too"))

	if err := e.mgr.Send(context.Background(), card.ID, "the first turn"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitForHistory(t, e, card.ID, 2)
	waitForIdle(t, e, card.ID)
	waitForCheckpoints(t, e, card.ID, 1)
	list, err := e.mgr.ListCheckpoints(context.Background(), card.ID)
	if err != nil || len(list.Checkpoints) == 0 {
		t.Fatalf("ListCheckpoints = %+v, %v", list, err)
	}

	if _, err := e.mgr.RestoreCheckpoint(context.Background(), card.ID, list.Checkpoints[0].ID, true); err != nil {
		t.Fatalf("RestoreCheckpoint: %v", err)
	}
	notes := systemNotes(t, e, card.ID)
	if len(notes) != 1 {
		t.Fatalf("the card's notes = %+v, want one", summaries(notes))
	}
	note := notes[0].Summary
	if !strings.Contains(note, "conversation is restored from here") {
		t.Errorf("note = %q, want the restore boundary recorded", note)
	}
	if !strings.Contains(note, "not rewound") {
		t.Errorf("note = %q, want it to say the agent's own context is not rewound", note)
	}
}

// A card keeps its newest twenty restore points: one is made before every turn, so a card stopped
// and resumed many times would otherwise grow a list with no end.
func TestOnlyTheNewestTwentyRestorePointsAreKept(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := startCard(t, e, e.card(t, project.ID, "Long runner"))

	const turns = 21
	for i := 1; i <= turns; i++ {
		if err := e.mgr.Send(context.Background(), card.ID, fmt.Sprintf("turn %d", i)); err != nil {
			t.Fatalf("Send turn %d: %v", i, err)
		}
		waitForHistory(t, e, card.ID, 2*i)
		waitForIdle(t, e, card.ID)
	}
	waitForCheckpoints(t, e, card.ID, 1)

	list, err := e.mgr.ListCheckpoints(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("ListCheckpoints: %v", err)
	}
	if len(list.Checkpoints) != 20 {
		t.Fatalf("the card keeps %d restore points, want 20", len(list.Checkpoints))
	}
	if newest := list.Checkpoints[0].Label; newest != "before turn 21" {
		t.Errorf("the newest restore point is %q, want the one before the last turn", newest)
	}
	if oldest := list.Checkpoints[len(list.Checkpoints)-1].Label; oldest != "before turn 2" {
		t.Errorf("the oldest kept restore point is %q, want the second turn's", oldest)
	}
	if rows := checkpointRows(t, e, card.ID); len(rows) != 20 {
		t.Errorf("the store holds %d rows, want the same twenty", len(rows))
	}
}
