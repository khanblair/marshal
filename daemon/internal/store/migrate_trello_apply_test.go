package store

import (
	"context"
	"database/sql"
	"testing"
)

// Migration 0036 (0036_trello_apply.sql) adds what the Trello apply flow remembers: each card's Trello
// side, how checklist lines and comments match, dismissed changes, and when a board was read. These
// tests prove the rules the sync leans on, on the schema linksSchema builds.

func countTrelloRows(ctx context.Context, t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

func TestATrelloSnapshotBelongsToOneRealCardAndGoesWithIt(t *testing.T) {
	ctx := context.Background()
	db := linksSchema(ctx, t)
	insert := `INSERT INTO trello_snapshots (card_id, trello_card_id, from_trello, snapshot_json, fetched_at)
		VALUES (?, ?, ?, '{}', ?)`

	if _, err := db.ExecContext(ctx, insert, "no-such-card", "t1", 1, linksNow); err == nil {
		t.Error("a snapshot of a card that is not there was accepted")
	}
	if _, err := db.ExecContext(ctx, insert, "card1", "t1", 2, linksNow); err == nil {
		t.Error("a from_trello of 2 was accepted, want only 0 or 1")
	}
	if _, err := db.ExecContext(ctx, insert, "card1", "t1", 1, linksNow); err != nil {
		t.Fatalf("snapshot a card: %v", err)
	}
	if _, err := db.ExecContext(ctx, insert, "card1", "t2", 0, linksNow); err == nil {
		t.Error("a second snapshot of the same card was accepted, want one per card")
	}

	if _, err := db.ExecContext(ctx, `DELETE FROM cards WHERE id = ?`, "card1"); err != nil {
		t.Fatalf("delete the card: %v", err)
	}
	if left := countTrelloRows(ctx, t, db, `SELECT count(*) FROM trello_snapshots`); left != 0 {
		t.Errorf("%d snapshots survived their card", left)
	}
}

func TestATrelloItemLinkMatchesOneItemBothWaysAndGoesWithItsCard(t *testing.T) {
	ctx := context.Background()
	db := linksSchema(ctx, t)
	insert := `INSERT INTO trello_item_links (card_id, kind, marshal_id, trello_id) VALUES (?, ?, ?, ?)`

	if _, err := db.ExecContext(ctx, insert, "card1", "label", "m1", "t1"); err == nil {
		t.Error("a link of an unknown kind was accepted, want checklist, item, or comment")
	}
	for _, kind := range []string{"checklist", "item", "comment"} {
		if _, err := db.ExecContext(ctx, insert, "card1", kind, "m1", "t1"); err != nil {
			t.Fatalf("link a %s: %v", kind, err)
		}
	}
	if _, err := db.ExecContext(ctx, insert, "card1", "item", "m1", "t9"); err == nil {
		t.Error("one Marshal item was linked to two Trello items on a card")
	}
	if _, err := db.ExecContext(ctx, insert, "card1", "item", "m2", "t1"); err == nil {
		t.Error("two Marshal items were linked to one Trello item on a card, which would send a tick to the wrong line")
	}
	// The same Trello id is a different thing on another card, so it may match there.
	if _, err := db.ExecContext(ctx, insert, "card2", "item", "m1", "t1"); err != nil {
		t.Errorf("the same ids on another card were refused: %v", err)
	}

	if _, err := db.ExecContext(ctx, `DELETE FROM cards WHERE id = ?`, "card1"); err != nil {
		t.Fatalf("delete the card: %v", err)
	}
	if left := countTrelloRows(ctx, t, db, `SELECT count(*) FROM trello_item_links WHERE card_id = ?`, "card1"); left != 0 {
		t.Errorf("%d links survived their card", left)
	}
	if kept := countTrelloRows(ctx, t, db, `SELECT count(*) FROM trello_item_links WHERE card_id = ?`, "card2"); kept != 1 {
		t.Errorf("the other card has %d links, want its one", kept)
	}
}

func TestDismissedTrelloChangesAndSyncStateBelongToTheirProject(t *testing.T) {
	ctx := context.Background()
	db := linksSchema(ctx, t)
	dismiss := `INSERT INTO trello_dismissed (project_id, change_id, dismissed_at) VALUES (?, ?, ?)`
	state := `INSERT INTO trello_sync_state (project_id, board_name, fetched_at) VALUES (?, ?, ?)`

	if _, err := db.ExecContext(ctx, dismiss, "no-such-project", "c1", linksNow); err == nil {
		t.Error("a dismissed change of a project that is not there was accepted")
	}
	if _, err := db.ExecContext(ctx, state, "no-such-project", "Board", linksNow); err == nil {
		t.Error("the sync state of a project that is not there was accepted")
	}
	if _, err := db.ExecContext(ctx, dismiss, "proj1", "c1", linksNow); err != nil {
		t.Fatalf("dismiss a change: %v", err)
	}
	if _, err := db.ExecContext(ctx, dismiss, "proj1", "c1", linksNow); err == nil {
		t.Error("the same change was dismissed twice, want one row for it")
	}
	if _, err := db.ExecContext(ctx, state, "proj1", "Board", linksNow); err != nil {
		t.Fatalf("save the sync state: %v", err)
	}
	if _, err := db.ExecContext(ctx, state, "proj1", "Board", linksNow); err == nil {
		t.Error("two sync states of one project were accepted, want one")
	}

	if _, err := db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, "proj1"); err != nil {
		t.Fatalf("delete the project: %v", err)
	}
	for _, table := range []string{"trello_dismissed", "trello_sync_state"} {
		if left := countTrelloRows(ctx, t, db, `SELECT count(*) FROM `+table); left != 0 {
			t.Errorf("%d rows of %s survived their project", left, table)
		}
	}
}
