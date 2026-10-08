package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// homeDaysZoneKey is the settings row that says which zone Home's stored days were cut in. A day is
// the midnight of the person's zone, so choosing another zone means the last 90 days are cut again.
const homeDaysZoneKey = "home.days.zone"

// dayRecutter cuts Home's stored daily numbers again in a zone.
type dayRecutter interface {
	Recut(ctx context.Context, loc *time.Location) error
}

// recutHomeDays cuts Home's days again in loc, when they were cut in another zone, and says which
// zone they are cut in now. A daemon that has never recorded a zone is taken to have cut them in the
// machine's own, which is what it did before the zone was a setting.
func recutHomeDays(ctx context.Context, st *store.Store, home dayRecutter, loc *time.Location, log *slog.Logger) {
	cutIn, err := st.Queries().GetSetting(ctx, homeDaysZoneKey)
	if err != nil && !store.IsNotFound(err) {
		log.Error("could not read the zone Home's days were cut in", "error", err)
		return
	}
	switch {
	case store.IsNotFound(err) && sameDays(time.Local, loc, time.Now()):
		// Nothing to cut again: the machine's zone and the chosen one draw the same days.
		saveHomeDaysZone(ctx, st, loc, log)
		return
	case store.IsNotFound(err):
		cutIn = time.Local.String()
	}
	if cutIn == loc.String() {
		return
	}
	if err := home.Recut(ctx, loc); err != nil {
		log.Error("could not cut Home's days again in the new time zone", "zone", loc.String(), "error", err)
		return
	}
	saveHomeDaysZone(ctx, st, loc, log)
}

func saveHomeDaysZone(ctx context.Context, st *store.Store, loc *time.Location, log *slog.Logger) {
	err := st.Write(ctx, func(q *db.Queries) error {
		return q.SetSetting(ctx, db.SetSettingParams{Key: homeDaysZoneKey, ValueJSON: loc.String()})
	})
	if err != nil {
		log.Error("could not save the zone Home's days are cut in", "error", err)
	}
}

// homeDaysSpan is how many days back Home keeps its numbers, and so how far two zones are compared.
const homeDaysSpan = 91

// sameDays says two zones are the same distance from UTC at every hour of the last homeDaysSpan
// days, so each draws the same days (the same zone under two names, or one with the same offset).
func sameDays(a, b *time.Location, now time.Time) bool {
	for hour := 0; hour <= homeDaysSpan*24; hour++ {
		at := now.Add(-time.Duration(hour) * time.Hour)
		_, offA := at.In(a).Zone()
		_, offB := at.In(b).Zone()
		if offA != offB {
			return false
		}
	}
	return true
}
