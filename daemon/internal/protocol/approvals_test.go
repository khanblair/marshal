package protocol_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The approval wire shapes (docs/architecture.md section 11.4, docs/backend-checklist.md B3.4, N7).
// One Approval, the body that answers it, and the two events that carry it, each with a golden file
// the web client's mapper tests read too, so the two sides cannot drift.

func approvalNow() time.Time {
	return time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
}

// sampleApproval is one permission an agent asked a person for, in the waiting state, with the
// options a real agent offers.
func sampleApproval() protocol.Approval {
	return protocol.Approval{
		ID:         "app_01JQZ0000000000000000000AD",
		CardID:     "crd_01JQZ0000000000000000000AC",
		SessionID:  "ses_01JQZ0000000000000000000AB",
		ToolCallID: "call_7",
		Title:      "Run rm -rf build",
		Kind:       "execute",
		Command:    "rm -rf build",
		Options: []protocol.ApprovalOption{
			{ID: "allow-once", Name: "Allow once", Kind: "allow_once"},
			{ID: "allow-always", Name: "Always allow", Kind: "allow_always"},
			{ID: "reject-once", Name: "Reject", Kind: "reject_once"},
		},
		State: protocol.ChatApprovalStateWaiting,
		At:    protocol.NewTimestamp(approvalNow()),
	}
}

func TestApprovalGolden(t *testing.T) {
	testutil.Golden(t, "approval", sampleApproval())
}

func TestDecideApprovalRequestGolden(t *testing.T) {
	testutil.Golden(t, "decide-approval-request", protocol.DecideApprovalRequest{
		Decision: protocol.ApprovalDecisionApproved,
		OptionID: "allow-always",
	})
}

func TestApprovalRequestedEventGolden(t *testing.T) {
	testutil.Golden(t, "approval-requested-event", protocol.ApprovalRequestedEventData{
		CardID: "crd_01JQZ0000000000000000000AC", Approval: sampleApproval(),
	})
}

func TestApprovalResolvedEventGolden(t *testing.T) {
	testutil.Golden(t, "approval-resolved-event", protocol.ApprovalResolvedEventData{
		CardID: "crd_01JQZ0000000000000000000AC", ApprovalID: "app_01JQZ0000000000000000000AD",
		State: protocol.ChatApprovalStateApproved, DecidedBy: "person",
		At: protocol.NewTimestamp(approvalNow()),
	})
}

// TestAnApprovalDecisionsStateIsItsOwnState pins the one mapping the client relies on: a decision
// needs no translation table, because it is the state the approval moves to.
func TestAnApprovalDecisionsStateIsItsOwnState(t *testing.T) {
	tests := []struct {
		decision protocol.ApprovalDecision
		state    protocol.ChatApprovalState
	}{
		{protocol.ApprovalDecisionApproved, protocol.ChatApprovalStateApproved},
		{protocol.ApprovalDecisionDenied, protocol.ChatApprovalStateDenied},
	}
	for _, tt := range tests {
		if !tt.decision.Valid() {
			t.Errorf("%q is not a valid decision", tt.decision)
		}
		if got := tt.decision.State(); got != tt.state {
			t.Errorf("%q.State() = %q, want %q", tt.decision, got, tt.state)
		}
	}
	if protocol.ApprovalDecision("maybe").Valid() {
		t.Error(`"maybe" was accepted as a decision`)
	}
}

// TestAnApprovalsOptionsAreNeverNull guards the JSON contract: a request with no options must still
// encode as a list, because the client's mapper reads it as an array.
func TestAnApprovalsOptionsAreNeverNull(t *testing.T) {
	bare := sampleApproval()
	bare.ID, bare.Options = "app_01JQZ0000000000000000000AE", []protocol.ApprovalOption{}
	encoded, err := json.Marshal(bare)
	if err != nil {
		t.Fatalf("encode an approval with no options: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode the approval back: %v", err)
	}
	if _, ok := decoded["options"].([]any); !ok {
		t.Errorf(`options = %#v, want a list`, decoded["options"])
	}
}
