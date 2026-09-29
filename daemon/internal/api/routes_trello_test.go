package api_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/integrations/trello"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// stackTrelloCards is the board module a stack gives its Trello sync, the way cmd/marshald adapts
// the projects service for it.
type stackTrelloCards struct {
	proj *projects.Service
}

func (c stackTrelloCards) CreateCard(ctx context.Context, projectID string, in protocol.CreateCardRequest) (protocol.Card, error) {
	return c.proj.CreateCard(ctx, projectID, in)
}

func (c stackTrelloCards) MoveCard(ctx context.Context, id string, in protocol.MoveCardRequest) (protocol.Card, error) {
	return c.proj.MoveCard(ctx, id, in)
}

func (c stackTrelloCards) Card(ctx context.Context, id string) (protocol.Card, error) {
	return c.proj.Card(ctx, id)
}

// The Trello connection over the API (B8.2, build-plan 8.2): saving it, testing it, and believing a
// delivery signed with what was saved. Nothing here dials Trello: the stack's Trello client is
// pointed at a fake server, and the delivery is signed with the same rule Trello uses.

const trelloCallbackURL = "https://marshal.local/hooks/trello"

// trelloSign is the signature Trello itself sends: HMAC-SHA1 over the raw body followed by the
// callback URL, base64 encoded. It is written out here rather than borrowed, so a test cannot pass
// by signing and checking with the same wrong rule.
func trelloSign(secret string, body []byte) string {
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(body)
	mac.Write([]byte(trelloCallbackURL))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// trelloRequest is the body PUT /v1/integrations/trello takes: a board linked to one Marshal
// project, which is what the sync writes into.
func trelloRequest(webhookSecret, projectID string) protocol.SaveTrelloRequest {
	return protocol.SaveTrelloRequest{
		APIKey:        "a-key",
		Token:         "a-token",
		ProjectID:     projectID,
		BoardID:       "board-1",
		WebhookSecret: webhookSecret,
		CallbackURL:   trelloCallbackURL,
	}
}

// newTrelloStack builds a stack whose Trello API is a fake server answering one working board, so
// saving a connection and pressing Test never leaves this machine.
func newTrelloStack(t *testing.T, opts ...stackOption) *stack {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"board-1","name":"Sprint Board","url":"https://trello.com/b/board-1"}`))
	}))
	t.Cleanup(server.Close)
	return newStack(t, append([]stackOption{withTrelloBaseURL(server.URL)}, opts...)...)
}

// deliverTrello posts a body to the Trello webhook route with the given signature header.
func (st *stack) deliverTrello(t *testing.T, signature string, body []byte) reply {
	t.Helper()
	req := st.newRequest(http.MethodPost, "/hooks/trello", body)
	if signature != "" {
		req.Header.Set(trello.SignatureHeader, signature)
	}
	return st.send(req)
}

// TestTrelloConnectionSavesTestsAndRefusesToo proves the whole connection over the wire: the save
// stores both halves, the list shows it connected, Test runs against the board, and a delivery
// signed with what was saved is believed while a forged one is not.
func TestTrelloConnectionSavesTestsAndRefuses(t *testing.T) {
	const secret = "a-trello-webhook-secret"
	st := newTrelloStack(t)
	project, _ := st.addProject("small-repo")

	// Saving takes the connection's own shape, not GitHub's.
	st.do(http.MethodPut, "/v1/integrations/trello", trelloRequest(secret, project.ID)).want(t, http.StatusOK)
	// Saving runs the connection's test straight away, against the board the connection names.
	row := integrationRowFor(t, st, "trello")
	if row.Status != protocol.IntegrationStatusConnected {
		t.Errorf("the Trello row reads %q, want connected", row.Status)
	}
	if row.LastTest == nil || !row.LastTest.OK {
		t.Errorf("the Trello row carries %+v, want the test the save ran", row.LastTest)
	}

	// A delivery signed with the saved secret's rule is accepted without a bearer token.
	body := []byte(`{"action":{"type":"updateCard","data":{"card":{"id":"card-1"}}}}`)
	st.deliverTrello(t, trelloSign(secret, body), body).want(t, http.StatusOK)

	// A forged signature and a missing one are both refused.
	st.deliverTrello(t, trelloSign("the-wrong-secret", body), body).
		apiError(t, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized)
	st.deliverTrello(t, "", body).
		apiError(t, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized)
}

// TestTrelloDeliveryIsRefusedWithNothingSaved proves the route exists on every daemon but believes
// nothing until a connection with a webhook secret and callback URL is saved.
func TestTrelloDeliveryIsRefusedWithNothingSaved(t *testing.T) {
	st := newStack(t)
	body := []byte(`{"action":{"type":"updateCard"}}`)
	st.deliverTrello(t, trelloSign("any-secret", body), body).
		apiError(t, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized)
}

// TestThereIsNoTrelloWebhookRouteWithoutTheConnectionsService proves the address does not exist at
// all on a daemon with no connections service.
func TestThereIsNoTrelloWebhookRouteWithoutTheConnectionsService(t *testing.T) {
	st := newStack(t, withoutIntegrations())
	st.deliverTrello(t, "anything", []byte(`{}`)).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}

// TestTrelloSaveNeedsBothHalvesOfAWebhook proves the secret and the callback URL must arrive
// together: neither can check a signature alone.
func TestTrelloSaveNeedsBothHalvesOfAWebhook(t *testing.T) {
	st := newTrelloStack(t)
	project, _ := st.addProject("small-repo")

	onlySecret := trelloRequest("a-secret", project.ID)
	onlySecret.CallbackURL = ""
	st.do(http.MethodPut, "/v1/integrations/trello", onlySecret).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)

	onlyCallback := trelloRequest("", project.ID)
	onlyCallback.WebhookSecret = ""
	onlyCallback.CallbackURL = trelloCallbackURL
	st.do(http.MethodPut, "/v1/integrations/trello", onlyCallback).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)

	// A board with no project has nowhere to put an imported card, so it is refused too.
	noProject := trelloRequest("a-secret", "")
	st.do(http.MethodPut, "/v1/integrations/trello", noProject).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
}

// TestTrelloDeliveryImportsACardIntoTheLinkedProject proves the whole inbound half of the sync over
// the wire: a signed delivery for a card on the linked board's import list makes a real Marshal card
// in the linked project, and Trello redelivering the same event does not make a second one.
func TestTrelloDeliveryImportsACardIntoTheLinkedProject(t *testing.T) {
	const secret = "a-trello-webhook-secret"
	st := newTrelloStack(t)
	project, _ := st.addProject("small-repo")

	save := trelloRequest(secret, project.ID)
	save.NewCardListID = "list-new"
	st.do(http.MethodPut, "/v1/integrations/trello", save).want(t, http.StatusOK)

	body := []byte(`{"action":{"type":"createCard","id":"a1","data":{"board":{"id":"board-1"},` +
		`"card":{"id":"trello-card-1","name":"Fix the flaky test","desc":"It fails one run in ten.",` +
		`"idList":"list-new"}}},"model":{"id":"board-1"}}`)
	st.deliverTrello(t, trelloSign(secret, body), body).want(t, http.StatusOK)
	st.deliverTrello(t, trelloSign(secret, body), body).want(t, http.StatusOK)

	board := decode[protocol.BoardSnapshot](t,
		st.do(http.MethodGet, "/v1/projects/"+project.ID+"/board", nil).want(t, http.StatusOK))
	if len(board.Cards) != 1 {
		t.Fatalf("the board has %d cards, want the one imported card: %+v", len(board.Cards), board.Cards)
	}
	imported := board.Cards[0]
	if imported.Title != "Fix the flaky test" || imported.Body != "It fails one run in ten." {
		t.Errorf("the imported card = %+v, want the Trello card's name and description", imported)
	}
	if imported.State != protocol.CardStateBacklog {
		t.Errorf("the imported card is in %q, want the backlog", imported.State)
	}
}

// TestTrelloDeliveryForAnotherListImportsNothing proves a card added to a list Marshal does not watch
// is not imported: the import list is a choice, not every list on the board.
func TestTrelloDeliveryForAnotherListImportsNothing(t *testing.T) {
	const secret = "a-trello-webhook-secret"
	st := newTrelloStack(t)
	project, _ := st.addProject("small-repo")
	save := trelloRequest(secret, project.ID)
	save.NewCardListID = "list-new"
	st.do(http.MethodPut, "/v1/integrations/trello", save).want(t, http.StatusOK)

	body := []byte(`{"action":{"type":"createCard","id":"a2","data":{"board":{"id":"board-1"},` +
		`"card":{"id":"trello-card-2","name":"Not for Marshal","idList":"list-someday"}}},"model":{"id":"board-1"}}`)
	st.deliverTrello(t, trelloSign(secret, body), body).want(t, http.StatusOK)

	board := decode[protocol.BoardSnapshot](t,
		st.do(http.MethodGet, "/v1/projects/"+project.ID+"/board", nil).want(t, http.StatusOK))
	if len(board.Cards) != 0 {
		t.Errorf("a card on an unwatched list was imported: %+v", board.Cards)
	}
}

// integrationRowFor reads one connection's row out of GET /v1/integrations.
func integrationRowFor(t *testing.T, st *stack, id string) protocol.Integration {
	t.Helper()
	list := decode[protocol.IntegrationList](t,
		st.do(http.MethodGet, "/v1/integrations", nil).want(t, http.StatusOK))
	for _, row := range list.Integrations {
		if row.ID == id {
			return row
		}
	}
	t.Fatalf("the integrations list has no %q row: %+v", id, list.Integrations)
	return protocol.Integration{}
}
