package store

import (
	"bytes"
	"database/sql"
	"io/fs"
	"log/slog"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/khanblair/marshal/daemon/internal/store/db"
)

var migrationName = regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.sql$`)

func embeddedFiles(t *testing.T) fs.FS {
	t.Helper()
	files, err := embeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func migrationCount(t *testing.T, files fs.FS) int {
	t.Helper()
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	return len(names)
}

// appliedVersions reads goose's own record of which migrations ran.
func appliedVersions(t *testing.T, s *Store) []int64 {
	t.Helper()
	rows, err := s.reader.QueryContext(testContext(t), "SELECT version_id FROM goose_db_version WHERE version_id > 0 ORDER BY version_id")
	if err != nil {
		t.Fatalf("read the version table: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var versions []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return versions
}

func tableNames(t *testing.T, s *Store) map[string]bool {
	t.Helper()
	rows, err := s.reader.QueryContext(testContext(t), "SELECT name FROM sqlite_master WHERE type = 'table'")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	names := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names[name] = true
	}
	return names
}

func TestMigrationsRunOnOpenAndAgainOnReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marshal.db")
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	want := migrationCount(t, embeddedFiles(t))

	first, err := Open(testContext(t), path, WithLogger(log))
	if err != nil {
		t.Fatal(err)
	}
	versions := appliedVersions(t, first)
	if len(versions) != want {
		t.Fatalf("applied %v, want %d migrations", versions, want)
	}
	for _, table := range []string{"settings", "users", "devices", "user_progress"} {
		if !tableNames(t, first)[table] {
			t.Errorf("table %s is missing after the first open", table)
		}
	}
	if got := strings.Count(logs.String(), "applied a database migration"); got != want {
		t.Errorf("logged %d applied migrations on the first open, want %d", got, want)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	logs.Reset()
	second := openAt(t, path, WithLogger(log))
	if got := appliedVersions(t, second); !equalInts(got, versions) {
		t.Errorf("versions after reopen = %v, want %v", got, versions)
	}
	if logs.Len() != 0 {
		t.Errorf("the second open applied migrations again: %s", logs.String())
	}
}

func equalInts(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestEmbeddedMigrationsAreForwardOnly(t *testing.T) {
	if err := checkForwardOnly(embeddedFiles(t)); err != nil {
		t.Fatal(err)
	}
}

func TestCheckForwardOnlyFindsADownSection(t *testing.T) {
	up := "-- +goose Up\nCREATE TABLE a (id TEXT);\n"
	tests := []struct {
		name    string
		files   fstest.MapFS
		wantErr bool
	}{
		{name: "only up", files: fstest.MapFS{"0001_a.sql": {Data: []byte(up)}}},
		{name: "a comment that mentions down", files: fstest.MapFS{
			"0001_a.sql": {Data: []byte(up + "-- there is no goose down here\n")},
		}},
		{name: "a down section", wantErr: true, files: fstest.MapFS{
			"0001_a.sql": {Data: []byte(up + "-- +goose Down\nDROP TABLE a;\n")},
		}},
		{name: "lower case and no space", wantErr: true, files: fstest.MapFS{
			"0001_a.sql": {Data: []byte(up + "--+goose down\nDROP TABLE a;\n")},
		}},
		{name: "indented", wantErr: true, files: fstest.MapFS{
			"0001_a.sql": {Data: []byte(up)},
			"0002_b.sql": {Data: []byte(up + "   -- +goose Down\n")},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkForwardOnly(tt.files)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkForwardOnly = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMigrationFileNames(t *testing.T) {
	names, err := fs.Glob(embeddedFiles(t), "*")
	if err != nil {
		t.Fatal(err)
	}
	last := 0
	for _, name := range names {
		match := migrationName.FindStringSubmatch(name)
		if match == nil {
			t.Errorf("%s is not named NNNN_words.sql", name)
			continue
		}
		version, _ := strconv.Atoi(match[1])
		if version <= last {
			t.Errorf("%s repeats or goes back past version %d", name, last)
		}
		last = version
	}
}

// migrationsThrough returns the embedded migrations up to and including maxVersion, so a test can
// apply an earlier schema, put data in it, then apply the rest and check the data survived.
func migrationsThrough(t *testing.T, maxVersion int) fs.FS {
	t.Helper()
	all := embeddedFiles(t)
	names, err := fs.Glob(all, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	partial := fstest.MapFS{}
	for _, name := range names {
		match := migrationName.FindStringSubmatch(name)
		if match == nil {
			t.Fatalf("%s is not named NNNN_words.sql", name)
		}
		version, _ := strconv.Atoi(match[1])
		if version > maxVersion {
			continue
		}
		data, err := fs.ReadFile(all, name)
		if err != nil {
			t.Fatal(err)
		}
		partial[name] = &fstest.MapFile{Data: data}
	}
	return partial
}

// TestMigration0007KeepsSessionEventsWhenSessionsIsRebuilt proves the risk 0007_chats.sql's own
// comment calls out: rebuilding `sessions` (to drop the NOT NULL on card_id) drops a table that
// `session_events` points at with ON DELETE CASCADE. A database that already has history in it
// when it upgrades must not lose that history to the rebuild.
func TestMigration0007KeepsSessionEventsWhenSessionsIsRebuilt(t *testing.T) {
	ctx := testContext(t)
	path := filepath.Join(t.TempDir(), "marshal.db")
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	writer, err := openPool(ctx, dsn(path, false), writers)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Close() }()

	// Land the database on the schema exactly as it was before 0007, then populate it: a project, a
	// board, a card, a card-owned session, and one event of the history that session recorded.
	if err := migrate(ctx, writer, migrationsThrough(t, 6), log); err != nil {
		t.Fatalf("apply migrations through 0006: %v", err)
	}
	now := int64(1700000000000)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := writer.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("seed %q: %v", query, err)
		}
	}
	exec(`INSERT INTO projects (id, name, repo_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"proj1", "Small repo", "/tmp/small-repo", now, now)
	exec(`INSERT INTO boards (id, project_id, columns_json) VALUES (?, ?, ?)`,
		"board1", "proj1", "[]")
	exec(`INSERT INTO cards (id, project_id, number, board_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"card1", "proj1", 1, "board1", "Add a health check", now, now)
	exec(`INSERT INTO sessions (id, card_id, agent_kind, last_active_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"session1", "card1", "claude", now, now, now)
	exec(`INSERT INTO session_events (id, card_id, session_id, seq, kind, summary, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"event1", "card1", "session1", 1, "user", "wait for the pause", now)

	// Now bring the database fully up to date: 0007 rebuilds sessions underneath the event just
	// seeded, and 0008 runs after it.
	if err := migrate(ctx, writer, embeddedFiles(t), log); err != nil {
		t.Fatalf("apply the remaining migrations: %v", err)
	}

	var cardID, chatID sql.NullString
	row := writer.QueryRowContext(ctx, "SELECT card_id, chat_id FROM sessions WHERE id = ?", "session1")
	if err := row.Scan(&cardID, &chatID); err != nil {
		t.Fatalf("session1 did not survive the rebuild: %v", err)
	}
	if !cardID.Valid || cardID.String != "card1" {
		t.Errorf("session1.card_id after the rebuild = %+v, want card1", cardID)
	}
	if chatID.Valid {
		t.Errorf("session1.chat_id after the rebuild = %+v, want NULL", chatID)
	}

	var sessionID, summary string
	row = writer.QueryRowContext(ctx, "SELECT session_id, summary FROM session_events WHERE id = ?", "event1")
	if err := row.Scan(&sessionID, &summary); err != nil {
		t.Fatalf("event1 did not survive the sessions rebuild (foreign key cascade wiped it): %v", err)
	}
	if sessionID != "session1" || summary != "wait for the pause" {
		t.Errorf("event1 after the rebuild = {session_id: %s, summary: %s}, want session1 / unchanged", sessionID, summary)
	}

	var count int
	row = writer.QueryRowContext(ctx, "SELECT count(*) FROM session_events")
	if err := row.Scan(&count); err != nil || count != 1 {
		t.Errorf("session_events row count after the rebuild = %d (err %v), want exactly the 1 seeded", count, err)
	}
}

// TestSessionsCardCascadeDeleteTrigger proves the trigger 0007_chats.sql adds in place of the
// native foreign key it removed from sessions.card_id (a plain FK cannot coexist with the
// empty-string sentinel a chat-owned session stores there): deleting a card must still take its
// session with it, the same guarantee the old REFERENCES ... ON DELETE CASCADE gave.
func TestSessionsCardCascadeDeleteTrigger(t *testing.T) {
	ctx := testContext(t)
	s := openAt(t, filepath.Join(t.TempDir(), "marshal.db"))
	now := int64(1700000000000)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := s.writer.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("seed %q: %v", query, err)
		}
	}
	exec(`INSERT INTO projects (id, name, repo_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"proj1", "Small repo", "/tmp/small-repo", now, now)
	exec(`INSERT INTO boards (id, project_id, columns_json) VALUES (?, ?, ?)`, "board1", "proj1", "[]")
	exec(`INSERT INTO cards (id, project_id, number, board_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"card1", "proj1", 1, "board1", "Add a health check", now, now)
	exec(`INSERT INTO sessions (id, card_id, agent_kind, last_active_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"session1", "card1", "claude", now, now, now)
	exec(`INSERT INTO session_events (id, card_id, session_id, seq, kind, summary, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"event1", "card1", "session1", 1, "user", "hello", now)

	if err := s.Write(ctx, func(q *db.Queries) error {
		_, err := q.DeleteCard(ctx, "card1")
		return err
	}); err != nil {
		t.Fatalf("DeleteCard: %v", err)
	}

	var count int
	row := s.reader.QueryRowContext(ctx, "SELECT count(*) FROM sessions WHERE id = ?", "session1")
	if err := row.Scan(&count); err != nil || count != 0 {
		t.Errorf("sessions row count for the deleted card's session = %d (err %v), want 0", count, err)
	}
	row = s.reader.QueryRowContext(ctx, "SELECT count(*) FROM session_events WHERE id = ?", "event1")
	if err := row.Scan(&count); err != nil || count != 0 {
		t.Errorf("session_events row count for the deleted card's session = %d (err %v), want 0", count, err)
	}
}

func TestABrokenMigrationStopsOpen(t *testing.T) {
	ctx := testContext(t)
	writer, err := openPool(ctx, dsn(filepath.Join(t.TempDir(), "marshal.db"), false), writers)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Close() }()
	files := fstest.MapFS{"0001_bad.sql": {Data: []byte("-- +goose Up\nCREATE TABLE ok (id TEXT);\nNOT SQL AT ALL;\n")}}
	err = migrate(ctx, writer, files, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	if err == nil || !strings.Contains(err.Error(), "apply the migrations") {
		t.Fatalf("migrate = %v, want an apply error", err)
	}
	// The failed migration ran in a transaction, so its first statement is undone too.
	var count int
	row := writer.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE name = 'ok'")
	if err := row.Scan(&count); err != nil || count != 0 {
		t.Errorf("table ok exists after the migration failed (count %d, err %v)", count, err)
	}
}
