package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// HashToken returns the value stored in devices.token_hash: the SHA-256 of the token in lower
// case hex. Tokens are long random strings, so a plain hash is enough and a lookup by hash is
// exact. The token itself is never stored or logged.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// EnsureOwner returns the owner, the first user, and makes them from create on the first run. The
// caller supplies the id and times (an id from protocol.NewID and its own clock), so the store
// has no clock or random source of its own.
func (s *Store) EnsureOwner(ctx context.Context, create db.CreateUserParams) (db.User, error) {
	var owner db.User
	err := s.Write(ctx, func(q *db.Queries) error {
		found, err := q.GetOwner(ctx)
		switch {
		case err == nil:
			owner = found
			return nil
		case !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("find the owner: %w", err)
		}
		if owner, err = q.CreateUser(ctx, create); err != nil {
			return fmt.Errorf("create the owner: %w", err)
		}
		return nil
	})
	return owner, err
}

// SetTailnetIdentity records the Tailscale account the daemon's own node joined as, so the
// profile's Tailscale identity is read from the node rather than typed in (B9.1). It writes one
// column and nothing else: a node signing in is not a profile edit, so updated_at is left alone.
func (s *Store) SetTailnetIdentity(ctx context.Context, userID, identity string) error {
	return s.Write(ctx, func(q *db.Queries) error {
		_, err := q.SetUserTailnetIdentity(ctx, db.SetUserTailnetIdentityParams{
			TailnetIdentity: identity, ID: userID,
		})
		return err
	})
}
