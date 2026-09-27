package store

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"testing"
)

// Migration 0019 adds the memory tables: the card notes and their full-text index, file claims, and
// the MCP-server and skill registries (0019_memory.sql). These tests prove the facts the rest of
// Phase 7 leans on: that a card has one note and not a list, that the note's indexed text follows it
// through an edit and out through a delete, and that a claim is a claim on a thing and not a lock on
// it.

const memoryNow = int64(1758960000000)

// memorySchema opens a database with every migration applied and two cards in one project, and
// returns the open writer. The cards are what the notes and the claims hang from, so the foreign
// keys and the cascade have something real to act on.
func memorySchema(ctx context.Context, t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "marshal.db")
	writer, err := openPool(ctx, dsn(path, false), writers)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	if err := migrate(ctx, writer, embeddedFiles(t), log); err != nil {
		t.Fatalf("apply every migration: %v", err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := writer.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("seed %q: %v", query, err)
		}
	}
	exec(`INSERT INTO projects (id, name, repo_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"proj1", "Small repo", "/tmp/small-repo", memoryNow, memoryNow)
	exec(`INSERT INTO boards (id, project_id, columns_json) VALUES (?, ?, ?)`, "board1", "proj1", "[]")
	for _, card := range []struct {
		id     string
		number int
		title  string
	}{{"card1", 1, "Add a health check"}, {"card2", 2, "Write the runbook"}} {
		exec(`INSERT INTO cards (id, project_id, number, board_id, title, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			card.id, "proj1", card.number, "board1", card.title, memoryNow, memoryNow)
	}
	return writer
}

// searchIndex asks the full-text index which notes match a term. It is the one read in this file
// written as plain SQL rather than through the generated queries, because sqlc cannot parse FTS5's
// MATCH operator (that is why the note search lives in internal/memory).
func searchIndex(ctx context.Context, t *testing.T, writer *sql.DB, term string) []string {
	t.Helper()
	rows, err := writer.QueryContext(ctx, `SELECT id FROM notes_fts WHERE notes_fts MATCH ? ORDER BY id`, term)
	if err != nil {
		t.Fatalf("search the note index for %q: %v", term, err)
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

// A note is one per card, which is what the Notes tab is: (project_id, card_id) is the unique key
// among card notes (the partial index `notes_card_note_unique`, scoped to `kind = 'card_note'` so a
// project's lessons - which never have a card_id - are a separate uniqueness world), and the second
// write of a card's note replaces the first rather than adding to it. The id and the birthday stay,
// so a note that is edited is the same note.
func TestANoteIsOnePerCard(t *testing.T) {
	ctx := testContext(t)
	writer := memorySchema(ctx, t)
	first := `INSERT INTO notes (id, project_id, card_id, author, body, created_at, updated_at)
		VALUES ('note-1', 'proj1', 'card1', 'person', 'Goal: add a health check.', ?, ?)`
	if _, err := writer.ExecContext(ctx, first, memoryNow, memoryNow); err != nil {
		t.Fatalf("write the first note: %v", err)
	}
	second := `INSERT INTO notes (id, project_id, card_id, author, body, created_at, updated_at)
		VALUES ('note-2', 'proj1', 'card1', 'agent', 'Goal: add a health check and a probe.', ?, ?)
		ON CONFLICT (project_id, card_id) WHERE kind = 'card_note' DO UPDATE SET
			author = excluded.author, body = excluded.body, updated_at = excluded.updated_at`
	if _, err := writer.ExecContext(ctx, second, memoryNow+1000, memoryNow+1000); err != nil {
		t.Fatalf("write the second note for the same card: %v", err)
	}
	var count int
	if err := writer.QueryRowContext(ctx, `SELECT count(*) FROM notes WHERE card_id = 'card1'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("a card has %d notes after being saved twice, want 1", count)
	}
	var id, author, body string
	var createdAt int64
	if err := writer.QueryRowContext(ctx,
		`SELECT id, author, body, created_at FROM notes WHERE card_id = 'card1'`).
		Scan(&id, &author, &body, &createdAt); err != nil {
		t.Fatal(err)
	}
	if id != "note-1" || createdAt != memoryNow {
		t.Errorf("the note came out as id %q born %d, want the first write's note-1 and %d", id, createdAt, memoryNow)
	}
	if author != "agent" || body != "Goal: add a health check and a probe." {
		t.Errorf("the edit left author %q and body %q, want the second write's", author, body)
	}
}

// The index follows the note through an edit and out through a delete, which is the whole job of the
// migration's triggers. Without them a search would answer with text the note no longer holds.
func TestTheNoteIndexFollowsTheNote(t *testing.T) {
	ctx := testContext(t)
	writer := memorySchema(ctx, t)
	if _, err := writer.ExecContext(ctx,
		`INSERT INTO notes (id, project_id, card_id, author, body, created_at, updated_at)
		 VALUES ('note-1', 'proj1', 'card1', 'person', 'Goal: add a health check.', ?, ?)`,
		memoryNow, memoryNow); err != nil {
		t.Fatalf("write a note: %v", err)
	}
	if got := searchIndex(ctx, t, writer, "health"); len(got) != 1 || got[0] != "note-1" {
		t.Fatalf("searching for the note's own words found %v, want [note-1]", got)
	}
	if _, err := writer.ExecContext(ctx,
		`UPDATE notes SET body = 'Goal: add a liveness probe.', updated_at = ? WHERE id = 'note-1'`,
		memoryNow+1000); err != nil {
		t.Fatalf("edit the note: %v", err)
	}
	if got := searchIndex(ctx, t, writer, "health"); len(got) != 0 {
		t.Errorf("the edited note is still found by the word it no longer holds: %v", got)
	}
	if got := searchIndex(ctx, t, writer, "liveness"); len(got) != 1 || got[0] != "note-1" {
		t.Errorf("searching for the edited note's words found %v, want [note-1]", got)
	}
	if _, err := writer.ExecContext(ctx, `DELETE FROM notes WHERE id = 'note-1'`); err != nil {
		t.Fatalf("delete the note: %v", err)
	}
	if got := searchIndex(ctx, t, writer, "liveness"); len(got) != 0 {
		t.Errorf("a deleted note is still indexed: %v", got)
	}
}

// A card deleted takes its note, its claims, and the note's indexed text with it. The claims go
// through file_claims' own foreign key; the note goes through the cards_notes_delete trigger the
// migration writes for exactly this case (card_id carries no foreign key of its own, so a lesson -
// which has no card - is never rejected for having none), and the indexed text follows the note
// through notes_fts_delete underneath it.
func TestDeletingACardTakesItsMemory(t *testing.T) {
	ctx := testContext(t)
	writer := memorySchema(ctx, t)
	if _, err := writer.ExecContext(ctx,
		`INSERT INTO notes (id, project_id, card_id, author, body, created_at, updated_at)
		 VALUES ('note-1', 'proj1', 'card1', 'person', 'Goal: add a liveness probe.', ?, ?)`,
		memoryNow, memoryNow); err != nil {
		t.Fatalf("write a note: %v", err)
	}
	if _, err := writer.ExecContext(ctx,
		`INSERT INTO file_claims (card_id, project_id, path_or_package, claimed_at) VALUES ('card1', 'proj1', 'internal/api', ?)`,
		memoryNow); err != nil {
		t.Fatalf("claim a path: %v", err)
	}
	if _, err := writer.ExecContext(ctx, `DELETE FROM cards WHERE id = 'card1'`); err != nil {
		t.Fatalf("delete the card: %v", err)
	}
	for _, table := range []string{"notes", "file_claims"} {
		var count int
		if err := writer.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("%s still holds %d rows for the deleted card, want 0", table, count)
		}
	}
	if got := searchIndex(ctx, t, writer, "liveness"); len(got) != 0 {
		t.Errorf("a deleted card's note is still indexed: %v", got)
	}
}

// A claim is a claim on a thing, not a lock on it: two cards may hold the same path, which is what
// the overlap warning is about (B7.2). The same card claiming it twice is one claim, so its age is
// how long the card has had it and not how many times it asked.
func TestTwoCardsMayClaimOnePathAndOneCardOnlyOnce(t *testing.T) {
	ctx := testContext(t)
	writer := memorySchema(ctx, t)
	claim := `INSERT INTO file_claims (card_id, project_id, path_or_package, claimed_at)
		VALUES (?, 'proj1', 'internal/api', ?) ON CONFLICT (card_id, path_or_package) DO NOTHING`
	for _, card := range []string{"card1", "card2"} {
		if _, err := writer.ExecContext(ctx, claim, card, memoryNow); err != nil {
			t.Fatalf("claim the path for %s: %v", card, err)
		}
	}
	if _, err := writer.ExecContext(ctx, claim, "card1", memoryNow+5000); err != nil {
		t.Fatalf("claim the path again for card1: %v", err)
	}
	var count int
	if err := writer.QueryRowContext(ctx, `SELECT count(*) FROM file_claims`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("two cards claiming one path twice between them left %d claims, want 2", count)
	}
	var claimedAt int64
	if err := writer.QueryRowContext(ctx,
		`SELECT claimed_at FROM file_claims WHERE card_id = 'card1'`).Scan(&claimedAt); err != nil {
		t.Fatal(err)
	}
	if claimedAt != memoryNow {
		t.Errorf("the re-claim moved card1's claim to %d, want it left at %d", claimedAt, memoryNow)
	}
	// The overlap read asks who else holds it, so the card asking is left out of its own answer.
	var others []string
	rows, err := writer.QueryContext(ctx,
		`SELECT card_id FROM file_claims WHERE project_id = 'proj1' AND path_or_package = 'internal/api' AND card_id <> 'card1'`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var card string
		if err := rows.Scan(&card); err != nil {
			t.Fatal(err)
		}
		others = append(others, card)
	}
	if len(others) != 1 || others[0] != "card2" {
		t.Errorf("the other card on the path came out as %v, want [card2]", others)
	}
}

// The four tables and the index are on the schema after the migration runs. A missing one would only
// show up as a failed query much later, so they are named here.
func TestMigration0019AddsTheMemoryTables(t *testing.T) {
	ctx := testContext(t)
	writer := memorySchema(ctx, t)
	for _, name := range []string{"notes", "file_claims", "mcp_servers", "skills", "notes_fts"} {
		var sql string
		err := writer.QueryRowContext(ctx,
			`SELECT sql FROM sqlite_master WHERE name = ?`, name).Scan(&sql)
		if err != nil {
			t.Errorf("the %s table is not on the schema: %v", name, err)
			continue
		}
		if name == "notes_fts" && !bytes.Contains([]byte(sql), []byte("VIRTUAL TABLE")) {
			t.Errorf("notes_fts is %q, want a virtual table", sql)
		}
	}
	// A server is looked up by the name a role names it with, and the schema refuses two rows with
	// one name.
	if _, err := writer.ExecContext(ctx,
		`INSERT INTO mcp_servers (id, name, transport_json) VALUES ('m1', 'github', '{}')`); err != nil {
		t.Fatalf("save a server: %v", err)
	}
	if _, err := writer.ExecContext(ctx,
		`INSERT INTO mcp_servers (id, name, transport_json) VALUES ('m2', 'github', '{}')`); err == nil {
		t.Error("two MCP servers share a name, want the schema to refuse the second")
	}
}
