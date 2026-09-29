package integrations

// This file owns the Trello connection: where its settings and its two secrets live, what saving one
// does, and how the typed client and the webhook secret are read back (B8.2, docs/architecture.md
// section 18). It is the Trello twin of github.go, and the same split applies: the typed API client
// itself lives in the subpackage internal/integrations/trello, and this file is what ties it to the
// store, the keychain, and a route.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/integrations/trello"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// TrelloID is the Trello connection's own id, and the id its `integrations` row, its keychain entry,
// and its saved test result are filed under.
const TrelloID = trello.ID

// KindTrello is the kind the Trello connection's row and its test are filed under.
const KindTrello = "trello"

// trelloConfig is the non-secret half of the connection, stored in the `integrations` row's
// config_json. The callback URL is here and not in the keychain because it is not a secret: it is a
// public address Trello itself was told about, and a screen shows it so a person can check it
// against the webhook they made.
type trelloConfig struct {
	// APIKey is the public half of the credential.
	APIKey string `json:"apiKey"`
	// ProjectID is the Marshal project the board is linked to. It is here rather than derived from
	// the board's name because the link is a decision a person makes, not a guess from a title.
	ProjectID string `json:"projectId"`
	// BoardID is the board Marshal watches.
	BoardID string `json:"boardId"`
	// NewCardListID is the list a new Trello card is imported from, or "" when Trello never creates
	// a Marshal card.
	NewCardListID string `json:"newCardListId,omitempty"`
	// CallbackURL is the address the webhook was registered with, exactly as Trello knows it. It is
	// half of what a delivery's signature is computed over.
	CallbackURL string `json:"callbackUrl"`
}

// trelloSecrets is the secret half, stored in the OS keychain as one JSON document under the
// connection's own id. The token is what stands for the person to the Trello API; the webhook secret
// is what makes a delivery believable. They are written together and never come back out.
type trelloSecrets struct {
	// Token is the Trello token the API key is used with.
	Token string `json:"token"`
	// WebhookSecret is what Trello signs a delivery with.
	WebhookSecret string `json:"webhookSecret"`
}

// SaveTrello stores the Trello connection, replacing whatever was there. It is the write half of
// B8.2; the owner creates the real webhook and copies the token (docs/backend-checklist.md section
// 3).
//
// The key and the token are required because nothing can be asked of Trello without them. The board
// is required because a connection with no board has nothing to sync. The webhook secret and the
// callback URL are optional, but they must arrive together: a secret with no URL cannot check a
// signature (Trello signs over the URL), and a URL with no secret cannot either, so a half-set pair
// would leave a webhook route that refuses every delivery with a message about a missing secret -
// the wrong thing to tell someone who thinks they configured it.
//
// The secret goes into the keychain before the settings go into the row, for the same reason as
// GitHub's: a secret with no settings is inert, while settings with no secret would be a connection
// that answers every delivery with 401.
func (s *Service) SaveTrello(ctx context.Context, req protocol.SaveTrelloRequest) error {
	if strings.TrimSpace(req.APIKey) == "" || strings.TrimSpace(req.Token) == "" {
		return protocol.InvalidArgument(
			"Marshal needs both a Trello API key and a token, so it can reach your boards.")
	}
	if strings.TrimSpace(req.BoardID) == "" {
		return protocol.InvalidArgument("Choose the Trello board Marshal should watch.")
	}
	// One project links to one board (docs/marshal-product-scope.md section 19.2): a board with no
	// project has nowhere to put an imported card, and a project is what the sync writes into.
	if !protocol.ValidProjectID(req.ProjectID) {
		return protocol.InvalidArgument("Choose the Marshal project this Trello board is linked to.").
			With("projectId", req.ProjectID)
	}
	hasSecret := strings.TrimSpace(req.WebhookSecret) != ""
	hasCallback := strings.TrimSpace(req.CallbackURL) != ""
	if hasSecret != hasCallback {
		return protocol.InvalidArgument(
			"Enter both the webhook secret and the callback URL, or neither: Marshal needs both to check a delivery.")
	}
	secrets, err := json.Marshal(trelloSecrets{Token: req.Token, WebhookSecret: req.WebhookSecret})
	if err != nil {
		return fmt.Errorf("write the Trello connection's secrets: %w", err)
	}
	if err := s.keys.Set(TrelloID, string(secrets)); err != nil {
		return fmt.Errorf("save the Trello connection's secrets in the keychain: %w", err)
	}
	config, err := json.Marshal(trelloConfig{
		APIKey: req.APIKey, ProjectID: req.ProjectID, BoardID: req.BoardID,
		NewCardListID: req.NewCardListID, CallbackURL: req.CallbackURL,
	})
	if err != nil {
		return fmt.Errorf("write the Trello connection's settings: %w", err)
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		return q.UpsertIntegration(ctx, db.UpsertIntegrationParams{
			ID:          TrelloID,
			Kind:        KindTrello,
			ConfigJSON:  string(config),
			KeychainRef: TrelloID,
		})
	})
	if err != nil {
		return fmt.Errorf("save the Trello connection's settings: %w", err)
	}
	s.forgetTrello()
	return nil
}

// TrelloClient answers the typed Trello client and the board Marshal watches, built from the stored
// connection. It answers ErrNotConnected when nothing is stored, which is the ordinary state of a
// daemon nobody has connected Trello to.
//
// Nothing is cached: a Trello client holds no token of its own, unlike the GitHub App's installation
// token, so building one per call costs nothing and a saved token takes effect at once.
func (s *Service) TrelloClient(ctx context.Context) (*trello.Client, string, error) {
	config, secrets, err := s.readTrello(ctx)
	if err != nil {
		return nil, "", err
	}
	if secrets.Token == "" || config.APIKey == "" {
		return nil, "", ErrNotConnected
	}
	opts := make([]trello.ClientOption, 0, 1)
	if s.trelloBase != "" {
		opts = append(opts, trello.WithBaseURL(s.trelloBase))
	}
	return trello.NewClient(config.APIKey, secrets.Token, opts...), config.BoardID, nil
}

// TrelloWebhook is what the trello webhook route needs to believe a delivery: the secret it is signed
// with, and the callback URL it was signed for. An empty secret is the honest answer when no
// connection, or no webhook secret, is stored, and the route refuses such a delivery rather than
// believing it.
func (s *Service) TrelloWebhook(ctx context.Context) (secret, callbackURL string, err error) {
	config, secrets, err := s.readTrello(ctx)
	if err != nil {
		return "", "", err
	}
	return secrets.WebhookSecret, config.CallbackURL, nil
}

// TrelloLink is the project a board is linked to: the Marshal project, the board itself, and the
// list a new Trello card is imported from. It answers ok=false when no connection is stored, or when
// the stored one has no project or board to sync, which is the ordinary state of a daemon nobody has
// linked a board to.
func (s *Service) TrelloLink(ctx context.Context) (link TrelloLinkInfo, ok bool, err error) {
	config, _, err := s.readTrello(ctx)
	if err != nil {
		return TrelloLinkInfo{}, false, err
	}
	if config.ProjectID == "" || config.BoardID == "" {
		return TrelloLinkInfo{}, false, nil
	}
	return TrelloLinkInfo{
		ProjectID: config.ProjectID, BoardID: config.BoardID, NewCardListID: config.NewCardListID,
	}, true, nil
}

// TrelloLinkInfo is one linked board: which Marshal project it writes into, which board it is, and
// which list a new card is imported from. It is the whole of what the sync needs to know about the
// connection, so the sync never reads the connection itself.
//
// NewCardListID may be empty, which means cards created in Trello are not imported: a link with no
// list is a read-only connection, and the sync says so rather than guessing at a list.
type TrelloLinkInfo struct {
	ProjectID     string
	BoardID       string
	NewCardListID string
}

// testTrello is the real Trello connection test: the stored credential, and the board it can reach.
// It asks the subpackage's own test, which is the same code a person's "Test connection" presses.
func (s *Service) testTrello(ctx context.Context, info Info) (protocol.TestResult, error) {
	if err := ctx.Err(); err != nil {
		return protocol.TestResult{}, err
	}
	client, boardID, err := s.TrelloClient(ctx)
	if errors.Is(err, ErrNotConnected) {
		return protocol.NewTestResult(info.ID, []protocol.TestCheck{{
			Name:    CheckSummary,
			State:   protocol.CheckStateFailed,
			Message: "Marshal has no Trello connection saved, so it cannot use Trello.",
			Fix:     "Add a Trello API key, token, and board in Settings, under Integrations.",
		}}, s.now()), nil
	}
	if err != nil {
		return protocol.TestResult{}, err
	}
	return trello.TestConnection(ctx, client, boardID, s.now()), nil
}

// readTrello reads the stored config and secrets. A missing row or a missing keychain entry is not an
// error: both mean "nothing is stored", which is what a connection nobody has set up looks like.
func (s *Service) readTrello(ctx context.Context) (trelloConfig, trelloSecrets, error) {
	var config trelloConfig
	var found bool
	err := s.store.Read(ctx, func(q *db.Queries) error {
		row, err := q.GetIntegration(ctx, TrelloID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("read the Trello connection's settings: %w", err)
		}
		found = true
		if row.ConfigJSON == "" {
			return nil
		}
		return json.Unmarshal([]byte(row.ConfigJSON), &config)
	})
	if err != nil {
		return trelloConfig{}, trelloSecrets{}, err
	}
	if !found {
		return trelloConfig{}, trelloSecrets{}, nil
	}
	raw, err := s.keys.Get(TrelloID)
	if err != nil && !errors.Is(err, security.ErrNoKey) {
		return trelloConfig{}, trelloSecrets{}, fmt.Errorf(
			"read the Trello connection's secrets from the keychain: %w", err)
	}
	var secrets trelloSecrets
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &secrets); err != nil {
			return trelloConfig{}, trelloSecrets{}, fmt.Errorf("read the Trello connection's secrets: %w", err)
		}
	}
	return config, secrets, nil
}

// forgetTrello drops nothing cached, because nothing about this connection is cached (TrelloClient).
// It exists so SaveTrello and Remove read the same as GitHub's, and so a future cache has one place
// to be dropped.
func (s *Service) forgetTrello() {}
