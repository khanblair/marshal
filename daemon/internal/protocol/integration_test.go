package protocol_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The wire shape of a connection (B6.7, section 18): a service Marshal has been set up with, its
// status, and the last test's own answer.

// sampleIntegrationList is the answer to GET /v1/integrations with the three states on show: a
// GitHub App that is set up and working, a Trello that is not connected, and a calendar whose last
// test found the token refused.
func sampleIntegrationList() protocol.IntegrationList {
	github := protocol.TestResult{
		ConnectionID: "github",
		Checks: []protocol.TestCheck{
			{Name: "App", State: protocol.CheckStatePassed, Message: "The GitHub App is installed."},
			{Name: "Repositories", State: protocol.CheckStatePassed, Message: "3 repositories are visible."},
			{Name: "Permissions", State: protocol.CheckStatePassed, Message: "Issues, pull requests, and Actions are allowed."},
			{Name: "Webhook", State: protocol.CheckStatePassed, Message: "A ping reached Marshal."},
		},
		OK:    true,
		RanAt: protocol.NewTimestamp(providersNow),
	}
	calendar := protocol.TestResult{
		ConnectionID: "calendar",
		Checks: []protocol.TestCheck{{
			Name: "Token", State: protocol.CheckStateFailed,
			Message: "Google refused this connection's token.",
			Fix:     "Connect the calendar again in Settings.",
		}},
		RanAt: protocol.NewTimestamp(providersNow.Add(-2 * time.Hour)),
	}
	return protocol.NewIntegrationList([]protocol.Integration{
		{ID: "github", Kind: "github", Status: protocol.IntegrationStatusConnected,
			Detail: "GitHub App installed on 3 repositories", LastTest: &github},
		{ID: "trello", Kind: "trello", Status: protocol.IntegrationStatusNone},
		{ID: "calendar", Kind: "calendar", Status: protocol.IntegrationStatusError,
			Detail: "The Google Calendar connection needs attention", LastTest: &calendar},
	}, providersNow)
}

func TestIntegrationListGolden(t *testing.T) {
	testutil.Golden(t, "integration-list", sampleIntegrationList())
}

// The three status words are the ones the settings screen already reads for an integration row. A
// fourth value - an install in progress - is not stored: it is a fact about the browser a person is
// in, and the ruling is recorded in the phase report.
func TestIntegrationStatusesAreTheThreeTheScreenReads(t *testing.T) {
	want := []string{"connected", "none", "error"}
	got := protocol.IntegrationStatusValues()
	if len(got) != len(want) {
		t.Fatalf("IntegrationStatusValues = %v, want %v", got, want)
	}
	for i, name := range want {
		if string(got[i]) != name {
			t.Fatalf("IntegrationStatusValues = %v, want %v", got, want)
		}
	}
	if !protocol.IntegrationStatusConnected.Valid() || protocol.IntegrationStatus("installing").Valid() {
		t.Error("Valid accepts a status the daemon never stores")
	}
}

// A connection nothing is stored for carries no last test at all rather than an empty one, so a
// screen can tell "never tested" from "tested and nothing was found".
func TestAnUntestedConnectionCarriesNoTest(t *testing.T) {
	body, err := json.Marshal(sampleIntegrationList())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"id":"trello","kind":"trello","st":"none","detail":""`) {
		t.Errorf("an untested connection encoded as %s, want no lastTest field", body)
	}
	if strings.Contains(string(body), `"st":"none","detail":"","lastTest":`) {
		t.Errorf("an untested connection carries an empty test: %s", body)
	}
	if !strings.Contains(string(body), `"serverTime":"2026-09-27T09:30:00.000Z"`) {
		t.Errorf("an answer carries the daemon's time: %s", body)
	}
}

// An install with nothing to list still answers [] and not null.
func TestIntegrationListNeverEncodesNull(t *testing.T) {
	body, err := json.Marshal(protocol.NewIntegrationList(nil, providersNow))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); !strings.Contains(got, `"integrations":[]`) {
		t.Errorf("an empty list encoded as %s, want []", got)
	}
}

// A save names the App's two ids, its key, and its webhook secret. The key and the secret are
// written to the keychain and never come back from a route.
func TestSaveGitHubRequestGolden(t *testing.T) {
	testutil.Golden(t, "save-github-request", protocol.SaveGitHubRequest{
		AppID:          1282340,
		InstallationID: 55123907,
		PrivateKey:     "-----BEGIN RSA PRIVATE KEY-----\nMIIEogIBAAKCAQEA…\n-----END RSA PRIVATE KEY-----\n",
		WebhookSecret:  "a-synthetic-webhook-secret",
	})
}
