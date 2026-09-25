package api

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/khanblair/marshal/daemon/internal/platform"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

const (
	ownerName     = "Owner"
	cliDeviceName = "Command line"
	devDeviceName = "Dev token"
)

// AccountsConfig says what kind of daemon is starting and where its files are.
type AccountsConfig struct {
	// DataDir is the data folder, where the token files live.
	DataDir string
	// Dev is true for a dev daemon.
	Dev bool
	// Now is the clock.
	Now func() time.Time
	// Log receives what was done. It is told where a token file is, never what is in it.
	Log *slog.Logger
}

// EnsureAccounts makes the owner and the first device on the first start and finds them again
// on every later start. Nothing else creates a token in Phase 1 (pairing arrives with remote
// access).
//
//   - A dev daemon uses the dev token from <data>/dev-token. The returned DevAccess says which dev
//     device the token stands for. The dev device's own stored hash belongs to a random value that
//     nobody holds, so the token can never sign in through the database, only through DevAccess.
//   - A normal daemon makes one `cli` device with a random token and writes the token, once, to
//     <data>/owner-token for the desktop app and the `marshal` command. The database keeps only
//     the hash. The result is nil.
func EnsureAccounts(ctx context.Context, st *store.Store, cfg AccountsConfig) (*DevAccess, error) {
	now := cfg.Now()
	id, err := protocol.NewID(now, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("make the owner id: %w", err)
	}
	owner, err := st.EnsureOwner(ctx, db.CreateUserParams{
		ID: id, Name: ownerName, CreatedAt: store.Millis(now), UpdatedAt: store.Millis(now),
	})
	if err != nil {
		return nil, fmt.Errorf("prepare the owner: %w", err)
	}
	if cfg.Dev {
		return ensureDevAccess(ctx, st, owner, cfg)
	}
	return nil, ensureOwnerToken(ctx, st, owner, cfg)
}

// ensureDevAccess reads or makes the dev token and finds or makes the dev device it stands for.
func ensureDevAccess(ctx context.Context, st *store.Store, owner db.User, cfg AccountsConfig) (*DevAccess, error) {
	path := filepath.Join(cfg.DataDir, platform.DevTokenFile)
	token, err := platform.EnsureToken(path)
	if err != nil {
		return nil, fmt.Errorf("prepare the dev token: %w", err)
	}
	devices, err := st.Queries().ListDevices(ctx, owner.ID)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	deviceID := activeDeviceOfKind(devices, protocol.DeviceKindDev)
	if deviceID == "" {
		// The stored hash is of a value that is thrown away, so no token can match it.
		unusable, err := platform.NewToken()
		if err != nil {
			return nil, err
		}
		device := newDevice{UserID: owner.ID, Name: devDeviceName, Kind: protocol.DeviceKindDev, Token: unusable}
		if deviceID, err = createDevice(ctx, st, cfg.Now(), device); err != nil {
			return nil, err
		}
	}
	cfg.Log.Info("dev token ready", "file", path)
	return &DevAccess{
		Token:  token,
		Caller: Caller{DeviceID: deviceID, DeviceKind: protocol.DeviceKindDev, UserID: owner.ID},
	}, nil
}

// ensureOwnerToken makes the owner's `cli` device and its token file, once. The token file is
// created only when no `cli` device exists yet, active or revoked, so a token that was revoked
// cannot come back on a restart.
func ensureOwnerToken(ctx context.Context, st *store.Store, owner db.User, cfg AccountsConfig) error {
	path := filepath.Join(cfg.DataDir, platform.OwnerTokenFile)
	devices, err := st.Queries().ListDevices(ctx, owner.ID)
	if err != nil {
		return fmt.Errorf("list devices: %w", err)
	}
	if hasDeviceOfKind(devices, protocol.DeviceKindCLI) {
		if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
			cfg.Log.Warn("the owner token file is missing and no new token is made", "file", path)
		}
		return nil
	}
	token, created, err := ownerTokenFile(path)
	if err != nil {
		return err
	}
	device := newDevice{UserID: owner.ID, Name: cliDeviceName, Kind: protocol.DeviceKindCLI, Token: token}
	if _, err := createDevice(ctx, st, cfg.Now(), device); err != nil {
		if created {
			// The file is ours and no device matches it, so leave nothing behind for the next start.
			err = errors.Join(err, os.Remove(path))
		}
		return err
	}
	cfg.Log.Info("owner token ready", "file", path)
	return nil
}

// ownerTokenFile returns the token in the owner token file. When the file does not exist it makes
// a token and writes it. A file that is already there means an earlier start stopped after
// writing it and before saving the device, so its token is the one to register.
func ownerTokenFile(path string) (token string, created bool, err error) {
	token, err = platform.ReadTokenFile(path)
	switch {
	case err == nil:
		return token, false, nil
	case !errors.Is(err, os.ErrNotExist):
		return "", false, err
	}
	if token, err = platform.NewToken(); err != nil {
		return "", false, err
	}
	if err := platform.CreateTokenFile(path, token); err != nil {
		return "", false, fmt.Errorf("write %s (delete the file if it is empty): %w", path, err)
	}
	return token, true, nil
}

// newDevice is what createDevice needs beyond the store and the clock, bundled so the function
// stays inside the parameter limit.
type newDevice struct {
	UserID, Name string
	Kind         protocol.DeviceKind
	Token        string
}

func createDevice(ctx context.Context, st *store.Store, now time.Time, d newDevice) (string, error) {
	id, err := protocol.NewID(now, rand.Reader)
	if err != nil {
		return "", fmt.Errorf("make a device id: %w", err)
	}
	err = st.Write(ctx, func(q *db.Queries) error {
		return q.CreateDevice(ctx, db.CreateDeviceParams{
			ID: id, UserID: d.UserID, Name: d.Name, Kind: string(d.Kind),
			TokenHash: store.HashToken(d.Token), PairedAt: store.Millis(now),
		})
	})
	if err != nil {
		return "", fmt.Errorf("save the %s device: %w", d.Kind, err)
	}
	return id, nil
}

func hasDeviceOfKind(devices []db.ListDevicesRow, kind protocol.DeviceKind) bool {
	for _, device := range devices {
		if device.Kind == string(kind) {
			return true
		}
	}
	return false
}

// activeDeviceOfKind returns the id of the first device of that kind that is not revoked, or "".
func activeDeviceOfKind(devices []db.ListDevicesRow, kind protocol.DeviceKind) string {
	for _, device := range devices {
		if device.Kind == string(kind) && device.RevokedAt == nil {
			return device.ID
		}
	}
	return ""
}
