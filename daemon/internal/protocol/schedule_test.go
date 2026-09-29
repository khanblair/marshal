package protocol_test

import (
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
		ID:      "01H1234567890ABCDEFGHJKMNP",
		Name:    "Morning brief",
		Kind:    "brief",
		Icon:    "sun",
		Trigger: "Cron",
		When:    "Every weekday at 09:00",
		Time:    "09:00",
		Days:    []int{1, 2, 3, 4, 5},
		Action:  "morning_brief",
		Project: "small-repo",
		Enabled: true,
		Missed:  "run_now",
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
			Missed: "Skip",
		},
	}, scheduleNow)
	testutil.Golden(t, "schedule-list", list)
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
