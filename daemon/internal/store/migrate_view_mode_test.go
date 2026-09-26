package store

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Migration 0012 adds the view a card's agent runs in to its session (0012_view_mode.sql). These
// tests prove that a database that already has sessions keeps them in the chat view, that the
// column takes only the two views and only a card's session may be in the terminal one, and that a
// session that stops goes back to the chat view through the one write that ends every session.

// migratedFrom0011 opens a database at the schema before 0012 with one card session and one chat
// session, then applies the rest and returns the writer.
func migratedFrom0011(ctx context.Context, t *testing.T) *sql.DB {
	t.Helper()
	writer, _ := seedThrough0008(ctx, t)
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	if err := migrate(ctx, writer, migrationsThrough(t, 11), log); err != nil {
		t.Fatalf("apply migrations through 0011: %v", err)
	}
	if _, err := writer.ExecContext(ctx,
		`INSERT INTO chats (id, project_id, title, last_active_at, created_at, updated_at) VALUES ('chat1', 'proj1', 'New chat', ?, ?, ?)`,
		historyNow, historyNow, historyNow); err != nil {
		t.Fatalf("seed a chat: %v", err)
	}
	if _, err := writer.ExecContext(ctx,
		`INSERT INTO sessions (id, card_id, chat_id, agent_kind, last_active_at, created_at, updated_at) VALUES ('session-chat1', '', 'chat1', 'claude', ?, ?, ?)`,
		historyNow, historyNow, historyNow); err != nil {
		t.Fatalf("seed a chat's session: %v", err)
	}
	if err := migrate(ctx, writer, embeddedFiles(t), log); err != nil {
		t.Fatalf("apply the remaining migrations: %v", err)
	}
	return writer
}

func TestMigration0012PutsEveryExistingSessionInTheChatView(t *testing.T) {
	ctx := testContext(t)
	writer := migratedFrom0011(ctx, t)
	rows, err := writer.QueryContext(ctx, `SELECT id, view_mode FROM sessions ORDER BY id`)
	if err != nil {
		t.Fatalf("read the sessions: %v", err)
	}
	defer func() { _ = rows.Close() }()
	seen := 0
	for rows.Next() {
		var id, mode string
		if err := rows.Scan(&id, &mode); err != nil {
			t.Fatal(err)
		}
		if mode != "chat" {
			t.Errorf("session %s is in the %q view after the migration, want chat", id, mode)
		}
		seen++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != 2 {
		t.Errorf("found %d sessions, want the card's and the chat's", seen)
	}
}

func TestMigration0012OnlyAcceptsTheTwoViewsAndOnlyACardCanUseTheTerminal(t *testing.T) {
	ctx := testContext(t)
	writer := migratedFrom0011(ctx, t)
	set := func(id, mode string) error {
		_, err := writer.ExecContext(ctx, `UPDATE sessions SET view_mode = ? WHERE id = ?`, mode, id)
		return err
	}
	if err := set("session1", "terminal"); err != nil {
		t.Errorf("a card's session in the terminal view: %v", err)
	}
	if err := set("session1", "chat"); err != nil {
		t.Errorf("a card's session back in the chat view: %v", err)
	}
	if err := set("session-chat1", "chat"); err != nil {
		t.Errorf("a chat's session in the chat view: %v", err)
	}
	for name, err := range map[string]error{
		"a view that does not exist":     set("session1", "hologram"),
		"an empty view":                  set("session1", ""),
		"a chat's session in a terminal": set("session-chat1", "terminal"),
	} {
		if err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

// A session that stops is back in the chat view, and every other state keeps the view it is in.
func TestASessionThatStopsIsBackInTheChatView(t *testing.T) {
	ctx := testContext(t)
	writer := migratedFrom0011(ctx, t)
	queries := db.New(writer)
	write := func(state string) {
		t.Helper()
		if _, err := queries.UpdateSessionRuntime(ctx, db.UpdateSessionRuntimeParams{
			State: state, AgentSessionID: "agent-1", LastActiveAt: historyNow, UpdatedAt: historyNow, ID: "session1",
		}); err != nil {
			t.Fatalf("write %s: %v", state, err)
		}
	}
	view := func() string {
		t.Helper()
		row, err := queries.GetSession(ctx, "session1")
		if err != nil {
			t.Fatal(err)
		}
		return row.ViewMode
	}
	setView := func(mode string) {
		t.Helper()
		if n, err := queries.UpdateSessionView(ctx, db.UpdateSessionViewParams{ViewMode: mode, UpdatedAt: historyNow, ID: "session1"}); err != nil || n != 1 {
			t.Fatalf("UpdateSessionView = %d, %v", n, err)
		}
	}
	setView("terminal")
	for _, state := range []string{"awake", "working", "asleep", "waking"} {
		write(state)
		if got := view(); got != "terminal" {
			t.Errorf("a session written %s left the terminal view for %q", state, got)
		}
	}
	write("stopped")
	if got := view(); got != "chat" {
		t.Errorf("a session that stopped is in the %q view, want chat", got)
	}
}
