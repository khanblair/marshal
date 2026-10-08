package main

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/store"
)

type countingRecutter struct{ zones []string }

func (c *countingRecutter) Recut(_ context.Context, loc *time.Location) error {
	c.zones = append(c.zones, loc.String())
	return nil
}

func TestHomeDaysAreCutAgainOnlyWhenTheZoneChanges(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "marshal.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	home := &countingRecutter{}
	log := slog.New(slog.DiscardHandler)
	kampala, _ := time.LoadLocation("Africa/Kampala")

	// A machine in Los Angeles whose person chose Kampala: the days were cut in the machine's zone.
	machine, _ := time.LoadLocation("America/Los_Angeles")
	was := time.Local
	time.Local = machine
	t.Cleanup(func() { time.Local = was })

	recutHomeDays(ctx, st, home, kampala, log)
	recutHomeDays(ctx, st, home, kampala, log)
	if len(home.zones) != 1 || home.zones[0] != "Africa/Kampala" {
		t.Fatalf("cut in %v, want Kampala once", home.zones)
	}
	utc, _ := time.LoadLocation("UTC")
	recutHomeDays(ctx, st, home, utc, log)
	if len(home.zones) != 2 || home.zones[1] != "UTC" {
		t.Fatalf("cut in %v, want a second cut in UTC", home.zones)
	}
}

func TestAMachineWithNoChosenZoneIsLeftAlone(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "marshal.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	home := &countingRecutter{}
	recutHomeDays(ctx, st, home, time.Local, slog.New(slog.DiscardHandler))
	if len(home.zones) != 0 {
		t.Fatalf("cut in %v for a daemon still on the machine's zone", home.zones)
	}
}

func TestAChosenZoneThatDrawsTheSameDaysAsTheMachineCutsNothing(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "marshal.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	kampala, _ := time.LoadLocation("Africa/Kampala")
	nairobi, _ := time.LoadLocation("Africa/Nairobi")
	was := time.Local
	time.Local = kampala
	t.Cleanup(func() { time.Local = was })

	home := &countingRecutter{}
	recutHomeDays(ctx, st, home, nairobi, slog.New(slog.DiscardHandler))
	if len(home.zones) != 0 {
		t.Fatalf("cut in %v for a zone with the same offset", home.zones)
	}
}
