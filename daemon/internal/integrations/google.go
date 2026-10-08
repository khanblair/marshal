package integrations

// This file owns the Google connections: the OAuth client, the consent flow, and the stored tokens
// (B8.3). All six share one client, saved under Calendar, and each asks for its own consent and keeps
// its own token, so a person who only wants the calendar is never asked for Gmail's restricted scope.
// googleConnections is the table of them. Events are in google_events.go, files in google_files.go.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"github.com/khanblair/marshal/daemon/internal/integrations/gmailread"
	"github.com/khanblair/marshal/daemon/internal/integrations/googlecal"
	"github.com/khanblair/marshal/daemon/internal/integrations/googleclient"
	"github.com/khanblair/marshal/daemon/internal/integrations/googlefiles"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// GCalID is the Google Calendar connection's own id.
const GCalID = "gcal"

// KindGCal is the kind the Google Calendar connection's row and test are filed under.
const KindGCal = "calendar"

// googleRevokeURL is where Google ends an access.
const googleRevokeURL = "https://oauth2.googleapis.com/revoke"

// revokeTimeout bounds the call that tells Google to end an access, so a Disconnect never waits long.
const revokeTimeout = 5 * time.Second

// consentTTL is how long a consent link stays good. Longer than a person needs to read Google's page.
const consentTTL = 10 * time.Minute

// ErrNoGoogleClient means no OAuth client is saved yet, so there is nothing to start a consent
// flow with. It is the ordinary state before SaveGoogleCalendar is ever called.
var ErrNoGoogleClient = errors.New("no Google OAuth client is saved")

// ErrNeedsReconnect means Google no longer accepts the stored access: the person revoked it, or the
// refresh token expired (an app in Google's Testing mode loses it after about a week). Only a new
// consent fixes it.
var ErrNeedsReconnect = errors.New("Google no longer accepts Marshal's access")

type gcalConfig struct {
	ClientID string `json:"clientId"`
	// Calendars are the ids of the calendars Marshal reads, when the person has chosen them. With
	// CalendarsChosen false they follow what is ticked in Google Calendar's own side list.
	Calendars       []string `json:"calendars,omitempty"`
	CalendarsChosen bool     `json:"calendarsChosen,omitempty"`
}

// gcalSecrets is the keychain half: the client secret, and Calendar's token once consent finishes.
// Token is nil until the consent flow completes.
type gcalSecrets struct {
	ClientSecret string        `json:"clientSecret"`
	Token        *oauth2.Token `json:"token,omitempty"`
}

// googleTokenSecrets is the keychain entry of every Google connection but Calendar: its own token,
// granted by its own consent, filed under the connection's own id.
type googleTokenSecrets struct {
	Token *oauth2.Token `json:"token,omitempty"`
}

// googleConnection is one Google connection: its id, the kind its row and its test are filed under,
// the name a person reads, and the scopes its consent asks for and nothing more.
type googleConnection struct {
	ID     string
	Kind   string
	Name   string
	Scopes []string
}

// googleConnections is every Google connection, in the order the settings screen shows them. It is
// the one place that says which connections share the OAuth client: the consent, the stored token,
// the scope check, the revoke, and the callback's words all read it.
func googleConnections() []googleConnection {
	return []googleConnection{
		{GCalID, KindGCal, "Google Calendar", []string{googlecal.Scope}},
		{GmailID, KindGmail, "Gmail", []string{gmailread.Scope}},
		{GDriveID, KindGDrive, "Google Drive", []string{googlefiles.ScopeDriveFile}},
		{GDocsID, KindGDocs, "Google Docs", []string{googlefiles.ScopeDocsReadonly, googlefiles.ScopeDriveFile}},
		{GSheetsID, KindGSheets, "Google Sheets", []string{googlefiles.ScopeSheetsReadonly, googlefiles.ScopeDriveFile}},
		{GSlidesID, KindGSlides, "Google Slides", []string{googlefiles.ScopeSlidesReadonly, googlefiles.ScopeDriveFile}},
	}
}

// googleConnectionOf finds one Google connection by id.
func googleConnectionOf(id string) (googleConnection, bool) {
	for _, conn := range googleConnections() {
		if conn.ID == id {
			return conn, true
		}
	}
	return googleConnection{}, false
}

// GoogleName is the name a person reads for a Google connection, or "" when id is not one.
func GoogleName(id string) string {
	conn, _ := googleConnectionOf(id)
	return conn.Name
}

// IsGoogleFile says id is one of the four file connections: Drive, Docs, Sheets and Slides.
func IsGoogleFile(id string) bool {
	return id == GDriveID || id == GDocsID || id == GSheetsID || id == GSlidesID
}

// pendingConsent is one consent flow that has been started and not finished.
type pendingConsent struct {
	id       string
	verifier string
	expires  time.Time
}

// googleState is what the Google connections keep in memory: the consent flows under way, where
// Google's own endpoints are (a test points them at a fake), and what was last read.
type googleState struct {
	endpoint oauth2.Endpoint
	// revokeURL is where an access is ended at Google.
	revokeURL string
	// bundled is Marshal's own Google client, when this build has one. A person's own client, saved
	// in Settings, is used instead whenever there is one.
	bundled googleclient.Client

	mu       sync.Mutex
	consents map[string]pendingConsent
	read     readCache

	// folderMu keeps two saves from both making Marshal's folder. folders holds the ids found so far,
	// by grant and folder name, until a connection changes.
	folderMu sync.Mutex
	folders  map[string]string
}

// configure reads Marshal's own Google client and, for a test, points Google's endpoints elsewhere.
// The client comes from the options when they carry one (the daemon passes what the environment
// says, and a test its own), and from the build otherwise.
func (g *googleState) configure(opts Options) {
	if opts.GoogleAuthURL != "" || opts.GoogleTokenURL != "" {
		g.endpoint = oauth2.Endpoint{AuthURL: opts.GoogleAuthURL, TokenURL: opts.GoogleTokenURL}
	}
	g.revokeURL = googleRevokeURL
	if opts.GoogleRevokeURL != "" {
		g.revokeURL = opts.GoogleRevokeURL
	}
	if !opts.NoBundledGoogleClient {
		g.bundled = googleclient.Bundled()
	}
	if override := (googleclient.Client{ID: opts.GoogleClientID, Secret: opts.GoogleClientSecret}); override.Valid() {
		g.bundled = override
	}
}

// effectiveClient is the OAuth client a consent flow and a token refresh use: the person's own, when
// they saved one in Settings, and Marshal's own otherwise. own says which it is.
func (s *Service) effectiveClient(config gcalConfig, secrets gcalSecrets) (client googleclient.Client, own bool) {
	mine := googleclient.Client{ID: config.ClientID, Secret: secrets.ClientSecret}
	if mine.Valid() {
		return mine, true
	}
	return s.google.bundled, false
}

// GoogleClientInfo says which Google client Marshal would use: its own, built in, so a person
// connects with one click; and whether they saved one of their own in Settings.
func (s *Service) GoogleClientInfo(ctx context.Context) (protocol.GoogleClientInfo, error) {
	config, secrets, err := s.readGCal(ctx)
	if err != nil {
		return protocol.GoogleClientInfo{}, err
	}
	_, own := s.effectiveClient(config, secrets)
	return protocol.GoogleClientInfo{Bundled: s.google.bundled.Valid(), Own: own}, nil
}

// oauthConfig builds the OAuth2 config for one Google connection, asking for its scopes and nothing
// else.
func (s *Service) oauthConfig(id, clientID, clientSecret string) (oauth2.Config, error) {
	conn, ok := googleConnectionOf(id)
	if !ok {
		return oauth2.Config{}, protocol.NotFound("connection").With("id", id)
	}
	return googlecal.Config(clientID, clientSecret, s.gcalRedirectURL, s.google.endpoint, conn.Scopes...), nil
}

// SaveGoogleCalendar stores the OAuth client. It does not connect anything yet - the owner still
// has to open the consent URL and grant access. Saving a different client drops the tokens the old
// one granted, because Google will not honor them for the new one. The choice of calendars is kept.
func (s *Service) SaveGoogleCalendar(ctx context.Context, req protocol.SaveGoogleCalendarRequest) error {
	if strings.TrimSpace(req.ClientID) == "" || strings.TrimSpace(req.ClientSecret) == "" {
		return protocol.InvalidArgument("Marshal needs both a Google OAuth client id and secret.")
	}
	before, beforeSecrets, err := s.readGCal(ctx)
	if err != nil {
		return err
	}
	secrets := gcalSecrets{ClientSecret: req.ClientSecret}
	sameClient := before.ClientID == req.ClientID && beforeSecrets.ClientSecret == req.ClientSecret
	if sameClient {
		secrets.Token = beforeSecrets.Token
	} else if err := s.dropOtherGoogleTokens(); err != nil {
		return err
	}
	raw, err := json.Marshal(secrets)
	if err != nil {
		return fmt.Errorf("write the Google Calendar connection's secrets: %w", err)
	}
	if err := s.keys.Set(GCalID, string(raw)); err != nil {
		return fmt.Errorf("save the Google Calendar connection's secrets: %w", err)
	}
	config := before
	config.ClientID = req.ClientID
	s.forgetGoogleReads()
	return s.writeGCal(ctx, config)
}

// dropOtherGoogleTokens forgets the token of every Google connection but Calendar. They were granted
// to the old client, and Google will not honor them for a new one.
func (s *Service) dropOtherGoogleTokens() error {
	for _, conn := range googleConnections() {
		if conn.ID == GCalID {
			continue
		}
		if err := s.keys.Remove(conn.ID); err != nil && !errors.Is(err, security.ErrNoKey) {
			return fmt.Errorf("drop %s's token, which belonged to the old client: %w", conn.Name, err)
		}
	}
	return nil
}

// writeGCal stores Calendar's settings.
func (s *Service) writeGCal(ctx context.Context, config gcalConfig) error {
	raw, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("write the Google Calendar connection's settings: %w", err)
	}
	return s.store.Write(ctx, func(q *db.Queries) error {
		return q.UpsertIntegration(ctx, db.UpsertIntegrationParams{
			ID: GCalID, Kind: KindGCal, ConfigJSON: string(raw), KeychainRef: GCalID,
		})
	})
}

// AuthorizeGoogleCalendar answers the consent URL for Calendar, for the owner's own browser to open.
func (s *Service) AuthorizeGoogleCalendar(ctx context.Context) (string, error) {
	return s.AuthorizeGoogle(ctx, GCalID)
}

// AuthorizeGmail answers the consent URL for Gmail. It uses the client saved under Calendar, and
// asks for Gmail's scope alone.
func (s *Service) AuthorizeGmail(ctx context.Context) (string, error) {
	return s.AuthorizeGoogle(ctx, GmailID)
}

// AuthorizeGoogle starts the consent flow of one Google connection and answers where the owner goes
// to grant access. The flow is kept in memory, with a single-use state and a PKCE verifier; a restart
// mid-flow just means starting over.
func (s *Service) AuthorizeGoogle(ctx context.Context, id string) (string, error) {
	if _, ok := googleConnectionOf(id); !ok {
		return "", protocol.NotFound("connection").With("id", id)
	}
	config, secrets, err := s.readGCal(ctx)
	if err != nil {
		return "", err
	}
	client, _ := s.effectiveClient(config, secrets)
	if !client.Valid() {
		return "", ErrNoGoogleClient
	}
	cfg, err := s.oauthConfig(id, client.ID, client.Secret)
	if err != nil {
		return "", err
	}
	state, err := randomToken()
	if err != nil {
		return "", fmt.Errorf("make the consent flow's state: %w", err)
	}
	verifier := oauth2.GenerateVerifier()
	s.google.mu.Lock()
	if s.google.consents == nil {
		s.google.consents = map[string]pendingConsent{}
	}
	now := s.now()
	for key, pending := range s.google.consents {
		if now.After(pending.expires) {
			delete(s.google.consents, key)
		}
	}
	s.google.consents[state] = pendingConsent{id: id, verifier: verifier, expires: now.Add(consentTTL)}
	s.google.mu.Unlock()
	return cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce,
		oauth2.S256ChallengeOption(verifier)), nil
}

// FinishGoogle exchanges the callback's code for a token and stores it as the connection's own, once
// state matches a flow AuthorizeGoogle started. It answers which connection was granted.
func (s *Service) FinishGoogle(ctx context.Context, code, state string) (string, error) {
	s.google.mu.Lock()
	pending, found := s.google.consents[state]
	delete(s.google.consents, state)
	s.google.mu.Unlock()
	if !found || s.now().After(pending.expires) {
		return "", protocol.InvalidArgument("This consent link has expired or was already used.")
	}
	if strings.TrimSpace(code) == "" {
		return "", protocol.InvalidArgument("Google did not send a code. Start the connection again.")
	}
	config, secrets, err := s.readGCal(ctx)
	if err != nil {
		return "", err
	}
	client, _ := s.effectiveClient(config, secrets)
	if !client.Valid() {
		return "", ErrNoGoogleClient
	}
	cfg, err := s.oauthConfig(pending.id, client.ID, client.Secret)
	if err != nil {
		return "", err
	}
	token, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(pending.verifier))
	if err != nil {
		return "", fmt.Errorf("exchange the Google consent code: %w", err)
	}
	if token.RefreshToken == "" {
		return "", protocol.InvalidArgument("Google gave no lasting access. Remove Marshal at myaccount.google.com/permissions, then connect again.")
	}
	conn, _ := googleConnectionOf(pending.id)
	if missingScope(token, conn) {
		// Google lets a person untick a box on its consent page. Nothing is stored, and the access
		// is not revoked either: Google would end every scope this client holds, not only this one.
		return "", protocol.InvalidArgument(partialGrantSentence)
	}
	// A grant makes its own row when there is none, so the row reads connected. A person who never
	// saved a client of their own has no Google Calendar row, and the row's settings say only that
	// Marshal's own client is in use. Gmail's row is made by saving its label, which a grant is not.
	if conn.ID != GmailID {
		if err := s.ensureGoogleRow(ctx, conn); err != nil {
			return "", err
		}
	}
	if err := s.saveGoogleToken(ctx, pending.id, secrets, token); err != nil {
		return "", err
	}
	s.forgetGoogleReads()
	return pending.id, nil
}

// partialGrantSentence is what a person reads when Google's consent page was left with a box unticked.
const partialGrantSentence = "Google did not give Marshal every permission it asked for. Connect again and leave every box ticked."

// missingScope says Google reported what it granted and left out a scope the connection needs. A
// token that carries no scope list is taken as granted in full, which is how Google answers when
// nothing was left out.
func missingScope(token *oauth2.Token, conn googleConnection) bool {
	granted, _ := token.Extra("scope").(string)
	have := strings.Fields(granted)
	if len(have) == 0 {
		return false
	}
	for _, want := range conn.Scopes {
		if !slices.Contains(have, want) {
			return true
		}
	}
	return false
}

// ensureGoogleRow makes a connection's row when it has none, and fills one that is empty. Running a
// test before any grant leaves a row with no settings and no keychain name, which is not a row a
// grant can leave alone. A settings value a person already saved is never replaced.
func (s *Service) ensureGoogleRow(ctx context.Context, conn googleConnection) error {
	row, found, err := s.row(ctx, conn.ID)
	if err != nil {
		return err
	}
	if found && row.saved() {
		return nil
	}
	config := row.config
	if strings.TrimSpace(config) == "" {
		config = defaultGoogleConfig(conn.ID)
	}
	return s.store.Write(ctx, func(q *db.Queries) error {
		return q.UpsertIntegration(ctx, db.UpsertIntegrationParams{
			ID: conn.ID, Kind: conn.Kind, ConfigJSON: config, KeychainRef: conn.ID,
		})
	})
}

// defaultGoogleConfig is the settings a connection starts with: none, except that Drive names its
// folder.
func defaultGoogleConfig(id string) string {
	if id == GDriveID {
		return `{"folder":"` + defaultFolder + `"}`
	}
	return "{}"
}

// saveGoogleToken stores a token as the connection's own. Calendar's keeps the client secret beside
// it, and every other connection's holds the token alone.
func (s *Service) saveGoogleToken(_ context.Context, id string, calendar gcalSecrets, token *oauth2.Token) error {
	var raw []byte
	var err error
	if id == GCalID {
		calendar.Token = token
		raw, err = json.Marshal(calendar)
	} else {
		raw, err = json.Marshal(googleTokenSecrets{Token: token})
	}
	if err != nil {
		return fmt.Errorf("write the %s connection's token: %w", id, err)
	}
	if err := s.keys.Set(id, string(raw)); err != nil {
		return fmt.Errorf("save the %s connection's token: %w", id, err)
	}
	return nil
}

// GoogleCalendarClient answers a client built from Calendar's stored token, refreshing it through
// oauth2's own TokenSource and saving a refreshed token back. ErrNotConnected means no consent has
// completed yet, and ErrNeedsReconnect that Google no longer honors the one that did.
func (s *Service) GoogleCalendarClient(ctx context.Context) (*googlecal.Client, error) {
	fresh, err := s.freshGoogleToken(ctx, GCalID)
	if err != nil {
		return nil, err
	}
	return googlecal.New(ctx, oauth2.NewClient(ctx, oauth2.StaticTokenSource(fresh)), s.gcalBase)
}

// freshGoogleToken is the stored token of one Google connection, refreshed when it has expired and
// saved back when it was. ErrNotConnected means no consent has completed for it yet.
func (s *Service) freshGoogleToken(ctx context.Context, id string) (*oauth2.Token, error) {
	if _, ok := googleConnectionOf(id); !ok {
		return nil, protocol.NotFound("connection").With("id", id)
	}
	config, secrets, err := s.readGCal(ctx)
	if err != nil {
		return nil, err
	}
	token := secrets.Token
	if id != GCalID {
		token, err = s.readGoogleToken(id)
		if err != nil {
			return nil, err
		}
	}
	if token == nil {
		return nil, ErrNotConnected
	}
	client, _ := s.effectiveClient(config, secrets)
	if !client.Valid() {
		return nil, ErrNoGoogleClient
	}
	cfg, err := s.oauthConfig(id, client.ID, client.Secret)
	if err != nil {
		return nil, err
	}
	fresh, err := cfg.TokenSource(ctx, token).Token()
	if err != nil {
		return nil, refreshFailure(err)
	}
	if fresh.AccessToken != token.AccessToken {
		if err := s.saveGoogleToken(ctx, id, secrets, fresh); err != nil {
			s.log.Warn("a refreshed Google token could not be saved", "connection", id, "err", err)
		}
	}
	return fresh, nil
}

// refreshFailure says what a failed refresh means. Google answers invalid_grant for an access that
// was revoked or has expired, and nothing a retry does will change that.
func refreshFailure(err error) error {
	var retrieve *oauth2.RetrieveError
	if errors.As(err, &retrieve) {
		if retrieve.ErrorCode == "invalid_grant" || retrieve.Response != nil &&
			(retrieve.Response.StatusCode == http.StatusBadRequest || retrieve.Response.StatusCode == http.StatusUnauthorized) {
			return fmt.Errorf("%w: %v", ErrNeedsReconnect, err)
		}
	}
	return fmt.Errorf("refresh the Google token: %w", err)
}

// refused says Google answered that the token is not good, whatever the call was.
func refused(err error) bool {
	var api *googleapi.Error
	return errors.As(err, &api) && api.Code == http.StatusUnauthorized
}

func (s *Service) testGCal(ctx context.Context, info Info) (protocol.TestResult, error) {
	client, err := s.GoogleCalendarClient(ctx)
	switch {
	case errors.Is(err, ErrNotConnected) || errors.Is(err, ErrNoGoogleClient):
		return protocol.NewTestResult(info.ID, []protocol.TestCheck{{
			Name:    CheckSummary,
			State:   protocol.CheckStateFailed,
			Message: "Google Calendar is not connected.",
			Fix:     "Choose Connect with Google in Settings, under Integrations.",
		}}, s.now()), nil
	case errors.Is(err, ErrNeedsReconnect):
		return protocol.NewTestResult(info.ID, []protocol.TestCheck{{
			Name: CheckSummary, State: protocol.CheckStateFailed,
			Message: "Google no longer accepts Marshal's access to Calendar.",
			Fix:     "Reconnect Google Calendar in Settings.",
		}}, s.now()), nil
	case err != nil:
		return protocol.TestResult{}, err
	}
	calendars, err := client.Calendars(ctx)
	if err != nil {
		return protocol.NewTestResult(info.ID, []protocol.TestCheck{{
			Name: CheckSummary, State: protocol.CheckStateFailed,
			Message: "Marshal's Google token did not work: " + err.Error(),
			Fix:     "Reconnect Google Calendar in Settings.",
		}}, s.now()), nil
	}
	message := fmt.Sprintf("Reading %d calendar(s).", len(calendars))
	for _, calendar := range calendars {
		// The primary calendar is named after the Google account, so the row says whose it is.
		if calendar.Primary {
			message = fmt.Sprintf("Reading %d calendar(s) of %s.", len(calendars), calendar.ID)
			break
		}
	}
	return protocol.NewTestResult(info.ID, []protocol.TestCheck{{
		Name: CheckSummary, State: protocol.CheckStatePassed, Message: message,
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

// readGoogleToken reads the token of a Google connection that keeps its own secret entry: every one
// but Calendar. Nil means its consent has not completed.
func (s *Service) readGoogleToken(id string) (*oauth2.Token, error) {
	raw, err := s.keys.Get(id)
	if errors.Is(err, security.ErrNoKey) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the %s connection's secrets: %w", GoogleName(id), err)
	}
	var secrets googleTokenSecrets
	if err := json.Unmarshal([]byte(raw), &secrets); err != nil {
		return nil, fmt.Errorf("read the %s connection's secrets: %w", GoogleName(id), err)
	}
	return secrets.Token, nil
}

// randomToken makes a consent flow's CSRF token: 32 random bytes, hex-encoded.
func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// storedGrant is the token one Google connection holds, or nil when it holds none.
func (s *Service) storedGrant(ctx context.Context, id string) (*oauth2.Token, error) {
	if id != GCalID {
		return s.readGoogleToken(id)
	}
	_, secrets, err := s.readGCal(ctx)
	return secrets.Token, err
}

// grantToken is the token one Google connection holds, or nil when it holds none or its secrets
// cannot be read.
func (s *Service) grantToken(ctx context.Context, id string) *oauth2.Token {
	token, err := s.storedGrant(ctx, id)
	if err != nil {
		s.log.Warn("a Google connection's token could not be read", "connection", id, "err", err)
	}
	return token
}

// dropGoogleAccess runs when a Google connection is removed, before its secrets go. When it was the
// last one holding a token, Google is asked to end the access. Google ends every scope when one token
// is revoked, so the grant is left alone while any other holds one. An unreadable token counts as held.
func (s *Service) dropGoogleAccess(ctx context.Context, id string) {
	token := s.grantToken(ctx, id)
	s.forgetGoogleReads()
	if token == nil {
		return
	}
	for _, other := range googleConnections() {
		if other.ID == id {
			continue
		}
		if held, err := s.storedGrant(ctx, other.ID); err != nil || held != nil {
			return
		}
	}
	s.revokeAtGoogle(ctx, token)
}

// revokeAtGoogle asks Google to end an access. It is best effort: a Google that cannot be reached, or
// that already ended the access, never stops a Disconnect, and the person can still remove Marshal at
// myaccount.google.com/permissions.
func (s *Service) revokeAtGoogle(ctx context.Context, token *oauth2.Token) {
	value := token.RefreshToken
	if value == "" {
		value = token.AccessToken
	}
	if value == "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, revokeTimeout)
	defer cancel()
	body := strings.NewReader(url.Values{"token": {value}}.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.google.revokeURL, body)
	if err != nil {
		s.log.Warn("the request to end Google's access could not be made", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.log.Warn("Google could not be asked to end the access; it stays until removed at myaccount.google.com/permissions", "err", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		s.log.Info("Google did not end the access", "status", resp.StatusCode)
	}
}
