package memory_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/memory"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The vault watcher (docs/architecture.md section 12, docs/backend-checklist.md B7.4, build-plan
// task 7.8): the pass that brings the index up to date after a person edits a note in Obsidian.
//
// It is driven by hand here rather than by its own ticker, because a test that waits out a wall-clock
// interval is slow and would be racing a timer. Nothing in the fixture starts the watcher, so a call
// to Reindex is the only pass that ever runs in these tests.

// writeNoteFileAt puts a file in the vault by hand, the way a person does in Obsidian: the bytes are
// the test's, and the file's own time is set to a time the test chose, because that time is what the
// watcher compares against the row and the machine's clock is not something a test can assert on.
func writeNoteFileAt(t *testing.T, f *fixture, rel, body string, modified time.Time) {
	t.Helper()
	full := filepath.Join(f.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatalf("make the folder of %s: %v", rel, err)
	}
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	setNoteFileTime(t, f, rel, modified)
}

// setNoteFileTime moves a file's own modification time, which is the whole signal the watcher has.
func setNoteFileTime(t *testing.T, f *fixture, rel string, modified time.Time) {
	t.Helper()
	full := filepath.Join(f.root, filepath.FromSlash(rel))
	if err := os.Chtimes(full, modified, modified); err != nil {
		t.Fatalf("set the time of %s: %v", rel, err)
	}
}

func TestReindexOfAVaultThatIsNotThereYetIsNoWork(t *testing.T) {
	f := newFixture(t)
	report, err := f.svc.Reindex(context.Background())
	if err != nil {
		t.Fatalf("a vault that is not there is not an error: %v", err)
	}
	if report != (memory.ReindexReport{}) {
		t.Fatalf("an empty vault reported %+v, want nothing looked at", report)
	}
}

func TestReindexBringsTheIndexUpToDateWithAnEditMadeInObsidian(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SaveNote(ctx, cardID, "the daemon wrote this", protocol.NoteAuthorPerson); err != nil {
		t.Fatalf("save the note: %v", err)
	}
	before, err := f.store.Queries().GetNote(ctx, db.GetNoteParams{ProjectID: projectID, CardID: cardID})
	if err != nil {
		t.Fatalf("read the note row: %v", err)
	}

	// The person opens the vault in Obsidian and edits the note an hour later.
	edited := testNow.Add(time.Hour)
	writeNoteFileAt(t, f, notePath, "the person edited this in Obsidian\n", edited)

	report, err := f.svc.Reindex(ctx)
	if err != nil {
		t.Fatalf("re-index the vault: %v", err)
	}
	if report != (memory.ReindexReport{Projects: 1, Files: 1, Indexed: 1}) {
		t.Fatalf("the pass reported %+v, want one project, one file, one re-indexed", report)
	}

	// The index follows the file: the row is the index search matches against, and the file is what
	// the note says, so after the pass the person can find what they wrote.
	found, err := f.svc.SearchNotes(ctx, projectID, "Obsidian", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(found) != 1 || found[0].CardID != cardID {
		t.Fatalf("search after the edit found %d notes, want the edited one", len(found))
	}
	gone, err := f.svc.SearchNotes(ctx, projectID, "daemon", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(gone) != 0 {
		t.Fatalf("the text the person replaced is still searchable: %d matches", len(gone))
	}

	// An edit is the same note: it keeps the id and the birthday it was first saved with, and the
	// author it had. A person who edits an agent's note does not become its author.
	after, err := f.store.Queries().GetNote(ctx, db.GetNoteParams{ProjectID: projectID, CardID: cardID})
	if err != nil {
		t.Fatalf("read the note row again: %v", err)
	}
	if after.ID != before.ID {
		t.Fatalf("the edit made a second note: id %q became %q", before.ID, after.ID)
	}
	if after.CreatedAt != before.CreatedAt {
		t.Fatalf("the edit moved the note's birthday from %d to %d", before.CreatedAt, after.CreatedAt)
	}
	if after.Author != before.Author {
		t.Fatalf("the edit changed the note's author from %q to %q", before.Author, after.Author)
	}
	if after.Body != "the person edited this in Obsidian\n" {
		t.Fatalf("the row holds %q, want the file's text", after.Body)
	}
	if after.UpdatedAt != edited.UnixMilli() {
		t.Fatalf("the row's time = %d, want the file's %d", after.UpdatedAt, edited.UnixMilli())
	}
}

func TestReindexKeepsTheAuthorOfANoteAnAgentWrote(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SaveNote(ctx, cardID, "what the agent found", protocol.NoteAuthorAgent); err != nil {
		t.Fatalf("save the note: %v", err)
	}
	// The agent edits its own note, which is what a later turn of the same card does.
	rewritten := testNow.Add(2 * time.Minute)
	writeNoteFileAt(t, f, notePath, "what the agent found, and then more", rewritten)

	if _, err := f.svc.Reindex(ctx); err != nil {
		t.Fatalf("re-index the vault: %v", err)
	}
	row, err := f.store.Queries().GetNote(ctx, db.GetNoteParams{ProjectID: projectID, CardID: cardID})
	if err != nil {
		t.Fatalf("read the note row: %v", err)
	}
	if row.Author != string(protocol.NoteAuthorAgent) {
		t.Fatalf("the row's author = %q, want the agent that wrote it to keep it", row.Author)
	}
	if row.Body != "what the agent found, and then more" {
		t.Fatalf("the row holds %q, want the file's text", row.Body)
	}
}

func TestReindexLeavesANoteTheDaemonJustWroteAlone(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SaveNote(ctx, cardID, "the daemon wrote this", protocol.NoteAuthorPerson); err != nil {
		t.Fatalf("save the note: %v", err)
	}
	// A save writes the file at the machine's clock and stamps the row from the test's, so the file's
	// time is set here: what this test is about is the common case, where the file is not newer than
	// the row and the pass writes nothing at all.
	for _, tc := range []struct {
		name string
		at   time.Time
	}{
		{"a file as old as its row", testNow},
		{"a file older than its row", testNow.Add(-time.Hour)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setNoteFileTime(t, f, notePath, tc.at)
			report, err := f.svc.Reindex(ctx)
			if err != nil {
				t.Fatalf("re-index the vault: %v", err)
			}
			if report.Indexed != 0 {
				t.Fatalf("the pass re-indexed %d files, want none", report.Indexed)
			}
			// It still looked at the file, which is the difference between "nothing changed" and
			// "nothing was checked".
			if report.Files != 1 || report.Projects != 1 {
				t.Fatalf("the pass reported %+v, want it to have looked at one file", report)
			}
		})
	}

	// Nothing was written: the row is exactly as the save left it.
	row, err := f.store.Queries().GetNote(ctx, db.GetNoteParams{ProjectID: projectID, CardID: cardID})
	if err != nil {
		t.Fatalf("read the note row: %v", err)
	}
	if row.UpdatedAt != testNow.UnixMilli() || row.Body != "the daemon wrote this" {
		t.Fatalf("an idle pass changed the row to %+v", row)
	}
}

func TestReindexIndexesANoteFileNothingHasSavedYet(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	// The name card #8's note would have (the number, the title, `.md`): the watcher finds the card by
	// the number the file's name starts with, because that is the only thing tying the two together.
	rel := "small-repo/cards/8-fix-the-retry-loop.md"
	writeNoteFileAt(t, f, rel, "written straight into the vault\n", testNow)

	report, err := f.svc.Reindex(ctx)
	if err != nil {
		t.Fatalf("re-index the vault: %v", err)
	}
	if report != (memory.ReindexReport{Projects: 1, Files: 1, Indexed: 1}) {
		t.Fatalf("the pass reported %+v, want one project, one file, one indexed", report)
	}
	row, err := f.store.Queries().GetNote(ctx, db.GetNoteParams{ProjectID: projectID, CardID: sibling})
	if err != nil {
		t.Fatalf("read the note row: %v", err)
	}
	if row.Body != "written straight into the vault\n" {
		t.Fatalf("the row holds %q, want the file's text", row.Body)
	}
	// A note that was born in the vault is dated by its own file: there is no earlier save to
	// remember, and the file's time is what says when it was written.
	if row.CreatedAt != testNow.UnixMilli() || row.UpdatedAt != testNow.UnixMilli() {
		t.Fatalf("the row is dated %d/%d, want the file's %d", row.CreatedAt, row.UpdatedAt, testNow.UnixMilli())
	}
	if row.Author != string(protocol.NoteAuthorPerson) {
		t.Fatalf("the row's author = %q, want person (a file is the only word on who wrote it)", row.Author)
	}
}

func TestReindexCountsWhatItLookedAtAndSkipsWhatIsNotANote(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	// The vault root holds folders that are not a project: `briefs/` is one the daemon itself makes
	// (section 12), and a person may leave anything else in there. Neither is memory, and neither is
	// touched.
	writeNoteFileAt(t, f, "briefs/2026-09-27.md", "a brief\n", testNow)
	writeNoteFileAt(t, f, "a-folder-i-made/notes.md", "not memory\n", testNow)

	writeNoteFileAt(t, f, "small-repo/cards/7-add-a-health-check.md", "a real note\n", testNow)
	writeNoteFileAt(t, f, "small-repo/cards/readme.md", "no number in the name\n", testNow)
	writeNoteFileAt(t, f, "small-repo/cards/999-a-card-that-is-gone.md", "the card is not there\n", testNow)
	writeNoteFileAt(t, f, "small-repo/cards/notes.txt", "not markdown\n", testNow)
	writeNoteFileAt(t, f, "small-repo/cards/archive/7-add-a-health-check.md", "a copy in a subfolder\n", testNow)

	report, err := f.svc.Reindex(ctx)
	if err != nil {
		t.Fatalf("re-index the vault: %v", err)
	}
	// One project (the vault holds no folder for the other one), three files - the note, the name with
	// no number in it, and the name whose card is gone - and one of those was indexed. A subfolder and
	// a file that is not markdown are not notes.
	if report != (memory.ReindexReport{Projects: 1, Files: 3, Indexed: 1}) {
		t.Fatalf("the pass reported %+v, want one project, three files, one indexed", report)
	}
	// The card the file names is the one that got a row, and the others got none.
	if _, err := f.store.Queries().GetNote(ctx, db.GetNoteParams{ProjectID: projectID, CardID: cardID}); err != nil {
		t.Fatalf("the note file of a real card was not indexed: %v", err)
	}
	for _, id := range []string{sibling, third} {
		if _, err := f.store.Queries().GetNote(ctx, db.GetNoteParams{ProjectID: projectID, CardID: id}); !store.IsNotFound(err) {
			t.Fatalf("card %s got a row from a file that does not name it (%v)", id, err)
		}
	}
}

func TestReindexStopsWhenTheDaemonIsShuttingDown(t *testing.T) {
	f := newFixture(t)
	// The vault has to be there with something in it for a pass to reach its first check.
	writeNoteFileAt(t, f, "briefs/2026-09-27.md", "a brief\n", testNow)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.svc.Reindex(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("a pass during shutdown = %v, want the cancellation", err)
	}
}
