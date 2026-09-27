package store

import (
	"context"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// searchNotesQuery and searchLessonsQuery are the two statements in this package that sqlc does not
// generate, and they are here rather than in internal/memory for the reason every other statement is
// here: this package is the one that owns the SQL.
//
// They search the full-text index over card notes and lessons (migration 0019, `notes_fts`), which
// is the search docs/architecture.md section 10 calls for ("Session search uses SQLite full-text
// search over `session_events.summary` and card notes"), now also lessons (task 7.6, section 11.4's
// search_memory). sqlc models the virtual table's columns well enough to list them, but it cannot
// parse FTS5's `MATCH` operator: `WHERE notes_fts MATCH ?` and `WHERE f MATCH ?` both stop the
// generator with `column notes_fts does not exist`. That was measured rather than assumed, and it is
// why these statements are written out by hand.
//
// Each query is filtered to its own kind: a person searching card notes from the Notes tab, or an
// agent's search_memory searching lessons, is never shown the other's rows by surprise. The join is
// on the index's own id. `ORDER BY f.rank` is FTS5's own relevance order, best match first, and the
// save time breaks a tie so two equally good matches come back in a stable order rather than in
// whatever order the index happens to hold them.
const searchNotesQuery = `
SELECT n.id, n.project_id, n.card_id, n.kind, n.slug, n.title, n.author, n.body, n.created_at, n.updated_at
FROM notes_fts f
JOIN notes n ON n.id = f.id
WHERE f.project_id = ? AND f.kind = 'card_note' AND notes_fts MATCH ?
ORDER BY f.rank, n.updated_at DESC
LIMIT ?`

const searchLessonsQuery = `
SELECT n.id, n.project_id, n.card_id, n.kind, n.slug, n.title, n.author, n.body, n.created_at, n.updated_at
FROM notes_fts f
JOIN notes n ON n.id = f.id
WHERE f.project_id = ? AND f.kind = 'lesson' AND notes_fts MATCH ?
ORDER BY f.rank, n.updated_at DESC
LIMIT ?`

// SearchNotes returns the matching card notes of one project, best match first, at most limit of
// them.
//
// match is an FTS5 match expression, and it must be one FTS5 accepts: a stray quote or an unclosed
// parenthesis is a syntax error that comes back from here as a failed query. internal/memory builds
// the expression from a person's words (one prefix term per word, nothing else), which is what makes
// that safe to hand over; it is the caller's job and not this one's.
func (s *Store) SearchNotes(ctx context.Context, projectID string, match string, limit int) ([]db.Note, error) {
	return s.searchNotesTable(ctx, searchNotesQuery, "notes", projectID, match, limit)
}

// SearchLessons returns the matching lessons of one project, best match first, at most limit of
// them. Same match-expression rules as SearchNotes; internal/memory builds the expression the same
// way for both.
func (s *Store) SearchLessons(ctx context.Context, projectID string, match string, limit int) ([]db.Note, error) {
	return s.searchNotesTable(ctx, searchLessonsQuery, "lessons", projectID, match, limit)
}

// searchNotesTable runs one of the two queries above and scans the rows they share a shape with.
// noun names what is being searched in an error, so a failure says "search the lessons" or "search
// the notes" and not just "search".
func (s *Store) searchNotesTable(ctx context.Context, query, noun, projectID, match string, limit int) ([]db.Note, error) {
	rows, err := s.reader.QueryContext(ctx, query, projectID, match, limit)
	if err != nil {
		return nil, fmt.Errorf("search the %s: %w", noun, contextError(ctx, err))
	}
	defer func() { _ = rows.Close() }()
	var out []db.Note
	for rows.Next() {
		var note db.Note
		if err := rows.Scan(&note.ID, &note.ProjectID, &note.CardID, &note.Kind, &note.Slug, &note.Title,
			&note.Author, &note.Body, &note.CreatedAt, &note.UpdatedAt); err != nil {
			return nil, fmt.Errorf("read a %s match: %w", noun, err)
		}
		out = append(out, note)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the %s matches: %w", noun, contextError(ctx, err))
	}
	return out, nil
}
