package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// deviceServer answers every sign-in endpoint with the next canned body and records the last form.
type deviceServer struct {
	srv    *httptest.Server
	body   map[string]any
	status int
	form   map[string][]string
	accept string
}

func newDeviceServer(t *testing.T) *deviceServer {
	t.Helper()
	d := &deviceServer{status: http.StatusOK}
	d.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		d.form, d.accept = r.PostForm, r.Header.Get("Accept")
		w.WriteHeader(d.status)
		_ = json.NewEncoder(w).Encode(d.body)
	}))
	t.Cleanup(d.srv.Close)
	return d
}

func (d *deviceServer) flow(t *testing.T) *DeviceFlow {
	t.Helper()
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	flow, err := NewDeviceFlow("Iv-test", d.srv.URL, nil, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDeviceFlow: %v", err)
	}
	return flow
}

func TestStartAsksForACodeWithOnlyTheClientID(t *testing.T) {
	d := newDeviceServer(t)
	d.body = map[string]any{"device_code": "dc", "user_code": "AB-12", "verification_uri": "https://github.com/login/device", "expires_in": 900, "interval": 5}
	code, err := d.flow(t).Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if code.UserCode != "AB-12" || code.DeviceCode != "dc" || code.Interval != 5*time.Second || code.ExpiresIn != 15*time.Minute {
		t.Fatalf("code = %+v", code)
	}
	if d.accept != "application/json" || len(d.form) != 1 || d.form["client_id"][0] != "Iv-test" {
		t.Fatalf("request: accept=%q form=%v, want JSON and only client_id", d.accept, d.form)
	}
}

func TestStartNamesAnAppWithTheDeviceFlowOff(t *testing.T) {
	d := newDeviceServer(t)
	d.body = map[string]any{"error": "device_flow_disabled"}
	if _, err := d.flow(t).Start(context.Background()); !errors.Is(err, ErrDeviceFlowDisabled) {
		t.Fatalf("Start answered %v, want ErrDeviceFlowDisabled", err)
	}
}

func TestPollMapsGitHubsAnswersToErrorsAndTokens(t *testing.T) {
	d := newDeviceServer(t)
	flow := d.flow(t)
	cases := []struct {
		name string
		body map[string]any
		want error
	}{
		{"pending", map[string]any{"error": "authorization_pending"}, ErrAuthorizationPending},
		{"expired", map[string]any{"error": "expired_token"}, ErrExpiredCode},
		{"denied", map[string]any{"error": "access_denied"}, ErrAccessDenied},
		{"disabled", map[string]any{"error": "device_flow_disabled"}, ErrDeviceFlowDisabled},
	}
	for _, tc := range cases {
		d.body = tc.body
		if _, err := flow.Poll(context.Background(), "dc"); !errors.Is(err, tc.want) {
			t.Fatalf("%s: Poll answered %v, want %v", tc.name, err, tc.want)
		}
	}

	d.body = map[string]any{"error": "slow_down", "interval": 10}
	_, err := flow.Poll(context.Background(), "dc")
	var slow *SlowDownError
	if !errors.As(err, &slow) || slow.Interval != 10*time.Second {
		t.Fatalf("slow_down answered %v, want a SlowDownError of 10s", err)
	}

	d.body = map[string]any{"access_token": "ghu_1", "refresh_token": "ghr_1", "expires_in": 28800, "refresh_token_expires_in": 15897600}
	set, err := flow.Poll(context.Background(), "dc")
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	wantExpiry := time.Date(2026, 10, 2, 20, 0, 0, 0, time.UTC)
	if set.AccessToken != "ghu_1" || set.RefreshToken != "ghr_1" || !set.ExpiresAt.Equal(wantExpiry) || set.RefreshExpiresAt.IsZero() {
		t.Fatalf("token set = %+v", set)
	}
	if d.form["grant_type"][0] != "urn:ietf:params:oauth:grant-type:device_code" || d.form["device_code"][0] != "dc" {
		t.Fatalf("poll form = %v", d.form)
	}
}

func TestATokenThatDoesNotExpireHasNoExpiry(t *testing.T) {
	d := newDeviceServer(t)
	d.body = map[string]any{"access_token": "ghu_forever"}
	set, err := d.flow(t).Poll(context.Background(), "dc")
	if err != nil || !set.ExpiresAt.IsZero() || set.RefreshToken != "" {
		t.Fatalf("set = %+v err = %v, want a token with no expiry", set, err)
	}
}

func TestRefreshSendsNoClientSecretAndMapsARejection(t *testing.T) {
	d := newDeviceServer(t)
	d.body = map[string]any{"access_token": "ghu_2", "refresh_token": "ghr_2", "expires_in": 28800}
	if _, err := d.flow(t).Refresh(context.Background(), "ghr_1"); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if _, has := d.form["client_secret"]; has || d.form["grant_type"][0] != "refresh_token" || d.form["refresh_token"][0] != "ghr_1" {
		t.Fatalf("refresh form = %v, want grant_type, client_id and refresh_token only", d.form)
	}
	d.body = map[string]any{"error": "bad_refresh_token"}
	if _, err := d.flow(t).Refresh(context.Background(), "ghr_1"); !errors.Is(err, ErrRefreshRejected) {
		t.Fatalf("Refresh answered %v, want ErrRefreshRejected", err)
	}
}

func TestAServerErrorIsNotMistakenForAnApproval(t *testing.T) {
	d := newDeviceServer(t)
	d.status, d.body = http.StatusBadGateway, map[string]any{}
	if _, err := d.flow(t).Poll(context.Background(), "dc"); err == nil || errors.Is(err, ErrAuthorizationPending) {
		t.Fatalf("a 502 answered %v, want a plain error", err)
	}
	if _, err := NewDeviceFlow(" ", "", nil, nil); err == nil {
		t.Fatal("a flow with no client id was built")
	}
}
