package store

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

// Migration 0009 rebuilds session_events so an event belongs to a card or a chat
// (0009_chat_history.sql). SQLite cannot change a column's constraints in place, so the table is
// dropped and made again, which loses anything the old table carried that the new one does not
// restate: its rows, its uniqueness, and its foreign key. These tests prove each of the three.

const historyNow = int64(1700000000000)

// seedThrough0008 opens a database on the schema as it was before 0009 and writes a project, a
// board, a card, its session, and three events of its history, returning the open writer.
func seedThrough0008(ctx context.Context, t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "marshal.db")
	writer, err := openPool(ctx, dsn(path, false), writers)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	if err := migrate(ctx, writer, migrationsThrough(t, 8), log); err != nil {
		t.Fatalf("apply migrations through 0008: %v", err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := writer.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("seed %q: %v", query, err)
		}
	}
	exec(`INSERT INTO projects (id, name, repo_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"proj1", "Small repo", "/tmp/small-repo", historyNow, historyNow)
	exec(`INSERT INTO boards (id, project_id, columns_json) VALUES (?, ?, ?)`, "board1", "proj1", "[]")
	exec(`INSERT INTO cards (id, project_id, number, board_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"card1", "proj1", 1, "board1", "Add a health check", historyNow, historyNow)
	exec(`INSERT INTO sessions (id, card_id, agent_kind, last_active_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"session1", "card1", "claude", historyNow, historyNow, historyNow)
	for seq, kind := range []string{"user", "tool_call", "agent"} {
		exec(`INSERT INTO session_events (id, card_id, session_id, seq, kind, state, summary, detail_json, log_ref, created_at)
		      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			"event"+kind, "card1", "session1", seq+1, kind, "ok", "summary of "+kind, `{"id":"`+kind+`"}`, "ref", historyNow+int64(seq))
	}
	return writer, path
}

// The rows of a card's history are copied as they are: every column, in the same order.
func TestMigration0009KeepsACardsHistoryAsItWas(t *testing.T) {
	ctx := testContext(t)
	writer, _ := seedThrough0008(ctx, t)
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	if err := migrate(ctx, writer, embeddedFiles(t), log); err != nil {
		t.Fatalf("apply the remaining migrations: %v", err)
	}

	rows, err := writer.QueryContext(ctx,
		`SELECT id, card_id, chat_id, session_id, seq, kind, state, summary, detail_json, log_ref, created_at
		 FROM session_events ORDER BY seq`)
	if err != nil {
		t.Fatalf("read the history: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var kinds []string
	for rows.Next() {
		var id, cardID, sessionID, kind, state, summary, detail, ref string
		var chatID sql.NullString
		var seq, at int64
		if err := rows.Scan(&id, &cardID, &chatID, &sessionID, &seq, &kind, &state, &summary, &detail, &ref, &at); err != nil {
			t.Fatal(err)
		}
		if id != "event"+kind || cardID != "card1" || chatID.Valid || sessionID != "session1" || state != "ok" ||
			summary != "summary of "+kind || detail != `{"id":"`+kind+`"}` || ref != "ref" || at != historyNow+seq-1 {
			t.Errorf("event %d after the rebuild = %s %s %v %s %s %s %s %s %d", seq, id, cardID, chatID, sessionID, kind, state, summary, detail, at)
		}
		kinds = append(kinds, kind)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(kinds, ",") != "user,tool_call,agent" {
		t.Errorf("the history after the rebuild = %v, want its three events in order", kinds)
	}
}

// Each owner numbers its events from one, and the rules the old table had are still there: an event
// belongs to a card or a chat and never both or neither, a card's events number uniquely, and an
// event cannot name a card that is not there.
func TestMigration0009ConstrainsWhoOwnsAnEvent(t *testing.T) {
	ctx := testContext(t)
	writer, _ := seedThrough0008(ctx, t)
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	if err := migrate(ctx, writer, embeddedFiles(t), log); err != nil {
		t.Fatalf("apply the remaining migrations: %v", err)
	}
	insert := func(id, cardID string, chatID any, sessionID string, seq int) error {
		_, err := writer.ExecContext(ctx,
			`INSERT INTO session_events (id, card_id, chat_id, session_id, seq, kind, created_at) VALUES (?, ?, ?, ?, ?, 'user', ?)`,
			id, cardID, chatID, sessionID, seq, historyNow)
		return err
	}
	for _, chat := range []string{"chat1", "chat2"} {
		if _, err := writer.ExecContext(ctx,
			`INSERT INTO chats (id, project_id, title, last_active_at, created_at, updated_at) VALUES (?, 'proj1', 'New chat', ?, ?, ?)`,
			chat, historyNow, historyNow, historyNow); err != nil {
			t.Fatalf("seed a chat: %v", err)
		}
		if _, err := writer.ExecContext(ctx,
			`INSERT INTO sessions (id, card_id, chat_id, agent_kind, last_active_at, created_at, updated_at) VALUES (?, '', ?, 'claude', ?, ?, ?)`,
			"session-"+chat, chat, historyNow, historyNow, historyNow); err != nil {
			t.Fatalf("seed a chat's session: %v", err)
		}
	}

	// Two chats each start at one, and a chat's numbers do not touch a card's.
	if err := insert("a1", "", "chat1", "session-chat1", 1); err != nil {
		t.Errorf("a chat's first event: %v", err)
	}
	if err := insert("b1", "", "chat2", "session-chat2", 1); err != nil {
		t.Errorf("another chat's first event: %v", err)
	}
	if err := insert("a2", "", "chat1", "session-chat1", 2); err != nil {
		t.Errorf("a chat's second event: %v", err)
	}

	refused := map[string]error{
		"a chat's seq twice":          insert("a1again", "", "chat1", "session-chat1", 1),
		"a card's seq twice":          insert("c1again", "card1", nil, "session1", 1),
		"both a card and a chat":      insert("both", "card1", "chat1", "session1", 9),
		"neither a card nor a chat":   insert("neither", "", nil, "session1", 9),
		"a card that is not there":    insert("ghost", "nocard", nil, "session1", 9),
		"a chat that is not there":    insert("nochat", "", "nochat", "session-chat1", 9),
		"a session that is not there": insert("nosession", "", "chat1", "nosession", 9),
		"a seq below one":             insert("zero", "", "chat1", "session-chat1", 0),
	}
	for what, err := range refused {
		if err == nil {
			t.Errorf("the table took an event with %s", what)
		}
	}
	if err := insert("c4", "card1", nil, "session1", 4); err != nil {
		t.Errorf("a card's next event: %v", err)
	}

	// Deleting a chat takes its events and leaves the card's and the other chat's.
	if _, err := writer.ExecContext(ctx, `DELETE FROM chats WHERE id = 'chat1'`); err != nil {
		t.Fatalf("delete a chat: %v", err)
	}
	count := func(where string) int {
		t.Helper()
		var n int
		if err := writer.QueryRowContext(ctx, `SELECT count(*) FROM session_events WHERE `+where).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`chat_id = 'chat1'`); n != 0 {
		t.Errorf("%d events of the deleted chat are left", n)
	}
	if n := count(`chat_id = 'chat2'`); n != 1 {
		t.Errorf("the other chat has %d events, want its 1", n)
	}
	if n := count(`card_id = 'card1'`); n != 4 {
		t.Errorf("the card has %d events, want its 4", n)
	}
}
