package integrations

import (
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrations/googlecal"
)

func TestAnAllDayEventStartsWhenThePersonsOwnDayDoes(t *testing.T) {
	cached := []googlecal.Event{
		{Title: "Independence Day", AllDay: true, StartDate: "2026-10-09", EndDate: "2026-10-10"},
		{Title: "Standup", StartAt: time.Date(2026, time.October, 9, 6, 45, 0, 0, time.UTC)},
	}
	kampala, _ := time.LoadLocation("Africa/Kampala")
	losAngeles, _ := time.LoadLocation("America/Los_Angeles")

	inKampala := anchorAllDay(cached, kampala)
	if want := time.Date(2026, time.October, 8, 21, 0, 0, 0, time.UTC); !inKampala[0].StartAt.Equal(want) {
		t.Errorf("Kampala's Oct 9 starts at %v, want %v", inKampala[0].StartAt, want)
	}
	if want := time.Date(2026, time.October, 9, 21, 0, 0, 0, time.UTC); !inKampala[0].EndAt.Equal(want) {
		t.Errorf("Kampala's end is %v, want %v", inKampala[0].EndAt, want)
	}
	inLosAngeles := anchorAllDay(cached, losAngeles)
	if want := time.Date(2026, time.October, 9, 7, 0, 0, 0, time.UTC); !inLosAngeles[0].StartAt.Equal(want) {
		t.Errorf("Los Angeles' Oct 9 starts at %v, want %v", inLosAngeles[0].StartAt, want)
	}
	if !inKampala[1].StartAt.Equal(cached[1].StartAt) {
		t.Error("a timed event was moved")
	}
	if !cached[0].StartAt.IsZero() {
		t.Error("the cached event was changed instead of a copy")
	}
}
