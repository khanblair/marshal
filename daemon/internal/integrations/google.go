package integrations

// This file owns the Google Calendar connection: its OAuth client, the consent flow, and the
// stored token (B8.3). Gmail (gmail.go) is a separate connection, sibling id "gmail", that shares
// this one client and token rather than asking for a second consent.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/oauth2"

	"github.com/khanblair/marshal/daemon/internal/integrations/gmailread"
	"github.com/khanblair/marshal/daemon/internal/integrations/googlecal"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// GCalID is the Google Calendar connection's own id.
const GCalID = "gcal"

// KindGCal is the kind the Google Calendar connection's row and test are filed under.
const KindGCal = "calendar"

// ErrNoGoogleClient means no OAuth client is saved yet, so there is nothing to start a consent
// flow with. It is the ordinary state before SaveGoogleCalendar is ever called.
var ErrNoGoogleClient = errors.New("no Google OAuth client is saved")

type gcalConfig struct {
	ClientID string `json:"clientId"`
}

// gcalSecrets is the keychain half: the client secret, and the token once consent finishes.
// Token is nil until AuthorizeGoogleCalendar's flow completes.
type gcalSecrets struct {
	ClientSecret string        `json:"clientSecret"`
	Token        *oauth2.Token `json:"token,omitempty"`
}

// SaveGoogleCalendar stores the OAuth client. It does not connect anything yet - the owner still
// has to open AuthorizeGoogleCalendar's URL and grant access.
func (s *Service) SaveGoogleCalendar(ctx context.Context, req protocol.SaveGoogleCalendarRequest) error {
	if strings.TrimSpace(req.ClientID) == "" || strings.TrimSpace(req.ClientSecret) == "" {
		return protocol.InvalidArgument("Marshal needs both a Google OAuth client id and secret.")
	}
	secrets, err := json.Marshal(gcalSecrets{ClientSecret: req.ClientSecret})
	if err != nil {
		return fmt.Errorf("write the Google Calendar connection's secrets: %w", err)
	}
	if err := s.keys.Set(GCalID, string(secrets)); err != nil {
		return fmt.Errorf("save the Google Calendar connection's secrets: %w", err)
	}
	config, err := json.Marshal(gcalConfig{ClientID: req.ClientID})
	if err != nil {
		return fmt.Errorf("write the Google Calendar connection's settings: %w", err)
	}
	return s.store.Write(ctx, func(q *db.Queries) error {
		return q.UpsertIntegration(ctx, db.UpsertIntegrationParams{
			ID: GCalID, Kind: KindGCal, ConfigJSON: string(config), KeychainRef: GCalID,
		})
	})
}

// AuthorizeGoogleCalendar answers the consent URL for the owner's own browser to open. state is
// kept in memory and checked by the callback; a restart mid-flow just means starting over.
func (s *Service) AuthorizeGoogleCalendar(ctx context.Context) (string, error) {
	config, secrets, err := s.readGCal(ctx)
	if err != nil {
		return "", err
	}
	if config.ClientID == "" || secrets.ClientSecret == "" {
		return "", ErrNoGoogleClient
	}
	state, err := randomState()
	if err != nil {
		return "", fmt.Errorf("make the consent flow's state: %w", err)
	}
	s.mu.Lock()
	s.gcalState = state
	s.mu.Unlock()
	cfg := googlecal.Config(config.ClientID, secrets.ClientSecret, s.gcalRedirectURL, gmailread.Scope)
	return cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce), nil
}

// FinishGoogleCalendar exchanges the callback's code for a token and stores it, once state
// matches the one AuthorizeGoogleCalendar handed out.
func (s *Service) FinishGoogleCalendar(ctx context.Context, code, state string) error {
	s.mu.Lock()
	want := s.gcalState
	s.gcalState = ""
	s.mu.Unlock()
	if want == "" || state != want {
		return protocol.InvalidArgument("This consent link has expired or was already used.")
	}
	config, secrets, err := s.readGCal(ctx)
	if err != nil {
		return err
	}
	if config.ClientID == "" || secrets.ClientSecret == "" {
		return ErrNoGoogleClient
	}
	cfg := googlecal.Config(config.ClientID, secrets.ClientSecret, s.gcalRedirectURL, gmailread.Scope)
	token, err := cfg.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("exchange the Google consent code: %w", err)
	}
	secrets.Token = token
	raw, err := json.Marshal(secrets)
	if err != nil {
		return fmt.Errorf("write the Google Calendar connection's token: %w", err)
	}
	return s.keys.Set(GCalID, string(raw))
}

// GoogleCalendarClient answers a client built from the stored token, refreshing it through
// oauth2's own TokenSource and saving a refreshed token back. ErrNotConnected means no consent has
// completed yet.
func (s *Service) GoogleCalendarClient(ctx context.Context) (*googlecal.Client, error) {
	fresh, err := s.freshGoogleToken(ctx)
	if err != nil {
		return nil, err
	}
	return googlecal.New(ctx, oauth2.NewClient(ctx, oauth2.StaticTokenSource(fresh)), s.gcalBase)
}

// freshGoogleToken is the one shared client and token Google Calendar and Gmail both read
// through: it refreshes the stored token when it has expired, through oauth2's own TokenSource,
// and saves a refreshed one back. ErrNotConnected means no consent has completed yet.
func (s *Service) freshGoogleToken(ctx context.Context) (*oauth2.Token, error) {
	config, secrets, err := s.readGCal(ctx)
	if err != nil {
		return nil, err
	}
	if secrets.Token == nil {
		return nil, ErrNotConnected
	}
	cfg := googlecal.Config(config.ClientID, secrets.ClientSecret, s.gcalRedirectURL, gmailread.Scope)
	source := cfg.TokenSource(ctx, secrets.Token)
	fresh, err := source.Token()
	if err != nil {
		return nil, fmt.Errorf("refresh the Google token: %w", err)
	}
	if fresh.AccessToken != secrets.Token.AccessToken {
		secrets.Token = fresh
		if raw, err := json.Marshal(secrets); err == nil {
			_ = s.keys.Set(GCalID, string(raw))
		}
	}
	return fresh, nil
}

func (s *Service) testGCal(ctx context.Context, info Info) (protocol.TestResult, error) {
	client, err := s.GoogleCalendarClient(ctx)
	if errors.Is(err, ErrNotConnected) || errors.Is(err, ErrNoGoogleClient) {
		return protocol.NewTestResult(info.ID, []protocol.TestCheck{{
			Name:    CheckSummary,
			State:   protocol.CheckStateFailed,
			Message: "Marshal has no Google Calendar connection saved.",
			Fix:     "Add a Google OAuth client in Settings, under Integrations, then grant access.",
		}}, s.now()), nil
	}
	if err != nil {
		return protocol.TestResult{}, err
	}
	calendars, err := client.About(ctx)
	if err != nil {
		return protocol.NewTestResult(info.ID, []protocol.TestCheck{{
			Name: CheckSummary, State: protocol.CheckStateFailed,
			Message: "Marshal's Google token did not work: " + err.Error(),
			Fix:     "Reconnect Google Calendar in Settings.",
		}}, s.now()), nil
	}
	return protocol.NewTestResult(info.ID, []protocol.TestCheck{{
		Name: CheckSummary, State: protocol.CheckStatePassed,
		Message: fmt.Sprintf("Reading %d calendar(s).", calendars),
	}}, s.now()), nil
}

func (s *Service) readGCal(ctx context.Context) (gcalConfig, gcalSecrets, error) {
	var config gcalConfig
	var found bool
	err := s.store.Read(ctx, func(q *db.Queries) error {
		row, err := q.GetIntegration(ctx, GCalID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("read the Google Calendar connection's settings: %w", err)
		}
		found = true
		if row.ConfigJSON == "" {
			return nil
		}
		return json.Unmarshal([]byte(row.ConfigJSON), &config)
	})
	if err != nil || !found {
		return gcalConfig{}, gcalSecrets{}, err
	}
	raw, err := s.keys.Get(GCalID)
	if err != nil && !errors.Is(err, security.ErrNoKey) {
		return gcalConfig{}, gcalSecrets{}, fmt.Errorf(
			"read the Google Calendar connection's secrets: %w", err)
	}
	var secrets gcalSecrets
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &secrets); err != nil {
			return gcalConfig{}, gcalSecrets{}, fmt.Errorf(
				"read the Google Calendar connection's secrets: %w", err)
		}
	}
	return config, secrets, nil
}

// randomState makes a consent flow's CSRF token: 32 random bytes, hex-encoded.
func randomState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
