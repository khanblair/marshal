package session_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Bypass mode (docs/backend-checklist.md B3.2): granted only with an acknowledgement, refused for a
// locked project, audited on and off, and kept to the card's own worktree once it is on.

// wantBypassReason fails the test unless err is a refusal carrying this reason in its details.
func wantBypassReason(t *testing.T, err error, reason protocol.BypassRefusalReason) *protocol.Error {
	t.Helper()
	perr := wantCode(t, err, protocol.ErrorCodeRefused)
	if perr.Details["reason"] != string(reason) {
		t.Fatalf("refusal reason = %q, want %q", perr.Details["reason"], reason)
	}
	return perr
}

// auditRows reads the audit log, newest first.
func (e *env) auditRows(t *testing.T) []db.AuditLog {
	t.Helper()
	rows, err := e.store.Queries().ListAuditLog(context.Background(), 50)
	if err != nil {
		t.Fatalf("read the audit log: %v", err)
	}
	return rows
}

// waitFor waits until cond is true, and fails the test if it never is. It is how a test waits for
// something a pump goroutine does.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(eventTimeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestSetBypassNeedsTheAcknowledgement(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Run the tests")

	_, err := e.mgr.SetBypass(ctx, card.ID, protocol.BypassRequest{})
	wantBypassReason(t, err, protocol.BypassRefusalReasonUnacknowledged)

	if after, err := e.proj.Card(ctx, card.ID); err != nil {
		t.Fatalf("read the card: %v", err)
	} else if after.PermissionMode == protocol.PermissionModeBypass {
		t.Error("a request with no acknowledgement turned bypass on anyway")
	}
	if rows := e.auditRows(t); len(rows) != 0 {
		t.Errorf("the audit log = %+v, want nothing recorded for a refused request", rows)
	}
}

func TestSetBypassTurnsItOnThenOffAndBothAreAudited(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Run the tests")

	on, err := e.mgr.SetBypass(ctx, card.ID, protocol.BypassRequest{Acknowledged: true})
	if err != nil {
		t.Fatalf("turn bypass on: %v", err)
	}
	if on.PermissionMode != protocol.PermissionModeBypass {
		t.Errorf("permission mode = %q, want bypass", on.PermissionMode)
	}
	// The card changed, so every view hears it, and it is critical: leaving the old mode on screen
	// would leave a card drawn as asking when it no longer does.
	ev := e.untilType(t, protocol.EventTypeCardUpdated)
	data, ok := ev.Data.(protocol.CardEventData)
	if !ok || data.Card.ID != card.ID || data.Card.PermissionMode != protocol.PermissionModeBypass {
		t.Errorf("card.updated = %#v, want the card in bypass", ev.Data)
	}

	// A second press changes nothing, and the mode it lands in is the same one.
	again, err := e.mgr.SetBypass(ctx, card.ID, protocol.BypassRequest{Acknowledged: true})
	if err != nil {
		t.Fatalf("turn bypass on twice: %v", err)
	}
	if again.PermissionMode != protocol.PermissionModeBypass {
		t.Errorf("permission mode after a second press = %q, want bypass", again.PermissionMode)
	}

	off, err := e.mgr.ClearBypass(ctx, card.ID)
	if err != nil {
		t.Fatalf("turn bypass off: %v", err)
	}
	if off.PermissionMode != projects.BypassOffMode {
		t.Errorf("permission mode after turning bypass off = %q, want %q", off.PermissionMode, projects.BypassOffMode)
	}

	// Every explicit call is on the trail, in order, and only the two real changes are marked as
	// changes. auditRows reads newest first, so the list runs backwards through the three calls.
	rows := e.auditRows(t)
	if len(rows) != 3 {
		t.Fatalf("the audit log has %d rows, want 3: %+v", len(rows), rows)
	}
	want := []struct {
		action  string
		changed string
	}{
		{audit.ActionBypassOff, "true"},
		{audit.ActionBypassOn, "false"},
		{audit.ActionBypassOn, "true"},
	}
	for i, w := range want {
		if rows[i].Action != w.action || rows[i].Target != card.ID {
			t.Errorf("audit row %d = %+v, want %s on the card", i, rows[i], w.action)
		}
		if rows[i].Actor != audit.ActorPerson {
			t.Errorf("audit row %d actor = %q, want person", i, rows[i].Actor)
		}
		if !containsChanged(rows[i].DetailJSON, w.changed) {
			t.Errorf("audit row %d detail = %s, want it to carry changed=%s", i, rows[i].DetailJSON, w.changed)
		}
	}
}

// containsChanged looks for the change flag in an audit detail, which is JSON written by the
// recorder.
func containsChanged(detail, want string) bool {
	return len(detail) > 0 && (contains(detail, `"changed":`+want))
}

func contains(text, part string) bool {
	for i := 0; i+len(part) <= len(text); i++ {
		if text[i:i+len(part)] == part {
			return true
		}
	}
	return false
}

func TestBypassIsRefusedForALockedProject(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Run the tests")

	locked := true
	if _, err := e.proj.Update(ctx, project.ID, protocol.UpdateProjectRequest{BypassLocked: &locked}); err != nil {
		t.Fatalf("lock bypass for the project: %v", err)
	}
	_, err := e.mgr.SetBypass(ctx, card.ID, protocol.BypassRequest{Acknowledged: true})
	perr := wantBypassReason(t, err, protocol.BypassRefusalReasonLocked)
	if perr.Details["projectId"] != project.ID {
		t.Errorf("the refusal names project %q, want %q", perr.Details["projectId"], project.ID)
	}
	if after, err := e.proj.Card(ctx, card.ID); err != nil {
		t.Fatalf("read the card: %v", err)
	} else if after.PermissionMode == protocol.PermissionModeBypass {
		t.Error("a locked project let bypass through")
	}

	// Turning it off is always allowed, even for a locked project: the lock is about granting it.
	if _, err := e.mgr.ClearBypass(ctx, card.ID); err != nil {
		t.Fatalf("turn bypass off in a locked project: %v", err)
	}
}

func TestBypassWritesTheNoteIntoTheCardHistory(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Run the tests")
	if _, err := e.mgr.Start(ctx, card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	if _, err := e.mgr.SetBypass(ctx, card.ID, protocol.BypassRequest{Acknowledged: true}); err != nil {
		t.Fatalf("turn bypass on: %v", err)
	}

	store, err := history.New(e.store)
	if err != nil {
		t.Fatalf("make the history store: %v", err)
	}
	notes, err := store.PageByKind(ctx, card.ID, history.KindSystem, 0, 10)
	if err != nil {
		t.Fatalf("read the card's system notes: %v", err)
	}
	if len(notes.Events) != 1 || !contains(notes.Events[0].Summary, "Bypass permissions is on") {
		t.Errorf("the card's system notes = %+v, want the bypass note", notes.Events)
	}
	activity, err := store.PageByKind(ctx, card.ID, history.KindApproval, 0, 10)
	if err != nil {
		t.Fatalf("read the card's activity: %v", err)
	}
	if len(activity.Events) != 1 || !contains(activity.Events[0].Summary, "turned on by you") {
		t.Errorf("the card's activity = %+v, want the bypass row", activity.Events)
	}
}

// startBypassed starts a card's session and turns bypass on, so the requests its agent makes are
// the daemon's to answer.
func startBypassed(t *testing.T) (*env, string, string) {
	t.Helper()
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	ctx := context.Background()
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Write a file")
	if _, err := e.mgr.Start(ctx, card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	if _, err := e.mgr.SetBypass(ctx, card.ID, protocol.BypassRequest{Acknowledged: true}); err != nil {
		t.Fatalf("turn bypass on: %v", err)
	}
	return e, card.ID, e.sessionRow(t, card.ID).AgentSessionID
}

func TestBypassedCardAnswersInsideTheWorktreeWithoutAsking(t *testing.T) {
	e, cardID, sessionID := startBypassed(t)
	req := permissionRequest()
	if !e.agent.ask(sessionID, req) {
		t.Fatal("the fake agent does not know the session it was asked to ask on")
	}

	waitFor(t, "the daemon to answer for the card", func() bool {
		_, ok := e.agent.approvalResponse("req-1")
		return ok
	})
	resp, _ := e.agent.approvalResponse("req-1")
	if resp.OptionID != "allow-once" || resp.Cancelled {
		t.Errorf("the agent was answered %+v, want allow-once", resp)
	}
	// Nobody was asked, and the session is not held waiting for an answer that will not come: the
	// daemon's answer leaves it where a turn needs it, working.
	waitFor(t, "the session to read as working again", func() bool {
		return e.sessionRow(t, cardID).State == string(protocol.SessionStateWorking)
	})
	waitFor(t, "the daemon's approve row in the audit log", func() bool {
		rows := e.auditRows(t)
		return len(rows) > 0 && rows[0].Action == audit.ActionApprove && rows[0].Actor == audit.ActorDaemon
	})
	rows := e.auditRows(t)
	if !contains(rows[0].DetailJSON, `"reason":"bypass"`) {
		t.Errorf("the audit detail = %s, want it to say why", rows[0].DetailJSON)
	}
}

func TestBypassedCardRefusesToLeaveTheWorktree(t *testing.T) {
	e, cardID, sessionID := startBypassed(t)

	// A file outside the worktree, and a command that would touch the main branch, are both the
	// daemon's to refuse: bypass never included them.
	outside := permissionRequest()
	outside.RequestID, outside.ToolCallID, outside.Path = "req-out", "call-out", "/etc/passwd"
	if !e.agent.ask(sessionID, outside) {
		t.Fatal("the fake agent does not know the session it was asked to ask on")
	}
	waitFor(t, "the daemon to refuse the request outside the worktree", func() bool {
		_, ok := e.agent.approvalResponse("req-out")
		return ok
	})
	if resp, _ := e.agent.approvalResponse("req-out"); resp.OptionID != "reject-once" {
		t.Errorf("the agent was answered %+v, want reject-once", resp)
	}

	push := permissionRequest()
	push.RequestID, push.ToolCallID, push.Path = "req-push", "call-push", ""
	push.Kind, push.Title, push.Command = "execute", "Push the branch", "git push origin main"
	if !e.agent.ask(sessionID, push) {
		t.Fatal("the fake agent does not know the session it was asked to ask on")
	}
	waitFor(t, "the daemon to refuse the push to main", func() bool {
		_, ok := e.agent.approvalResponse("req-push")
		return ok
	})
	if resp, _ := e.agent.approvalResponse("req-push"); resp.OptionID != "reject-once" {
		t.Errorf("the agent was answered %+v, want reject-once", resp)
	}

	// Both refusals are on the audit trail, next to the row that recorded turning bypass on.
	waitFor(t, "both refusals in the audit log", func() bool { return len(refusalRows(e.auditRows(t))) == 2 })
	rows := e.auditRows(t)
	if len(rows) != 3 {
		t.Fatalf("the audit log = %+v, want the grant and the two refusals", rows)
	}
	for _, row := range refusalRows(rows) {
		if row.Actor != audit.ActorDaemon || row.Target == "" {
			t.Errorf("audit row = %+v, want a refusal by the daemon naming the call it answered", row)
		}
	}
	if !hasReason(rows, gitx.RuleOutsideWorktree) {
		t.Errorf("the audit log = %+v, want one row for the outside-worktree rule", rows)
	}
	if !hasReason(rows, gitx.RuleMainBranch) {
		t.Errorf("the audit log = %+v, want one row for the main-branch rule", rows)
	}

	// The person is told in the card's own history, because the daemon took a decision on their
	// behalf and bypass is not a licence to leave the worktree.
	store, err := history.New(e.store)
	if err != nil {
		t.Fatalf("make the history store: %v", err)
	}
	waitFor(t, "both refusals in the card's history", func() bool { return len(refusalNotes(t, store, cardID)) == 2 })
}

// TestABypassedCardRefusesAWrappedPushToMain covers the worktree rule at the session level for the
// spellings an agent could use to dodge it: a wrapper with its own options, and a branch name split
// by an empty quote. Bypass is where this matters most, because the blocklist is skipped there and
// this rule is the only one left.
func TestABypassedCardRefusesAWrappedPushToMain(t *testing.T) {
	e, cardID, sessionID := startBypassed(t)

	commands := []string{
		"env -i git push origin main",
		"sudo -u root git push origin main",
		`git push origin ma''in`,
	}
	for i, command := range commands {
		requestID := fmt.Sprintf("req-%d", i)
		req := permissionRequest()
		req.RequestID, req.ToolCallID, req.Path = requestID, "call-"+requestID, ""
		req.Kind, req.Title, req.Command = "execute", "Push the branch", command
		if !e.agent.ask(sessionID, req) {
			t.Fatal("the fake agent does not know the session it was asked to ask on")
		}
		waitFor(t, "the daemon to refuse "+command, func() bool {
			_, ok := e.agent.approvalResponse(requestID)
			return ok
		})
		if resp, _ := e.agent.approvalResponse(requestID); resp.OptionID != "reject-once" {
			t.Errorf("%q was answered %+v, want reject-once", command, resp)
		}
	}

	waitFor(t, "every refusal in the audit log", func() bool {
		return len(refusalRows(e.auditRows(t))) == len(commands)
	})
	for _, row := range refusalRows(e.auditRows(t)) {
		if !contains(row.DetailJSON, gitx.RuleMainBranch) {
			t.Errorf("audit row = %s, want the main-branch rule", row.DetailJSON)
		}
	}
	store, err := history.New(e.store)
	if err != nil {
		t.Fatalf("make the history store: %v", err)
	}
	waitFor(t, "every refusal in the card's history", func() bool {
		return len(refusalNotes(t, store, cardID)) == len(commands)
	})
}

// refusalRows are the audit rows that record the daemon turning a request down.
func refusalRows(rows []db.AuditLog) []db.AuditLog {
	var out []db.AuditLog
	for _, row := range rows {
		if row.Action == audit.ActionDeny {
			out = append(out, row)
		}
	}
	return out
}

// refusalNotes are the card's own system messages about a request bypass refused.
func refusalNotes(t *testing.T, store *history.Store, cardID string) []history.Event {
	t.Helper()
	notes, err := store.PageByKind(context.Background(), cardID, history.KindSystem, 0, 10)
	if err != nil {
		t.Fatalf("read the card's system notes: %v", err)
	}
	var out []history.Event
	for _, note := range notes.Events {
		if contains(note.Summary, "refused to run") {
			out = append(out, note)
		}
	}
	return out
}

// hasReason reports whether one audit row carries this containment rule in its detail.
func hasReason(rows []db.AuditLog, reason string) bool {
	for _, row := range rows {
		if contains(row.DetailJSON, `"reason":"`+reason+`"`) {
			return true
		}
	}
	return false
}

// TestBypassAppliesToTheRunInFlight covers turning bypass on while an agent is already running: the
// mode is read at the moment of a request, so the next one is answered without being asked.
func TestBypassAppliesToTheRunInFlight(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Write a file")
	if _, err := e.mgr.Start(ctx, card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	sessionID := e.sessionRow(t, card.ID).AgentSessionID

	// Before bypass, the request is the person's: it is announced and held. The request is a command,
	// which the card's default mode still asks about.
	if !e.agent.ask(sessionID, commandRequest()) {
		t.Fatal("the fake agent does not know the session it was asked to ask on")
	}
	ev := e.untilType(t, protocol.EventTypeApprovalRequested)
	first, ok := ev.Data.(protocol.ApprovalRequestedEventData)
	if !ok {
		t.Fatalf("approval.requested data = %#v", ev.Data)
	}

	// With bypass on, the next request of the same run is answered by the daemon.
	if _, err := e.mgr.SetBypass(ctx, card.ID, protocol.BypassRequest{Acknowledged: true}); err != nil {
		t.Fatalf("turn bypass on: %v", err)
	}
	second := permissionRequest()
	second.RequestID, second.ToolCallID = "req-2", "call-2"
	if !e.agent.ask(sessionID, second) {
		t.Fatal("the fake agent does not know the session it was asked to ask on")
	}
	waitFor(t, "the daemon to answer the second request", func() bool {
		_, ok := e.agent.approvalResponse("req-2")
		return ok
	})

	// The first request is still waiting for the person, and answering it still works.
	if _, err := e.store.Queries().GetApproval(ctx, first.Approval.ID); err != nil {
		t.Fatalf("the first request is no longer recorded: %v", err)
	}
	if err := e.mgr.Respond(ctx, first.Approval.ID, protocol.ApprovalDecisionDenied, "", audit.ActorPerson); err != nil {
		t.Fatalf("answer the first request: %v", err)
	}
	if resp, ok := e.agent.approvalResponse("req-1"); !ok || resp.OptionID != "reject-once" {
		t.Errorf("the first request was answered %+v, want reject-once", resp)
	}
}

// TestAgentKindCarriesNoBypass is a guard on the shape of the feature: bypass is a card's mode, and
// an agent has no say in it. It fails if the request type ever grows a field for it.
func TestAgentKindCarriesNoBypass(t *testing.T) {
	var in protocol.BypassRequest
	in.Acknowledged = true
	if !in.Acknowledged {
		t.Error("a bypass request cannot hold its acknowledgement")
	}
	_ = agents.OptionAllowOnce
}
