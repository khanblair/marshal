package protocol_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The wire shape of a card's note (B7.4, N11, task 7.12): the markdown the Notes tab shows, where
// the file lives in the vault, who wrote it, and whether anything has been saved for the card yet.

// sampleNote is a card's note as the tab sees it after a person has saved it.
func sampleNote() protocol.Note {
	return protocol.NewNote(
		"card-7f2a", "small-repo", "small-repo/cards/7-add-a-health-check.md",
		"# Add a health check\n\nGoal: add a health check.\n\nThe probe goes beside the server.\n\nLinks\n[[small-repo]]\n",
		protocol.NoteAuthorPerson, providersNow,
	)
}

func TestNoteGolden(t *testing.T) {
	testutil.Golden(t, "note", sampleNote())
}

// A card nothing has been saved for answers the note the daemon writes for it, with no save time.
// That is what the tab shows for a card nobody has written a note on, and the null is what tells a
// client the file does not exist yet.
func TestANoteNothingHasBeenSavedForGolden(t *testing.T) {
	untouched := protocol.NewNote(
		"card-9c14", "small-repo", "small-repo/cards/9-write-the-runbook.md",
		"# Write the runbook\n\nGoal: write the runbook.\n\nLinks\n[[small-repo]]\n",
		protocol.NoteAuthorPerson, time.Time{},
	)
	testutil.Golden(t, "note-unsaved", untouched)
}

// The two authors are the two words the rest of the schema uses for the same question.
func TestNoteAuthorsAreTheTwoTheSchemaUses(t *testing.T) {
	want := []string{"person", "agent"}
	got := protocol.NoteAuthorValues()
	if len(got) != len(want) {
		t.Fatalf("NoteAuthorValues = %v, want %v", got, want)
	}
	for i, name := range want {
		if string(got[i]) != name {
			t.Fatalf("NoteAuthorValues = %v, want %v", got, want)
		}
	}
	if !protocol.NoteAuthorAgent.Valid() || protocol.NoteAuthor("system").Valid() {
		t.Error("Valid accepts an author the daemon never stores")
	}
}

// A note is one string and not a list, and a client that reads it can hand the whole body back
// unchanged. The point of the check is that the body's own punctuation - the markdown, the links,
// the newlines - survives the round trip, because the file is what a person edits in Obsidian.
func TestANotesBodyRoundTrips(t *testing.T) {
	body := sampleNote().Body
	encoded, err := json.Marshal(protocol.SaveNoteRequest{Body: body})
	if err != nil {
		t.Fatal(err)
	}
	var back protocol.SaveNoteRequest
	if err := json.Unmarshal(encoded, &back); err != nil {
		t.Fatal(err)
	}
	if back.Body != body {
		t.Errorf("the note came back as %q, want %q", back.Body, body)
	}
}

// A note that has been saved carries the daemon's time and a path relative to the vault root: the
// vault lives wherever the person put it, so an absolute path from this machine would be
// meaningless to a phone.
func TestASavedNoteCarriesItsTimeAndARelativePath(t *testing.T) {
	encoded, err := json.Marshal(sampleNote())
	if err != nil {
		t.Fatal(err)
	}
	got := string(encoded)
	if !strings.Contains(got, `"updatedAt":"2026-09-27T09:30:00.000Z"`) {
		t.Errorf("a saved note encoded as %s, want the daemon's time", got)
	}
	if !strings.Contains(got, `"path":"small-repo/cards/7-add-a-health-check.md"`) {
		t.Errorf("a saved note encoded as %s, want a vault-relative path", got)
	}
	if strings.HasPrefix(sampleNote().Path, "/") {
		t.Error("the note's path is absolute, want it relative to the vault root")
	}
}

// A note nothing has been saved for encodes a null time rather than refusing to encode or writing a
// date in the year 1. This is the whole of what "no note" means on the wire.
func TestANoteNothingHasBeenSavedForEncodesANullTime(t *testing.T) {
	encoded, err := json.Marshal(protocol.NewNote("card-9c14", "small-repo", "small-repo/cards/9-write-the-runbook.md", "body", protocol.NoteAuthorPerson, time.Time{}))
	if err != nil {
		t.Fatalf("encode a note with no save time: %v", err)
	}
	if !strings.Contains(string(encoded), `"updatedAt":null`) {
		t.Errorf("a note nothing has been saved for encoded as %s, want a null time", encoded)
	}
}
