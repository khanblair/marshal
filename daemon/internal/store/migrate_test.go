package store

import (
	"bytes"
	"io/fs"
	"log/slog"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
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
