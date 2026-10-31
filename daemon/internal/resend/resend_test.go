package resend

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testKey = "re_test_secret_key_123"

// fake answers the Resend API with the given status and body for every path, and records what
// the client sent, so a test can check the key and the request.
func fake(t *testing.T, status int, body string) (*httptest.Server, *[]*http.Request) {
	t.Helper()
	srv, seen, _ := fakeWithBodies(t, status, body)
	return srv, seen
}

// fakeWithBodies is fake, and also keeps each request's body, which is read while the request is
// still open.
func fakeWithBodies(t *testing.T, status int, body string) (*httptest.Server, *[]*http.Request, *[][]byte) {
	t.Helper()
	var seen []*http.Request
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		seen = append(seen, r)
		bodies = append(bodies, raw)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &seen, &bodies
}

func clientFor(t *testing.T, base string) *Client {
	t.Helper()
	c, err := New(Config{BaseURL: base, Key: testKey, From: "Marshal <onboarding@resend.dev>"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// noKeyIn fails the test if the key appears in an error's text.
func noKeyIn(t *testing.T, err error) {
	t.Helper()
	if err != nil && strings.Contains(err.Error(), testKey) {
		t.Errorf("the key is in the error: %v", err)
	}
}

func TestCheckReadsDomainsWithTheKeyInTheHeaderOnly(t *testing.T) {
	srv, seen := fake(t, http.StatusOK, `{"data":[{"name":"kolaborate.co","status":"verified"}]}`)
	domains, err := clientFor(t, srv.URL).Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(domains) != 1 || domains[0].Name != "kolaborate.co" || domains[0].Status != "verified" {
		t.Errorf("domains = %+v, want kolaborate.co verified", domains)
	}
	req := (*seen)[0]
	if req.Method != http.MethodGet || req.URL.Path != "/domains" {
		t.Errorf("request = %s %s, want GET /domains", req.Method, req.URL.Path)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer "+testKey {
		t.Errorf("Authorization = %q, want the bearer key", got)
	}
	if strings.Contains(req.URL.String(), testKey) {
		t.Error("the key is in the address")
	}
}

func TestSendPostsOneEmailAndAnswersItsID(t *testing.T) {
	srv, seen, bodies := fakeWithBodies(t, http.StatusOK, `{"id":"msg_123"}`)
	id, err := clientFor(t, srv.URL).Send(context.Background(), "owner@example.com", "Brief", "Hello")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if id != "msg_123" {
		t.Errorf("id = %q, want msg_123", id)
	}
	req := (*seen)[0]
	if req.Method != http.MethodPost || req.URL.Path != "/emails" {
		t.Errorf("request = %s %s, want POST /emails", req.Method, req.URL.Path)
	}
	raw := (*bodies)[0]
	var sent struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
	}
	if err := json.Unmarshal(raw, &sent); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if sent.From != "Marshal <onboarding@resend.dev>" || len(sent.To) != 1 || sent.To[0] != "owner@example.com" || sent.Subject != "Brief" {
		t.Errorf("sent = %+v", sent)
	}
	if strings.Contains(string(raw), testKey) {
		t.Error("the key is in the body")
	}
}

// Each refusal keeps Resend's own sentence and its status, and the key never reaches the error.
func TestRefusalsKeepTheStatusAndTheReasonWithoutTheKey(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		reason string
	}{
		{"an unverified domain", http.StatusForbidden,
			`{"message":"You can only send testing emails to your own email address. verify a domain at resend.com/domains","name":"validation_error"}`,
			"verify a domain"},
		{"a key Resend does not know", http.StatusUnauthorized, `{"message":"API key is invalid"}`, "API key is invalid"},
		{"too many requests", http.StatusTooManyRequests, `{"message":"Rate limit exceeded"}`, "Rate limit exceeded"},
		{"a body that echoes the key", http.StatusUnauthorized, "bad key " + testKey, "bad key <key>"},
		{"an empty body", http.StatusInternalServerError, "", "no reason given"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := fake(t, tc.status, tc.body)
			_, err := clientFor(t, srv.URL).Send(context.Background(), "owner@example.com", "Brief", "Hello")
			noKeyIn(t, err)
			if !Refused(err, tc.status) {
				t.Fatalf("err = %v, want a refusal with status %d", err, tc.status)
			}
			if !strings.Contains(err.Error(), tc.reason) {
				t.Errorf("err = %v, want the reason %q", err, tc.reason)
			}
		})
	}
}

func TestANetworkFailureNeverShowsTheKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close()
	_, err := clientFor(t, srv.URL).Check(context.Background())
	if err == nil {
		t.Fatal("Check against a closed server succeeded")
	}
	noKeyIn(t, err)
}

func TestNewRefusesWhatResendRefusesWithoutACall(t *testing.T) {
	if _, err := New(Config{Key: " ", From: "a@b.co"}); err == nil {
		t.Error("New accepted an empty key")
	}
	if _, err := New(Config{Key: testKey, From: ""}); err == nil {
		t.Error("New accepted an empty sender")
	}
	if _, err := New(Config{Key: testKey, From: "a@b.co"}); err != nil {
		t.Errorf("New refused a key and a sender: %v", err)
	}
}

func TestRefusedMatchesOnlyTheStatusItIsAskedAbout(t *testing.T) {
	err := &APIError{Status: http.StatusForbidden, Message: "no"}
	if !Refused(err, http.StatusForbidden) || Refused(err, http.StatusUnauthorized) || Refused(errors.New("x"), http.StatusForbidden) {
		t.Error("Refused does not match the status it is asked about")
	}
}

func TestAnyTwoHundredSeriesStatusIsASuccessAndAnUnreadableSendAnswerIsNotAFailure(t *testing.T) {
	srv, _ := fake(t, http.StatusAccepted, `{"id":"msg_9"}`)
	id, err := clientFor(t, srv.URL).Send(context.Background(), "owner@example.com", "Brief", "Hello")
	if err != nil || id != "msg_9" {
		t.Errorf("a 202 send = %q, %v; want it read as sent", id, err)
	}
	srv, _ = fake(t, http.StatusOK, "not json")
	id, err = clientFor(t, srv.URL).Send(context.Background(), "owner@example.com", "Brief", "Hello")
	if err != nil || id != "" {
		t.Errorf("a send with an unreadable answer = %q, %v; want no error, the mail has gone", id, err)
	}
}

func TestRestrictedMatchesOnlyAKeyThatMaySendOnly(t *testing.T) {
	srv, _ := fake(t, http.StatusUnauthorized, `{"name":"restricted_api_key","message":"This API key is restricted to only send emails"}`)
	_, err := clientFor(t, srv.URL).Check(context.Background())
	if !Restricted(err) {
		t.Errorf("Restricted(%v) = false, want true for a sending-only key", err)
	}
	srv, _ = fake(t, http.StatusUnauthorized, `{"message":"API key is invalid"}`)
	_, err = clientFor(t, srv.URL).Check(context.Background())
	if Restricted(err) {
		t.Errorf("Restricted(%v) = true, want false for a key Resend does not know", err)
	}
}
