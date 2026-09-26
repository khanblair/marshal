package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// wantHoldRefusal checks the sentence and the stable reason of a refused pause or sleep.
func wantHoldRefusal(t *testing.T, got protocol.Error, message string, reason protocol.HoldRefusalReason) {
	t.Helper()
	if got.Message != message {
		t.Errorf("message = %q\nwant      %q", got.Message, message)
	}
	if got.Details["reason"] != string(reason) {
		t.Errorf("details.reason = %q, want %q", got.Details["reason"], reason)
	}
}

// One whole hold over HTTP and the event stream: a card is started, paused, sent a message that
// waits, resumed, slept, woken, pinned, and unpinned.
func TestSessionHoldThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Hold me")
	base := "/v1/cards/" + card.ID
	stream := st.dial(protocol.CardTopic(card.ID))

	st.do(http.MethodPost, base+"/start", nil).want(t, http.StatusOK)
	stream.until(stateOf(card.ID, protocol.SessionStateAwake))

	paused := decode[protocol.Card](t, st.do(http.MethodPost, base+"/pause", nil).want(t, http.StatusOK))
	if !paused.Paused || paused.State != protocol.CardStateWorking {
		t.Fatalf("the paused card = %+v, want a working card with paused set", paused)
	}

	// A message sent during the pause is taken and waits. A sleep would lose it, so it is refused.
	st.do(http.MethodPost, base+"/messages", protocol.SendMessageRequest{Text: "wait for me"}).want(t, http.StatusNoContent)
	got := st.do(http.MethodPost, base+"/sleep", nil).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	wantHoldRefusal(t, got, "This card has a message waiting for you to resume it. Resume the card first.",
		protocol.HoldRefusalReasonSleepHoldingMessages)

	// Resuming the card delivers the message it was holding, and the agent answers it. The stub
	// agent plays its scripted default scenario for any message (it does not echo the text sent;
	// see tools/stub-agent/scenarios/default.json), so this checks that the held message's turn
	// really ran, not what it says.
	st.do(http.MethodPost, base+"/unpause", nil).want(t, http.StatusOK)
	seen := stream.until(stateOf(card.ID, protocol.SessionStateWorking))
	seen = append(seen, stream.until(stateOf(card.ID, protocol.SessionStateAwake))...)
	if texts := strings.Join(messageTexts(seen), ""); !strings.Contains(texts, "Turn 1. I remember 0 earlier turns.") {
		t.Errorf("the answer to the held message was %q, want the stub agent's turn", texts)
	}

	// Pausing again and sleeping stops the process, and waking brings the session back.
	st.do(http.MethodPost, base+"/pause", nil).want(t, http.StatusOK)
	st.do(http.MethodPost, base+"/sleep", nil).want(t, http.StatusNoContent)
	stream.until(stateOf(card.ID, protocol.SessionStateAsleep))
	st.do(http.MethodPost, base+"/wake", nil).want(t, http.StatusNoContent)
	stream.until(stateOf(card.ID, protocol.SessionStateWaking))
	stream.until(stateOf(card.ID, protocol.SessionStateAwake))

	pinned := decode[protocol.Card](t, st.do(http.MethodPost, base+"/pin", nil).want(t, http.StatusOK))
	if !pinned.Pinned {
		t.Errorf("the pinned card = %+v", pinned)
	}
	unpinned := decode[protocol.Card](t, st.do(http.MethodPost, base+"/unpin", nil).want(t, http.StatusOK))
	if unpinned.Pinned {
		t.Errorf("the unpinned card = %+v", unpinned)
	}
	// The hold routes take no body, and a body that is sent is not read.
	st.do(http.MethodPost, base+"/pin", `{"anything":true}`).want(t, http.StatusOK)
}

// Every refusal of architecture.md 5.1 through HTTP, each with its own sentence and reason, and
// each one leaving the card exactly as it was.
func TestSessionHoldRefusalsThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Not started")
	base := "/v1/cards/" + card.ID
	ctx := context.Background()

	got := st.do(http.MethodPost, base+"/pause", nil).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	wantHoldRefusal(t, got, "Only working cards can be paused.", protocol.HoldRefusalReasonPauseNotWorking)

	got = st.do(http.MethodPost, base+"/sleep", nil).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	wantHoldRefusal(t, got, "This card has no awake session.", protocol.HoldRefusalReasonSleepNoSession)

	// A card that was never started has no session for a wake to resume.
	got = st.do(http.MethodPost, base+"/wake", nil).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got.Message != "This card has not been started, so there is nothing to resume. Start it first." {
		t.Errorf("wake on a card that was never started said %q", got.Message)
	}

	// A working card that nobody paused does not sleep.
	busy := st.addCard(project.ID, "Busy")
	st.do(http.MethodPost, "/v1/cards/"+busy.ID+"/start", nil).want(t, http.StatusOK)
	got = st.do(http.MethodPost, "/v1/cards/"+busy.ID+"/sleep", nil).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	wantHoldRefusal(t, got, "Working cards don't sleep. Pause the card first.", protocol.HoldRefusalReasonSleepWorking)

	// A card that waits on a person stays awake, even while its session is running.
	waiting := st.addCard(project.ID, "Waiting on you")
	st.do(http.MethodPost, "/v1/cards/"+waiting.ID+"/start", nil).want(t, http.StatusOK)
	if _, err := st.proj.SetState(ctx, waiting.ID, protocol.CardStateNeeds); err != nil {
		t.Fatalf("move the card to needs: %v", err)
	}
	got = st.do(http.MethodPost, "/v1/cards/"+waiting.ID+"/sleep", nil).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	wantHoldRefusal(t, got, "This card is waiting on you, so it stays awake.", protocol.HoldRefusalReasonSleepNeedsYou)

	// A session that has stopped cannot be woken; Start is the route that continues it.
	st.do(http.MethodPost, "/v1/cards/"+busy.ID+"/stop", nil).want(t, http.StatusNoContent)
	got = st.do(http.MethodPost, "/v1/cards/"+busy.ID+"/wake", nil).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got.Message != "This card's session has stopped and cannot be resumed." {
		t.Errorf("wake on a stopped session said %q", got.Message)
	}

	// A card that does not exist is not found, the same as on every other card route.
	for _, path := range []string{"/pause", "/unpause", "/sleep", "/wake", "/pin", "/unpin"} {
		st.do(http.MethodPost, "/v1/cards/"+sampleCardID+path, nil).
			apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	}

	// Nothing above changed the card it was about.
	after := decode[protocol.Card](t, st.do(http.MethodGet, base, nil).want(t, http.StatusOK))
	if after.State != protocol.CardStateBacklog || after.Paused || after.Pinned {
		t.Errorf("the refusals changed the card: %+v", after)
	}
}
