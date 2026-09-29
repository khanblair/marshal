package trello_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrations/trello"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// sign is the signing rule Trello itself uses, written out here rather than borrowed from the
// package under test: HMAC-SHA1 over the raw body followed by the callback URL, base64 encoded. A
// test that signed with the package's own helper would pass even if both were wrong together.
func sign(secret, callbackURL string, body []byte) string {
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(body)
	mac.Write([]byte(callbackURL))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	const (
		secret      = "trello-secret-key"
		callbackURL = "https://example.com/hooks/trello"
	)
	body := []byte(`{"action":{"type":"updateCard"}}`)
	valid := sign(secret, callbackURL, body)

	tests := []struct {
		name      string
		secret    string
		signature string
		body      []byte
		want      bool
	}{
		{"the delivery Trello sent", secret, valid, body, true},
		{"a tampered body", secret, valid, []byte(`{"action":{"type":"deleteCard"}}`), false},
		{"the wrong secret", "another-secret", valid, body, false},
		{"no signature", secret, "", body, false},
		{"no secret saved", "", valid, body, false},
		{"a signature that is not base64 of the right length", secret, "bogus", body, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trello.VerifySignature(tt.secret, callbackURL, tt.body, tt.signature); got != tt.want {
				t.Errorf("VerifySignature = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTrelloClientAndConnectionTest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/boards/board-123":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"board-123","name":"Sprint Board","url":"https://trello.com/b/board-123"}`))
		case "/boards/board-123/cards":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":"card-1","name":"Card One","idList":"list-1"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := trello.NewClient("fake-key", "fake-token", trello.WithBaseURL(server.URL))
	ctx := context.Background()

	board, err := client.GetBoard(ctx, "board-123")
	if err != nil {
		t.Fatalf("GetBoard error: %v", err)
	}
	if board.Name != "Sprint Board" {
		t.Errorf("GetBoard name = %q, want Sprint Board", board.Name)
	}

	cards, err := client.ListCards(ctx, "board-123")
	if err != nil {
		t.Fatalf("ListCards error: %v", err)
	}
	if len(cards) != 1 || cards[0].Name != "Card One" {
		t.Errorf("ListCards = %+v, want 1 card named Card One", cards)
	}

	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	res := trello.TestConnection(ctx, client, "board-123", now)
	if !res.OK {
		t.Fatalf("TestConnection OK = false, checks: %+v", res.Checks)
	}
	if res.ConnectionID != trello.ID {
		t.Errorf("ConnectionID = %q, want %q", res.ConnectionID, trello.ID)
	}
	if len(res.Checks) != 3 {
		t.Fatalf("got %d checks, want 3 (summary, credential, board)", len(res.Checks))
	}
	if res.Checks[0].Name != trello.CheckSummary {
		t.Errorf("first check = %q, want the summary first", res.Checks[0].Name)
	}
	if res.Checks[2].Name != trello.CheckBoardRead || res.Checks[2].State != protocol.CheckStatePassed {
		t.Errorf("board check = %+v, want a passed %q", res.Checks[2], trello.CheckBoardRead)
	}
}

// TestConnectionTestNamesTheProblem proves the test answers rather than errors when something is
// wrong: no credential, and a board the token cannot read.
func TestConnectionTestNamesTheProblem(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()

	noCredential := trello.TestConnection(ctx, trello.NewClient("", ""), "board-123", now)
	if noCredential.OK {
		t.Error("a test with no credential reported OK")
	}
	if _, ok := noCredential.FirstFailed(); !ok {
		t.Error("a test with no credential reported no failed check")
	}
	if len(noCredential.Checks) != 2 || noCredential.Checks[1].Name != trello.CheckCredentials ||
		noCredential.Checks[1].State != protocol.CheckStateFailed {
		t.Errorf("checks = %+v, want a failed credential check after the summary", noCredential.Checks)
	}
	if noCredential.Checks[0].Name != trello.CheckSummary {
		t.Errorf("first check = %q, want the summary first", noCredential.Checks[0].Name)
	}

	// A client whose requests all fail: the board check is the one that reports it, and the
	// credential check still passes.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer dead.Close()
	unreadable := trello.TestConnection(ctx, trello.NewClient("k", "t", trello.WithBaseURL(dead.URL)), "board-1", now)
	if unreadable.OK {
		t.Error("a test with an unreadable board reported OK")
	}
	if unreadable.Checks[1].State != protocol.CheckStatePassed {
		t.Errorf("credential check = %+v, want passed", unreadable.Checks[1])
	}
	if unreadable.Checks[2].Name != trello.CheckBoardRead || unreadable.Checks[2].State != protocol.CheckStateFailed {
		t.Errorf("board check = %+v, want a failed board check", unreadable.Checks[2])
	}
}
