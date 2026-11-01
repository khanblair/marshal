package session_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// startedCard starts a card and waits until its agent is awake.
func awakeCard(t *testing.T, e *env) protocol.Card {
	t.Helper()
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	e.untilType(t, protocol.EventTypeSessionStateChanged)
	return card
}

// untilDoingNow reads card updates until one of this card has a "doing now" line the test accepts.
func untilDoingNow(t *testing.T, e *env, cardID string, ok func(string) bool) string {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev, open := <-e.sub.C():
			if !open {
				t.Fatal("the subscription closed")
			}
			if ev.Type != string(protocol.EventTypeCardUpdated) {
				continue
			}
			if data, isCard := ev.Data.(protocol.CardEventData); isCard && data.Card.ID == cardID && ok(data.Card.DoingNow) {
				return data.Card.DoingNow
			}
		case <-deadline:
			t.Fatal("no card update with the expected doing now line arrived")
			return ""
		}
	}
}

func TestAnAgentWaitingForTheNetworkShowsItOnTheCardAndTheNextOutputTakesItDown(t *testing.T) {
	e := newEnv(t)
	card := awakeCard(t, e)
	e.agent.setTurnScript(
		agents.Retrying{Attempt: 3, Max: 10, Network: true},
		agents.MessageChunk{Text: "Back online."},
	)
	if err := e.mgr.Send(context.Background(), card.ID, "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	line := untilDoingNow(t, e, card.ID, func(s string) bool { return s != "" })
	if line != "Waiting for network, retry 3 of 10" {
		t.Errorf("doing now = %q, want the network wait with its count", line)
	}
	if !strings.HasPrefix(line, protocol.RetryDoingNowPrefix) {
		t.Errorf("doing now = %q, want it to start with the prefix a screen shows as a badge", line)
	}
	untilDoingNow(t, e, card.ID, func(s string) bool { return s == "" })
	got, err := e.proj.Card(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Card: %v", err)
	}
	if got.DoingNow != "" || got.State == protocol.CardStateNeeds {
		t.Errorf("card = %s with %q, want the wait gone and the card not in Needs you", got.State, got.DoingNow)
	}
}

func TestAWaitThatIsNotTheNetworkSaysTheModelIsBusyWithoutTheBadgePrefix(t *testing.T) {
	e := newEnv(t)
	card := awakeCard(t, e)
	e.agent.setTurnScript(agents.Retrying{Attempt: 1, Max: 10}, agents.MessageChunk{Text: "ok"})
	if err := e.mgr.Send(context.Background(), card.ID, "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	line := untilDoingNow(t, e, card.ID, func(s string) bool { return s != "" })
	if line != "The model is busy, retry 1 of 10" || strings.HasPrefix(line, protocol.RetryDoingNowPrefix) {
		t.Errorf("doing now = %q, want the busy line without the network prefix", line)
	}
}

func TestATurnTheNetworkKilledStopsTheAgentAndLeavesTheCardInNeedsWithAResumeWay(t *testing.T) {
	e := newEnv(t)
	card := awakeCard(t, e)
	e.agent.setTurnScript(
		agents.Retrying{Attempt: 10, Max: 10, Network: true},
		agents.Failed{Message: "API Error: Unable to connect to API", Detail: "ConnectionRefused"},
	)
	if err := e.mgr.Send(context.Background(), card.ID, "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	var data protocol.SessionStateChangedEventData
	for data.State != protocol.SessionStateStopped {
		ev := e.untilType(t, protocol.EventTypeSessionStateChanged)
		data, _ = ev.Data.(protocol.SessionStateChangedEventData)
	}
	if !strings.Contains(data.Reason, "network") || !strings.Contains(data.Reason, "Resume agent") {
		t.Errorf("reason = %q, want it to say the network dropped and to press Resume agent", data.Reason)
	}
	got, err := e.proj.Card(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Card: %v", err)
	}
	if got.State != protocol.CardStateNeeds || got.NeedsReason == nil ||
		got.NeedsReason.Kind != protocol.NeedsReasonKindStuck ||
		!strings.Contains(got.NeedsReason.Text, "network") || !strings.Contains(got.NeedsReason.Text, "Resume agent") ||
		!strings.Contains(got.NeedsReason.Text, got.Key) {
		t.Errorf("card = %s %+v, want needs you, stuck, with the network reason and the way forward", got.State, got.NeedsReason)
	}
	if got.DoingNow != "" {
		t.Errorf("doing now = %q, want the wait taken off before the card stopped", got.DoingNow)
	}
}

// A failure that says the network failed, with no retry line before it (an adapter that does not report
// retries, or the line was missed), is still the network's doing.
func TestAFailureThatSaysTheNetworkFailedIsTreatedAsTheNetwork(t *testing.T) {
	e := newEnv(t)
	card := awakeCard(t, e)
	e.agent.setTurnScript(agents.Failed{Message: "Connection error.", Detail: "dial tcp: lookup api.anthropic.com: no such host"})
	if err := e.mgr.Send(context.Background(), card.ID, "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	var data protocol.SessionStateChangedEventData
	for data.State != protocol.SessionStateStopped {
		ev := e.untilType(t, protocol.EventTypeSessionStateChanged)
		data, _ = ev.Data.(protocol.SessionStateChangedEventData)
	}
	got, _ := e.proj.Card(context.Background(), card.ID)
	if got.NeedsReason == nil || !strings.Contains(got.NeedsReason.Text, "network") {
		t.Errorf("needs reason = %+v, want the network reason", got.NeedsReason)
	}
}

// A turn that fails for another reason is not the network's: the agent stays up, as before.
func TestAFailureThatIsNotTheNetworkLeavesTheAgentRunning(t *testing.T) {
	e := newEnv(t)
	card := awakeCard(t, e)
	e.agent.setTurnScript(agents.Failed{Message: "The agent could not finish its turn.", Detail: "error_during_execution"})
	if err := e.mgr.Send(context.Background(), card.ID, "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	var data protocol.SessionStateChangedEventData
	for data.State != protocol.SessionStateAwake {
		ev := e.untilType(t, protocol.EventTypeSessionStateChanged)
		data, _ = ev.Data.(protocol.SessionStateChangedEventData)
	}
	got, _ := e.proj.Card(context.Background(), card.ID)
	if got.NeedsReason != nil && strings.Contains(got.NeedsReason.Text, "network") {
		t.Errorf("needs reason = %+v, want no network reason for another failure", got.NeedsReason)
	}
}

// The agent sets its own "doing now" line through the board tool. A wait puts its own line over that
// one, and takes it down by putting the agent's line back, not by blanking it.
func TestAWaitPutsTheAgentsOwnDoingNowLineBackWhenItIsOver(t *testing.T) {
	e := newEnv(t)
	card := awakeCard(t, e)
	own := "Reading the routes"
	if _, err := e.proj.UpdateCard(context.Background(), card.ID, protocol.UpdateCardRequest{DoingNow: &own}); err != nil {
		t.Fatalf("UpdateCard: %v", err)
	}
	e.agent.setTurnScript(agents.Retrying{Attempt: 2, Max: 10, Network: true}, agents.MessageChunk{Text: "ok"})
	if err := e.mgr.Send(context.Background(), card.ID, "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	untilDoingNow(t, e, card.ID, func(s string) bool { return strings.HasPrefix(s, protocol.RetryDoingNowPrefix) })
	untilDoingNow(t, e, card.ID, func(s string) bool { return s == own })
}

// A wait line that an earlier session left behind, because the daemon stopped mid-wait, is taken off
// when the next session for the card starts.
func TestAStaleWaitLineIsClearedWhenTheNextSessionStarts(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	stale := "Waiting for network, retry 4 of 10"
	if _, err := e.proj.UpdateCard(context.Background(), card.ID, protocol.UpdateCardRequest{DoingNow: &stale}); err != nil {
		t.Fatalf("UpdateCard: %v", err)
	}
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	untilDoingNow(t, e, card.ID, func(s string) bool { return s == "" })
}

// A later failure that is not the network is not the network's doing, however many network retries came
// before it: the last wait said the model was busy.
func TestALaterFailureIsNotTheNetworksAfterAnEarlierNetworkWait(t *testing.T) {
	e := newEnv(t)
	card := awakeCard(t, e)
	e.agent.setTurnScript(
		agents.Retrying{Attempt: 2, Max: 10, Network: true},
		agents.Retrying{Attempt: 3, Max: 10, Network: false},
		agents.Failed{Message: "The agent could not finish its turn.", Detail: "error_during_execution"},
	)
	if err := e.mgr.Send(context.Background(), card.ID, "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	var data protocol.SessionStateChangedEventData
	for data.State != protocol.SessionStateAwake {
		ev := e.untilType(t, protocol.EventTypeSessionStateChanged)
		data, _ = ev.Data.(protocol.SessionStateChangedEventData)
	}
	got, _ := e.proj.Card(context.Background(), card.ID)
	if got.State == protocol.CardStateNeeds {
		t.Errorf("card = %s %+v, want the agent left running: the failure was not the network", got.State, got.NeedsReason)
	}
}

// A signed-out agent is told so, even when it was waiting for the network just before.
func TestASignedOutAgentIsToldSoEvenAfterANetworkWait(t *testing.T) {
	e := newEnv(t)
	card := awakeCard(t, e)
	e.agent.setTurnScript(
		agents.Retrying{Attempt: 2, Max: 10, Network: true},
		agents.Failed{Message: "Failed to authenticate", SignedOut: true},
	)
	if err := e.mgr.Send(context.Background(), card.ID, "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	var data protocol.SessionStateChangedEventData
	for data.State != protocol.SessionStateStopped {
		ev := e.untilType(t, protocol.EventTypeSessionStateChanged)
		data, _ = ev.Data.(protocol.SessionStateChangedEventData)
	}
	got, _ := e.proj.Card(context.Background(), card.ID)
	if got.NeedsReason == nil || !strings.Contains(got.NeedsReason.Text, "/login") || strings.Contains(got.NeedsReason.Text, "network") {
		t.Errorf("needs reason = %+v, want the sign-in way forward and no network reason", got.NeedsReason)
	}
}
