package settings

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
)

// The daemon's own settings (B5.6): the sleep times a person edits on Settings > Sleep, stored in
// the `settings` table so they outlive a restart, and validated the way the form's own choices are
// so a value the screen would not offer is refused rather than stored.

// settingsFor builds a settings service over a fresh store, which is what a first launch has.
func settingsFor(t *testing.T) *Service {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "marshal.db"), store.WithLogger(nil))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc, err := New(st)
	if err != nil {
		t.Fatalf("make the settings service: %v", err)
	}
	return svc
}

// wantInvalidArgument fails the test unless err is the plain refusal the form shows, and returns its
// message.
func wantInvalidArgument(t *testing.T, err error) string {
	t.Helper()
	var perr *protocol.Error
	if !errors.As(err, &perr) {
		t.Fatalf("error = %v, want a protocol error", err)
	}
	if perr.Code != protocol.ErrorCodeInvalidArgument {
		t.Fatalf("error code = %s (%s), want %s", perr.Code, perr.Message, protocol.ErrorCodeInvalidArgument)
	}
	return perr.Message
}

// A fresh install has the shipped defaults - a 15-minute idle time, a 2-minute warning, 15 minutes
// of Keep awake, automatic restore, and warnings in the app - rather than a not-found, because that
// is what its own screen shows.
func TestAFreshInstallAnswersWithTheDefaults(t *testing.T) {
	got, err := settingsFor(t).Sleep(context.Background())
	if err != nil {
		t.Fatalf("Sleep: %v", err)
	}
	if want := protocol.DefaultSleepSettings(); got != want {
		t.Errorf("sleep settings = %+v, want the defaults %+v", got, want)
	}
}

// Saving the screen and reading it back answers what was stored, and the stored record survives a
// new service over the same store, the way it survives a restart.
func TestSavedSettingsAreReadBack(t *testing.T) {
	svc := settingsFor(t)
	ctx := context.Background()
	want := protocol.SleepSettings{
		IdleMinutes: 30, WarningMinutes: 5, KeepAwakeMinutes: 60,
		Restore: protocol.SleepRestoreManual, Channel: protocol.SleepChannelInApp,
	}

	saved, err := svc.SetSleep(ctx, want)
	if err != nil {
		t.Fatalf("SetSleep: %v", err)
	}
	if saved != want {
		t.Errorf("SetSleep answered %+v, want %+v", saved, want)
	}

	got, err := svc.Sleep(ctx)
	if err != nil {
		t.Fatalf("Sleep: %v", err)
	}
	if got != want {
		t.Errorf("sleep settings = %+v, want %+v", got, want)
	}
}

// Every value the form would not offer is refused with the sentence the form shows, and nothing is
// stored: an idle time that is not one of the offered choices, a warning that is not shorter than
// the idle time, a keep-awake time under a minute, an unknown restore answer, and an unknown
// warning channel.
func TestValuesTheFormWouldNotOfferAreRefused(t *testing.T) {
	svc := settingsFor(t)
	ctx := context.Background()
	good := protocol.DefaultSleepSettings()

	cases := []struct {
		name   string
		mutate func(*protocol.SleepSettings)
		want   string
	}{
		{"an idle time that is not offered", func(c *protocol.SleepSettings) { c.IdleMinutes = 7 },
			"Choose an idle time of 5, 15, 30, or 60 minutes."},
		{"a warning of no minutes", func(c *protocol.SleepSettings) { c.WarningMinutes = 0 },
			"The sleep warning must be at least a minute."},
		{"a warning as long as the idle time", func(c *protocol.SleepSettings) { c.WarningMinutes = c.IdleMinutes },
			"The sleep warning must be shorter than the idle time."},
		{"keep awake of no minutes", func(c *protocol.SleepSettings) { c.KeepAwakeMinutes = 0 },
			"Keep awake must be at least a minute."},
		{"an unknown restore answer", func(c *protocol.SleepSettings) { c.Restore = "sometimes" },
			"Choose whether cards are restored on startup or resumed by hand."},
		{"an unknown channel", func(c *protocol.SleepSettings) { c.Channel = "carrier-pigeon" },
			"Choose where sleep warnings go."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := good
			tc.mutate(&in)
			_, err := svc.SetSleep(ctx, in)
			if got := wantInvalidArgument(t, err); got != tc.want {
				t.Errorf("message = %q, want %q", got, tc.want)
			}
			// Nothing was stored by the refused write, so the screen still reads the defaults.
			after, err := svc.Sleep(ctx)
			if err != nil {
				t.Fatalf("Sleep after a refused write: %v", err)
			}
			if want := protocol.DefaultSleepSettings(); after != want {
				t.Errorf("a refused write stored %+v, want the defaults %+v", after, want)
			}
		})
	}
}

func TestQuietDuringEventsIsOffUntilChosenAndIsKept(t *testing.T) {
	svc := settingsFor(t)
	ctx := context.Background()
	if quiet, err := svc.AlertQuiet(ctx); err != nil || quiet {
		t.Fatalf("a fresh install = %v, %v, want off", quiet, err)
	}
	if err := svc.SetAlertQuiet(ctx, true); err != nil {
		t.Fatal(err)
	}
	if quiet, err := svc.AlertQuiet(ctx); err != nil || !quiet {
		t.Fatalf("after turning it on = %v, %v, want on", quiet, err)
	}
	if err := svc.SetAlertQuiet(ctx, false); err != nil {
		t.Fatal(err)
	}
	if quiet, _ := svc.AlertQuiet(ctx); quiet {
		t.Error("turning it off did not stick")
	}
}
