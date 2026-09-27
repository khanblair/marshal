package memory

import (
	"context"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Search over a card's past sessions: the stored one-line summaries of what a session did
// (docs/architecture.md sections 10 and 3, docs/backend-checklist.md B7.4, build-plan task 7.10).
//
// Section 10 puts session search beside note search and says how both are done: "Session search
// uses SQLite full-text search over `session_events.summary` and card notes". The notes half is
// SearchNotes above; this is the session half, reading the `session_events_fts` index (migration
// 0021) which holds one row per stored event that has a summary.
//
// It is here rather than in the module that writes the events because section 3 gives session
// search to the memory module, and because a search for "where did we do this before" is the same
// question as a search of the notes: the memory module answers both from the vault and its indexes,
// and the palette reads the two through one call.

// sessionExcerptRun is how much of a stored summary a search answer shows. A summary is already one
// line, so this is a guard against a line written by something that did not keep it short rather
// than the usual case.
const sessionExcerptRun = 200

// SearchSessions returns a project's past session events whose summaries match a person's words,
// best match first. It reads the full-text index and not the events, so it is one query rather than
// a walk of every card's history; the card the event belongs to is read to name it, which is what
// SearchNotes does for a note's card too.
//
// A query with no words in it matches nothing and answers an empty list rather than every event,
// for the same reason a note search does: a search that answers everything is not a search.
//
// The CardID, Key, and Title of each hit are filled here, because the card is what this module
// reads; ProjectID comes from the card, and ProjectName is left to the caller, which is already
// walking the project the event belongs to (internal/search).
//
// The Excerpt is the summary as it was stored, trimmed and not shortened. A summary is already the
// one line the daemon kept for a moment of a session (migration 0006), so there is nothing to cut
// down, and the caller that orders the hits reads the same text the index matched against.
func (s *Service) SearchSessions(ctx context.Context, projectID, query string, limit int) ([]protocol.SessionHit, error) {
	match := matchExpression(query)
	if match == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	limit = min(limit, MaxSearchLimit)
	rows, err := s.store.SearchSessionEvents(ctx, projectID, match, limit)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.SessionHit, 0, len(rows))
	for _, row := range rows {
		card, err := s.cards.Card(ctx, row.CardID)
		if err != nil {
			// An event whose card is gone is a row the foreign key should not have allowed, and it
			// is not worth failing a search over: it is skipped and the rest are answered. This is
			// the same rule a note search follows.
			continue
		}
		out = append(out, protocol.SessionHit{
			CardID: card.ID, Key: card.Key, Title: card.Title, ProjectID: card.ProjectID,
			Excerpt: strings.TrimSpace(row.Summary),
			At:      protocol.NewTimestamp(time.UnixMilli(row.CreatedAt).UTC()),
		})
	}
	return out, nil
}
