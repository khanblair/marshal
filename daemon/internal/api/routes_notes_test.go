package api_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A card's note over the API (docs/architecture.md sections 10 and 12, docs/backend-checklist.md
// B7.4 and N11, build-plan task 7.12). The note is a markdown file in the vault, so these tests read
// the file back off disk as well as the answer: the row and the file are what the module keeps in
// step, and the route is the only way a client sees either.
//
// The path and placeholder rules are the memory module's (internal/memory's vault.go), and the
// search half is tested beside the rest of the search in routes_search_test.go. What is here is the
// route: what it answers, that it writes the vault, and what it refuses.

// notePathOf is the vault path a card's note is written to: the project, the folder, and the card's
// number with a readable part of its title.
func notePathOf(card protocol.Card, slug string) string {
	return card.ProjectID + "/cards/" + strconv.Itoa(card.Number) + "-" + slug + ".md"
}

func TestNoteRouteThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Add a health check")
	route := "/v1/cards/" + card.ID + "/note"
	wantPath := notePathOf(card, "add-a-health-check")

	// A card nothing has been saved for answers the note the daemon would start one from, with no
	// save time, and reading writes nothing: the vault is still empty afterwards.
	got := st.do(http.MethodGet, route, nil).want(t, http.StatusOK)
	sameShape(t, "note-unsaved", got.Body)
	note := decode[protocol.Note](t, got)
	if note.CardID != card.ID || note.ProjectID != project.ID || note.Path != wantPath {
		t.Errorf("the note names card %q project %q path %q", note.CardID, note.ProjectID, note.Path)
	}
	if note.Author != protocol.NoteAuthorPerson {
		t.Errorf("the author of an unsaved note = %q, want person", note.Author)
	}
	if note.UpdatedAt != nil {
		t.Errorf("an unsaved note is stamped %v, want no time at all", note.UpdatedAt)
	}
	if note.Body != "# Add a health check\n\nGoal: add a health check.\n\nLinks\n[[small-repo]]\n" {
		t.Errorf("the placeholder note = %q", note.Body)
	}
	if _, err := os.Stat(filepath.Join(st.vault, filepath.FromSlash(wantPath))); !os.IsNotExist(err) {
		t.Errorf("a read wrote a file (stat: %v)", err)
	}

	// Saving replaces the note and answers it as a read would, and the file is in the vault.
	body := "# Add a health check\n\nThe probe goes beside the server.\n"
	saved := decode[protocol.Note](t, st.do(http.MethodPut, route, protocol.SaveNoteRequest{Body: body}).
		want(t, http.StatusOK))
	if saved.Body != body || saved.Author != protocol.NoteAuthorPerson {
		t.Errorf("the saved note = %+v", saved)
	}
	if saved.UpdatedAt == nil || saved.UpdatedAt.Time().IsZero() {
		t.Errorf("a saved note has no save time: %+v", saved.UpdatedAt)
	}
	onDisk, err := os.ReadFile(filepath.Join(st.vault, filepath.FromSlash(wantPath)))
	if err != nil {
		t.Fatalf("read the note file: %v", err)
	}
	if string(onDisk) != body {
		t.Errorf("the vault holds %q, want the body that was saved", onDisk)
	}
	again := decode[protocol.Note](t, st.do(http.MethodGet, route, nil).want(t, http.StatusOK))
	if again.Body != body || again.Author != protocol.NoteAuthorPerson || again.UpdatedAt == nil {
		t.Errorf("a read after a save = %+v, want the saved note", again)
	}

	// A note may be saved empty: deleting it is a thing done in Obsidian, not something a save does
	// by accident.
	emptied := decode[protocol.Note](t, st.do(http.MethodPut, route, protocol.SaveNoteRequest{}).want(t, http.StatusOK))
	if emptied.Body != "" {
		t.Errorf("an empty save answered %q, want an empty note", emptied.Body)
	}

	// A card that is not there is not found, and neither method invents one.
	st.do(http.MethodGet, "/v1/cards/no-such-card/note", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodPut, "/v1/cards/no-such-card/note", protocol.SaveNoteRequest{Body: "x"}).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)

	// The note is written by a person here, so a body that is not the request shape is refused and
	// nothing is written.
	st.do(http.MethodPut, route, "{not json").
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	if after, _ := os.ReadFile(filepath.Join(st.vault, filepath.FromSlash(wantPath))); string(after) != "" {
		t.Errorf("a refused save wrote %q", after)
	}

	// The note is read and replaced, and nothing else.
	st.do(http.MethodPost, route, protocol.SaveNoteRequest{Body: "x"}).
		apiError(t, http.StatusMethodNotAllowed, protocol.ErrorCodeMethodNotAllowed)
	st.do(http.MethodDelete, route, nil).
		apiError(t, http.StatusMethodNotAllowed, protocol.ErrorCodeMethodNotAllowed)
}

// A note written for one card is not the answer for another, and a card of another project keeps
// its own note under its own project's folder.
func TestANoteBelongsToItsCardAndItsProject(t *testing.T) {
	st := newStack(t)
	small, _ := st.addProject("small-repo")
	other, _ := st.addProject("monorepo")
	first := st.addCard(small.ID, "Add a health check")
	second := st.addCard(small.ID, "Trim the bundle")
	away := st.addCard(other.ID, "Elsewhere work")

	st.do(http.MethodPut, "/v1/cards/"+first.ID+"/note", protocol.SaveNoteRequest{Body: "the first card's note"}).
		want(t, http.StatusOK)

	if got := decode[protocol.Note](t, st.do(http.MethodGet, "/v1/cards/"+second.ID+"/note", nil).want(t, http.StatusOK)); got.Body == "the first card's note" {
		t.Error("one card's note answered for another")
	}
	awayNote := decode[protocol.Note](t, st.do(http.MethodGet, "/v1/cards/"+away.ID+"/note", nil).want(t, http.StatusOK))
	if awayNote.ProjectID != other.ID || awayNote.Path != notePathOf(away, "elsewhere-work") {
		t.Errorf("a card of another project answered %+v", awayNote)
	}
}

// The search answers the two kinds that live in the vault and its index: a card's note and a stored
// moment of a past session (docs/architecture.md section 10). Both are read through the memory
// module and both are named by their card, so the palette opens the card the work happened on.
func TestSearchFindsANoteAndAPastSessionThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Add a health check")
	notePath := notePathOf(card, "add-a-health-check")

	noteBody := "# Add a health check\n\nThe probe goes beside the server.\n"
	st.do(http.MethodPut, "/v1/cards/"+card.ID+"/note", protocol.SaveNoteRequest{Body: noteBody}).
		want(t, http.StatusOK)
	// A stored moment of a past session, written the way the session manager writes one.
	addHistory(t, st, card.ID,
		history.Record{Kind: history.KindAgent, Summary: "Read the probe callers before editing."},
	)

	got, body := st.searchFor("probe")
	sameShape(t, "search", body)
	if len(got.Notes) != 1 {
		t.Fatalf("notes = %+v, want the card's note", got.Notes)
	}
	note := got.Notes[0]
	if note.CardID != card.ID || note.Key != card.Key || note.Title != card.Title ||
		note.Path != notePath || note.Author != protocol.NoteAuthorPerson ||
		note.ProjectID != project.ID || note.ProjectName != project.Name {
		t.Errorf("note hit = %+v", note)
	}
	if len(got.Sessions) != 1 {
		t.Fatalf("sessions = %+v, want the stored moment", got.Sessions)
	}
	session := got.Sessions[0]
	if session.CardID != card.ID || session.Key != card.Key || session.Title != card.Title ||
		session.ProjectID != project.ID || session.ProjectName != project.Name {
		t.Errorf("session hit = %+v", session)
	}
	if session.Excerpt != "Read the probe callers before editing." {
		t.Errorf("the session excerpt = %q, want the stored summary", session.Excerpt)
	}
	if got.Totals.Notes != 1 || got.Totals.Sessions != 1 {
		t.Errorf("totals = %+v, want one note and one session", got.Totals)
	}

	// The index follows the note: a word the new text no longer holds finds nothing of it, and one
	// the new text holds finds it. The old note's row is replaced, not added to.
	st.do(http.MethodPut, "/v1/cards/"+card.ID+"/note", protocol.SaveNoteRequest{
		Body: "# Add a health check\n\nThe liveness probe goes beside the server.\n",
	}).want(t, http.StatusOK)
	if found, _ := st.searchFor("probe"); len(found.Notes) != 1 {
		t.Errorf("after the edit, the note is found %d times, want once", len(found.Notes))
	}

	// A card deleted takes its note and its stored sessions out of the search with it.
	st.do(http.MethodDelete, "/v1/cards/"+card.ID, nil).want(t, http.StatusNoContent)
	gone, _ := st.searchFor("liveness")
	if len(gone.Notes)+len(gone.Sessions) != 0 {
		t.Errorf("a deleted card is still found through its note or its past sessions: %+v", gone)
	}
	if got, _ := st.searchFor("probe"); got.Totals != (protocol.SearchTotals{}) {
		t.Errorf("a deleted card's past session is still found: %+v", got.Totals)
	}
}
