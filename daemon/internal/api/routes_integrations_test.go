package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The connections Marshal is set up with, apart from model providers (B6.1, B6.7, architecture.md
// section 18). Saving is a whole-connection call and the answer is the whole list every time, so a
// screen redraws from one answer. Nothing secret ever comes back.

// passingTester is what the stack's connections service uses when no test is overridden: a test that
// found nothing wrong, so a route test never dials GitHub.
func passingTester(_ context.Context, info integrations.Info) (protocol.TestResult, error) {
	return protocol.NewTestResult(info.ID, []protocol.TestCheck{{
		Name:    integrations.CheckSummary,
		State:   protocol.CheckStatePassed,
		Message: "The GitHub App works and Marshal can use it.",
	}}, time.Now()), nil
}

func TestTheIntegrationListShowsEveryConnectionWithNothingStored(t *testing.T) {
	st := newStack(t, withIntegrationsTester(passingTester))
	got := decode[protocol.IntegrationList](t, st.do(http.MethodGet, "/v1/integrations", nil).want(t, http.StatusOK))

	want := []string{"github", "trello", "gcal", "gmail", "telegram", "discord", "ntfy", "obsidian"}
	if len(got.Integrations) != len(want) {
		t.Fatalf("the list has %d connections, want the %d rows the settings screen shows",
			len(got.Integrations), len(want))
	}
	for i, id := range want {
		if got.Integrations[i].ID != id {
			t.Fatalf("connection %d is %q, want %q", i, got.Integrations[i].ID, id)
		}
		if got.Integrations[i].Status != protocol.IntegrationStatusNone {
			t.Fatalf("%s reads %q with nothing stored, want none", id, got.Integrations[i].Status)
		}
	}
	if got.ServerTime.Time().IsZero() {
		t.Fatal("the answer carries no server time")
	}
}

func TestSavingTheGitHubAppConnectsItAndTestsItInTheSameCall(t *testing.T) {
	const secret = "s3cr3t"
	st := newStack(t, withIntegrationsTester(passingTester))

	got := decode[protocol.IntegrationList](t, st.do(http.MethodPut, "/v1/integrations/github",
		githubAppRequest(secret)).want(t, http.StatusOK))

	github := got.Integrations[0]
	if github.Status != protocol.IntegrationStatusConnected {
		t.Fatalf("GitHub reads %q right after being saved, want connected", github.Status)
	}
	// Section 18: a test runs automatically right after a connection is added, and the answer the
	// save returns already carries its result.
	if github.LastTest == nil || !github.LastTest.OK {
		t.Fatalf("GitHub's row carries no passing test after a save: %+v", github.LastTest)
	}
	if github.Detail == "" {
		t.Fatal("a connected row has no sentence to show")
	}
	// Nothing secret came back: neither the key nor the webhook secret is anywhere in the answer.
	body := string(st.do(http.MethodGet, "/v1/integrations", nil).want(t, http.StatusOK).Body)
	if strings.Contains(body, secret) || strings.Contains(body, "PRIVATE KEY") {
		t.Fatalf("the list answer carries a secret: %s", body)
	}
}

func TestSavingAnAppThatCouldNotWorkIsRefusedAndChangesNothing(t *testing.T) {
	st := newStack(t, withIntegrationsTester(passingTester))
	req := githubAppRequest("s3cr3t")
	req.WebhookSecret = ""

	st.do(http.MethodPut, "/v1/integrations/github", req).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)

	got := decode[protocol.IntegrationList](t, st.do(http.MethodGet, "/v1/integrations", nil).want(t, http.StatusOK))
	if got.Integrations[0].Status != protocol.IntegrationStatusNone {
		t.Fatalf("a refused save left GitHub reading %q, want none", got.Integrations[0].Status)
	}
}

func TestSavingAnUnknownConnectionIsNotFound(t *testing.T) {
	st := newStack(t, withIntegrationsTester(passingTester))
	st.do(http.MethodPut, "/v1/integrations/myspace", githubAppRequest("s3cr3t")).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}

func TestTestingAConnectionAnswersItsResultAndThenRefusesInsideTheCooldown(t *testing.T) {
	st := newStack(t, withIntegrationsTester(passingTester))

	got := decode[protocol.TestResult](t, st.do(http.MethodPost, "/v1/integrations/github/test", nil).
		want(t, http.StatusOK))
	if !got.OK || got.ConnectionID != "github" {
		t.Fatalf("the test answered %+v, want a passing result for github", got)
	}
	// The cooldown is there to respect the service's rate limits: a second press is refused with how
	// long to wait rather than calling GitHub again.
	refused := st.do(http.MethodPost, "/v1/integrations/github/test", nil).
		apiError(t, http.StatusConflict, protocol.ErrorCodeConflict)
	if refused.Details["retryAfterMs"] == "" {
		t.Fatalf("the refusal carries no retryAfterMs: %+v", refused.Details)
	}
}

func TestTestingAnUnknownConnectionIsNotFound(t *testing.T) {
	st := newStack(t, withIntegrationsTester(passingTester))
	st.do(http.MethodPost, "/v1/integrations/myspace/test", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}

func TestRemovingTheConnectionForgetsIt(t *testing.T) {
	st := newStack(t, withIntegrationsTester(passingTester))
	st.do(http.MethodPut, "/v1/integrations/github", githubAppRequest("s3cr3t")).want(t, http.StatusOK)

	got := decode[protocol.IntegrationList](t, st.do(http.MethodDelete, "/v1/integrations/github", nil).
		want(t, http.StatusOK))
	if got.Integrations[0].Status != protocol.IntegrationStatusNone {
		t.Fatalf("GitHub reads %q after being removed, want none", got.Integrations[0].Status)
	}
	// Removing a connection that is not there is not an error: the answer is the same list.
	st.do(http.MethodDelete, "/v1/integrations/github", nil).want(t, http.StatusOK)
}

func TestTheIntegrationRoutesFollowTheirService(t *testing.T) {
	// Every service the server can be given has a stack without it, so a route that needs one is
	// proven to follow it rather than being registered against a nil service.
	st := newStack(t, withoutIntegrations())
	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/integrations"},
		{http.MethodPut, "/v1/integrations/github"},
		{http.MethodDelete, "/v1/integrations/github"},
		{http.MethodPost, "/v1/integrations/github/test"},
	} {
		st.do(route.method, route.path, nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	}
}
