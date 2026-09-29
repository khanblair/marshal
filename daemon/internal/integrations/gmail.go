package integrations

// This file owns the Gmail connection (B8.3): which label to watch and which project a labeled
// email becomes a card in. It shares Google Calendar's OAuth client and token (google.go) rather
// than asking for a second consent - one Google Cloud app, one grant, two rows on the screen.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/oauth2"

	"github.com/khanblair/marshal/daemon/internal/integrations/gmailread"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// GmailID is the Gmail connection's own id.
const GmailID = "gmail"

// KindGmail is the kind the Gmail connection's row and test are filed under.
const KindGmail = "gmail"

type gmailConfig struct {
	Label     string `json:"label"`
	ProjectID string `json:"projectId"`
}

// SaveGmail stores which label to watch and which project a labeled email becomes a card in. It
// has no secret of its own: Google Calendar's own client and token, once granted, cover Gmail too.
func (s *Service) SaveGmail(ctx context.Context, req protocol.SaveGmailRequest) error {
	if strings.TrimSpace(req.Label) == "" {
		return protocol.InvalidArgument("Choose the Gmail label Marshal should watch.")
	}
	if !protocol.ValidProjectID(req.ProjectID) {
		return protocol.InvalidArgument("Choose the Marshal project a labeled email becomes a card in.").
			With("projectId", req.ProjectID)
	}
	config, err := json.Marshal(gmailConfig{Label: req.Label, ProjectID: req.ProjectID})
	if err != nil {
		return fmt.Errorf("write the Gmail connection's settings: %w", err)
	}
	return s.store.Write(ctx, func(q *db.Queries) error {
		return q.UpsertIntegration(ctx, db.UpsertIntegrationParams{ID: GmailID, Kind: KindGmail, ConfigJSON: string(config)})
	})
}

// GmailClient answers a client built from Google Calendar's own token. ErrNotConnected means
// Google Calendar's own consent has not completed yet - the same answer either row gives.
func (s *Service) GmailClient(ctx context.Context) (*gmailread.Client, error) {
	fresh, err := s.freshGoogleToken(ctx)
	if err != nil {
		return nil, err
	}
	return gmailread.New(ctx, oauth2.NewClient(ctx, oauth2.StaticTokenSource(fresh)), s.gmailBase)
}

func (s *Service) readGmail(ctx context.Context) (gmailConfig, error) {
	var config gmailConfig
	err := s.store.Read(ctx, func(q *db.Queries) error {
		row, err := q.GetIntegration(ctx, GmailID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("read the Gmail connection's settings: %w", err)
		}
		if row.ConfigJSON == "" {
			return nil
		}
		return json.Unmarshal([]byte(row.ConfigJSON), &config)
	})
	return config, err
}

func (s *Service) testGmail(ctx context.Context, info Info) (protocol.TestResult, error) {
	config, err := s.readGmail(ctx)
	if err != nil {
		return protocol.TestResult{}, err
	}
	if config.Label == "" {
		return protocol.NewTestResult(info.ID, []protocol.TestCheck{{
			Name: CheckSummary, State: protocol.CheckStateFailed,
			Message: "Marshal has no Gmail label saved.",
			Fix:     "Choose a label and a project in Settings, under Integrations.",
		}}, s.now()), nil
	}
	client, err := s.GmailClient(ctx)
	if errors.Is(err, ErrNotConnected) || errors.Is(err, ErrNoGoogleClient) {
		return protocol.NewTestResult(info.ID, []protocol.TestCheck{{
			Name: CheckSummary, State: protocol.CheckStateFailed,
			Message: "Google Calendar is not connected yet, so Gmail has no token to read with.",
			Fix:     "Connect Google Calendar first; Gmail shares its access.",
		}}, s.now()), nil
	}
	if err != nil {
		return protocol.TestResult{}, err
	}
	total, err := client.About(ctx)
	if err != nil {
		return protocol.NewTestResult(info.ID, []protocol.TestCheck{{
			Name: CheckSummary, State: protocol.CheckStateFailed,
			Message: "Marshal's Google token did not work for Gmail: " + err.Error(),
			Fix:     "Reconnect Google Calendar in Settings; Gmail needs the gmail.readonly scope too.",
		}}, s.now()), nil
	}
	return protocol.NewTestResult(info.ID, []protocol.TestCheck{{
		Name: CheckSummary, State: protocol.CheckStatePassed,
		Message: fmt.Sprintf("Reading label %q (%d messages in the mailbox).", config.Label, total),
	}}, s.now()), nil
}
