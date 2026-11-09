package testutil_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

func TestFixtureDatabasesAreIsolatedAndReopenPreservesData(t *testing.T) {
	ctx := t.Context()
	firstPath := filepath.Join(t.TempDir(), "first.db")
	first, err := testutil.OpenStore(ctx, firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Write(ctx, func(q *db.Queries) error {
		return q.SetSetting(ctx, db.SetSettingParams{Key: "marker", ValueJSON: `"first"`})
	}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := testutil.OpenStore(ctx, filepath.Join(t.TempDir(), "second.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	if _, err := second.Queries().GetSetting(ctx, "marker"); !store.IsNotFound(err) {
		t.Fatalf("another fixture inherited data: %v", err)
	}
	reopened, err := testutil.OpenStore(ctx, firstPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	if value, err := reopened.Queries().GetSetting(ctx, "marker"); err != nil || value != `"first"` {
		t.Fatalf("reopen lost data: %q %v", value, err)
	}
	info, err := os.Stat(firstPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("database permissions: %v %v", info, err)
	}
}

func TestFixtureOpenHonorsCancellationAndDoesNotReplaceExistingFiles(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	path := filepath.Join(t.TempDir(), "cancelled.db")
	if _, err := testutil.OpenStore(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled open: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("cancelled fixture created a file")
	}
	if err := os.WriteFile(path, []byte("existing content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.OpenStore(t.Context(), path); err == nil {
		t.Fatal("invalid existing database should fail")
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "existing content" {
		t.Fatal("existing file was overwritten")
	}
}

func TestFixtureSchemaSupportsConcurrentIsolatedDatabases(t *testing.T) {
	for range 8 {
		t.Run("independent", func(t *testing.T) {
			t.Parallel()
			database, err := testutil.OpenStore(t.Context(), filepath.Join(t.TempDir(), "marshal.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = database.Close() }()
			if err := database.Write(t.Context(), func(q *db.Queries) error {
				return q.SetSetting(t.Context(), db.SetSettingParams{Key: "marker", ValueJSON: `true`})
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func BenchmarkFixtureDatabaseSetup(b *testing.B) {
	for _, cached := range []bool{false, true} {
		name := "migrations"
		if cached {
			name = "schema_copy"
		}
		b.Run(name, func(b *testing.B) {
			root := b.TempDir()
			for i := 0; i < b.N; i++ {
				path := filepath.Join(root, "marshal.db")
				open := store.Open
				if cached {
					open = testutil.OpenStore
				}
				database, err := open(b.Context(), path)
				if err != nil {
					b.Fatal(err)
				}
				if err := database.Close(); err != nil {
					b.Fatal(err)
				}
				if err := os.Remove(path); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
