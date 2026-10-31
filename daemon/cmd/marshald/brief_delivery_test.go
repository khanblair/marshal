package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The owner is the first person created, not the first by name: with a second person added later
// whose name sorts first, email still goes to the owner's address.
func TestOwnerEmailIsTheOwnersNotTheFirstByName(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "marshal.db"), store.WithLogger(nil))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if got, err := ownerEmail(ctx, st); err != nil || got != "" {
		t.Fatalf("with no one in the store, ownerEmail = %q, %v; want empty and no error", got, err)
	}
	err = st.Write(ctx, func(q *db.Queries) error {
		for _, u := range []db.CreateUserParams{
			{ID: "01OWNER", Name: "Zed Owner", Email: " owner@example.com ", CreatedAt: 1, UpdatedAt: 1},
			{ID: "01LATER", Name: "Ada Later", Email: "later@example.com", CreatedAt: 2, UpdatedAt: 2},
		} {
			if _, err := q.CreateUser(ctx, u); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("make the users: %v", err)
	}
	got, err := ownerEmail(ctx, st)
	if err != nil || got != "owner@example.com" {
		t.Errorf("ownerEmail = %q, %v; want the owner's address, trimmed", got, err)
	}
}
