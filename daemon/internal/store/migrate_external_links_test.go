package store

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"testing"
)

// Migration 0023 adds the external links: which outside item a card is (0023_external_links.sql,
// B8.2, docs/architecture.md section 10). These tests prove the two facts the Trello sync leans on:
// that one outside item belongs to exactly one card (so a replayed delivery cannot import a second
// one), and that a card takes its links with it when it goes.

const linksNow = int64(1758960000000)

// linksSchema opens a database with every migration applied and two cards in one project, so a link
// has real cards to hang from.
func linksSchema(ctx context.Context, t *testing.T) *sql.DB {
	t.Helper()
	writer, err := openPool(ctx, dsn(filepath.Join(t.TempDir(), "marshal.db"), false), writers)
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
		"proj1", "Small repo", "/tmp/small-repo", linksNow, linksNow)
	exec(`INSERT INTO boards (id, project_id, columns_json) VALUES (?, ?, ?)`, "board1", "proj1", "[]")
	for _, card := range []struct {
		id     string
		number int
	}{{"card1", 1}, {"card2", 2}} {
		exec(`INSERT INTO cards (id, project_id, number, board_id, title, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			card.id, "proj1", card.number, "board1", "Card", linksNow, linksNow)
	}
	return writer
}

// TestAnOutsideItemBelongsToOneCard proves the unique index and not just the primary key: the same
// Trello card cannot be linked to two Marshal cards, which is what a duplicate delivery racing the
// first one hits.
func TestAnOutsideItemBelongsToOneCard(t *testing.T) {
	ctx := context.Background()
	db := linksSchema(ctx, t)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO external_links (card_id, kind, external_id) VALUES (?, ?, ?)`,
		"card1", "trello", "trello-card-1"); err != nil {
		t.Fatalf("link the first card: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO external_links (card_id, kind, external_id) VALUES (?, ?, ?)`,
		"card2", "trello", "trello-card-1"); err == nil {
		t.Fatal("a second card claimed a Trello card that was already linked")
	}
	// The same card may link to a different item of the same kind, but only by replacing its own:
	// the pair is the primary key, so the second insert for card1 overwrites.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO external_links (card_id, kind, external_id) VALUES (?, ?, ?)
		 ON CONFLICT (card_id, kind) DO UPDATE SET external_id = excluded.external_id`,
		"card1", "trello", "trello-card-2"); err != nil {
		t.Fatalf("relink the first card: %v", err)
	}
	var externalID string
	if err := db.QueryRowContext(ctx,
		`SELECT external_id FROM external_links WHERE card_id = ? AND kind = ?`, "card1", "trello").
		Scan(&externalID); err != nil {
		t.Fatalf("read the link back: %v", err)
	}
	if externalID != "trello-card-2" {
		t.Errorf("the link reads %q, want the item it was replaced with", externalID)
	}
	var links int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM external_links WHERE card_id = ?`, "card1").Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 1 {
		t.Errorf("card1 has %d links, want one: a relink replaces, it does not add", links)
	}
}

// TestACardTakesItsLinksWithIt proves the cascade: a deleted card leaves no link pointing at nothing,
// the same way its note and its file claims go with it.
func TestACardTakesItsLinksWithIt(t *testing.T) {
	ctx := context.Background()
	db := linksSchema(ctx, t)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO external_links (card_id, kind, external_id) VALUES (?, ?, ?)`,
		"card1", "trello", "trello-card-1"); err != nil {
		t.Fatalf("link the card: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM cards WHERE id = ?`, "card1"); err != nil {
		t.Fatalf("delete the card: %v", err)
	}
	var left int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM external_links WHERE kind = ?`, "trello").Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d links survived their card", left)
	}
}

// TestALinkNeedsACard proves the foreign key: an outside item cannot be linked to a card that is not
// there, so a delivery for a card Marshal has never made cannot invent a link.
func TestALinkNeedsACard(t *testing.T) {
	ctx := context.Background()
	db := linksSchema(ctx, t)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO external_links (card_id, kind, external_id) VALUES (?, ?, ?)`,
		"no-such-card", "trello", "trello-card-1"); err == nil {
		t.Fatal("a link was made to a card that does not exist")
	}
}
