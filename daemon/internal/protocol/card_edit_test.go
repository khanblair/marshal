package protocol_test

import (
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The golden files for the card edits, moves, labels, and the Home answer. A Go test writes them
// and the TypeScript test reads them, so the two sides cannot drift apart.

func sampleDate() protocol.Timestamp {
	return protocol.NewTimestamp(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
}

func TestUpdateCardRequestGolden(t *testing.T) {
	title, role, pkg := "Add a health check endpoint", "Implementer", "packages/api"
	agent, mode := protocol.AgentKindClaude, protocol.PermissionModeAutoEdits
	plannedStart := protocol.DateChange{At: ptr(sampleDate())}
	due := protocol.DateChange{Clear: true}
	testutil.Golden(t, "update-card-request", protocol.UpdateCardRequest{
		Title: &title, Role: &role, Package: &pkg, Agent: &agent, PermissionMode: &mode,
		PlannedStart: &plannedStart, Due: &due, Labels: &[]string{"01M3C107JB04106105A"},
	})
}

// A date that is being cleared carries no moment at all, so nothing has to be invented for it.
func TestUpdateCardRequestClearingADateLeavesTheMomentOut(t *testing.T) {
	cleared := protocol.DateChange{Clear: true}
	testutil.Golden(t, "update-card-request-clear-date", protocol.UpdateCardRequest{Due: &cleared})
}

func TestMoveCardRequestGolden(t *testing.T) {
	testutil.Golden(t, "move-card-request", protocol.MoveCardRequest{State: protocol.CardStateWorking})
}

func TestLabelSnapshotGolden(t *testing.T) {
	testutil.Golden(t, "label-snapshot", protocol.LabelSnapshot{
		ProjectID: sampleProjectID,
		Labels: []protocol.Label{
			{ID: "01M3C107JB041061050R3GG28B", ProjectID: sampleProjectID, Name: "backend", Color: protocol.LabelColorBlue, CreatedAt: sampleDate()},
			{ID: "01M3C107JB041061050R3GG28C", ProjectID: sampleProjectID, Name: "urgent", Color: protocol.LabelColorRed, CreatedAt: sampleDate()},
		},
		ServerTime: sampleDate(),
	})
}

func TestCreateLabelRequestGolden(t *testing.T) {
	testutil.Golden(t, "create-label-request", protocol.CreateLabelRequest{
		Name: "urgent", Color: protocol.LabelColorRed,
	})
}

func TestUpdateLabelRequestGolden(t *testing.T) {
	name := "urgent"
	testutil.Golden(t, "update-label-request", protocol.UpdateLabelRequest{Name: &name})
}

// The Home answer a client draws on first load, with the two lists and the tile counts.
func TestHomeSnapshotGolden(t *testing.T) {
	waiting := sampleDate()
	testutil.Golden(t, "home-snapshot", protocol.HomeSnapshot{
		Needs: []protocol.NeedsCard{{
			CardID: sampleCardID, Key: "web-dashboard#12", Number: 12,
			ProjectID: sampleProjectID, ProjectName: "web-dashboard", Title: "Add a health check endpoint",
			Reason: protocol.NeedsReason{
				Kind: protocol.NeedsReasonKindPlanReady,
				Text: "The plan is ready for review.",
			},
			WaitingSince: &waiting, Role: "Implementer",
		}},
		Awake: []protocol.AwakeCard{{
			CardID: sampleCardID, Key: "web-dashboard#12", Number: 12,
			ProjectID: sampleProjectID, ProjectName: "web-dashboard", Title: "Add a health check endpoint",
			State: protocol.CardStateWorking, Session: protocol.SessionStateWorking,
			DoingNow: "Writing the handler", Pinned: true, Paused: false, ContextUsed: 42,
			AwakeSince: &waiting,
		}},
		Tiles:      protocol.HomeTiles{Needs: 1, Working: 1, MergedToday: 3},
		ServerTime: sampleDate(),
	})
}

// An empty Home answer carries empty lists, never null.
func TestHomeSnapshotWithNothingToShow(t *testing.T) {
	testutil.Golden(t, "home-snapshot-empty", protocol.HomeSnapshot{
		Needs:      []protocol.NeedsCard{},
		Awake:      []protocol.AwakeCard{},
		Tiles:      protocol.HomeTiles{},
		ServerTime: sampleDate(),
	})
}

// ptr is for the request fields that are pointers, so this file can set one inline.
func ptr[T any](v T) *T { return &v }
