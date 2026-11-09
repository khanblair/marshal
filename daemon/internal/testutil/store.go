package testutil

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/store"
)

// A fixture needs an isolated database, not a repeated test of every historical migration.
// Build this from the actual migrations once per test process. It lives only in memory, so a
// changed migration or a new test binary can never reuse an old schema from disk or CI caches.
var fixtureSchema = sync.OnceValues(buildFixtureSchema) //nolint:gochecknoglobals // immutable schema shared by test fixtures

// OpenStore gives each fixture a private copy of the fully migrated empty database. Existing
// files still go through the normal open path, preserving real restart and persistence checks.
// Migration tests use store.Open directly and exercise the entire migration chain.
func OpenStore(ctx context.Context, path string, opts ...store.Option) (*store.Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		return store.Open(ctx, path, opts...)
	}
	image, err := fixtureSchema()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	// Exclusive creation keeps a second fixture from overwriting a database already in use.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	_, writeErr := file.Write(image)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return nil, err
	}
	return store.Open(ctx, path, opts...)
}

func buildFixtureSchema() ([]byte, error) {
	dir, err := os.MkdirTemp("", "marshal-test-schema-")
	if err != nil {
		return nil, err
	}
	// A closed SQLite database has checkpointed its WAL, so the single file is the whole image.
	defer func() { _ = os.RemoveAll(dir) }()
	const setupTimeout = time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	defer cancel()
	path := filepath.Join(dir, "schema.db")
	database, err := store.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := database.Close(); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}
