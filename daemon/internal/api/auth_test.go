package api_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/api"
	"github.com/khanblair/marshal/daemon/internal/platform"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

func TestWhoAmIThroughHTTP(t *testing.T) {
	tests := []struct {
		name string
		opts []stackOption
		kind protocol.DeviceKind
		mode string
	}{
		{"a dev daemon", nil, protocol.DeviceKindDev, "dev"},
		{"a normal daemon", []stackOption{normalDaemon()}, protocol.DeviceKindCLI, "normal"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := newStack(t, tc.opts...)
			r := st.do(http.MethodGet, "/v1/auth/whoami", nil).want(t, http.StatusOK)
			sameShape(t, "whoami", r.Body)
			got := decode[protocol.WhoAmI](t, r)
			if got.DeviceKind != tc.kind || got.Mode != tc.mode || got.DeviceID == "" || got.UserID == "" {
				t.Errorf("whoami = %+v, want a %s device on a %s daemon", got, tc.kind, tc.mode)
			}
			st.doWith("wrong", http.MethodGet, "/v1/auth/whoami", nil).apiError(t, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized)
		})
	}
}

// A normal daemon looks tokens up in the database. A token that was revoked stops working when the
// short memory of it runs out, and the dev token is not a way in.
func TestANormalDaemonChecksTokensAgainstTheDatabase(t *testing.T) {
	st := newStack(t, normalDaemon())
	ctx := context.Background()
	who := decode[protocol.WhoAmI](t, st.do(http.MethodGet, "/v1/auth/whoami", nil).want(t, http.StatusOK))
	st.do(http.MethodGet, "/v1/auth/whoami", nil).want(t, http.StatusOK) // from the short memory

	devices, err := st.store.Queries().ListDevices(ctx, who.UserID)
	if err != nil || len(devices) != 1 || devices[0].LastSeenAt == nil {
		t.Fatalf("devices = %+v (error %v), want one that has been seen", devices, err)
	}
	err = st.store.Write(ctx, func(q *db.Queries) error {
		_, err := q.RevokeDevice(ctx, db.RevokeDeviceParams{RevokedAt: store.Millis(time.Now()), ID: who.DeviceID})
		return err
	})
	if err != nil {
		t.Fatalf("revoke the device: %v", err)
	}
	st.do(http.MethodGet, "/v1/auth/whoami", nil).want(t, http.StatusOK) // still remembered, for a few seconds
	st.advance(time.Minute)
	st.do(http.MethodGet, "/v1/auth/whoami", nil).apiError(t, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized)
}

// A dev device signs in with the dev token only. Its stored token belongs to a value that nobody
// holds, so a daemon that is not in dev mode does not let a dev device in.
func TestADevDeviceCannotSignInThroughTheDatabase(t *testing.T) {
	st := newStack(t)
	devices, err := st.store.Queries().ListDevices(context.Background(), st.dev.Caller.UserID)
	if err != nil || len(devices) != 1 || devices[0].Kind != string(protocol.DeviceKindDev) {
		t.Fatalf("devices = %+v (error %v), want the one dev device", devices, err)
	}
	// The dev token is a file, and it is checked by hash in the dev daemon only.
	normal := newStack(t, normalDaemon())
	normal.doWith(st.token, http.MethodGet, "/v1/auth/whoami", nil).apiError(t, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized)
}

func newAccountsFixture(t *testing.T) (*stack, string) {
	t.Helper()
	st := newStack(t, normalDaemon())
	return st, st.dir
}

func ensureAccounts(t *testing.T, st *stack, dir string, dev bool) *api.DevAccess {
	t.Helper()
	got, err := api.EnsureAccounts(context.Background(), st.store, api.AccountsConfig{DataDir: dir, Dev: dev, Now: time.Now, Log: st.log})
	if err != nil {
		t.Fatalf("EnsureAccounts: %v", err)
	}
	return got
}

// Starting again finds the owner and the first device and makes nothing new.
func TestEnsureAccountsFindsWhatIsThere(t *testing.T) {
	st, dir := newAccountsFixture(t)
	first, err := os.ReadFile(filepath.Join(dir, platform.OwnerTokenFile))
	if err != nil {
		t.Fatal(err)
	}
	if got := ensureAccounts(t, st, dir, false); got != nil {
		t.Errorf("a normal daemon has no dev access, got %+v", got)
	}
	again, err := os.ReadFile(filepath.Join(dir, platform.OwnerTokenFile))
	if err != nil || string(again) != string(first) {
		t.Errorf("the owner token file changed on a second start (error %v)", err)
	}
	who := decode[protocol.WhoAmI](t, st.do(http.MethodGet, "/v1/auth/whoami", nil).want(t, http.StatusOK))
	devices, err := st.store.Queries().ListDevices(context.Background(), who.UserID)
	if err != nil || len(devices) != 1 {
		t.Errorf("devices = %+v (error %v), want still only one", devices, err)
	}

	// A token file that was lost is not made again: a device already exists, and only its owner
	// can decide to replace it.
	if err := os.Remove(filepath.Join(dir, platform.OwnerTokenFile)); err != nil {
		t.Fatal(err)
	}
	ensureAccounts(t, st, dir, false)
	if _, err := os.Stat(filepath.Join(dir, platform.OwnerTokenFile)); !os.IsNotExist(err) {
		t.Errorf("a new token file was made (error %v)", err)
	}
	if !strings.Contains(st.logText(), "the owner token file is missing") {
		t.Error("the missing token file was not reported in the log")
	}
}

// A first start that stopped after writing the token file and before saving the device is
// finished by the next one, with the token that is in the file.
func TestEnsureAccountsFinishesAnInterruptedFirstStart(t *testing.T) {
	st := newStack(t, normalDaemon())
	other := t.TempDir()
	token, err := platform.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := platform.CreateTokenFile(filepath.Join(other, platform.OwnerTokenFile), token); err != nil {
		t.Fatal(err)
	}
	// The stack's store already has a device, so this checks the file's token with a store of its own.
	fresh := openStore(t)
	got, err := api.EnsureAccounts(context.Background(), fresh, api.AccountsConfig{DataDir: other, Now: time.Now, Log: st.log})
	if err != nil || got != nil {
		t.Fatalf("EnsureAccounts = %+v, %v", got, err)
	}
	if _, err := fresh.Queries().GetActiveDeviceByTokenHash(context.Background(), store.HashToken(token)); err != nil {
		t.Errorf("the token in the file was not registered: %v", err)
	}
}

// A dev daemon makes one dev device and finds it again.
func TestEnsureAccountsOnADevDaemonReusesItsDevice(t *testing.T) {
	st := newStack(t)
	again := ensureAccounts(t, st, st.dir, true)
	if again == nil || again.Token != st.dev.Token || again.Caller != st.dev.Caller {
		t.Errorf("a second start gave %+v, want the same dev token and device as %+v", again, st.dev)
	}
}
