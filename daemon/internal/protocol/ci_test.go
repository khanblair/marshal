package protocol_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The wire shape of CI (B6.2, B6.4): one workflow's newest state on one branch, the CI health of a
// project, and what a simulated failure answers with.

// sampleCardRun is a failed run on a card's branch, the one the fix loop runs on.
func sampleCardRun() protocol.CiRun {
	started := providersNow.Add(-4 * time.Minute)
	return protocol.CiRun{
		ID:        "01JD7Q4M2X8K9V0P5T3RB6NHC1",
		ProjectID: sampleProjectID,
		CardID:    sampleCardID,
		Branch:    "marshal/card-12-fix-the-report",
		Workflow:  "ci",
		Status:    protocol.CIStateFailed,
		URL:       "https://github.com/khanblair/web-dashboard/actions/runs/9931",
		StartedAt: timestampPtr(started),
		UpdatedAt: protocol.NewTimestamp(providersNow),
	}
}

// timestampPtr wraps a time for a field that may be absent.
func timestampPtr(t time.Time) *protocol.Timestamp {
	value := protocol.NewTimestamp(t)
	return &value
}

// sampleMainRun is the project's default branch, which is what the CI health page shows first.
func sampleMainRun() protocol.CiRun {
	started := providersNow.Add(-42 * time.Minute)
	return protocol.CiRun{
		ID:        "01JD7Q4M2X8K9V0P5T3RB6NHC2",
		ProjectID: sampleProjectID,
		Branch:    "main",
		Workflow:  "ci packages/web",
		Status:    protocol.CIStatePassed,
		URL:       "https://github.com/khanblair/web-dashboard/actions/runs/9928",
		StartedAt: timestampPtr(started),
		UpdatedAt: protocol.NewTimestamp(started.Add(6 * time.Minute)),
	}
}

// sampleQueuedRun is a run Marshal has been told about and that has not started: its start time is
// null, because a queued run has not started.
func sampleQueuedRun() protocol.CiRun {
	return protocol.CiRun{
		ID:        "01JD7Q4M2X8K9V0P5T3RB6NHC3",
		ProjectID: sampleProjectID,
		CardID:    sampleCardID,
		Branch:    "marshal/card-12-fix-the-report",
		Workflow:  "lint",
		Status:    protocol.CIStateQueued,
		URL:       "https://github.com/khanblair/web-dashboard/actions/runs/9932",
		UpdatedAt: protocol.NewTimestamp(providersNow),
	}
}

func sampleCISnapshot() protocol.CISnapshot {
	return protocol.NewCISnapshot([]protocol.ProjectCI{{
		ProjectID: sampleProjectID,
		Status:    protocol.CIStatePassed,
		Runs:      []protocol.CiRun{sampleQueuedRun(), sampleCardRun(), sampleMainRun()},
	}}, providersNow)
}

func TestCiRunGolden(t *testing.T) {
	testutil.Golden(t, "ci-run", sampleCardRun())
}

func TestCiSnapshotGolden(t *testing.T) {
	testutil.Golden(t, "ci-snapshot", sampleCISnapshot())
}

func TestCiEventGolden(t *testing.T) {
	project := sampleCISnapshot().Projects[0]
	snapshot := sampleCISnapshot()
	// One event carries either the project's own health or the whole snapshot, never both.
	testutil.Golden(t, "ci-event-project", protocol.CIEventData{Project: &project})
	testutil.Golden(t, "ci-event-home", protocol.CIEventData{Snapshot: &snapshot})
}

// A queued run does not claim a start time. A Timestamp that was never set does not encode at all,
// so a start time that has not happened must be a null and not a zero.
func TestAQueuedRunCarriesNoStartTime(t *testing.T) {
	body, err := json.Marshal(sampleQueuedRun())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"startedAt":null`) {
		t.Errorf("a queued run encoded as %s, want a null startedAt", body)
	}
	if got := sampleCardRun().StartedAt; got == nil {
		t.Error("a run that started carries the moment it started")
	}
}

// A run Marshal has no card for carries an empty card id rather than a made-up one: the default
// branch is not a card's branch.
func TestAMainBranchRunHasNoCard(t *testing.T) {
	if got := sampleMainRun().CardID; got != "" {
		t.Errorf("CardID = %q, want empty for a run on main", got)
	}
}

// A project with no runs is left out of the answer entirely, and a project that has runs never
// encodes a null list.
func TestCiSnapshotNeverEncodesNull(t *testing.T) {
	empty, err := json.Marshal(protocol.NewCISnapshot(nil, providersNow))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(empty); !strings.Contains(got, `"projects":[]`) {
		t.Errorf("an empty answer encoded as %s, want []", got)
	}
	body, err := json.Marshal(protocol.NewCISnapshot([]protocol.ProjectCI{{ProjectID: sampleProjectID}}, providersNow))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); !strings.Contains(got, `"runs":[]`) {
		t.Errorf("a project with no runs encoded as %s, want []", got)
	}
	if !strings.Contains(string(body), `"serverTime":"2026-09-27T09:30:00.000Z"`) {
		t.Errorf("an answer carries the daemon's time: %s", body)
	}
}

func TestSimulateModeValuesAreTheTwoModes(t *testing.T) {
	got := protocol.SimulateModeValues()
	want := []protocol.SimulateMode{protocol.SimulateModeSynthetic, protocol.SimulateModeReal}
	if len(got) != len(want) {
		t.Fatalf("SimulateModeValues = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SimulateModeValues = %v, want %v", got, want)
		}
	}
	if !protocol.SimulateModeSynthetic.Valid() || !protocol.SimulateModeReal.Valid() {
		t.Error("Valid refuses one of the two modes")
	}
	if protocol.SimulateMode("fake").Valid() || protocol.SimulateMode("").Valid() {
		t.Error("Valid accepts something that is not a mode")
	}
}

func TestSimulateCIFailureRequestGolden(t *testing.T) {
	testutil.Golden(t, "simulate-ci-failure-request", protocol.SimulateCIFailureRequest{
		Mode: protocol.SimulateModeSynthetic,
	})
}

func TestSimulateCIFailureResultGolden(t *testing.T) {
	testutil.Golden(t, "simulate-ci-failure-result", protocol.SimulateCIFailureResult{
		CardID:     sampleCardID,
		Mode:       protocol.SimulateModeSynthetic,
		Run:        sampleCardRun(),
		FixStarted: true,
	})
}
