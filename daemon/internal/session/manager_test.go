package session_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
)

func TestStartCreatesWorktreeStartsTheSessionAndMovesTheCardToWorking(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() {
		if err := e.mgr.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")

	updated, err := e.mgr.Start(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if updated.State != protocol.CardStateWorking {
		t.Errorf("card state = %s, want %s", updated.State, protocol.CardStateWorking)
	}
	if updated.Branch == "" {
		t.Error("the card's branch is empty")
	}

	path, branch, err := e.proj.Worktree(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if path == "" || branch == "" {
		t.Fatalf("worktree = %q, %q, want both set", path, branch)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the worktree folder is missing: %v", err)
	}

	row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if row.State != string(protocol.SessionStateAwake) || row.AgentSessionID == "" {
		t.Errorf("session row = %+v, want state awake and an agent session id", row)
	}

	ev := e.untilType(t, protocol.EventTypeSessionStateChanged)
	data, ok := ev.Data.(protocol.SessionStateChangedEventData)
	if !ok || data.CardID != card.ID || data.State != protocol.SessionStateAwake {
		t.Errorf("session.state_changed data = %+v, ok=%v", ev.Data, ok)
	}
	if !ev.Critical {
		t.Error("session.state_changed must be critical")
	}
}

func TestStartRefusesWhenAlreadyLiveInThisProcess(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}

	_, err := e.mgr.Start(context.Background(), card.ID)
	_ = wantCode(t, err, protocol.ErrorCodeRefused)
}

func TestStartOnADanglingRowResumesInsteadOfRefusing(t *testing.T) {
	e := newEnv(t)
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Close leaves the row exactly as it was (awake): this is how a crash that never called Stop
	// looks in the database, and it is exactly what makes the card_id UNIQUE constraint matter.
	if err := e.mgr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	fresh := e.reopen(t)
	t.Cleanup(func() { _ = fresh.Close() })
	updated, err := fresh.Start(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Start on a dangling row: %v", err)
	}
	if updated.State != protocol.CardStateWorking {
		t.Errorf("card state = %s, want %s", updated.State, protocol.CardStateWorking)
	}
	row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if row.State != string(protocol.SessionStateAwake) {
		t.Errorf("session state = %s, want %s", row.State, protocol.SessionStateAwake)
	}
}

func TestStartOnAStoppedRowIsRefused(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.mgr.Stop(context.Background(), card.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	_, err := e.mgr.Start(context.Background(), card.ID)
	_ = wantCode(t, err, protocol.ErrorCodeRefused)
}

func TestSendWithNoLiveSessionIsRefused(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	err := e.mgr.Send(context.Background(), "no-such-card", "hello")
	perr := wantCode(t, err, protocol.ErrorCodeRefused)
	if !strings.Contains(perr.Error(), "no live session") {
		t.Errorf("error = %v, want it to wrap ErrNoLiveSession", err)
	}
}

func TestSendQueuesWhileBusyAndDeliversAfterTheTurnEnds(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	e.agent.hold = make(chan struct{})
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	e.untilType(t, protocol.EventTypeSessionStateChanged) // the "awake" from Start

	if err := e.mgr.Send(context.Background(), card.ID, "first"); err != nil {
		t.Fatalf("Send(first): %v", err)
	}
	working := e.untilType(t, protocol.EventTypeSessionStateChanged)
	if data, _ := working.Data.(protocol.SessionStateChangedEventData); data.State != protocol.SessionStateWorking {
		t.Fatalf("state after the first send = %+v, want working", working.Data)
	}

	// A turn is running (held by e.agent.hold), so this is queued rather than sent, and does not
	// return an error.
	if err := e.mgr.Send(context.Background(), card.ID, "second"); err != nil {
		t.Fatalf("Send(second) while busy: %v", err)
	}

	e.agent.hold <- struct{}{} // let the first turn finish
	first := e.untilType(t, protocol.EventTypeSessionOutput)
	firstData, _ := first.Data.(protocol.SessionOutputEventData)
	if !strings.Contains(firstData.Text, "turn 1") || !strings.Contains(firstData.Text, "first") {
		t.Errorf("first turn output = %+v", firstData)
	}
	// TurnEnded moves the session back to awake, then delivers the queued message at once,
	// starting a second turn (working again) without another Send.
	backToAwake := e.untilType(t, protocol.EventTypeSessionStateChanged)
	if data, _ := backToAwake.Data.(protocol.SessionStateChangedEventData); data.State != protocol.SessionStateAwake {
		t.Fatalf("state right after the first turn ended = %+v, want awake", backToAwake.Data)
	}
	secondWorking := e.untilType(t, protocol.EventTypeSessionStateChanged)
	if data, _ := secondWorking.Data.(protocol.SessionStateChangedEventData); data.State != protocol.SessionStateWorking {
		t.Fatalf("state after the queued message was delivered = %+v, want working", secondWorking.Data)
	}

	e.agent.hold <- struct{}{} // let the second turn finish
	second := e.untilType(t, protocol.EventTypeSessionOutput)
	secondData, _ := second.Data.(protocol.SessionOutputEventData)
	if !strings.Contains(secondData.Text, "turn 2") || !strings.Contains(secondData.Text, "second") {
		t.Errorf("second turn output = %+v", secondData)
	}
}

func TestSendQueueRefusesBeyondItsBound(t *testing.T) {
	e := newEnv(t)
	e.agent.hold = make(chan struct{})
	t.Cleanup(func() { close(e.agent.hold); _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "first"); err != nil {
		t.Fatalf("Send(first): %v", err)
	}
	for i := range 50 {
		if err := e.mgr.Send(context.Background(), card.ID, "queued"); err != nil {
			t.Fatalf("Send(queued #%d): %v", i, err)
		}
	}
	err := e.mgr.Send(context.Background(), card.ID, "one too many")
	_ = wantCode(t, err, protocol.ErrorCodeRefused)
}

func TestStopEndsTheProcessAndMarksTheSessionStopped(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	e.untilType(t, protocol.EventTypeSessionStateChanged)

	if err := e.mgr.Stop(context.Background(), card.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	stopped := e.untilType(t, protocol.EventTypeSessionStateChanged)
	if data, _ := stopped.Data.(protocol.SessionStateChangedEventData); data.State != protocol.SessionStateStopped {
		t.Errorf("state after Stop = %+v, want stopped", stopped.Data)
	}
	row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if row.State != string(protocol.SessionStateStopped) {
		t.Errorf("session row state = %s, want stopped", row.State)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "hi"); err == nil {
		t.Error("Send after Stop should be refused")
	}
}

func TestCloseDoesNotMarkAwakeSessionsStopped(t *testing.T) {
	e := newEnv(t)
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.mgr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if row.State != string(protocol.SessionStateAwake) {
		t.Errorf("session row state after Close = %s, want it left as awake", row.State)
	}
}

func TestRestoreAllResumesAwakeSessionsInAutoMode(t *testing.T) {
	e := newEnv(t)
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.mgr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	fresh := e.reopen(t)
	t.Cleanup(func() { _ = fresh.Close() })
	if err := fresh.RestoreAll(context.Background()); err != nil {
		t.Fatalf("RestoreAll: %v", err)
	}
	if err := fresh.Send(context.Background(), card.ID, "still here?"); err != nil {
		t.Fatalf("Send after RestoreAll: %v", err)
	}
	got := e.untilType(t, protocol.EventTypeSessionOutput)
	data, _ := got.Data.(protocol.SessionOutputEventData)
	if !strings.Contains(data.Text, "turn 1") {
		t.Errorf("output after resume = %+v, want a first turn (the fake agent keeps its own state per session id)", data)
	}
}

func TestRestoreAllInManualModeLeavesSessionsAsTheyAre(t *testing.T) {
	e := newEnv(t)
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.mgr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	manual := e.reopen(t, func(c *session.Config) { c.ResumeMode = session.ResumeModeManual })
	t.Cleanup(func() { _ = manual.Close() })
	if err := manual.RestoreAll(context.Background()); err != nil {
		t.Fatalf("RestoreAll in manual mode: %v", err)
	}
	row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if row.State != string(protocol.SessionStateAwake) {
		t.Errorf("session row state after a manual RestoreAll = %s, want it left as awake", row.State)
	}
	if err := manual.Send(context.Background(), card.ID, "hi"); err == nil {
		t.Error("Send should still be refused: manual mode did not resume the session")
	}
}

func TestResumeExplicitlyResumesOneCard(t *testing.T) {
	e := newEnv(t)
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.mgr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	manual := e.reopen(t, func(c *session.Config) { c.ResumeMode = session.ResumeModeManual })
	t.Cleanup(func() { _ = manual.Close() })
	if err := manual.RestoreAll(context.Background()); err != nil {
		t.Fatalf("RestoreAll: %v", err)
	}
	if err := manual.Resume(context.Background(), card.ID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if err := manual.Send(context.Background(), card.ID, "hi"); err != nil {
		t.Errorf("Send after an explicit Resume: %v", err)
	}
}

func TestACardThatCannotResumeMovesToNeeds(t *testing.T) {
	e := newEnv(t)
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.mgr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	e.agent.resumeErr = agents.ErrCannotResume

	fresh := e.reopen(t)
	t.Cleanup(func() { _ = fresh.Close() })
	if err := fresh.RestoreAll(context.Background()); err != nil {
		t.Fatalf("RestoreAll: %v", err)
	}
	updated, err := e.proj.Card(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Card: %v", err)
	}
	if updated.State != protocol.CardStateNeeds {
		t.Errorf("card state after a failed resume = %s, want %s", updated.State, protocol.CardStateNeeds)
	}
	row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if row.State != string(protocol.SessionStateStopped) {
		t.Errorf("session row state after a failed resume = %s, want stopped", row.State)
	}
}

func TestSendReturnsAPlainErrorWhenTheAgentRefuses(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	e.agent.setSendErr(errors.New("the agent process is gone"))

	err := e.mgr.Send(context.Background(), card.ID, "hello")
	if err == nil || !strings.Contains(err.Error(), "the agent process is gone") {
		t.Errorf("Send error = %v, want it to wrap the agent's own error", err)
	}
	// The turn was never really claimed, so a later, ordinary send still works once the agent
	// is healthy again.
	e.agent.setSendErr(nil)
	if err := e.mgr.Send(context.Background(), card.ID, "hello again"); err != nil {
		t.Errorf("Send after the agent recovered: %v", err)
	}
}

func TestSendReconcilesWithTheAgentsOwnBusyBookkeeping(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	fs, err := e.agent.find(row.AgentSessionID)
	if err != nil {
		t.Fatalf("find the fake session: %v", err)
	}
	// The manager's own bookkeeping believes the session is free, but the agent itself says a
	// turn is already running: Send must queue the message instead of losing it, and must then
	// believe a turn is running too, exactly as if it had claimed the turn itself.
	fs.mu.Lock()
	fs.busy = true
	fs.mu.Unlock()

	if err := e.mgr.Send(context.Background(), card.ID, "queued by a race"); err != nil {
		t.Fatalf("Send while the agent disagreed about busy-ness: %v", err)
	}
	// claimTurn now also says busy, so this queues too, without calling agent.Send again.
	if err := e.mgr.Send(context.Background(), card.ID, "queued behind it"); err != nil {
		t.Errorf("a second Send after the reconciliation: %v", err)
	}
}

func TestStartUndoesTheWorktreeWhenTheAgentFailsToStart(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	e.agent.startErr = errors.New("the agent program is missing")

	_, err := e.mgr.Start(context.Background(), card.ID)
	if err == nil {
		t.Fatal("Start with a failing agent should return an error")
	}
	path, branch, err := e.proj.Worktree(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if path != "" || branch != "" {
		t.Errorf("worktree = %q, %q, want both cleared after the agent failed to start", path, branch)
	}
	if _, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID); err == nil {
		t.Error("no session row should exist after a failed start")
	}
}

func TestAPTYStyleSessionIsNeverTrackedAsBusy(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	e.agent.caps = agents.Capabilities{Resume: true, StructuredEvents: false}
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Neither send ever queues, because this bookkeeping never marks a non-structured session
	// busy in the first place (see the report's Ruling on this).
	if err := e.mgr.Send(context.Background(), card.ID, "one"); err != nil {
		t.Fatalf("Send(one): %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "two"); err != nil {
		t.Fatalf("Send(two): %v", err)
	}
}

func TestAnUnexpectedExitMovesTheCardToNeeds(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	e.untilType(t, protocol.EventTypeSessionStateChanged) // awake, from Start

	e.agent.crash(row.AgentSessionID, "the process died")

	needs := e.untilType(t, protocol.EventTypeSessionStateChanged)
	data, _ := needs.Data.(protocol.SessionStateChangedEventData)
	if data.State != protocol.SessionStateStopped || data.Reason == "" {
		t.Errorf("state after a crash = %+v, want stopped with a reason", data)
	}
	updated, err := e.proj.Card(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Card: %v", err)
	}
	if updated.State != protocol.CardStateNeeds {
		t.Errorf("card state after a crash = %s, want %s", updated.State, protocol.CardStateNeeds)
	}
}

func TestResumeOfACardWithNoSessionIsNotFound(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	err := e.mgr.Resume(context.Background(), "no-such-card")
	_ = wantCode(t, err, protocol.ErrorCodeNotFound)
}

func TestResumeOfAStoppedSessionIsRefused(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.mgr.Stop(context.Background(), card.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	err := e.mgr.Resume(context.Background(), card.ID)
	_ = wantCode(t, err, protocol.ErrorCodeRefused)
}

func TestStopReturnsAnErrorWhenTheAgentFailsToStop(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	e.agent.stopErr = errors.New("the process will not die")

	err := e.mgr.Stop(context.Background(), card.ID)
	if err == nil || !strings.Contains(err.Error(), "the process will not die") {
		t.Errorf("Stop error = %v, want it to wrap the agent's own error", err)
	}
	row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if row.State == string(protocol.SessionStateStopped) {
		t.Error("the session row should not be marked stopped when the agent itself did not stop")
	}
	// Let the deferred Close actually end the session, so its pump goroutine does not leak.
	e.agent.stopErr = nil
}

func TestPlanUpdatesArePublishedAsSessionOutput(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	fs, err := e.agent.find(row.AgentSessionID)
	if err != nil {
		t.Fatalf("find the fake session: %v", err)
	}
	fs.sink.Emit(agents.PlanUpdate{Steps: []agents.PlanStep{
		{Text: "Read the router", Status: agents.PlanCompleted},
		{Text: "Add the route", Status: agents.PlanInProgress},
	}})

	ev := e.untilType(t, protocol.EventTypeSessionOutput)
	data, ok := ev.Data.(protocol.SessionOutputEventData)
	if !ok || data.Kind != "plan" || len(data.Plan) != 2 || data.Plan[0].Text != "Read the router" {
		t.Errorf("plan output = %+v, ok=%v", data, ok)
	}
}

func TestStartPassesACardsThinkingSetting(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card, err := e.proj.CreateCard(context.Background(), project.ID,
		protocol.CreateCardRequest{Title: "Add a health check", Thinking: protocol.ThinkingModeHigh})
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if row.Thinking != string(protocol.ThinkingModeHigh) {
		t.Errorf("session thinking = %q, want %q", row.Thinking, protocol.ThinkingModeHigh)
	}
}

func TestConcurrentStartsOfTheSameCardRefuseTheLoser(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	// Hold every agent start, so the winner of the reserve race is reliably still holding its
	// reservation by the time the loser calls Start and finds the card already pending.
	e.agent.startHold = make(chan struct{})

	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := e.mgr.Start(context.Background(), card.ID)
			results <- err
		}()
	}
	time.Sleep(50 * time.Millisecond) // let both goroutines reach the held agent.Start
	close(e.agent.startHold)
	first, second := <-results, <-results
	refused := 0
	for _, err := range []error{first, second} {
		if err != nil {
			_ = wantCode(t, err, protocol.ErrorCodeRefused)
			refused++
		}
	}
	if refused != 1 {
		t.Errorf("refused starts = %d, want exactly 1 of the 2 to be refused (got %v, %v)", refused, first, second)
	}
}

func TestNewManagerValidatesItsDependencies(t *testing.T) {
	if _, err := session.NewManager(nil, nil, nil, nil, nil, session.Config{DataDir: "/tmp"}); err == nil {
		t.Error("NewManager with every dependency missing should fail")
	}
}

func TestNewManagerValidatesItsConfig(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	_, err := session.NewManager(e.store, e.bus, e.proj, e.registry, e.git, session.Config{DataDir: "relative/path"})
	if err == nil {
		t.Error("NewManager with a relative data folder should fail")
	}
}

func TestAFailedQueuedDeliveryReleasesTheTurn(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	e.agent.hold = make(chan struct{})
	t.Cleanup(func() { close(e.agent.hold) }) // registered after Close, so it runs first
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "first"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "second"); err != nil {
		t.Fatalf("Send(second): %v", err)
	}
	e.agent.setSendErr(errors.New("cannot deliver right now"))
	e.agent.hold <- struct{}{} // let the first turn finish; onTurnEnded tries and fails to deliver "second"

	awake := e.untilType(t, protocol.EventTypeSessionStateChanged)
	if data, _ := awake.Data.(protocol.SessionStateChangedEventData); data.State != protocol.SessionStateAwake {
		t.Fatalf("state after the first turn ended = %+v, want awake even though delivery failed", awake.Data)
	}
	// onTurnEnded publishes "awake" before it tries to deliver the queued message, so give that
	// attempt a moment to actually run (and fail) before resetting sendErr: otherwise the reset
	// can win a race against the pump's own delivery attempt and this would test nothing.
	time.Sleep(50 * time.Millisecond)
	e.agent.setSendErr(nil)
	// The failed delivery must still release the turn, so an ordinary send is accepted directly
	// (not queued behind a message that will never be delivered): a "working" event follows at
	// once, which only happens for a direct send, never for one that was merely queued.
	if err := e.mgr.Send(context.Background(), card.ID, "third"); err != nil {
		t.Errorf("Send after a failed queued delivery: %v", err)
	}
	working := e.untilType(t, protocol.EventTypeSessionStateChanged)
	if data, _ := working.Data.(protocol.SessionStateChangedEventData); data.State != protocol.SessionStateWorking {
		t.Errorf("state after the third send = %+v, want working (the turn must have been released)", working.Data)
	}
}

// TestSendMarksTheSessionWorkingBeforeAskingTheAgent proves Send writes "working" before it calls
// agent.Send, not after: a fast turn (a real agent, or the stub at full speed) can otherwise reach
// the pump's onTurnEnded, which writes "awake", before Send's own goroutine gets back to writing
// "working", leaving the row stuck on "working" for a session that is actually idle. The fake
// agent's beforeSend hook runs synchronously inside Send itself, before it decides anything, so
// this checks the ordering directly instead of trying to win a real race.
func TestSendMarksTheSessionWorkingBeforeAskingTheAgent(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}

	var stateAtSend string
	e.agent.beforeSend = func() {
		row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
		if err != nil {
			t.Errorf("GetSessionByCard inside beforeSend: %v", err)
			return
		}
		stateAtSend = row.State
	}
	if err := e.mgr.Send(context.Background(), card.ID, "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if stateAtSend != string(protocol.SessionStateWorking) {
		t.Errorf("session state at the moment agent.Send was called = %q, want %q", stateAtSend, protocol.SessionStateWorking)
	}
}

// TestStopDuringATurnDoesNotResurrectTheSession proves that a TurnEnded which arrives because of
// Stop's own graceful cancel (a real adapter, per docs/architecture.md 4.1's Interrupt rule, can
// end an in-flight turn with TurnEnded(cancelled) before the process actually exits) cannot undo
// what Stop already wrote: the row must stay "stopped", never bounce back to "awake", and no
// queued message may be delivered to a session that is on its way out.
func TestStopDuringATurnDoesNotResurrectTheSession(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	e.agent.hold = make(chan struct{})
	t.Cleanup(func() { close(e.agent.hold) })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "first"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "queued behind the held turn"); err != nil {
		t.Fatalf("Send(queued): %v", err)
	}

	// The turn is held open (blocked inside the fake's runTurn) when Stop is called: the fake
	// agent's Stop (see fakeAgent.Stop) sees the session busy and queues TurnEnded(cancelled)
	// before Exited, exactly mirroring a real adapter's graceful stop.
	if err := e.mgr.Stop(context.Background(), card.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if row.State != string(protocol.SessionStateStopped) {
		t.Fatalf("session row state right after Stop = %s, want stopped", row.State)
	}

	// Exactly three events were published before the held turn was ever released: awake (Start),
	// working (the first Send), and stopped (Stop itself, synchronously, before it returned).
	// Draining exactly these lets the check after prove nothing else follows.
	wantStates := []protocol.SessionState{protocol.SessionStateAwake, protocol.SessionStateWorking, protocol.SessionStateStopped}
	for _, want := range wantStates {
		ev := e.untilType(t, protocol.EventTypeSessionStateChanged)
		if data, _ := ev.Data.(protocol.SessionStateChangedEventData); data.State != want {
			t.Fatalf("state = %+v, want %s", ev.Data, want)
		}
	}

	// The pump is draining TurnEnded(cancelled) and Exited concurrently with this goroutine now;
	// poll for a while to prove the row never gets rewritten back to awake, and that no further
	// event (in particular, no "working" for the queued message being wrongly delivered) follows.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
		if err != nil {
			t.Fatalf("GetSessionByCard: %v", err)
		}
		if row.State != string(protocol.SessionStateStopped) {
			t.Fatalf("session row state = %s while the pump drained a stopped session, want it to stay stopped", row.State)
		}
		select {
		case ev := <-e.sub.C():
			t.Fatalf("an event arrived after stopped, want none: %+v", ev)
		default:
		}
	}
}
