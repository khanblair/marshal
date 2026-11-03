package session_test

import (
	"context"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func TestAnsweringOneApprovalKeepsAnotherVisible(t *testing.T) {
	e := newEnv(t)
	a := e.startAsking(t)
	second := permissionRequest()
	second.RequestID, second.ToolCallID = "req-2", "call-2"
	if !e.agent.ask(a.sessionID, second) {
		t.Fatal("second request not delivered")
	}
	ev := e.untilType(t, protocol.EventTypeApprovalRequested)
	id := ev.Data.(protocol.ApprovalRequestedEventData).Approval.ID
	if err := e.mgr.Respond(context.Background(), id, protocol.ApprovalDecisionApproved, "", audit.ActorPerson); err != nil {
		t.Fatal(err)
	}
	card, err := e.proj.Card(context.Background(), a.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if card.State != protocol.CardStateNeeds || card.NeedsReason == nil || card.NeedsReason.ApprovalID != a.approval.ID {
		t.Fatalf("remaining approval hidden: %+v", card)
	}
	if state := e.sessionRow(t, a.card.ID).State; state != string(protocol.SessionStateWaitingApproval) {
		t.Fatalf("session state = %s", state)
	}
	if err := e.mgr.Respond(context.Background(), a.approval.ID, protocol.ApprovalDecisionDenied, "", audit.ActorPerson); err != nil {
		t.Fatal(err)
	}
	card, err = e.proj.Card(context.Background(), a.card.ID)
	if err != nil || card.State != protocol.CardStateWorking {
		t.Fatalf("finished approvals did not clear needs: %+v, %v", card, err)
	}
}
