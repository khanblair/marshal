package integrations_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The Trello connection (B8.2): what saving it stores, what its own test asks, and what removing it
// forgets. Nothing here dials Trello: the client is pointed at a fake server by TrelloBaseURL, which
// is the one seam a test sets.

// trelloFixture is a connections service whose Trello API is a fake server under this test's own
// control, plus the requests that server saw, so a test can prove what the connection did and did
// not ask for.
type trelloFixture struct {
	*fixture
	server   *httptest.Server
	requests []string
	// status is what the fake Trello answers every request with. 200 is a working board.
	status int
}

func newTrelloFixture(t *testing.T) *trelloFixture {
	t.Helper()
	tf := &trelloFixture{status: http.StatusOK}
	tf.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tf.requests = append(tf.requests, r.URL.Path)
		if tf.status != http.StatusOK {
			http.Error(w, "trello said no", tf.status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"board-1","name":"Sprint Board","url":"https://trello.com/b/board-1"}`))
	}))
	t.Cleanup(tf.server.Close)
	tf.fixture = newFixture(t, func(o *integrations.Options) {
		o.TrelloBaseURL = tf.server.URL
		o.Now = func() time.Time { return time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC) }
	})
	return tf
}

// trelloSave is a complete Trello connection: a board linked to one Marshal project, the credential,
// and the two halves a delivery is checked against. A test that wants a connection missing one piece
// clears that field from a copy.
func trelloSave() protocol.SaveTrelloRequest {
	return protocol.SaveTrelloRequest{
		APIKey:        "a-key",
		Token:         "a-token",
		ProjectID:     "small-repo",
		BoardID:       "board-1",
		WebhookSecret: "a-webhook-secret",
		CallbackURL:   "https://marshal.local/hooks/trello",
	}
}

// TestTrelloConnectionSavesAndTests proves the round trip: a saved connection builds a client, its
// own test reads the board it can reach, and the connection reads as connected in the list.
func TestTrelloConnectionSavesAndTests(t *testing.T) {
	tf := newTrelloFixture(t)
	ctx := context.Background()

	if err := tf.svc.SaveTrello(ctx, trelloSave()); err != nil {
		t.Fatalf("SaveTrello: %v", err)
	}

	client, boardID, err := tf.svc.TrelloClient(ctx)
	if err != nil {
		t.Fatalf("TrelloClient: %v", err)
	}
	if client == nil || boardID != "board-1" {
		t.Errorf("TrelloClient answered %v and board %q, want a client and board-1", client, boardID)
	}

	result, err := tf.svc.Test(ctx, integrations.TrelloID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !result.OK {
		t.Fatalf("the Trello test answered %+v, want OK", result.Checks)
	}
	if result.ConnectionID != integrations.TrelloID {
		t.Errorf("the test is filed under %q, want %q", result.ConnectionID, integrations.TrelloID)
	}
	if len(tf.requests) == 0 || tf.requests[0] != "/boards/board-1" {
		t.Errorf("the test asked for %v, want the configured board first", tf.requests)
	}

	// The list shows the connection as set up. The test's own result reaches the row through the
	// connection-test runner that saves it, which the API route drives; here the row's status is
	// what a saved connection reads as.
	list, err := tf.svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	row := rowFor(t, list, integrations.TrelloID)
	if row.Status != protocol.IntegrationStatusConnected {
		t.Errorf("the Trello row reads %q, want connected", row.Status)
	}
}

// TestTrelloConnectionNamesWhatIsWrong proves a refused or unreachable board is an answer and not an
// error, which is what makes the row show a fix.
func TestTrelloConnectionNamesWhatIsWrong(t *testing.T) {
	tf := newTrelloFixture(t)
	ctx := context.Background()
	tf.status = http.StatusUnauthorized
	if err := tf.svc.SaveTrello(ctx, trelloSave()); err != nil {
		t.Fatalf("SaveTrello: %v", err)
	}
	result, err := tf.svc.Test(ctx, integrations.TrelloID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if result.OK {
		t.Fatal("a refused board reported OK")
	}
	failed, ok := result.FirstFailed()
	if !ok || failed.Fix == "" {
		t.Errorf("the failed result = %+v, want a failure naming a fix", result.Checks)
	}
}

// TestTrelloConnectionRefusesAHalfSetWebhook proves the secret and the callback URL must arrive
// together: neither can check a signature alone, so a half-set pair is refused rather than stored.
func TestTrelloConnectionRefusesAHalfSetWebhook(t *testing.T) {
	tf := newTrelloFixture(t)
	ctx := context.Background()

	onlySecret := trelloSave()
	onlySecret.CallbackURL = ""
	if err := tf.svc.SaveTrello(ctx, onlySecret); err == nil {
		t.Error("a webhook secret with no callback URL was accepted")
	}
	onlyCallback := trelloSave()
	onlyCallback.WebhookSecret = ""
	if err := tf.svc.SaveTrello(ctx, onlyCallback); err == nil {
		t.Error("a callback URL with no webhook secret was accepted")
	}

	// A key, a token, and a board are each required: nothing can be asked of Trello without them.
	noKey := trelloSave()
	noKey.APIKey = ""
	if err := tf.svc.SaveTrello(ctx, noKey); err == nil {
		t.Error("a connection with no API key was accepted")
	}
	noBoard := trelloSave()
	noBoard.BoardID = ""
	if err := tf.svc.SaveTrello(ctx, noBoard); err == nil {
		t.Error("a connection with no board was accepted")
	}
}

// TestTrelloConnectionRemoves proves removing the connection forgets both halves: the row is gone
// and so is the keychain entry, so a removed connection cannot test or verify anything.
func TestTrelloConnectionRemoves(t *testing.T) {
	tf := newTrelloFixture(t)
	ctx := context.Background()
	if err := tf.svc.SaveTrello(ctx, trelloSave()); err != nil {
		t.Fatalf("SaveTrello: %v", err)
	}
	if err := tf.svc.Remove(ctx, integrations.TrelloID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, _, err := tf.svc.TrelloClient(ctx); !errors.Is(err, integrations.ErrNotConnected) {
		t.Errorf("TrelloClient after remove = %v, want ErrNotConnected", err)
	}
	secret, callback, err := tf.svc.TrelloWebhook(ctx)
	if err != nil {
		t.Fatalf("TrelloWebhook: %v", err)
	}
	if secret != "" || callback != "" {
		t.Errorf("a removed connection still answers secret %q and URL %q", secret, callback)
	}
}

// TestTrelloWebhookReadsBothHalves proves the two things a delivery is checked against come back
// from what was saved, and read as empty before anything is.
func TestTrelloWebhookReadsBothHalves(t *testing.T) {
	tf := newTrelloFixture(t)
	ctx := context.Background()

	secret, callback, err := tf.svc.TrelloWebhook(ctx)
	if err != nil {
		t.Fatalf("TrelloWebhook with nothing saved: %v", err)
	}
	if secret != "" || callback != "" {
		t.Errorf("an empty daemon answers secret %q and URL %q, want both empty", secret, callback)
	}

	if err := tf.svc.SaveTrello(ctx, trelloSave()); err != nil {
		t.Fatalf("SaveTrello: %v", err)
	}
	secret, callback, err = tf.svc.TrelloWebhook(ctx)
	if err != nil {
		t.Fatalf("TrelloWebhook: %v", err)
	}
	if secret != "a-webhook-secret" || callback != "https://marshal.local/hooks/trello" {
		t.Errorf("TrelloWebhook answered %q and %q, want what was saved", secret, callback)
	}
}

// rowFor finds one connection's row in a list, failing when it is missing.
func rowFor(t *testing.T, list []protocol.Integration, id string) protocol.Integration {
	t.Helper()
	for _, row := range list {
		if row.ID == id {
			return row
		}
	}
	t.Fatalf("the list has no %q row: %+v", id, list)
	return protocol.Integration{}
}
