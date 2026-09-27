package store

import (
	"context"
	"fmt"
)

// searchSessionEventsQuery is the second statement in this package that sqlc does not generate, and
// it is beside searchNotesQuery for the same reason: it is the other half of what
// docs/architecture.md section 10 asks for ("Session search uses SQLite full-text search over
// `session_events.summary` and card notes"), and sqlc cannot parse FTS5's `MATCH` operator.
//
// The index it searches is `session_events_fts` (migration 0021), which holds only a card's events
// with a summary. The join back to `session_events` is by the event's own TEXT id, which the index
// keeps in an UNINDEXED column; the join to `cards` is what scopes a search to one project, because
// an event stores the card it belongs to and not the project. `ORDER BY f.rank` is FTS5's own
// relevance order, best match first, and the event's time breaks a tie so two equally good matches
// come back newest first rather than in whatever order the index holds them.
const searchSessionEventsQuery = `
SELECT e.id, e.card_id, e.summary, e.created_at
FROM session_events_fts f
JOIN session_events e ON e.id = f.id
JOIN cards c ON c.id = e.card_id
WHERE c.project_id = ? AND session_events_fts MATCH ?
ORDER BY f.rank, e.created_at DESC
LIMIT ?`

// SessionMatch is one stored session event a search found. It is the four fields a search answer
// needs to name the moment and the card it happened on: the event's id, the card it belongs to, the
// one-line summary that was matched, and when it happened. The rest of the event - its kind, its
// state, and its detail - is read from the card's own chat when the person opens it, which is why a
// search does not carry it.
type SessionMatch struct {
	// ID is the event's own id.
	ID string
	// CardID is the card the event belongs to.
	CardID string
	// Summary is the one line that was matched.
	Summary string
	// CreatedAt is when the event happened, in Unix milliseconds.
	CreatedAt int64
}

// SearchSessionEvents returns the matching stored events of one project's cards, best match first,
// at most limit of them.
//
// match is an FTS5 match expression, and it must be one FTS5 accepts: a stray quote or an unclosed
// parenthesis is a syntax error that comes back from here as a failed query. internal/search builds
// the expression from a person's words, which is what makes that safe to hand over; it is the
// caller's job and not this one's.
func (s *Store) SearchSessionEvents(ctx context.Context, projectID string, match string, limit int) ([]SessionMatch, error) {
	rows, err := s.reader.QueryContext(ctx, searchSessionEventsQuery, projectID, match, limit)
	if err != nil {
		return nil, fmt.Errorf("search the past sessions: %w", contextError(ctx, err))
	}
	defer func() { _ = rows.Close() }()
	var out []SessionMatch
	for rows.Next() {
		var m SessionMatch
		if err := rows.Scan(&m.ID, &m.CardID, &m.Summary, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("read a session match: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the session matches: %w", contextError(ctx, err))
	}
	return out, nil
}
