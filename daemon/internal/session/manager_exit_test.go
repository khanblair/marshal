package session_test

import (
	"context"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

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
	// The card says why, so a person is not left with "Needs you" and nothing to do.
	want := "The agent for " + updated.Key + " stopped unexpectedly. Press Resume agent to start it again where it left off."
	if updated.NeedsReason == nil || updated.NeedsReason.Kind != protocol.NeedsReasonKindStuck ||
		updated.NeedsReason.Text != want {
		t.Errorf("needs reason after a crash = %+v, want stuck with %q", updated.NeedsReason, want)
	}
}

// What the agent printed as it died is said on the card, so a person is told why and not only that.
func TestAnUnexpectedExitSaysWhatTheAgentPrintedLast(t *testing.T) {
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

	e.agent.crashWith(row.AgentSessionID, agents.Failed{
		Message: "The agent stopped unexpectedly.", Detail: "exit status 1\nNo conversation found with session ID: abc.\n",
	})
	e.untilType(t, protocol.EventTypeSessionStateChanged)

	updated, err := e.proj.Card(context.Background(), card.ID)
	if err != nil || updated.NeedsReason == nil {
		t.Fatalf("Card = %+v, %v, want a needs reason", updated, err)
	}
	want := "The agent for " + updated.Key + " stopped unexpectedly: No conversation found with session ID: abc. " +
		"Press Resume agent to start it again where it left off."
	if updated.NeedsReason.Text != want {
		t.Errorf("needs reason = %q, want %q", updated.NeedsReason.Text, want)
	}
}

// A signed-out agent cannot do anything, and its process stays alive, so the card is not left
// reading "working": it ends the idle process and says what to do.
func TestASignedOutAgentMovesTheCardToNeedsWithTheWayForward(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	e.untilType(t, protocol.EventTypeSessionStateChanged) // awake, from Start
	e.agent.setTurnScript(agents.Failed{Message: "Failed to authenticate", SignedOut: true})
	if err := e.mgr.Send(context.Background(), card.ID, "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	var data protocol.SessionStateChangedEventData
	for data.State != protocol.SessionStateStopped {
		ev := e.untilType(t, protocol.EventTypeSessionStateChanged)
		data, _ = ev.Data.(protocol.SessionStateChangedEventData)
	}
	if !strings.Contains(data.Reason, "signed out") {
		t.Errorf("reason = %q, want it to say Claude is signed out", data.Reason)
	}
	updated, err := e.proj.Card(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Card: %v", err)
	}
	if updated.State != protocol.CardStateNeeds || updated.NeedsReason == nil ||
		updated.NeedsReason.Kind != protocol.NeedsReasonKindStuck ||
		!strings.Contains(updated.NeedsReason.Text, "/login") || !strings.Contains(updated.NeedsReason.Text, updated.Key) {
		t.Errorf("card = %s %+v, want needs you, stuck, with the sign-in way forward", updated.State, updated.NeedsReason)
	}
}
