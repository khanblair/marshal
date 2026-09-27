package store

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// Migration 0021 adds the full-text index over a card's past sessions, the second half of what
// docs/architecture.md section 10 asks for: "Session search uses SQLite full-text search over
// `session_events.summary` and card notes" (docs/backend-checklist.md B7.4, build-plan task 7.10).
// These tests prove the facts the session search leans on: that the index follows an event through
// an edit and out through a delete, that a chat's events are left out of it, and that an event with
// no summary is out too.

// seedSession adds a card's session, which is what a session event points at. The sessions table
// holds one row per card and one per chat, and its own foreign keys are what these events hang from,
// so a real row has to be there before an event can be written.
func seedSession(ctx context.Context, t *testing.T, writer *sql.DB, id, card string) {
	t.Helper()
	if _, err := writer.ExecContext(ctx,
		`INSERT INTO sessions (id, card_id, agent_kind, last_active_at, created_at, updated_at)
		 VALUES (?, ?, 'claude', ?, ?, ?)`,
		id, card, memoryNow, memoryNow, memoryNow); err != nil {
		t.Fatalf("seed the session %s: %v", id, err)
	}
}

// seedSessionEvent adds one stored event of a card's session: the one line the index is built over.
func seedSessionEvent(ctx context.Context, t *testing.T, writer *sql.DB, id, card, session, summary string) {
	t.Helper()
	if _, err := writer.ExecContext(ctx,
		`INSERT INTO session_events (id, card_id, session_id, seq, kind, summary, created_at)
		 VALUES (?, ?, ?, (SELECT COALESCE(MAX(seq), 0) + 1 FROM session_events WHERE card_id = ?), 'message', ?, ?)`,
		id, card, session, card, summary, memoryNow); err != nil {
		t.Fatalf("seed the event %s: %v", id, err)
	}
}

// searchEventIndex asks the session index which events match a term. Like searchIndex above it is
// plain SQL, because sqlc cannot parse FTS5's MATCH operator (that is why the search lives in
// internal/store/search_sessions.go as hand-written SQL).
func searchEventIndex(ctx context.Context, t *testing.T, writer *sql.DB, term string) []string {
	t.Helper()
	rows, err := writer.QueryContext(ctx, `SELECT id FROM session_events_fts WHERE session_events_fts MATCH ? ORDER BY id`, term)
	if err != nil {
		t.Fatalf("search the session index for %q: %v", term, err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

// An event's summary is indexed as soon as the event is stored, and the index follows it through an
// edit and out through a delete. Without the triggers a search would answer with words the event no
// longer holds, or with an event that is gone.
func TestTheSessionIndexFollowsTheEvent(t *testing.T) {
	ctx := testContext(t)
	writer := memorySchema(ctx, t)
	seedSession(ctx, t, writer, "session-1", "card1")
	seedSessionEvent(ctx, t, writer, "event-1", "card1", "session-1", "Added the health check endpoint")

	if got := searchEventIndex(ctx, t, writer, "health"); len(got) != 1 || got[0] != "event-1" {
		t.Fatalf("searching for the event's own words found %v, want [event-1]", got)
	}
	if _, err := writer.ExecContext(ctx,
		`UPDATE session_events SET summary = 'Added the liveness probe endpoint' WHERE id = 'event-1'`); err != nil {
		t.Fatalf("edit the event: %v", err)
	}
	if got := searchEventIndex(ctx, t, writer, "health"); len(got) != 0 {
		t.Errorf("the edited event is still found by the word it no longer holds: %v", got)
	}
	if got := searchEventIndex(ctx, t, writer, "liveness"); len(got) != 1 || got[0] != "event-1" {
		t.Errorf("searching for the edited event's words found %v, want [event-1]", got)
	}
	if _, err := writer.ExecContext(ctx, `DELETE FROM session_events WHERE id = 'event-1'`); err != nil {
		t.Fatalf("delete the event: %v", err)
	}
	if got := searchEventIndex(ctx, t, writer, "liveness"); len(got) != 0 {
		t.Errorf("a deleted event is still indexed: %v", got)
	}
}

// Only a card's events are indexed. A chat's events store an empty card (0009) and are found through
// the Chats kind of the same search, so indexing them here would answer one chat twice; an event with
// no summary could never be found, and the index would carry a row per event for nothing.
func TestTheSessionIndexHoldsOnlyACardsEventsWithASummary(t *testing.T) {
	ctx := testContext(t)
	writer := memorySchema(ctx, t)
	seedSession(ctx, t, writer, "session-1", "card1")
	seedSessionEvent(ctx, t, writer, "event-1", "card1", "session-1", "")
	if got := searchEventIndex(ctx, t, writer, "health"); len(got) != 0 {
		t.Errorf("an event with no summary is indexed: %v", got)
	}
	// A chat's event: the card side is the empty string and the chat side is set.
	if _, err := writer.ExecContext(ctx,
		`INSERT INTO chats (id, project_id, title, created_at, updated_at, last_active_at)
		 VALUES ('chat1', 'proj1', 'Health check question', ?, ?, ?)`,
		memoryNow, memoryNow, memoryNow); err != nil {
		t.Fatalf("seed the chat: %v", err)
	}
	if _, err := writer.ExecContext(ctx,
		`INSERT INTO sessions (id, chat_id, agent_kind, last_active_at, created_at, updated_at)
		 VALUES ('session-2', 'chat1', 'claude', ?, ?, ?)`, memoryNow, memoryNow, memoryNow); err != nil {
		t.Fatalf("seed the chat's session: %v", err)
	}
	if _, err := writer.ExecContext(ctx,
		`INSERT INTO session_events (id, chat_id, session_id, seq, kind, summary, created_at)
		 VALUES ('chat-event', 'chat1', 'session-2', 1, 'message', 'a health check elsewhere', ?)`,
		memoryNow); err != nil {
		t.Fatalf("seed the chat's event: %v", err)
	}
	if got := searchEventIndex(ctx, t, writer, "health"); len(got) != 0 {
		t.Errorf("a chat's event is in the session index: %v", got)
	}
}

// A card deleted takes its events - and their indexed text - with it. The events go through the
// trigger migration 0009 writes; the indexed rows through the one this migration adds for exactly
// this case, which removes them by the card whether or not the per-row delete trigger fires.
func TestDeletingACardTakesItsSessionEventsFromTheIndex(t *testing.T) {
	ctx := testContext(t)
	writer := memorySchema(ctx, t)
	seedSession(ctx, t, writer, "session-1", "card1")
	seedSessionEvent(ctx, t, writer, "event-1", "card1", "session-1", "Added a liveness probe")
	if _, err := writer.ExecContext(ctx, `DELETE FROM cards WHERE id = 'card1'`); err != nil {
		t.Fatalf("delete the card: %v", err)
	}
	var events int
	if err := writer.QueryRowContext(ctx, `SELECT count(*) FROM session_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 0 {
		t.Errorf("the deleted card's events are still stored: %d rows", events)
	}
	if got := searchEventIndex(ctx, t, writer, "liveness"); len(got) != 0 {
		t.Errorf("a deleted card's event is still indexed: %v", got)
	}
}

// The index is on the schema after the migration runs, and it is the virtual table the query joins
// back to `session_events` through. A missing table would only show up as a failed search later.
func TestMigration0021AddsTheSessionSearchIndex(t *testing.T) {
	ctx := testContext(t)
	writer := memorySchema(ctx, t)
	var sql string
	if err := writer.QueryRowContext(ctx,
		`SELECT sql FROM sqlite_master WHERE name = 'session_events_fts'`).Scan(&sql); err != nil {
		t.Fatalf("the session index is not on the schema: %v", err)
	}
	for _, piece := range []string{"VIRTUAL TABLE", "id UNINDEXED", "card_id UNINDEXED"} {
		if !strings.Contains(sql, piece) {
			t.Errorf("session_events_fts is %q, want it to hold %q", sql, piece)
		}
	}
}
