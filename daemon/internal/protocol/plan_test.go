package protocol_test

import (
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The plan wire shapes (docs/backend-checklist.md B5.2, inventory N6): the body that replaces a
// plan's steps, and the event that says a plan changed. Each has a golden file the web client's
// mapper tests read too, so the two sides cannot drift.
//
// The plan block itself is drawn by chatMessages in history_test.go, which the same mapper test
// reads: a plan is a message in the card's chat, so it appears there with every other kind.

func planNow() time.Time {
	return time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
}

func TestEditPlanRequestGolden(t *testing.T) {
	testutil.Golden(t, "edit-plan-request", protocol.EditPlanRequest{
		Steps: []string{"Read the router", "Add the route"},
	})
}

func TestPlanUpdatedEventGolden(t *testing.T) {
	testutil.Golden(t, "plan-updated-event", protocol.PlanUpdatedEventData{
		CardID:    "crd_01JQZ0000000000000000000AC",
		MessageID: "evt_01JQZ0000000000000000000AE",
		Plan: protocol.ChatPlan{
			State: protocol.ChatPlanStateApproved,
			Steps: []string{"Read the router", "Add the route"},
			Files: []string{"internal/api/router.go"},
			Risks: []string{"A route that already exists is refused"},
			// A list the plan does not name is drawn as an empty list and never null, the same as
			// every other list on the wire.
			Checks: []string{},
		},
		At: protocol.NewTimestamp(planNow()),
	})
}
