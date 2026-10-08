package store

import (
	"bytes"
	"log/slog"
	"testing"
)

// Migration 0034 gives the cards already stuck on a stopped agent the reason they were never given
// (0034_stopped_card_reason.sql), and leaves every other card as it was.

func TestMigration0034ExplainsAStoppedCardAndNothingElse(t *testing.T) {
	ctx := testContext(t)
	writer, _ := seedThrough0008(ctx, t)
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	if err := migrate(ctx, writer, migrationsThrough(t, 33), log); err != nil {
		t.Fatalf("apply migrations through 0033: %v", err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := writer.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("seed %q: %v", query, err)
		}
	}
	card := func(id string, number int, reasonKind string) {
		exec(`INSERT INTO cards (id, project_id, number, board_id, title, state, needs_reason_kind, created_at, updated_at)
			VALUES (?, 'proj1', ?, 'board1', ?, 'needs', ?, 1, 1)`, id, number, id, reasonKind)
	}
	session := func(id, cardID, state string) {
		exec(`INSERT INTO sessions (id, card_id, agent_kind, state, last_active_at, created_at, updated_at)
			VALUES (?, ?, 'claude', ?, 1, 1, 1)`, id, cardID, state)
	}
	card("stopped-no-reason", 10, "")
	session("s1", "stopped-no-reason", "stopped")
	card("dragged-by-a-person", 11, "")
	card("stopped-with-reason", 12, "question")
	session("s3", "stopped-with-reason", "stopped")
	card("awake-no-reason", 13, "")
	session("s4", "awake-no-reason", "awake")

	if err := migrate(ctx, writer, embeddedFiles(t), log); err != nil {
		t.Fatalf("apply the remaining migrations: %v", err)
	}
	reasonOf := func(id string) (kind, text string) {
		t.Helper()
		if err := writer.QueryRowContext(ctx,
			`SELECT needs_reason_kind, needs_reason_text FROM cards WHERE id = ?`, id).Scan(&kind, &text); err != nil {
			t.Fatalf("read card %s: %v", id, err)
		}
		return kind, text
	}
	if kind, text := reasonOf("stopped-no-reason"); kind != "stuck" || text != "The agent stopped unexpectedly. Resume it to continue." {
		t.Errorf("the stopped card's reason is %q %q, want stuck with the sentence", kind, text)
	}
	for _, id := range []string{"dragged-by-a-person", "awake-no-reason"} {
		if kind, _ := reasonOf(id); kind != "" {
			t.Errorf("card %s was given the reason %q, want none", id, kind)
		}
	}
	if kind, _ := reasonOf("stopped-with-reason"); kind != "question" {
		t.Errorf("a card with its own reason now says %q, want question", kind)
	}
}
