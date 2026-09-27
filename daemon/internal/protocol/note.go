package protocol

import "time"

// The wire shape of a card's note: the markdown file Marshal keeps for one card in the vault, and
// the row that indexes it (docs/architecture.md section 10's `notes` row and section 12's vault
// layout; docs/backend-checklist.md B7.4, N11, build-plan task 7.12).
//
// A card has ONE note and not a list, which is what the Notes tab is: the design's
// `apps/web/src/views/card/card-note.ts` reads one string, `saveNote` replaces the whole thing, and
// `notePath` shows where in the vault it lives. The wire says the same things and nothing else, so
// the tab needs one call to draw itself.

// NoteAuthor is who wrote a note. It is the same two words the rest of the schema uses for the
// same question - `audit_log.actor` is "person", "agent", or "daemon" (migration 0013) - and it is
// all a note needs: there is one owner and one agent per card.
type NoteAuthor string

const (
	// NoteAuthorPerson is the owner, writing in the Notes tab or in Obsidian.
	NoteAuthorPerson NoteAuthor = "person"
	// NoteAuthorAgent is a card's own agent, writing through the `post_note` tool.
	NoteAuthorAgent NoteAuthor = "agent"
)

// NoteAuthorValues lists every note author. Keep it in the same order as the const block above; a
// test checks that.
func NoteAuthorValues() []NoteAuthor {
	return []NoteAuthor{NoteAuthorPerson, NoteAuthorAgent}
}

// Valid reports whether a is a note author.
func (a NoteAuthor) Valid() bool {
	for _, v := range NoteAuthorValues() {
		if v == a {
			return true
		}
	}
	return false
}

// Note is one card's note as a client sees it.
type Note struct {
	// CardID is the card the note belongs to.
	CardID string `json:"cardId"`
	// ProjectID is the project the card is in, and the name of the folder the note's file sits in
	// under the vault (see Path).
	ProjectID string `json:"projectId"`
	// Path is where the note lives, relative to the vault root, in the shape
	// `<project>/cards/<number>-<title>.md` - so the Notes tab shows something like
	// `small-repo/cards/7-add-a-health-check.md`. It is relative on purpose: the vault lives
	// wherever the person put it, and an absolute path from this machine would be meaningless to a
	// phone.
	Path string `json:"path"`
	// Body is the whole note, in markdown. The tab shows it as it is and replaces all of it when it
	// saves.
	Body string `json:"body"`
	// Author is who last wrote the note, and person for a note nothing has been saved for yet.
	Author NoteAuthor `json:"author"`
	// UpdatedAt is when the note was last saved, or null when nothing has been saved for the card.
	// Null is the whole of what "no note" means on the wire: Body is then the note the daemon writes
	// for the card (its title, its goal, and a link to its project), and the first save clears it.
	//
	// Reading a note that does not exist does not write one. The design's `ensureNote` makes the
	// note in the store as it draws the panel, which is fine for a mock and wrong for a daemon: a
	// GET must not create a file. So the placeholder is built in memory and returned, and the file
	// appears on the first save.
	//
	// It is a pointer because the repo's convention for a time that may be absent is one (a plain
	// Timestamp refuses to encode the zero time, which is a bug to fix rather than a date in the
	// year 1). The field carries no `omitempty`, so it is always sent, as null when nothing has
	// been saved; the `required` flag of the tstype tag tells the generated TypeScript the same
	// thing, so it is `updatedAt: Timestamp | null` and not an optional field (card.go's `session`
	// is the same shape for the same reason).
	UpdatedAt *Timestamp `json:"updatedAt" tstype:"Timestamp | null,required"`
}

// NewNote makes a note, stamping it with the daemon's clock when there is a save to stamp. A zero
// time means nothing has been saved, which is what a null UpdatedAt says on the wire.
func NewNote(cardID, projectID, path, body string, author NoteAuthor, updatedAt time.Time) Note {
	note := Note{
		CardID: cardID, ProjectID: projectID, Path: path, Body: body, Author: author,
	}
	if !updatedAt.IsZero() {
		stamp := NewTimestamp(updatedAt)
		note.UpdatedAt = &stamp
	}
	return note
}

// SaveNoteRequest is the body of the call that writes a card's note (B7.4, task 7.12). It is the
// whole note and not a patch: the tab's editor replaces the file, and a body that arrived with a
// part of the old one missing would be a merge nobody asked for.
type SaveNoteRequest struct {
	// Body is the note in full, in markdown. It may be empty, which saves an empty note rather than
	// deleting the file: deleting a note is its own thing, done in Obsidian, and it is not something
	// a save should do by accident.
	Body string `json:"body"`
}
