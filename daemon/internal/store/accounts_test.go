package store

import (
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/store/db"
)

func TestHashToken(t *testing.T) {
	tests := []struct{ name, token, want string }{
		// The published SHA-256 test vector for "abc".
		{"known vector", "abc", "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
		{"empty", "", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HashToken(tt.token); got != tt.want {
				t.Errorf("HashToken(%q) = %s, want %s", tt.token, got, tt.want)
			}
		})
	}
}

func TestSettings(t *testing.T) {
	s := openTemp(t)
	ctx := testContext(t)
	if _, err := s.Queries().GetSetting(ctx, "theme"); !IsNotFound(err) {
		t.Fatalf("GetSetting on an empty database = %v, want not found", err)
	}
	for _, value := range []string{`"dark"`, `"light"`} {
		err := s.Write(ctx, func(q *db.Queries) error {
			return q.SetSetting(ctx, db.SetSettingParams{Key: "theme", ValueJSON: value})
		})
		if err != nil {
			t.Fatalf("SetSetting %s: %v", value, err)
		}
		if got, err := s.Queries().GetSetting(ctx, "theme"); err != nil || got != value {
			t.Errorf("GetSetting = %q, %v, want %q", got, err, value)
		}
	}
}

func TestEnsureOwner(t *testing.T) {
	s := openTemp(t)
	ctx := testContext(t)
	first, err := s.EnsureOwner(ctx, db.CreateUserParams{ID: "u1", Name: "Ada", Email: "ada@example.com", CreatedAt: 10, UpdatedAt: 10})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != "u1" || first.Name != "Ada" || first.CreatedAt != 10 {
		t.Errorf("first owner = %+v", first)
	}
	again, err := s.EnsureOwner(ctx, db.CreateUserParams{ID: "u2", Name: "Someone else", CreatedAt: 20, UpdatedAt: 20})
	if err != nil {
		t.Fatal(err)
	}
	if again != first {
		t.Errorf("second EnsureOwner = %+v, want the first owner %+v", again, first)
	}
	var users int
	if err := s.reader.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&users); err != nil || users != 1 {
		t.Errorf("users = %d, %v, want exactly 1", users, err)
	}
}

const ownerID = "u1"

func newOwner(t *testing.T, s *Store) db.User {
	t.Helper()
	owner, err := s.EnsureOwner(testContext(t), db.CreateUserParams{ID: ownerID, Name: "Ada", CreatedAt: 1, UpdatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	return owner
}

func device(id, kind, token string, pairedAt int64) db.CreateDeviceParams {
	return db.CreateDeviceParams{ID: id, UserID: ownerID, Name: "Device " + id, Kind: kind, TokenHash: HashToken(token), PairedAt: pairedAt}
}

func TestDeviceLifeCycle(t *testing.T) {
	s := openTemp(t)
	ctx := testContext(t)
	owner := newOwner(t, s)
	write := func(fn func(q *db.Queries) error) {
		t.Helper()
		if err := s.Write(ctx, fn); err != nil {
			t.Fatal(err)
		}
	}
	write(func(q *db.Queries) error { return q.CreateDevice(ctx, device("d2", "cli", "second", 200)) })
	write(func(q *db.Queries) error { return q.CreateDevice(ctx, device("d1", "web", "first", 100)) })

	found, err := s.Queries().GetActiveDeviceByTokenHash(ctx, HashToken("first"))
	if err != nil || found.ID != "d1" || found.TokenHash != HashToken("first") || found.LastSeenAt != nil {
		t.Fatalf("lookup by token hash = %+v, %v", found, err)
	}
	if _, err := s.Queries().GetActiveDeviceByTokenHash(ctx, HashToken("unknown")); !IsNotFound(err) {
		t.Errorf("lookup of an unknown token = %v, want not found", err)
	}

	listed, err := s.Queries().ListDevices(ctx, owner.ID)
	if err != nil || len(listed) != 2 || listed[0].ID != "d1" || listed[1].ID != "d2" {
		t.Fatalf("ListDevices = %+v, %v, want d1 then d2 (oldest first)", listed, err)
	}

	write(func(q *db.Queries) error { return q.TouchDevice(ctx, db.TouchDeviceParams{ID: "d1", LastSeenAt: 500}) })
	if seen, _ := s.Queries().GetActiveDeviceByTokenHash(ctx, HashToken("first")); seen.LastSeenAt == nil || *seen.LastSeenAt != 500 {
		t.Errorf("last seen after a touch = %v, want 500", seen.LastSeenAt)
	}

	revoke := func() (rows int64) {
		write(func(q *db.Queries) (err error) {
			rows, err = q.RevokeDevice(ctx, db.RevokeDeviceParams{ID: "d1", RevokedAt: 600})
			return err
		})
		return rows
	}
	if rows := revoke(); rows != 1 {
		t.Errorf("first revoke changed %d rows, want 1", rows)
	}
	if rows := revoke(); rows != 0 {
		t.Errorf("second revoke changed %d rows, want 0", rows)
	}
	if _, err := s.Queries().GetActiveDeviceByTokenHash(ctx, HashToken("first")); !IsNotFound(err) {
		t.Errorf("a revoked device was found by its token: %v", err)
	}
	write(func(q *db.Queries) error { return q.TouchDevice(ctx, db.TouchDeviceParams{ID: "d1", LastSeenAt: 900}) })
	listed, _ = s.Queries().ListDevices(ctx, owner.ID)
	if listed[0].RevokedAt == nil || *listed[0].RevokedAt != 600 || *listed[0].LastSeenAt != 500 {
		t.Errorf("revoked device row = %+v, want revoked at 600 and still last seen at 500", listed[0])
	}
}

func TestDeviceConstraints(t *testing.T) {
	s := openTemp(t)
	ctx := testContext(t)
	newOwner(t, s)
	create := func(p db.CreateDeviceParams) error {
		return s.Write(ctx, func(q *db.Queries) error { return q.CreateDevice(ctx, p) })
	}
	for i, kind := range []string{"web", "desktop", "mobile", "cli", "dev"} {
		if err := create(device("ok"+kind, kind, "token-"+kind, int64(i))); err != nil {
			t.Errorf("kind %q was refused: %v", kind, err)
		}
	}
	tests := []struct {
		name string
		p    db.CreateDeviceParams
		want string
	}{
		{"a kind that is not allowed", device("x1", "tablet", "t1", 1), "CHECK"},
		{"a token hash that is already used", device("x2", "web", "token-web", 1), "UNIQUE"},
		{"an id that is already used", device("okweb", "web", "t3", 1), "UNIQUE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := create(tt.p); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("create = %v, want an error containing %q", err, tt.want)
			}
		})
	}
}
