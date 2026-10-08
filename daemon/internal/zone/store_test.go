package zone_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
	"github.com/khanblair/marshal/daemon/internal/zone"
)

func TestTheOwnersProfileZoneIsTheClocksZone(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "marshal.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	clock := zone.New(zone.StoreSource{Store: st})
	if clock.Location() != time.Local {
		t.Fatalf("a daemon with no owner read %v, want the machine's zone", clock.Location())
	}
	now := store.Millis(time.Now())
	owner, err := st.EnsureOwner(ctx, db.CreateUserParams{ID: "01M3C107JB041061050R3GG2U1", Name: "Owner", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Write(ctx, func(q *db.Queries) error {
		_, err := q.UpdateUserProfile(ctx, db.UpdateUserProfileParams{
			Name: owner.Name, Email: owner.Email, TimeZone: "Africa/Kampala", UpdatedAt: now, ID: owner.ID,
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !clock.Refresh(ctx) || clock.Location().String() != "Africa/Kampala" {
		t.Fatalf("after the profile changed the clock reads %v", clock.Location())
	}
}
