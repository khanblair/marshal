package session_test

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The approval flow (docs/backend-checklist.md B3.4): an agent that can ask waits, the request is
// recorded and announced, and answering it reaches the agent unchanged and is announced to every
// view of the same row.

// permissionRequest is one request the fake agent asks, with the two options every real agent
// offers a plain yes or no.
func permissionRequest() agents.PermissionRequested {
	return agents.PermissionRequested{
		RequestID: "req-1", ToolCallID: "call-1",
		Title: "Write hello.txt", Kind: "edit", Path: "hello.txt",
		Options: []agents.PermissionOption{
			{ID: "allow-once", Name: "Allow once", Kind: agents.OptionAllowOnce},
			{ID: "reject-once", Name: "Deny", Kind: agents.OptionRejectOnce},
		},
	}
}

// commandRequest is one request to run a command. It is the shape a card still asks a person about
// in every mode but full auto and bypass, which is why a test about the person being asked uses it.
func commandRequest() agents.PermissionRequested {
	req := permissionRequest()
	req.Title, req.Kind, req.Path, req.Command = "Run the tests", "execute", "", "go test ./..."
	return req
}

// sessionRow reads the stored session row of a card.
func (e *env) sessionRow(t *testing.T, cardID string) db.Session {
	t.Helper()
	row, err := e.store.Queries().GetSessionByCard(context.Background(), cardID)
	if err != nil {
		t.Fatalf("read the session of card %s: %v", cardID, err)
	}
	return row
}

// asked is what startAsking learned: the card, the agent session id, the approval as it was
// announced, and whether a waiting-approval state change was published before the request.
type asked struct {
	card      protocol.Card
	sessionID string
	approval  protocol.Approval
	held      bool
}

// startAsking starts a card's session and makes its agent ask for permission, reading events until
// the request is announced. The card is in ask mode, where every request is the person's, so what
// this proves is the ask flow itself and not one of the modes the harness answers on its own.
func (e *env) startAsking(t *testing.T) asked {
	t.Helper()
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.cardWithMode(t, project.ID, "Write a file", protocol.PermissionModeAsk)
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	row := e.sessionRow(t, card.ID)
	if !e.agent.ask(row.AgentSessionID, permissionRequest()) {
		t.Fatal("the fake agent does not know the session it was asked to ask on")
	}
	a := asked{card: card, sessionID: row.AgentSessionID}
	timeout := time.After(eventTimeout)
	for a.approval.ID == "" {
		select {
		case ev, ok := <-e.sub.C():
			if !ok {
				t.Fatal("the subscription closed before the request was announced")
			}
			switch data := ev.Data.(type) {
			case protocol.SessionStateChangedEventData:
				if data.State == protocol.SessionStateWaitingApproval && data.CardID == card.ID {
					a.held = true
				}
			case protocol.ApprovalRequestedEventData:
				a.approval = data.Approval
			}
		case <-timeout:
			t.Fatal("no approval.requested event arrived")
		}
	}
	return a
}

func TestPermissionRequestHoldsTheSessionAndIsAnnounced(t *testing.T) {
	e := newEnv(t)
	a := e.startAsking(t)

	if a.approval.State != protocol.ChatApprovalStateWaiting {
		t.Errorf("the announced approval state = %q, want waiting", a.approval.State)
	}
	if a.approval.Title != "Write hello.txt" || a.approval.Kind != "edit" || a.approval.Path != "hello.txt" {
		t.Errorf("the announced approval = %+v, want the request the agent made", a.approval)
	}
	if len(a.approval.Options) != 2 {
		t.Errorf("the announced approval has %d options, want 2", len(a.approval.Options))
	}
	if a.approval.CardID != a.card.ID || a.approval.SessionID == "" {
		t.Errorf("the announced approval = %+v, want it to name the card and its session", a.approval)
	}
	if !protocol.ValidID(a.approval.ID) {
		t.Errorf("the approval id %q is not an opaque id", a.approval.ID)
	}
	if !a.held {
		t.Error("no waiting-approval state change was published before the request")
	}
	if row := e.sessionRow(t, a.card.ID); row.State != string(protocol.SessionStateWaitingApproval) {
		t.Errorf("the stored session state = %q, want waiting-approval", row.State)
	}
}

func TestRespondAnswersTheAgentAndAnnouncesIt(t *testing.T) {
	e := newEnv(t)
	a := e.startAsking(t)

	err := e.mgr.Respond(context.Background(), a.approval.ID, protocol.ApprovalDecisionApproved, "", audit.ActorPerson)
	if err != nil {
		t.Fatalf("respond to the approval: %v", err)
	}

	// The person's answer reached the agent, with the plain allow option rather than the label.
	resp, ok := e.agent.approvalResponse("req-1")
	if !ok {
		t.Fatal("the agent was never answered")
	}
	if resp.OptionID != "allow-once" || resp.Cancelled {
		t.Errorf("the agent was answered %+v, want option allow-once and not cancelled", resp)
	}

	// Every view of the same row hears it.
	ev := e.untilType(t, protocol.EventTypeApprovalResolved)
	data, ok := ev.Data.(protocol.ApprovalResolvedEventData)
	if !ok {
		t.Fatalf("approval.resolved data = %#v", ev.Data)
	}
	if data.ApprovalID != a.approval.ID || data.State != protocol.ChatApprovalStateApproved {
		t.Errorf("approval.resolved = %+v, want the answered approval, approved", data)
	}
	if data.DecidedBy != audit.ActorPerson {
		t.Errorf("approval.resolved decidedBy = %q, want person", data.DecidedBy)
	}

	// The row is recorded and the session is put back where it was.
	row, err := e.store.Queries().GetApproval(context.Background(), a.approval.ID)
	if err != nil {
		t.Fatalf("read the approval row: %v", err)
	}
	if row.Decision != string(protocol.ApprovalDecisionApproved) || row.DecidedBy != audit.ActorPerson {
		t.Errorf("the stored approval = %+v, want it approved by a person", row)
	}
	if row := e.sessionRow(t, a.card.ID); row.State != string(protocol.SessionStateWorking) {
		t.Errorf("the session state after answering = %q, want working", row.State)
	}

	// The decision is on the audit trail.
	rows, err := e.store.Queries().ListAuditLog(context.Background(), 10)
	if err != nil {
		t.Fatalf("read the audit log: %v", err)
	}
	if len(rows) != 1 || rows[0].Action != audit.ActionApprove || rows[0].Target != a.approval.ID {
		t.Errorf("the audit log = %+v, want one approve row naming the approval", rows)
	}
}

// TestPermissionRequestMovesTheCardToNeedsYou is S8b's daemon-side half of B3.4's "the Home needs
// you list" done-when: before this, a waiting approval held the session but never touched the
// card's own state, so it never reached Home's needs-you list at all (a card-state driven list,
// section 3 of the phase report).
func TestPermissionRequestMovesTheCardToNeedsYou(t *testing.T) {
	e := newEnv(t)
	a := e.startAsking(t)

	card, err := e.proj.Card(context.Background(), a.card.ID)
	if err != nil {
		t.Fatalf("read the card: %v", err)
	}
	if card.State != protocol.CardStateNeeds {
		t.Errorf("the card's state = %q, want needs", card.State)
	}
	if card.NeedsReason == nil || card.NeedsReason.Kind != protocol.NeedsReasonKindApprovalNeeded {
		t.Fatalf("the card's needs reason = %+v, want approval-needed", card.NeedsReason)
	}
	if card.NeedsReason.ApprovalID != a.approval.ID {
		t.Errorf("the card's needs reason approval id = %q, want %q", card.NeedsReason.ApprovalID, a.approval.ID)
	}
}

// TestRespondMovesTheCardBackToWorking is the other half of the same gap: once the approval is
// answered, Home must stop showing the card, which means the card must leave Needs you again.
func TestRespondMovesTheCardBackToWorking(t *testing.T) {
	e := newEnv(t)
	a := e.startAsking(t)

	if err := e.mgr.Respond(context.Background(), a.approval.ID, protocol.ApprovalDecisionApproved, "", audit.ActorPerson); err != nil {
		t.Fatalf("respond to the approval: %v", err)
	}
	card, err := e.proj.Card(context.Background(), a.card.ID)
	if err != nil {
		t.Fatalf("read the card: %v", err)
	}
	if card.State != protocol.CardStateWorking {
		t.Errorf("the card's state after answering = %q, want working", card.State)
	}
	if card.NeedsReason != nil && card.NeedsReason.ApprovalID != "" {
		t.Errorf("the card's needs reason after answering = %+v, want no approval waiting", card.NeedsReason)
	}
}

func TestRespondRefusesARequestNobodyIsWaitingOn(t *testing.T) {
	e := newEnv(t)
	a := e.startAsking(t)
	ctx := context.Background()

	unknown, err := protocol.NewID(time.Now(), rand.Reader)
	if err != nil {
		t.Fatalf("make an id: %v", err)
	}
	wantCode(t, e.mgr.Respond(ctx, unknown, protocol.ApprovalDecisionApproved, "", audit.ActorPerson), protocol.ErrorCodeNotFound)

	// A decision that is not one of the two is refused, and the request is left waiting.
	wantCode(t, e.mgr.Respond(ctx, a.approval.ID, protocol.ApprovalDecision("maybe"), "", audit.ActorPerson), protocol.ErrorCodeInvalidArgument)
	if _, ok := e.agent.approvalResponse("req-1"); ok {
		t.Fatal("a request with a bad decision was answered anyway")
	}

	// Answering it once works, and a second answer is refused rather than overwriting the first.
	if err := e.mgr.Respond(ctx, a.approval.ID, protocol.ApprovalDecisionDenied, "", audit.ActorPerson); err != nil {
		t.Fatalf("deny the approval: %v", err)
	}
	wantCode(t, e.mgr.Respond(ctx, a.approval.ID, protocol.ApprovalDecisionApproved, "", audit.ActorPerson), protocol.ErrorCodeConflict)
}
