package memory

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// A card's note: the file in the vault, and the row that indexes it (docs/architecture.md section
// 10's `notes` row and section 12; docs/backend-checklist.md B7.4, N11, build-plan task 7.12).
//
// A card has one note and not a list, so there is no page of notes to walk and no page size here:
// the read answers the whole note of one card, and the save replaces it.
//
// The file is the truth about what a note says and the row is the index of it, which is why a read
// prefers the file when both are there. That is also what makes an edit made in Obsidian show up
// straight away in the Notes tab: the tab reads the file, and the row - which is what search reads -
// catches up when the vault watcher (task 7.8) runs.

const (
	// DefaultSearchLimit is how many notes a search answers with when the caller does not say.
	DefaultSearchLimit = 20
	// MaxSearchLimit is the most a search will answer with, however many it is asked for: a search
	// exists to point at the note a person wants, not to hand back the vault.
	MaxSearchLimit = 50
	// searchTermMax caps one word of a query. A longer run of letters is not a word anybody typed
	// on purpose, and the cap keeps a pasted document from becoming a match expression.
	searchTermMax = 64
)

// Note returns a card's note. A card nothing has been saved for answers the placeholder note the
// daemon writes for it, with no save time at all, and reading never writes anything: the file
// appears when something is first saved. A card that is not there comes back as the projects
// service's own not-found error.
func (s *Service) Note(ctx context.Context, cardID string) (protocol.Note, error) {
	card, project, err := s.cardAndProject(ctx, cardID)
	if err != nil {
		return protocol.Note{}, err
	}
	rel := noteRelPath(card)
	row, rowErr := s.store.Queries().GetNote(ctx, db.GetNoteParams{ProjectID: card.ProjectID, CardID: card.ID})
	if rowErr != nil && !store.IsNotFound(rowErr) {
		return protocol.Note{}, fmt.Errorf("read the note of card %s: %w", card.ID, rowErr)
	}
	body, modified, found, err := s.readNoteFile(rel)
	if err != nil {
		return protocol.Note{}, err
	}
	switch {
	case found:
		// The file is there, so it is what the note says, and the file's own time is how old that
		// text is - a note edited in Obsidian is newer than its row until the watcher catches up.
		author := protocol.NoteAuthorPerson
		if rowErr == nil {
			author = authorOf(row)
		}
		return protocol.NewNote(card.ID, card.ProjectID, rel, body, author, modified), nil
	case rowErr == nil:
		// The row is there and the file is not, which is what a vault that has been moved or
		// emptied by hand looks like. The row still knows what the note said, so it answers, and
		// the next save writes the file again.
		return protocol.NewNote(card.ID, card.ProjectID, rel, row.Body, authorOf(row),
			time.UnixMilli(row.UpdatedAt).UTC()), nil
	default:
		return protocol.NewNote(card.ID, card.ProjectID, rel, placeholderNote(card, project),
			protocol.NoteAuthorPerson, time.Time{}), nil
	}
}

// SaveNote writes a card's note: the file in the vault, then the row that indexes it. It answers the
// note as a read would, so a client that saves and draws the answer shows exactly what the next read
// would show.
//
// The file goes first on purpose. If the row's write fails, what is left is a note that reads right
// and is not yet searchable, which is the state the watcher repairs; the other order would leave a
// row indexing a note that is not in the vault, which reads as a note that lost its text.
//
// The author is written with the note so a reader can tell an agent's note from the owner's. It must
// be one of protocol.NoteAuthorValues: the schema stores that word and the check is in Go rather
// than in a CHECK constraint, for the reason migration 0016 gives about smell families.
func (s *Service) SaveNote(ctx context.Context, cardID string, body string, author protocol.NoteAuthor) (protocol.Note, error) {
	if !author.Valid() {
		return protocol.Note{}, protocol.InvalidArgument("A note is written by a person or by an agent.").
			With("author", string(author))
	}
	card, _, err := s.cardAndProject(ctx, cardID)
	if err != nil {
		return protocol.Note{}, err
	}
	rel := noteRelPath(card)
	if err := s.writeNoteFile(rel, body); err != nil {
		return protocol.Note{}, err
	}
	id, err := s.newID()
	if err != nil {
		return protocol.Note{}, fmt.Errorf("make a note id: %w", err)
	}
	now := s.now().UnixMilli()
	err = s.store.Write(ctx, func(q *db.Queries) error {
		return q.UpsertNote(ctx, db.UpsertNoteParams{
			ID: id, ProjectID: card.ProjectID, CardID: card.ID, Author: string(author),
			Body: body, CreatedAt: now, UpdatedAt: now,
		})
	})
	if err != nil {
		return protocol.Note{}, fmt.Errorf("save the note of card %s: %w", card.ID, err)
	}
	return s.Note(ctx, cardID)
}

// SearchNotes returns a project's notes whose text matches a person's words, best match first. It
// reads the index and not the files, so it is one query over rows rather than a walk that reads
// every note; a note changed in Obsidian becomes searchable when the watcher re-indexes it.
//
// A query with no words in it - spaces, punctuation, an emoji - matches nothing and answers an
// empty list rather than every note, because a search that answers everything is not a search.
func (s *Service) SearchNotes(ctx context.Context, projectID, query string, limit int) ([]protocol.Note, error) {
	match := matchExpression(query)
	if match == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	limit = min(limit, MaxSearchLimit)
	rows, err := s.store.SearchNotes(ctx, projectID, match, limit)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.Note, 0, len(rows))
	for _, row := range rows {
		card, err := s.cards.Card(ctx, row.CardID)
		if err != nil {
			// A note whose card is gone is a row the foreign key should not have allowed, and the
			// note is not worth failing a search over: it is skipped and the rest are answered.
			continue
		}
		out = append(out, protocol.NewNote(row.CardID, row.ProjectID, noteRelPath(card), row.Body,
			authorOf(row), time.UnixMilli(row.UpdatedAt).UTC()))
	}
	return out, nil
}

// matchExpression turns a person's words into an FTS5 match expression: one prefix term per word,
// joined so that every word must be found. It keeps only letters and digits, which is what makes
// handing the result to FTS5 safe - a query is a person's typing and a stray quote, hyphen, or
// parenthesis in it would otherwise be FTS5 syntax and would come back as a failed query.
//
// A prefix term is what makes search feel right as a person types: "heal" finds "health check".
// FTS5's own operator for that is a trailing `*`, and a term is only ever followed by a space or the
// end of the expression, so nothing here can be read as an operator.
func matchExpression(query string) string {
	words := strings.FieldsFunc(query, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	terms := make([]string, 0, len(words))
	for _, word := range words {
		if len(word) > searchTermMax {
			word = word[:searchTermMax]
		}
		terms = append(terms, word+"*")
	}
	return strings.Join(terms, " ")
}

// authorOf reads a row's author as one of the two words the wire knows. A row holding anything else
// is a database that was written by something other than this module, and the owner is the safe
// answer rather than a failed read.
func authorOf(row db.Note) protocol.NoteAuthor {
	author := protocol.NoteAuthor(row.Author)
	if author.Valid() {
		return author
	}
	return protocol.NoteAuthorPerson
}
