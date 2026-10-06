package protocol_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The wire shape of a schedule (B8.1, N-la, build-plan 8.1): one brief or job as the Schedules screen
// shows it, and the list a read answers with. The Go test writes the golden file and the web client's
// tests read the same one, so a change to the shape shows up on both sides.

// sampleSchedule is the Morning brief the screens seed with
// (apps/web/src/mock/seed/calendar.ts), so the golden file is a real row and not a made-up one.
func sampleSchedule() protocol.Schedule {
	return protocol.Schedule{
		ID:       "01H1234567890ABCDEFGHJKMNP",
		Name:     "Morning brief",
		Kind:     "brief",
		Icon:     "sun",
		Trigger:  "Cron",
		When:     "Every weekday at 09:00",
		Time:     "09:00",
		Days:     []int{1, 2, 3, 4, 5},
		Action:   "morning_brief",
		Project:  "small-repo",
		Enabled:  true,
		Missed:   "run_now",
		Template: "morning", Sections: []string{"calendar", "needs-you"}, Deliver: []string{"telegram"},
	}
}

// scheduleNow is the fixed time the schedule-list tests are stamped with, so nothing depends on the
// machine's clock.
var scheduleNow = time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC)

func TestScheduleGolden(t *testing.T) {
	testutil.Golden(t, "schedule", sampleSchedule())
}

// The answer to GET /v1/schedules: the rows and the daemon's own time beside them, so a screen can
// tell a stale answer from a fresh one.
func TestScheduleListGolden(t *testing.T) {
	list := protocol.NewScheduleList([]protocol.Schedule{
		sampleSchedule(),
		{
			ID: "01H1234567890ABCDEFGHJKMNPQ", Name: "Evening brief", Kind: "brief", Icon: "sunset",
			Trigger: "Cron", When: "Every weekday at 18:00", Time: "18:00", Days: []int{1, 2, 3, 4, 5},
			Action: "Send the brief to the app and Obsidian", Project: "All projects", Enabled: true,
			Missed: "Skip", Sections: []string{}, Deliver: []string{}, QuietWhenEmpty: true,
		},
	}, scheduleNow)
	testutil.Golden(t, "schedule-list", list)
}

// The answer to GET /v1/schedules/catalog: what the editor offers, with the daemon's own time.
func TestScheduleCatalogGolden(t *testing.T) {
	testutil.Golden(t, "schedule-catalog", protocol.ScheduleCatalog{
		Templates: []protocol.ScheduleTemplate{{
			Key: "morning", Name: "Morning brief", Summary: "Today's calendar and what waits on you.", Icon: "sunrise",
			Trigger: "Cron", When: "Every weekday at 8:00", Time: "08:00", Days: []int{1, 2, 3, 4, 5}, Missed: "Skip",
			Sections: []string{"calendar", "needs-you"}, QuietWhenEmpty: false,
		}},
		Sections:   []protocol.ScheduleSection{{ID: "calendar", Label: "Calendar", Hint: "Your Google Calendar events."}},
		Channels:   []protocol.ScheduleChannel{{ID: "telegram", Label: "Telegram"}},
		ServerTime: protocol.NewTimestamp(scheduleNow),
	})
}

// A list carries the daemon's own time beside the schedules, so a screen can tell a stale answer
// from a fresh one. An empty list is [] and never null, which is what a screen with nothing to draw
// reads.
func TestAScheduleListIsNeverNull(t *testing.T) {
	encoded, err := json.Marshal(protocol.NewScheduleList(nil, scheduleNow))
	if err != nil {
		t.Fatalf("encode an empty schedule list: %v", err)
	}
	if !strings.Contains(string(encoded), `"schedules":[]`) {
		t.Errorf("an empty list encoded as %s, want [] and not null", encoded)
	}
}

// NewScheduleList copies what it is handed, so a caller that keeps writing to its own slice does not
// change an answer that has already been built.
func TestAScheduleListCopiesItsSchedules(t *testing.T) {
	own := []protocol.Schedule{sampleSchedule()}
	list := protocol.NewScheduleList(own, scheduleNow)
	own[0].Name = "changed after the fact"
	if list.Schedules[0].Name != "Morning brief" {
		t.Errorf("the list answered %q, want the name it was built from", list.Schedules[0].Name)
	}
}

// A calendar with nothing connected sends empty lists, never null, so a client can read them.
func TestACalendarListWithNothingInItHasNoNullLists(t *testing.T) {
	body, err := json.Marshal(protocol.NewCalendarList(nil, nil, nil, protocol.GoogleReading{}, time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("null")) {
		t.Fatalf("the answer holds a null: %s", body)
	}
}
