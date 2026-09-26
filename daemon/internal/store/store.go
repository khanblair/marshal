// Package store owns the SQLite database: the connections, the migrations, and the typed queries
// that sqlc generates from the SQL in queries/ into the db package.
//
// The database is in WAL mode. All writes go through ONE connection (Store.Write, one
// transaction at a time) and reads use a small pool of read-only connections (Store.Queries and
// Store.Read), so a reader never waits for a writer and a writer never sees "database is locked".
//
// Rules for callers:
//
//   - No transaction wraps a model call or any other call to the outside. A transaction holds the
//     only writer, so anything slow inside it stalls every other write. Do the slow work first,
//     then write the result.
//   - Every time in the database is INTEGER Unix milliseconds in UTC, and every id is TEXT. Convert
//     to protocol.Timestamp at the edge with Timestamp and OptionalTimestamp.
//   - Client tokens are stored only as a hash, made by HashToken.
//   - Migrations are forward only. Each module adds its own numbered file to migrations/.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/khanblair/marshal/daemon/internal/store/db"

	// The driver registers itself as "sqlite". It is pure Go, so the daemon cross-compiles.
	_ "modernc.org/sqlite"
)

const (
	// DefaultReaders is how many read connections Open makes unless WithReaders says otherwise.
	DefaultReaders = 4
	// busyTimeoutMillis is how long a connection waits for a lock before it gives up.
	busyTimeoutMillis = 5000
	// The database holds token hashes and settings, so only its owner may read it. SQLite gives
	// the -wal and -shm files the same mode as the database file.
	fileMode = 0o600
	dirMode  = 0o700
	// writers is fixed: SQLite allows one writer at a time, so a second connection would only wait.
	writers = 1
)

// Store is an open database. It is safe for use by many goroutines.
type Store struct {
	writer *sql.DB
	reader *sql.DB
	reads  *db.Queries

	closeOnce sync.Once
	closeErr  error
}

// Option changes how Open works.
type Option func(*options)

type options struct {
	readers int
	log     *slog.Logger
}

// WithReaders sets the size of the read pool. The default is DefaultReaders. A value below 1 is
// treated as 1.
func WithReaders(n int) Option {
	return func(o *options) { o.readers = max(n, 1) }
}

// WithLogger sets where migration progress is logged. The default is to log nothing.
func WithLogger(log *slog.Logger) Option {
	return func(o *options) {
		if log != nil {
			o.log = log
		}
	}
}

// Open opens the database at path, creating the file and its folder if they are missing, and
// brings the schema up to date. The writer is opened and migrated before any reader exists, so a
// reader never sees a half-migrated schema.
func Open(ctx context.Context, path string, opts ...Option) (*Store, error) {
	cfg := options{readers: DefaultReaders, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for _, opt := range opts {
		opt(&cfg)
	}
	if err := prepareFile(path); err != nil {
		return nil, err
	}
	writer, err := openPool(ctx, dsn(path, false), writers)
	if err != nil {
		return nil, fmt.Errorf("open the database writer: %w", err)
	}
	files, err := embeddedMigrations()
	if err != nil {
		return nil, errors.Join(err, writer.Close())
	}
	if err := migrate(ctx, writer, files, cfg.log); err != nil {
		return nil, errors.Join(err, writer.Close())
	}
	reader, err := openPool(ctx, dsn(path, true), cfg.readers)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("open the database readers: %w", err), writer.Close())
	}
	return &Store{writer: writer, reader: reader, reads: db.New(reader)}, nil
}

// Close closes every connection. It is safe to call more than once.
func (s *Store) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = errors.Join(s.reader.Close(), s.writer.Close())
	})
	return s.closeErr
}

// Ping checks that the database answers. It uses the read pool, so a write in progress does not
// delay it.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.reader.PingContext(ctx); err != nil {
		return fmt.Errorf("ping the database: %w", err)
	}
	return nil
}

// Queries returns the generated queries on the read pool. Each call runs by itself, so two calls
// may see different data. Use Read when several reads must agree.
func (s *Store) Queries() *db.Queries { return s.reads }

// Read runs fn in one read-only transaction, so every query in it sees the same data even while
// a write commits. Nothing fn does can change the database.
func (s *Store) Read(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.reader.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("begin a read: %w", contextError(ctx, err))
	}
	defer func() { _ = tx.Rollback() }()
	return contextError(ctx, fn(db.New(tx)))
}

// contextError keeps the error a caller's own context caused, so a request the client hung up on is
// not reported as a failure of the daemon. SQLite reports a read that its context cancelled as its
// own "interrupted" code, which does not match context.Canceled by itself: the API layer logs
// context.Canceled at debug and everything else as an internal error, so without this a person
// closing a tab would put an ERROR line in the daemon's log.
func contextError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%w (%w)", ctxErr, err)
	}
	return err
}

// Write runs fn in one transaction on the writer. It commits when fn returns nil and rolls back
// when fn returns an error or panics, and it returns fn's error as it is, so errors.Is still
// works on it. Writes wait for each other, so keep fn short and never call a model or anything
// outside the daemon in it.
func (s *Store) Write(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin a write: %w", contextError(ctx, err))
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := fn(db.New(tx)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit a write: %w", err)
	}
	committed = true
	return nil
}

// IsNotFound reports whether err says a query that expected one row found none.
func IsNotFound(err error) bool { return errors.Is(err, sql.ErrNoRows) }

// dsn is the path with the settings every connection gets. They are set here, not after opening,
// because the pool makes new connections whenever it likes. A reader is also made read-only.
func dsn(path string, readOnly bool) string {
	pragmas := []string{
		fmt.Sprintf("busy_timeout(%d)", busyTimeoutMillis),
		"journal_mode(WAL)",
		"foreign_keys(1)",
		"synchronous(NORMAL)",
	}
	var query strings.Builder
	for _, pragma := range pragmas {
		query.WriteString("_pragma=" + pragma + "&")
	}
	if readOnly {
		query.WriteString("_query_only=1")
	} else {
		// A write takes the write lock when it begins instead of when it first writes.
		query.WriteString("_txlock=immediate")
	}
	return path + "?" + query.String()
}

// openPool opens a pool of at most conns connections and checks that one of them works.
func openPool(ctx context.Context, dataSource string, conns int) (*sql.DB, error) {
	pool, err := sql.Open("sqlite", dataSource)
	if err != nil {
		return nil, err
	}
	pool.SetMaxOpenConns(conns)
	pool.SetMaxIdleConns(conns)
	if err := pool.PingContext(ctx); err != nil {
		return nil, errors.Join(err, pool.Close())
	}
	return pool, nil
}

// prepareFile makes the folder and an empty database file that only the owner can use. SQLite
// would create the file with the default mode, which other users may read.
func prepareFile(path string) error {
	if path == "" {
		return errors.New("open the database: the path is empty")
	}
	if strings.Contains(path, "?") {
		return fmt.Errorf("open the database %s: the path cannot contain a question mark", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return fmt.Errorf("make the database folder: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, fileMode)
	if err != nil {
		return fmt.Errorf("create the database file: %w", err)
	}
	return file.Close()
}
