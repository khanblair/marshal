package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultAuthBaseURL is where GitHub's OAuth endpoints live.
const DefaultAuthBaseURL = "https://github.com"

// The device flow's own answers, as errors a caller switches on.
var (
	// ErrAuthorizationPending means the person has not approved the code yet.
	ErrAuthorizationPending = errors.New("the code has not been approved yet")
	// ErrExpiredCode means the code ran out before it was approved.
	ErrExpiredCode = errors.New("the code expired")
	// ErrAccessDenied means the person refused the code.
	ErrAccessDenied = errors.New("the code was refused")
	// ErrDeviceFlowDisabled means the GitHub App does not allow the device flow.
	ErrDeviceFlowDisabled = errors.New("the GitHub App has the device flow switched off")
	// ErrRefreshRejected means GitHub will not exchange the refresh token any more.
	ErrRefreshRejected = errors.New("GitHub rejected the refresh token")
)

// SlowDownError is GitHub asking for a longer wait between polls.
type SlowDownError struct {
	// Interval is the wait GitHub now wants. Zero means five seconds longer than before.
	Interval time.Duration
}

// Error implements error.
func (e *SlowDownError) Error() string { return "GitHub asked Marshal to poll less often" }

// DeviceCode is what starting the device flow answers: the code a person types and the secret
// Marshal polls with.
type DeviceCode struct {
	DeviceCode      string
	UserCode        string
	VerificationURI string
	ExpiresIn       time.Duration
	Interval        time.Duration
}

// TokenSet is a user access token and, when GitHub expires them, the pair that renews it.
type TokenSet struct {
	AccessToken      string
	RefreshToken     string
	ExpiresAt        time.Time
	RefreshExpiresAt time.Time
}

// DeviceFlow signs a person in through one GitHub App's public client id. It never holds a client
// secret: GitHub's device flow, and refreshing a token it made, do not need one.
type DeviceFlow struct {
	clientID string
	baseURL  string
	http     *http.Client
	now      func() time.Time
}

// NewDeviceFlow builds the flow client. Empty baseURL means github.com, nil hc a 20 second client,
// nil now time.Now.
func NewDeviceFlow(clientID, baseURL string, hc *http.Client, now func() time.Time) (*DeviceFlow, error) {
	if strings.TrimSpace(clientID) == "" {
		return nil, errors.New("the GitHub sign-in needs the App's client id")
	}
	if baseURL == "" {
		baseURL = DefaultAuthBaseURL
	}
	if hc == nil {
		hc = &http.Client{Timeout: DefaultTimeout}
	}
	if now == nil {
		now = time.Now
	}
	return &DeviceFlow{clientID: clientID, baseURL: strings.TrimRight(baseURL, "/"), http: hc, now: now}, nil
}

// Start asks GitHub for a code to show the person.
func (d *DeviceFlow) Start(ctx context.Context) (DeviceCode, error) {
	var answer struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
		oauthError
	}
	form := url.Values{"client_id": {d.clientID}}
	if err := d.post(ctx, "/login/device/code", form, &answer); err != nil {
		return DeviceCode{}, err
	}
	if answer.Error != "" {
		return DeviceCode{}, mapOAuthError(answer.oauthError)
	}
	if answer.DeviceCode == "" || answer.UserCode == "" {
		return DeviceCode{}, errors.New("GitHub answered without a device code")
	}
	code := DeviceCode{
		DeviceCode: answer.DeviceCode, UserCode: answer.UserCode, VerificationURI: answer.VerificationURI,
		ExpiresIn: time.Duration(answer.ExpiresIn) * time.Second,
		Interval:  time.Duration(answer.Interval) * time.Second,
	}
	if code.VerificationURI == "" {
		code.VerificationURI = d.baseURL + "/login/device"
	}
	if code.Interval <= 0 {
		code.Interval = 5 * time.Second
	}
	if code.ExpiresIn <= 0 {
		code.ExpiresIn = 15 * time.Minute
	}
	return code, nil
}

// Poll asks whether the code was approved. Until it is, the error says why not.
func (d *DeviceFlow) Poll(ctx context.Context, deviceCode string) (TokenSet, error) {
	form := url.Values{
		"client_id":   {d.clientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	return d.exchange(ctx, form)
}

// Refresh trades a refresh token for a new pair. GitHub rotates the refresh token on every use.
func (d *DeviceFlow) Refresh(ctx context.Context, refreshToken string) (TokenSet, error) {
	form := url.Values{
		"client_id":     {d.clientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	return d.exchange(ctx, form)
}

func (d *DeviceFlow) exchange(ctx context.Context, form url.Values) (TokenSet, error) {
	var answer struct {
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		ExpiresIn        int    `json:"expires_in"`
		RefreshExpiresIn int    `json:"refresh_token_expires_in"`
		Interval         int    `json:"interval"`
		oauthError
	}
	if err := d.post(ctx, "/login/oauth/access_token", form, &answer); err != nil {
		return TokenSet{}, err
	}
	if answer.Error != "" {
		err := mapOAuthError(answer.oauthError)
		var slow *SlowDownError
		if errors.As(err, &slow) && answer.Interval > 0 {
			slow.Interval = time.Duration(answer.Interval) * time.Second
		}
		return TokenSet{}, err
	}
	if answer.AccessToken == "" {
		return TokenSet{}, errors.New("GitHub answered without an access token")
	}
	now := d.now()
	set := TokenSet{AccessToken: answer.AccessToken, RefreshToken: answer.RefreshToken}
	if answer.ExpiresIn > 0 {
		set.ExpiresAt = now.Add(time.Duration(answer.ExpiresIn) * time.Second)
	}
	if answer.RefreshExpiresIn > 0 {
		set.RefreshExpiresAt = now.Add(time.Duration(answer.RefreshExpiresIn) * time.Second)
	}
	return set, nil
}

// oauthError is the error GitHub puts in an HTTP 200 answer.
type oauthError struct {
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

func mapOAuthError(e oauthError) error {
	switch e.Error {
	case "authorization_pending":
		return ErrAuthorizationPending
	case "slow_down":
		return &SlowDownError{}
	case "expired_token", "incorrect_device_code":
		return ErrExpiredCode
	case "access_denied":
		return ErrAccessDenied
	case "device_flow_disabled":
		return ErrDeviceFlowDisabled
	case "bad_refresh_token", "refresh_token_expired":
		return fmt.Errorf("%w: %s", ErrRefreshRejected, e.Error)
	default:
		return fmt.Errorf("GitHub refused the sign-in: %s %s", e.Error, e.Description)
	}
}

func (d *DeviceFlow) post(ctx context.Context, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build the request for %s: %w", path, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reply, err := d.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = reply.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(reply.Body, maxReadBody))
	if err != nil {
		return fmt.Errorf("read GitHub's answer: %w", err)
	}
	if reply.StatusCode < 200 || reply.StatusCode > 299 {
		return fmt.Errorf("GitHub answered %d to %s", reply.StatusCode, path)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("read GitHub's answer: %w", err)
	}
	return nil
}
