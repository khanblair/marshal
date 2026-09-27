package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// POST /v1/cards/{id}/handoff (docs/marshal-product-scope.md section 10.5, build-plan 7.5): a card's
// work continues on a different agent, from a summary of where it got to, and the answer is the card
// now on its new agent. A handoff that is refused changes nothing, and says so with a stable reason.

// startedCard adds and starts a card through the API, and waits for its session to be awake.
func startedCard(t *testing.T, st *stack, title string) protocol.Card {
	t.Helper()
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, title)
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).want(t, http.StatusOK)
	return card
}

// handoff asks for a card to continue on an agent kind, from a summary.
func (st *stack) handoff(cardID string, to protocol.AgentKind, summary string) reply {
	st.t.Helper()
	return st.do(http.MethodPost, "/v1/cards/"+cardID+"/handoff",
		protocol.HandoffRequest{To: to, Summary: summary})
}

// The whole way round: a started card is handed off to a second agent and the answer is the card on
// its new agent, still working on the one session it has.
func TestACardIsHandedOffToAnotherAgentThroughTheAPI(t *testing.T) {
	st := newStack(t, withSecondAgent())
	card := startedCard(t, st, "Hand me off")

	answer := st.handoff(card.ID, protocol.AgentKindGemini, "Goal: the thing. Done: the first half. Left: the rest.")
	got := decode[protocol.Card](t, answer.want(t, http.StatusOK))
	if got.ID != card.ID || got.Agent != protocol.AgentKindGemini {
		t.Errorf("the answer = card %s on %s, want card %s on gemini", got.ID, got.Agent, card.ID)
	}
	if got.State != protocol.CardStateWorking || got.Session == nil || *got.Session != protocol.SessionStateAwake {
		t.Errorf("the card = state %s, session %v, want working and awake", got.State, got.Session)
	}
	if reread := st.getCard(card.ID); reread.Agent != protocol.AgentKindGemini {
		t.Errorf("the card now reads as agent %s, want gemini", reread.Agent)
	}
}

// Every refusal of the handoff route: the status, the stable reason, and the sentence a person reads.
func TestTheHandoffRouteRefusals(t *testing.T) {
	t.Run("a card that does not exist, and an id that cannot be one", func(t *testing.T) {
		st := newStack(t, withSecondAgent())
		st.handoff("01M3C107JB041061050R3GG28A", protocol.AgentKindGemini, "").
			apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
		st.do(http.MethodPost, "/v1/cards/not-an-id/handoff", protocol.HandoffRequest{To: protocol.AgentKindGemini}).
			apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	})

	t.Run("a body that is not one", func(t *testing.T) {
		st := newStack(t, withSecondAgent())
		project, _ := st.addProject("small-repo")
		idle := st.addCard(project.ID, "Anything")
		for body, message := range map[string]string{
			`{"to":"hologram"}`:     "Marshal does not know that agent.",
			`{}`:                    "Marshal does not know that agent.",
			`{"to":"gemini","x":1}`: `The field "x" is not one Marshal knows. Remove it and try again.`,
			``:                      "The request body is empty. Send a JSON object.",
		} {
			var sent any = body
			if body == "" {
				sent = nil
			}
			got := st.do(http.MethodPost, "/v1/cards/"+idle.ID+"/handoff", sent).
				apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
			if got.Message != message {
				t.Errorf("body %q: message = %q, want %q", body, got.Message, message)
			}
		}
	})

	t.Run("a card that never started", func(t *testing.T) {
		st := newStack(t, withSecondAgent())
		project, _ := st.addProject("small-repo")
		idle := st.addCard(project.ID, "Never started")
		got := st.handoff(idle.ID, protocol.AgentKindGemini, "").
			apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
		if got.Details["reason"] != "handoff_no_session" || got.Message != "This card has no session to hand off. Start the card first." {
			t.Errorf("the refusal = %+v", got)
		}
	})

	t.Run("the agent the card already runs", func(t *testing.T) {
		st := newStack(t, withSecondAgent())
		card := startedCard(t, st, "Same agent")
		got := st.handoff(card.ID, protocol.AgentKindClaude, "carry on").
			apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
		if got.Details["reason"] != "handoff_same_agent" ||
			got.Message != "This card already runs this agent. Choose a different one to hand off to." {
			t.Errorf("the refusal = %+v", got)
		}
	})

	t.Run("an agent Marshal has no adapter for", func(t *testing.T) {
		// codex is a real kind, but the stack registers no adapter behind it.
		st := newStack(t)
		card := startedCard(t, st, "No adapter")
		got := st.handoff(card.ID, protocol.AgentKindCodex, "").
			apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
		if got.Details["reason"] != "handoff_unknown_agent" ||
			got.Message != "Marshal does not have that agent ready to run yet." {
			t.Errorf("the refusal = %+v", got)
		}
	})

	t.Run("a summary that is too long", func(t *testing.T) {
		st := newStack(t, withSecondAgent())
		card := startedCard(t, st, "Too long")
		got := st.handoff(card.ID, protocol.AgentKindGemini, strings.Repeat("x", protocol.MaxHandoffSummaryChars+1)).
			apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
		if got.Message != "That summary is too long. Keep it to 20,000 characters." {
			t.Errorf("the refusal = %+v", got)
		}
	})

	t.Run("no token", func(t *testing.T) {
		st := newStack(t, withSecondAgent())
		card := startedCard(t, st, "No token")
		st.doWith("", http.MethodPost, "/v1/cards/"+card.ID+"/handoff",
			protocol.HandoffRequest{To: protocol.AgentKindGemini}).
			apiError(t, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized)
	})
}
