package zone_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/zone"
)

type fakeSource struct {
	name string
	err  error
}

func (f *fakeSource) TimeZone(context.Context) (string, error) { return f.name, f.err }

var noon = time.Date(2026, time.October, 8, 12, 0, 0, 0, time.UTC)

func TestAChosenZoneIsWhereTheClockReadsTheTime(t *testing.T) {
	clock := zone.New(&fakeSource{name: "Africa/Kampala"}, zone.WithNow(func() time.Time { return noon }))
	got := clock.Now()
	if got.Hour() != 15 || zone.Label(got) != "EAT" {
		t.Fatalf("Now = %s (%s), want 15:00 EAT", got, zone.Label(got))
	}
}

func TestNoChoiceOrABadNameIsTheMachinesZone(t *testing.T) {
	for _, name := range []string{"", "Local", "Not/AZone"} {
		clock := zone.New(&fakeSource{name: name})
		if clock.Location() != time.Local {
			t.Errorf("zone %q gave %v, want the machine's zone", name, clock.Location())
		}
	}
	if clock := zone.New(&fakeSource{err: errors.New("db down")}); clock.Location() != time.Local {
		t.Errorf("an unreadable source gave %v, want the machine's zone", clock.Location())
	}
}

func TestRefreshTellsListenersOnlyWhenTheZoneChanged(t *testing.T) {
	src := &fakeSource{name: "UTC"}
	clock := zone.New(src)
	var heard []string
	clock.OnChange(func(loc *time.Location) { heard = append(heard, loc.String()) })
	if clock.Refresh(context.Background()) || len(heard) != 0 {
		t.Fatalf("an unchanged zone was announced: %v", heard)
	}
	src.name = "America/Los_Angeles"
	if !clock.Refresh(context.Background()) || len(heard) != 1 || heard[0] != "America/Los_Angeles" {
		t.Fatalf("a changed zone was heard as %v", heard)
	}
	if got := clock.Location().String(); got != "America/Los_Angeles" {
		t.Errorf("Location = %s after the change", got)
	}
}

func TestADayStartsWhereThePersonIs(t *testing.T) {
	// 23:00 UTC on Oct 8 is already Oct 9 in Kampala, and still Oct 8 in Los Angeles.
	late := time.Date(2026, time.October, 8, 23, 0, 0, 0, time.UTC)
	for name, day := range map[string]int{"Africa/Kampala": 9, "America/Los_Angeles": 8} {
		clock := zone.New(&fakeSource{name: name}, zone.WithNow(func() time.Time { return late }))
		if got := clock.Now().Day(); got != day {
			t.Errorf("%s: day %d, want %d", name, got, day)
		}
	}
}
