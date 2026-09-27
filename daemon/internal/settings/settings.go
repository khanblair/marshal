// Package settings owns the daemon's own settings: the small set of numbers and choices that are
// about the whole install rather than about one card, project, or person, stored as one JSON value
// per key in the `settings` table of migration 0001 (docs/architecture.md section 10).
//
// Phase 5 is where it starts being used: the sleep settings of checklist item B5.6 (inventory N5)
// are the idle time, the warning time, the keep-awake time, what happens to awake cards at restart,
// and where warnings go. The session manager reads them (the idle timer and Keep awake) and
// cmd/marshald reads the restore choice at start; the Settings screen (S26a) reads and writes them
// through internal/api.
//
// Why a table and not the config file: these are values a person edits on a screen while the daemon
// runs, and they must survive a restart the way a limit does. The config file and environment are
// how the daemon is started (internal/config); this is what it has been told since.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// sleepKey is the settings row the sleep settings live under. One row holds the whole record, since
// the fields are read and written together by one screen and one form.
const sleepKey = "sleep"

// The default sleep settings are protocol.DefaultSleepSettings, beside the wire type, so this
// service and the session manager cannot answer a fresh install with two different sets of numbers.

// idleChoices are the idle times the screen offers, in minutes. A time that is not one of them is
// refused, the way the limits form refuses a number it does not offer.
func idleChoices() []int { return []int{5, 15, 30, 60} }

// restoreChoices and channelChoices are the two other closed sets of the form.
func restoreChoices() []string {
	return []string{protocol.SleepRestoreAuto, protocol.SleepRestoreManual}
}

func channelChoices() []string { return []string{protocol.SleepChannelInApp, "telegram", "discord"} }

// Service reads and writes the settings in the `settings` table. It is safe for concurrent use: the
// session manager reads the sleep settings on a ticker while the screen writes them.
type Service struct {
	store *store.Store
}

// New builds a Service on an open database. The store is required: a settings service with nowhere
// to read would answer with defaults that are not the person's.
func New(st *store.Store) (*Service, error) {
	if st == nil {
		return nil, errors.New("the settings service needs the store")
	}
	return &Service{store: st}, nil
}

// Sleep reads the sleep settings. An install that has never saved them answers with the defaults,
// which is also what the first write stores.
func (s *Service) Sleep(ctx context.Context) (protocol.SleepSettings, error) {
	if err := s.ready(); err != nil {
		return protocol.SleepSettings{}, err
	}
	raw, err := s.store.Queries().GetSetting(ctx, sleepKey)
	switch {
	case err == nil:
		return decodeSleep(raw)
	case store.IsNotFound(err):
		return defaultSleep(), nil
	default:
		return protocol.SleepSettings{}, fmt.Errorf("read the sleep settings: %w", err)
	}
}

// SetSleep checks the settings, stores them, and answers with what was stored. The value is checked
// first, so a number the screen would not accept is refused with the sentence the form shows rather
// than stored and read back on the next open.
func (s *Service) SetSleep(ctx context.Context, in protocol.SleepSettings) (protocol.SleepSettings, error) {
	if err := s.ready(); err != nil {
		return protocol.SleepSettings{}, err
	}
	if err := checkSleep(in); err != nil {
		return protocol.SleepSettings{}, err
	}
	encoded, err := json.Marshal(in)
	if err != nil {
		return protocol.SleepSettings{}, fmt.Errorf("encode the sleep settings: %w", err)
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		return q.SetSetting(ctx, db.SetSettingParams{Key: sleepKey, ValueJSON: string(encoded)})
	})
	if err != nil {
		return protocol.SleepSettings{}, fmt.Errorf("save the sleep settings: %w", err)
	}
	return in, nil
}

// ready reports whether the service can reach its store, so a daemon wired without one fails loudly
// instead of answering with defaults nobody chose.
func (s *Service) ready() error {
	if s == nil || s.store == nil {
		return errors.New("settings: a settings service needs a store")
	}
	return nil
}

// defaultSleep is what a fresh install has.
func defaultSleep() protocol.SleepSettings {
	return protocol.DefaultSleepSettings()
}

// decodeSleep reads one stored record. A record that will not parse is an error rather than a
// silent fallback: answering with defaults would hide a row that a bug wrote.
func decodeSleep(raw string) (protocol.SleepSettings, error) {
	var out protocol.SleepSettings
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return protocol.SleepSettings{}, fmt.Errorf("read the sleep settings: %w", err)
	}
	return out, nil
}

// checkSleep refuses settings the screen would not be allowed to save, with the same plain
// sentences the form shows.
func checkSleep(in protocol.SleepSettings) error {
	if !slices.Contains(idleChoices(), in.IdleMinutes) {
		return protocol.InvalidArgument("Choose an idle time of 5, 15, 30, or 60 minutes.")
	}
	if in.WarningMinutes < 1 {
		return protocol.InvalidArgument("The sleep warning must be at least a minute.")
	}
	if in.WarningMinutes >= in.IdleMinutes {
		return protocol.InvalidArgument("The sleep warning must be shorter than the idle time.")
	}
	if in.KeepAwakeMinutes < 1 {
		return protocol.InvalidArgument("Keep awake must be at least a minute.")
	}
	if !slices.Contains(restoreChoices(), in.Restore) {
		return protocol.InvalidArgument("Choose whether cards are restored on startup or resumed by hand.")
	}
	if !slices.Contains(channelChoices(), in.Channel) {
		return protocol.InvalidArgument("Choose where sleep warnings go.")
	}
	return nil
}
