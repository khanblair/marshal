package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// testTimeout stops a stuck test instead of hanging the suite. A deadlock on the single writer
// connection would otherwise never end.
const testTimeout = 10 * time.Second

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	t.Cleanup(cancel)
	return ctx
}

// openAt opens a database at path and closes it when the test ends. Tests always use a file,
// because an in-memory database has no WAL and gives every connection its own empty copy.
func openAt(t *testing.T, path string, opts ...Option) *Store {
	t.Helper()
	s, err := Open(testContext(t), path, opts...)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func openTemp(t *testing.T, opts ...Option) *Store {
	t.Helper()
	return openAt(t, filepath.Join(t.TempDir(), "marshal.db"), opts...)
}

// pragma reads one setting from a connection.
func pragma(t *testing.T, conn *sql.Conn, name string) string {
	t.Helper()
	var value string
	if err := conn.QueryRowContext(testContext(t), "PRAGMA "+name).Scan(&value); err != nil {
		t.Fatalf("PRAGMA %s: %v", name, err)
	}
	return value
}

// holdConns takes n connections from a pool at once, so the pool has to make n different ones.
func holdConns(t *testing.T, pool *sql.DB, n int) []*sql.Conn {
	t.Helper()
	conns := make([]*sql.Conn, 0, n)
	for range n {
		conn, err := pool.Conn(testContext(t))
		if err != nil {
			t.Fatalf("take a connection: %v", err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		conns = append(conns, conn)
	}
	return conns
}

func TestEveryConnectionGetsThePragmas(t *testing.T) {
	s := openTemp(t)
	tests := []struct {
		name  string
		pool  *sql.DB
		conns int
		want  map[string]string
	}{
		{
			name: "the writer", pool: s.writer, conns: 1,
			want: map[string]string{
				"journal_mode": "wal", "foreign_keys": "1", "synchronous": "1",
				"busy_timeout": "5000", "query_only": "0",
			},
		},
		{
			name: "every reader", pool: s.reader, conns: DefaultReaders,
			want: map[string]string{
				"journal_mode": "wal", "foreign_keys": "1", "synchronous": "1",
				"busy_timeout": "5000", "query_only": "1",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i, conn := range holdConns(t, tt.pool, tt.conns) {
				for name, want := range tt.want {
					if got := pragma(t, conn, name); got != want {
						t.Errorf("connection %d: PRAGMA %s = %q, want %q", i, name, got, want)
					}
				}
			}
		})
	}
}

func TestPoolSizes(t *testing.T) {
	tests := []struct {
		name        string
		opts        []Option
		wantReaders int
	}{
		{name: "default readers", wantReaders: DefaultReaders},
		{name: "two readers", opts: []Option{WithReaders(2)}, wantReaders: 2},
		{name: "a value below one becomes one", opts: []Option{WithReaders(0)}, wantReaders: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := openTemp(t, tt.opts...)
			if got := s.writer.Stats().MaxOpenConnections; got != 1 {
				t.Errorf("writer connections = %d, want exactly 1", got)
			}
			if got := s.reader.Stats().MaxOpenConnections; got != tt.wantReaders {
				t.Errorf("reader connections = %d, want %d", got, tt.wantReaders)
			}
		})
	}
}

func TestOpenPreparesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "folder", "marshal.db")
	s := openAt(t, path)
	if err := s.Ping(testContext(t)); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the database file is missing: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != fileMode {
		t.Errorf("file mode = %v, want %v", info.Mode().Perm(), os.FileMode(fileMode))
	}
}

func TestOpenRefusesBadPaths(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), fileMode); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "empty", path: "", want: "path is empty"},
		{name: "question mark", path: filepath.Join(t.TempDir(), "a?b.db"), want: "question mark"},
		{name: "folder cannot be made", path: filepath.Join(blocker, "child", "marshal.db"), want: "database folder"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Open(testContext(t), tt.path)
			if err == nil {
				_ = s.Close()
				t.Fatal("Open succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	s := openTemp(t)
	first := s.Close()
	if second := s.Close(); !errors.Is(second, first) && second != first {
		t.Errorf("second Close = %v, want the first result %v", second, first)
	}
	if err := s.Ping(testContext(t)); err == nil {
		t.Error("Ping after Close succeeded, want an error")
	}
}

func TestForeignKeysAreEnforced(t *testing.T) {
	s := openTemp(t)
	err := s.Write(testContext(t), func(q *db.Queries) error {
		return q.CreateDevice(testContext(t), db.CreateDeviceParams{
			ID: "d1", UserID: "no-such-user", Name: "Laptop", Kind: "web", TokenHash: HashToken("t"), PairedAt: 1,
		})
	})
	if err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Fatalf("Write = %v, want a foreign key error", err)
	}
}

func TestReadersKeepWorkingDuringAWrite(t *testing.T) {
	s := openTemp(t)
	ctx := testContext(t)
	if err := s.Write(ctx, func(q *db.Queries) error {
		return q.SetSetting(ctx, db.SetSettingParams{Key: "theme", ValueJSON: `"dark"`})
	}); err != nil {
		t.Fatal(err)
	}

	inWrite, release, written := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		written <- s.Write(ctx, func(q *db.Queries) error {
			if err := q.SetSetting(ctx, db.SetSettingParams{Key: "theme", ValueJSON: `"light"`}); err != nil {
				return err
			}
			close(inWrite)
			<-release
			return nil
		})
	}()
	<-inWrite

	// Every reader connection is new while the write is open, so this also shows that making a
	// connection does not need the write lock.
	const readers = DefaultReaders
	var ready, done sync.WaitGroup
	ready.Add(readers)
	done.Add(readers)
	seen := make(chan string, readers)
	for range readers {
		go func() {
			defer done.Done()
			err := s.Read(ctx, func(q *db.Queries) error {
				ready.Done()
				ready.Wait() // Hold this connection until all four are open at once.
				value, err := q.GetSetting(ctx, "theme")
				seen <- value
				return err
			})
			if err != nil {
				t.Errorf("Read during a write: %v", err)
			}
		}()
	}
	done.Wait()
	close(seen)
	for value := range seen {
		if value != `"dark"` {
			t.Errorf("a reader saw %s before the commit, want the old value", value)
		}
	}

	close(release)
	if err := <-written; err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := s.Queries().GetSetting(ctx, "theme")
	if err != nil || got != `"light"` {
		t.Errorf("after the commit GetSetting = %q, %v, want the new value", got, err)
	}
}

var errSentinel = errors.New("write failed on purpose")

func TestAFailedWriteRollsBack(t *testing.T) {
	s := openTemp(t)
	ctx := testContext(t)
	set := func(q *db.Queries) error {
		return q.SetSetting(ctx, db.SetSettingParams{Key: "k", ValueJSON: "1"})
	}
	tests := []struct {
		name string
		fn   func(q *db.Queries) error
		want error
	}{
		{name: "error", fn: func(q *db.Queries) error { return errors.Join(set(q), errSentinel) }, want: errSentinel},
		{name: "panic", fn: func(q *db.Queries) error {
			if err := set(q); err != nil {
				return err
			}
			panic("boom")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := func() (err error) {
				defer func() {
					if r := recover(); r != nil {
						err = nil
					}
				}()
				return s.Write(ctx, tt.fn)
			}()
			if !errors.Is(err, tt.want) {
				t.Errorf("Write = %v, want %v", err, tt.want)
			}
			if _, err := s.Queries().GetSetting(ctx, "k"); !IsNotFound(err) {
				t.Errorf("GetSetting after a failed write = %v, want not found", err)
			}
			// The writer must be free again, or this hangs until the test timeout.
			if err := s.Write(ctx, func(*db.Queries) error { return nil }); err != nil {
				t.Errorf("Write after a failed write: %v", err)
			}
		})
	}
}

func TestReadsCannotWrite(t *testing.T) {
	s := openTemp(t)
	ctx := testContext(t)
	params := db.SetSettingParams{Key: "k", ValueJSON: "1"}
	if err := s.Queries().SetSetting(ctx, params); err == nil {
		t.Error("Queries().SetSetting succeeded on a read connection")
	}
	err := s.Read(ctx, func(q *db.Queries) error { return q.SetSetting(ctx, params) })
	if err == nil {
		t.Error("Read let fn write")
	}
}

func TestDataSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marshal.db")
	ctx := testContext(t)
	first, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := first.EnsureOwner(ctx, db.CreateUserParams{ID: "u1", Name: "Ada", CreatedAt: 5, UpdatedAt: 5})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Write(ctx, func(q *db.Queries) error {
		return q.SetSetting(ctx, db.SetSettingParams{Key: "theme", ValueJSON: `"dark"`})
	}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second := openAt(t, path)
	got, err := second.Queries().GetOwner(ctx)
	if err != nil || got != owner {
		t.Errorf("GetOwner after reopen = %+v, %v, want %+v", got, err, owner)
	}
	if value, err := second.Queries().GetSetting(ctx, "theme"); err != nil || value != `"dark"` {
		t.Errorf("GetSetting after reopen = %q, %v", value, err)
	}
}
