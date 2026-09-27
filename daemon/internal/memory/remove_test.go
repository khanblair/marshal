package memory_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Removing a project's memory (docs/architecture.md sections 12 and 16.1, docs/backend-checklist.md
// B7.4): the projects service asks for this through the `MemoryRemover` seam it already holds when a
// person removes a project and did not ask to keep what is remembered about it. Both halves go - the
// folder in the vault and the rows that index it - and both have to be right whichever order the
// projects service gets to them in, because the cards' own deletion already takes their note rows
// with them (migration 0019's `cards_notes_delete` trigger), and RemoveProjectMemory's own
// DeleteProjectNotes clears what that trigger does not reach: every lesson, and any note whose card
// this call runs before the cards are gone.

func TestRemoveProjectMemoryTakesBothHalvesAndLeavesTheOtherProjectAlone(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for _, id := range []string{cardID, sibling, awayCard} {
		if _, err := f.svc.SaveNote(ctx, id, "the widget report", protocol.NoteAuthorPerson); err != nil {
			t.Fatalf("save the note of %s: %v", id, err)
		}
	}

	if err := f.svc.RemoveProjectMemory(ctx, projectID); err != nil {
		t.Fatalf("remove the project's memory: %v", err)
	}

	// The folder is gone from the vault.
	if _, err := os.Stat(filepath.Join(f.root, projectID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the project's memory folder is still in the vault (stat: %v)", err)
	}
	// So are the rows that indexed it: a search of that project finds nothing, which proves the
	// indexed text went with the rows.
	found, err := f.svc.SearchNotes(ctx, projectID, "widget", 10)
	if err != nil {
		t.Fatalf("search the removed project: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("the removed project's notes are still searchable: %d matches", len(found))
	}
	if _, err := f.store.Queries().GetNote(ctx, db.GetNoteParams{ProjectID: projectID, CardID: cardID}); !store.IsNotFound(err) {
		t.Fatalf("the note row survived the removal: %v", err)
	}

	// The other project is untouched, both in the vault and in the index.
	if _, err := os.Stat(filepath.Join(f.root, otherProj)); err != nil {
		t.Fatalf("removing the memory of one project touched the other's folder: %v", err)
	}
	other, err := f.svc.SearchNotes(ctx, otherProj, "widget", 10)
	if err != nil {
		t.Fatalf("search the other project: %v", err)
	}
	if len(other) != 1 || other[0].CardID != awayCard {
		t.Fatalf("the other project's search answered %d notes, want its own one", len(other))
	}
}

func TestRemoveProjectMemoryIsNotAnErrorTheSecondTime(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SaveNote(ctx, cardID, "the widget report", protocol.NoteAuthorPerson); err != nil {
		t.Fatalf("save the note: %v", err)
	}
	if err := f.svc.RemoveProjectMemory(ctx, projectID); err != nil {
		t.Fatalf("remove the project's memory: %v", err)
	}
	// A removal that was finished by hand, or asked for twice, is the same state and not an error:
	// that is what lets the projects service call this for a project it half-removed already.
	if err := f.svc.RemoveProjectMemory(ctx, projectID); err != nil {
		t.Fatalf("removing memory that is already gone: %v", err)
	}
	// A project nothing was ever saved for is the same case.
	if err := f.svc.RemoveProjectMemory(ctx, otherProj); err != nil {
		t.Fatalf("removing the memory of a project that never had any: %v", err)
	}
}

func TestRemoveProjectMemoryRefusesANameThatIsNotAProjectFolder(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	// A folder beside the vault root, which a name that climbs out of the vault would reach: the
	// guard is about this, because removing a project's memory removes a whole tree.
	outside := filepath.Join(filepath.Dir(f.root), "keep-me")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatalf("make a folder beside the vault: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "a-note.md"), []byte("not the daemon's\n"), 0o600); err != nil {
		t.Fatalf("write a file beside the vault: %v", err)
	}

	for _, tc := range []struct {
		name string
		id   string
	}{
		{"nothing named", ""},
		{"the vault root itself", "."},
		{"above the vault root", ".."},
		{"a path out of the vault", "../keep-me"},
		{"a path with a folder in it", "small-repo/cards"},
		{"an absolute path", "/etc"},
		{"a windows separator", `..\keep-me`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := f.svc.RemoveProjectMemory(ctx, tc.id); err == nil {
				t.Fatalf("RemoveProjectMemory(%q) was allowed", tc.id)
			}
		})
	}

	// Nothing outside the vault was touched by any of the refusals.
	if _, err := os.Stat(filepath.Join(outside, "a-note.md")); err != nil {
		t.Fatalf("a refused removal deleted something outside the vault: %v", err)
	}
}

func TestRemoveProjectMemoryStopsWhenTheDaemonIsShuttingDown(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.SaveNote(context.Background(), cardID, "the widget report", protocol.NoteAuthorPerson); err != nil {
		t.Fatalf("save the note: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.svc.RemoveProjectMemory(ctx, projectID); !errors.Is(err, context.Canceled) {
		t.Fatalf("a removal during shutdown = %v, want the cancellation", err)
	}
	// A cancelled call did nothing at all: the folder and the row are still there.
	if _, err := os.Stat(filepath.Join(f.root, projectID)); err != nil {
		t.Fatalf("a cancelled removal deleted the folder: %v", err)
	}
	if _, err := f.store.Queries().GetNote(context.Background(), db.GetNoteParams{ProjectID: projectID, CardID: cardID}); err != nil {
		t.Fatalf("a cancelled removal deleted the row: %v", err)
	}
}
