package integrations

// This file owns the GitHub App connection: where its settings and its two secrets live, what saving
// one does, and how the App client and the webhook secret are read back. It is the only file that
// knows the shape of the GitHub App's stored config and its keychain entry.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	githubapp "github.com/khanblair/marshal/daemon/internal/integrations/github"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// ErrNotConnected is what reading the App answers when no GitHub App is set up. It is the expected
// answer for a daemon nobody has connected GitHub to, so a caller asks with errors.Is rather than
// treating it as a failure.
var ErrNotConnected = errors.New("no GitHub App is connected")

// githubConfig is the non-secret half of the connection, stored in the `integrations` row's
// config_json. Both ids are numbers GitHub itself uses and a screen may show; nothing secret is
// here, because config_json is a plain column in an unencrypted database.
type githubConfig struct {
	// AppID is the App's own numeric id.
	AppID int64 `json:"appId"`
	// InstallationID is the numeric id of the installation Marshal acts as.
	InstallationID int64 `json:"installationId"`
	// Mode and Login describe a user connection: "oauth" or "token", and who it is signed in as.
	Mode  string `json:"mode,omitempty"`
	Login string `json:"login,omitempty"`
}

// githubSecrets is the secret half, stored in the OS keychain as one JSON document under the
// connection's own id. The keychain keeps one secret per id, and the App needs both of these to work
// at all - a key to sign its requests and a webhook secret to believe its deliveries - so they are
// kept together and written together.
type githubSecrets struct {
	// PrivateKey is the App's private key, in PEM form, exactly as GitHub generated it.
	PrivateKey string `json:"privateKey"`
	// WebhookSecret is what the App's deliveries are signed with.
	WebhookSecret string `json:"webhookSecret"`
	// The fields below hold a user connection: a sign-in ("oauth") or a pasted token ("token").
	// Times are Unix milliseconds, and zero means the token does not expire.
	Mode             string `json:"mode,omitempty"`
	Token            string `json:"token,omitempty"`
	RefreshToken     string `json:"refreshToken,omitempty"`
	ExpiresAt        int64  `json:"expiresAt,omitempty"`
	RefreshExpiresAt int64  `json:"refreshExpiresAt,omitempty"`
}

// SaveGitHub stores the App's connection, replacing whatever was there - which is what setting it up
// again, or rotating its key, does. It is the write half of B6.1; the owner creates the real App
// (docs/backend-checklist.md section 3).
//
// The App is built before anything is written, so a key GitHub could never accept is refused with a
// sentence naming the field rather than saved and failing at the first call. The secret goes into
// the keychain before the settings go into the row, because a secret with no settings is inert while
// settings with no secret would be a connection that answers every delivery with 401.
func (s *Service) SaveGitHub(ctx context.Context, req protocol.SaveGitHubRequest) error {
	cfg := githubapp.AppConfig{
		AppID:          req.AppID,
		InstallationID: req.InstallationID,
		PrivateKey:     []byte(req.PrivateKey),
	}
	if err := cfg.Validate(); err != nil {
		return protocol.InvalidArgument(fmt.Sprintf("This GitHub App is not complete: %s.", err))
	}
	if strings.TrimSpace(req.WebhookSecret) == "" {
		return protocol.InvalidArgument(
			"Enter the webhook secret from the GitHub App's settings, so Marshal can check its deliveries.")
	}
	// Building the App is what proves the private key is a key GitHub could accept: it parses the
	// PEM and makes the signing transport. Nothing is sent anywhere.
	if _, err := githubapp.NewApp(cfg); err != nil {
		return protocol.InvalidArgument(fmt.Sprintf(
			"Marshal could not use that private key: %s.", err))
	}
	secrets, err := json.Marshal(githubSecrets{
		PrivateKey:    req.PrivateKey,
		WebhookSecret: req.WebhookSecret,
	})
	if err != nil {
		return fmt.Errorf("write the GitHub App's secrets: %w", err)
	}
	if err := s.keys.Set(GitHubID, string(secrets)); err != nil {
		return fmt.Errorf("save the GitHub App's secrets in the keychain: %w", err)
	}
	config, err := json.Marshal(githubConfig{AppID: req.AppID, InstallationID: req.InstallationID})
	if err != nil {
		return fmt.Errorf("write the GitHub App's settings: %w", err)
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		return q.UpsertIntegration(ctx, db.UpsertIntegrationParams{
			ID:          GitHubID,
			Kind:        KindGitHub,
			ConfigJSON:  string(config),
			KeychainRef: GitHubID,
		})
	})
	if err != nil {
		return fmt.Errorf("save the GitHub App's settings: %w", err)
	}
	s.forgetGitHub()
	s.forgetGitHubUser()
	return nil
}

// Remove forgets a connection entirely: its settings and its secret. Removing a connection that has
// none is not an error - the answer is the same list either way, which is how removing a provider
// key that is not there behaves. An id Marshal has no connection for is not found.
func (s *Service) Remove(ctx context.Context, id string) error {
	info, ok := Lookup(id)
	if !ok {
		return protocol.NotFound("connection").With("id", id)
	}
	if info.Wired {
		// Every wired connection that keeps a secret files it under its own id, so removing one is
		// the same call for all of them. A connection with no secret (the Obsidian vault, which is a
		// folder) answers ErrNoKey, which is not an error: there was nothing to remove.
		if err := s.keys.Remove(id); err != nil && !errors.Is(err, security.ErrNoKey) {
			return fmt.Errorf("remove the %q connection's secrets: %w", id, err)
		}
	}
	err := s.store.Write(ctx, func(q *db.Queries) error {
		return q.DeleteIntegration(ctx, id)
	})
	if err != nil {
		return fmt.Errorf("remove the %q connection: %w", id, err)
	}
	s.forgetGitHub()
	s.forgetTrello()
	if id == GitHubID {
		s.forgetGitHubUser()
	}
	return nil
}

// App answers the GitHub App client for the stored connection, building it the first time it is
// asked for and again after the connection changes. It answers ErrNotConnected when no App is set
// up, which is the ordinary state of a daemon nobody has connected GitHub to.
//
// The client is cached because ghinstallation holds a short-lived installation token and refreshes
// it when it is nearly expired: building one per call would throw that away and mint a token for
// every request.
func (s *Service) App(ctx context.Context) (*githubapp.App, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.built {
		if s.app == nil {
			return nil, ErrNotConnected
		}
		return s.app, nil
	}
	config, secrets, err := s.readGitHubLocked(ctx)
	if err != nil {
		return nil, err
	}
	if secrets.PrivateKey == "" || config.AppID <= 0 || config.InstallationID <= 0 {
		s.built, s.app = true, nil
		return nil, ErrNotConnected
	}
	app, err := githubapp.NewApp(githubapp.AppConfig{
		AppID:          config.AppID,
		InstallationID: config.InstallationID,
		PrivateKey:     []byte(secrets.PrivateKey),
	})
	if err != nil {
		return nil, fmt.Errorf("build the GitHub App: %w", err)
	}
	s.built, s.app = true, app
	return app, nil
}

// webhookSecret answers the secret GitHub's deliveries are signed with, read from the keychain. It
// is the source the verifier reads on every delivery, so a secret saved through Settings takes
// effect without a restart and a removed one stops being trusted at once. An empty string is the
// honest answer when nothing is saved, and the verifier refuses such a delivery rather than
// believing it.
func (s *Service) webhookSecret(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.secretsLoaded {
		return s.secretsVal.WebhookSecret, nil
	}
	_, secrets, err := s.readGitHubLocked(ctx)
	if err != nil {
		return "", err
	}
	return secrets.WebhookSecret, nil
}

// readGitHubLocked reads the stored config and secrets and fills the cache. The caller holds the
// mutex. A missing row or a missing keychain entry is not an error: both mean "nothing is stored",
// which is what a connection nobody has set up looks like.
func (s *Service) readGitHubLocked(ctx context.Context) (githubConfig, githubSecrets, error) {
	var config githubConfig
	var found bool
	err := s.store.Read(ctx, func(q *db.Queries) error {
		row, err := q.GetIntegration(ctx, GitHubID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("read the GitHub App's settings: %w", err)
		}
		found = true
		if row.ConfigJSON == "" {
			return nil
		}
		return json.Unmarshal([]byte(row.ConfigJSON), &config)
	})
	if err != nil {
		return githubConfig{}, githubSecrets{}, err
	}
	if !found {
		s.secretsLoaded, s.secretsVal = true, githubSecrets{}
		return githubConfig{}, githubSecrets{}, nil
	}
	raw, err := s.keys.Get(GitHubID)
	if err != nil && !errors.Is(err, security.ErrNoKey) {
		return githubConfig{}, githubSecrets{}, fmt.Errorf(
			"read the GitHub App's secrets from the keychain: %w", err)
	}
	secrets, err := parseGitHubSecrets(raw)
	if err != nil {
		return githubConfig{}, githubSecrets{}, err
	}
	s.secretsLoaded, s.secretsVal = true, secrets
	return config, secrets, nil
}

// forgetGitHub drops the cached App and secrets, so the next read rebuilds from what is stored now.
// It is called after the connection changes; a daemon that has never connected GitHub reads nothing.
func (s *Service) forgetGitHub() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.built, s.app = false, nil
	s.secretsLoaded, s.secretsVal = false, githubSecrets{}
}

// parseGitHubSecrets reads the keychain entry. A value that is not JSON is a personal token saved
// with `marshal keys` before connections had a shape of their own.
func parseGitHubSecrets(raw string) (githubSecrets, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return githubSecrets{}, nil
	}
	if !strings.HasPrefix(raw, "{") {
		return githubSecrets{Mode: githubModeToken, Token: raw}, nil
	}
	var secrets githubSecrets
	if err := json.Unmarshal([]byte(raw), &secrets); err != nil {
		return githubSecrets{}, fmt.Errorf("read the GitHub connection's secrets: %w", err)
	}
	return secrets, nil
}
