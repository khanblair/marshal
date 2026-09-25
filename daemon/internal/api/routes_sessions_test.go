package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
)

func TestStartSendStopThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Add a health check")
	stream := st.dial(protocol.CardTopic(card.ID))
	base := "/v1/cards/" + card.ID

	started := st.do(http.MethodPost, base+"/start", nil).want(t, http.StatusOK)
	sameShape(t, "card", started.Body)
	running := decode[protocol.Card](t, started)
	if running.ID != card.ID || running.State != protocol.CardStateWorking ||
		!strings.HasPrefix(running.Branch, "marshal/") || running.Number != card.Number {
		t.Errorf("the started card is %+v, want the same card, working, on a marshal/ branch", running)
	}
	if got := decode[protocol.Card](t, st.do(http.MethodGet, base, nil).want(t, http.StatusOK)); got.State != protocol.CardStateWorking || got.Branch != running.Branch {
		t.Errorf("reading the card back gives %+v", got)
	}
	stream.until(stateOf(card.ID, protocol.SessionStateAwake))

	// A card that already runs is refused, and nothing about it changes.
	got := st.do(http.MethodPost, base+"/start", nil).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got.Message != "This card's agent is already running." {
		t.Errorf("message = %q", got.Message)
	}
	got = st.do(http.MethodPost, base+"/resume", nil).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got.Message != "This card's agent is already running." {
		t.Errorf("resuming a running card said %q", got.Message)
	}

	sent := st.do(http.MethodPost, base+"/messages", protocol.SendMessageRequest{Text: "please add a health check"}).want(t, http.StatusNoContent)
	if len(sent.Body) != 0 {
		t.Errorf("a 204 has no body, got %q", sent.Body)
	}
	seen := stream.until(stateOf(card.ID, protocol.SessionStateWorking))
	seen = append(seen, stream.until(stateOf(card.ID, protocol.SessionStateAwake))...)
	if texts := strings.Join(messageTexts(seen), ""); !strings.Contains(texts, "Turn 1. I remember 0 earlier turns.") {
		t.Errorf("the agent's answer was %q, want the stub's first turn", texts)
	}
	st.mustNotLog("please add a health check")

	st.do(http.MethodPost, base+"/stop", nil).want(t, http.StatusNoContent)
	stream.until(stateOf(card.ID, protocol.SessionStateStopped))

	tests := []struct {
		name    string
		path    string
		body    any
		message string
	}{
		{"stopping it again", base + "/stop", nil, "This card has no agent running."},
		{"sending to it", base + "/messages", protocol.SendMessageRequest{Text: "hello?"}, "This card has no agent running."},
		{"resuming it", base + "/resume", nil, "This card's session has stopped and cannot be resumed."},
		{"starting it again", base + "/start", nil,
			"This card already had a session, and it has stopped. Starting it again is not supported yet."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := st.do(http.MethodPost, tc.path, tc.body).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
			if got.Message != tc.message {
				t.Errorf("message = %q\nwant      %q", got.Message, tc.message)
			}
		})
	}
}

// A card that was never started has no agent to stop, to send to, or to resume.
func TestACardThatWasNeverStarted(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Not started")
	base := "/v1/cards/" + card.ID
	tests := []struct {
		name    string
		path    string
		body    any
		message string
	}{
		{"stop", base + "/stop", nil, "This card has no agent running."},
		{"send", base + "/messages", protocol.SendMessageRequest{Text: "hello"}, "This card has no agent running."},
		{"resume", base + "/resume", nil, "This card has not been started, so there is nothing to resume. Start it first."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := st.do(http.MethodPost, tc.path, tc.body).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
			if got.Message != tc.message {
				t.Errorf("message = %q\nwant      %q", got.Message, tc.message)
			}
		})
	}
	if got := decode[protocol.Card](t, st.do(http.MethodGet, base, nil).want(t, http.StatusOK)); got.State != protocol.CardStateBacklog {
		t.Errorf("the refusals moved the card to %s", got.State)
	}
}

// Text that is empty, only spaces, or too long is refused before the agent is asked. Text at the
// limit is taken, and the limit counts characters and not bytes.
func TestMessageRules(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Talk to me")
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).want(t, http.StatusOK)
	path := "/v1/cards/" + card.ID + "/messages"

	refused := []struct {
		name    string
		body    any
		message string
	}{
		{"an empty text", `{"text":""}`, "Write a message first."},
		{"only spaces", `{"text":" \t\n "}`, "Write a message first."},
		{"no text field", `{}`, "Write a message first."},
		{"a text that is too long", protocol.SendMessageRequest{Text: strings.Repeat("a", protocol.MaxMessageChars+1)},
			"That message is too long. Send at most 100,000 characters at a time."},
		{"a text that is too long in characters that take three bytes", protocol.SendMessageRequest{Text: strings.Repeat("€", protocol.MaxMessageChars+1)},
			"That message is too long. Send at most 100,000 characters at a time."},
		{"a text of the wrong kind", `{"text":5}`, `The field "text" must be text.`},
		{"a field Marshal does not know", `{"text":"hi","urgent":true}`, `The field "urgent" is not one Marshal knows. Remove it and try again.`},
		{"no body", nil, "The request body is empty. Send a JSON object."},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			got := st.do(http.MethodPost, path, tc.body).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
			if got.Message != tc.message {
				t.Errorf("message = %q\nwant      %q", got.Message, tc.message)
			}
		})
	}
	st.do(http.MethodPost, path, protocol.SendMessageRequest{Text: strings.Repeat("€", protocol.MaxMessageChars)}).want(t, http.StatusNoContent)
}

// The routes that take no body do not need one, and take an empty one or a JSON one without
// reading it. A body that is not marked as JSON is refused like on every route.
func TestRoutesWithoutABodyAcceptAnyJSONBody(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "No body needed")
	base := "/v1/cards/" + card.ID

	st.do(http.MethodPost, base+"/start", `{}`).want(t, http.StatusOK)
	st.do(http.MethodPost, base+"/stop", "").want(t, http.StatusNoContent)
	st.do(http.MethodPost, "/v1/agents/refresh", `{"anything":true}`).want(t, http.StatusOK)

	req := st.newRequest(http.MethodPost, base+"/stop", "not json")
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Authorization", "Bearer "+st.token)
	st.send(req).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
}

// A card whose agent Marshal cannot start gets a plain answer, and leaves nothing behind.
func TestStartingACardWithAnAgentMarshalCannotRun(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	created := st.do(http.MethodPost, "/v1/projects/"+project.ID+"/cards",
		protocol.CreateCardRequest{Title: "Use gemini", Agent: protocol.AgentKindGemini}).want(t, http.StatusCreated)
	card := decode[protocol.Card](t, created)

	got := st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).apiError(t, http.StatusNotImplemented, protocol.ErrorCodeUnsupported)
	if got.Message != "Marshal does not have that agent ready to run yet." {
		t.Errorf("message = %q", got.Message)
	}
	if folders := st.worktreeFolders(project.ID); len(folders) != 0 {
		t.Errorf("the failed start left the worktree folders %v", folders)
	}
	after := decode[protocol.Card](t, st.do(http.MethodGet, "/v1/cards/"+card.ID, nil).want(t, http.StatusOK))
	if after.State != protocol.CardStateBacklog || after.Branch != "" {
		t.Errorf("the failed start left the card as %+v", after)
	}
	if _, err := st.store.Queries().GetSessionByCard(context.Background(), card.ID); !store.IsNotFound(err) {
		t.Errorf("the failed start left a session row (error %v)", err)
	}
}
