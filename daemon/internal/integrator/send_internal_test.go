package integrator

import (
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func TestASendIsRefusedForACardTheMergeCannotTakeYet(t *testing.T) {
	working := protocol.SessionStateWorking
	asleep := protocol.SessionStateAsleep
	tests := []struct {
		name   string
		card   protocol.Card
		reason string
	}{
		{"a merged card", protocol.Card{State: protocol.CardStateDone, Branch: "b"}, "send_not_open"},
		{"a card being merged", protocol.Card{State: protocol.CardStateMerging, Branch: "b"}, "send_not_open"},
		{"a backlog card", protocol.Card{State: protocol.CardStateBacklog}, "send_not_started"},
		{"a card with no branch", protocol.Card{State: protocol.CardStateWorking}, "send_not_started"},
		{"a card whose agent is writing", protocol.Card{State: protocol.CardStateWorking, Branch: "b", Session: &working}, "send_agent_working"},
		{"a card whose agent is asleep", protocol.Card{State: protocol.CardStateWorking, Branch: "b", Session: &asleep}, ""},
		{"a card in review", protocol.Card{State: protocol.CardStateReview, Branch: "b"}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := sendStateRefusal(tc.card)
			if tc.reason == "" {
				if err != nil {
					t.Fatalf("sendStateRefusal = %v, want none", err)
				}
				return
			}
			perr, ok := err.(*protocol.Error)
			if !ok || perr.Details["reason"] != tc.reason {
				t.Fatalf("sendStateRefusal = %v, want reason %q", err, tc.reason)
			}
		})
	}
}
