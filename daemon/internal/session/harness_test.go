package session_test

import (
	"context"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
	"github.com/khanblair/marshal/daemon/internal/session"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The harness gate (docs/backend-checklist.md B3.1, B3.3, B3.7): a request the session's mode, the
// command blocklist, or the deploy rule already has an answer for never reaches a person, and the
// daemon's answer is audited and told to the card's own history.

// startInMode starts a card's session in a permission mode and returns the environment, the card,
// and the agent's own session id, so a test can make the fake agent ask.
func startInMode(t *testing.T, mode protocol.PermissionMode) (*env, string, string) {
	t.Helper()
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.cardWithMode(t, project.ID, "Do the work", mode)
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	return e, card.ID, e.sessionRow(t, card.ID).AgentSessionID
}

// answeredAs waits for the daemon to answer a request and fails the test unless the agent was given
// this option.
func answeredAs(t *testing.T, e *env, requestID, wantOption string) {
	t.Helper()
	waitFor(t, "the daemon to answer "+requestID, func() bool {
		_, ok := e.agent.approvalResponse(requestID)
		return ok
	})
	resp, ok := e.agent.approvalResponse(requestID)
	if !ok {
		t.Fatalf("the agent was never answered for %s", requestID)
	}
	if resp.OptionID != wantOption || resp.Cancelled {
		t.Errorf("the agent was answered %+v, want option %s", resp, wantOption)
	}
}

// daemonAuditRow waits for the first audit row and returns it. It is the row a decision on the
// person's behalf leaves behind.
func daemonAuditRow(t *testing.T, e *env) db.AuditLog {
	t.Helper()
	waitFor(t, "an audit row from the daemon", func() bool {
		rows := e.auditRows(t)
		return len(rows) > 0 && rows[0].Actor == audit.ActorDaemon
	})
	return e.auditRows(t)[0]
}

func TestABlockedCommandIsRefusedWithoutAsking(t *testing.T) {
	e, cardID, sessionID := startInMode(t, protocol.PermissionModeFullAuto)
	req := commandRequest()
	req.Title, req.Command = "Clean up the disk", "rm -rf /"
	if !e.agent.ask(sessionID, req) {
		t.Fatal("the fake agent does not know the session it was asked to ask on")
	}

	answeredAs(t, e, "req-1", "reject-once")

	row := daemonAuditRow(t, e)
	if row.Action != audit.ActionDeny {
		t.Errorf("the audit row action = %q, want %q", row.Action, audit.ActionDeny)
	}
	if !contains(row.DetailJSON, `"reason":"`+security.RuleRecursiveDelete+`"`) {
		t.Errorf("the audit detail = %s, want the rule that refused it", row.DetailJSON)
	}
	if !contains(row.DetailJSON, `"mode":"full-auto"`) {
		t.Errorf("the audit detail = %s, want the mode it was decided in", row.DetailJSON)
	}

	// The person is told in the card's own history, because the daemon refused on their behalf.
	store, err := history.New(e.store)
	if err != nil {
		t.Fatalf("make the history store: %v", err)
	}
	waitFor(t, "the refusal in the card's history", func() bool {
		return len(refusalNotes(t, store, cardID)) == 1
	})
}

func TestADeployIsThePersonsEvenInFullAuto(t *testing.T) {
	e, cardID, sessionID := startInMode(t, protocol.PermissionModeFullAuto)
	req := commandRequest()
	req.Title, req.Command = "Ship it", "npm run deploy"
	if !e.agent.ask(sessionID, req) {
		t.Fatal("the fake agent does not know the session it was asked to ask on")
	}

	// The request is announced and held: a deploy is never the daemon's to allow outside bypass.
	ev := e.untilType(t, protocol.EventTypeApprovalRequested)
	data, ok := ev.Data.(protocol.ApprovalRequestedEventData)
	if !ok || data.Approval.CardID != cardID {
		t.Fatalf("approval.requested = %#v, want the deploy on the card", ev.Data)
	}
	if _, answered := e.agent.approvalResponse("req-1"); answered {
		t.Error("the daemon answered a deploy instead of asking the person")
	}
	if rows := e.auditRows(t); len(rows) != 0 {
		t.Errorf("the audit log = %+v, want nothing: nobody decided anything", rows)
	}
}

func TestPlanModeRefusesAnEdit(t *testing.T) {
	e, _, sessionID := startInMode(t, protocol.PermissionModePlan)
	if !e.agent.ask(sessionID, permissionRequest()) {
		t.Fatal("the fake agent does not know the session it was asked to ask on")
	}

	answeredAs(t, e, "req-1", "reject-once")

	row := daemonAuditRow(t, e)
	if row.Action != audit.ActionDeny {
		t.Errorf("the audit row action = %q, want %q", row.Action, audit.ActionDeny)
	}
	if !contains(row.DetailJSON, `"reason":"mode"`) {
		t.Errorf("the audit detail = %s, want it to say the mode refused it", row.DetailJSON)
	}
}

func TestFullAutoAllowsAnOrdinaryCommandWithoutAsking(t *testing.T) {
	e, cardID, sessionID := startInMode(t, protocol.PermissionModeFullAuto)
	if !e.agent.ask(sessionID, commandRequest()) {
		t.Fatal("the fake agent does not know the session it was asked to ask on")
	}

	answeredAs(t, e, "req-1", "allow-once")

	row := daemonAuditRow(t, e)
	if row.Action != audit.ActionApprove {
		t.Errorf("the audit row action = %q, want %q", row.Action, audit.ActionApprove)
	}
	if !contains(row.DetailJSON, `"reason":"mode"`) {
		t.Errorf("the audit detail = %s, want it to say the mode allowed it", row.DetailJSON)
	}
	// Nobody was asked, so the session is left where a turn needs it: working.
	waitFor(t, "the session to read as working again", func() bool {
		return e.sessionRow(t, cardID).State == string(protocol.SessionStateWorking)
	})
}

func TestAutoEditsAllowsAnEditWithoutAsking(t *testing.T) {
	e, _, sessionID := startInMode(t, protocol.PermissionModeAutoEdits)
	if !e.agent.ask(sessionID, permissionRequest()) {
		t.Fatal("the fake agent does not know the session it was asked to ask on")
	}
	answeredAs(t, e, "req-1", "allow-once")
}

// TestAnAskModeCardStillAsksThePerson is the guard on the other side of the gate: the careful mode
// keeps its promise, and nothing the harness knows answers for the person in it.
func TestAnAskModeCardStillAsksThePerson(t *testing.T) {
	e, cardID, sessionID := startInMode(t, protocol.PermissionModeAsk)
	if !e.agent.ask(sessionID, commandRequest()) {
		t.Fatal("the fake agent does not know the session it was asked to ask on")
	}
	ev := e.untilType(t, protocol.EventTypeApprovalRequested)
	data, ok := ev.Data.(protocol.ApprovalRequestedEventData)
	if !ok || data.Approval.CardID != cardID {
		t.Fatalf("approval.requested = %#v, want the command on the card", ev.Data)
	}
	if _, answered := e.agent.approvalResponse("req-1"); answered {
		t.Error("the daemon answered a request in ask mode")
	}
}

// TestTheProfileIsEnforcedThroughTheSession covers the profile at the session level, not only in the
// harness's own tests: a manager told to allow nothing refuses an edit its mode would otherwise
// allow, without asking the person.
func TestTheProfileIsEnforcedThroughTheSession(t *testing.T) {
	e := newEnv(t, func(c *session.Config) {
		profile := security.NothingAllowed()
		c.Profile = &profile
	})
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.cardWithMode(t, project.ID, "Do the work", protocol.PermissionModeFullAuto)
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	sessionID := e.sessionRow(t, card.ID).AgentSessionID
	if !e.agent.ask(sessionID, permissionRequest()) {
		t.Fatal("the fake agent does not know the session it was asked to ask on")
	}

	answeredAs(t, e, "req-1", "reject-once")

	row := daemonAuditRow(t, e)
	if row.Action != audit.ActionDeny {
		t.Errorf("the audit row action = %q, want %q", row.Action, audit.ActionDeny)
	}
	if !contains(row.DetailJSON, `"reason":"`+security.RuleProfile+`"`) {
		t.Errorf("the audit detail = %s, want the profile rule", row.DetailJSON)
	}
}
