// Package trello is the typed Trello API client and the two things that need Trello's own rules:
// checking a webhook delivery's signature, and the connection test the Integrations screen presses
// (docs/architecture.md section 18, docs/backend-checklist.md B8.2). There is no maintained official
// Go client (docs/library-docs.md), so the handful of calls Marshal needs are written here, on top of
// net/http, and tested against a fake server.
//
// It deliberately imports nothing from the parent internal/integrations package: it is the client
// and the rules, and how a connection's settings and secrets are stored is the parent's business.
package trello

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const (
	// ID is this connection's own id, and the id its `integrations` row, its keychain entry, and
	// its saved test result are filed under (internal/integrations.known).
	ID = "trello"
	// SignatureHeader is the header Trello signs a delivery with.
	SignatureHeader = "X-Trello-Webhook"
	// DefaultBaseURL is where the Trello API is reached when nothing else is configured.
	DefaultBaseURL = "https://api.trello.com/1"
)

// The action types Marshal acts on. Trello names every change in `action.type`, and only a couple of
// them mean anything to the sync so far; every other type is accepted and ignored, because a webhook
// Marshal did not ask for is not an error.
const (
	// ActionCreateCard is a card added to a list. It is what imports a new Marshal card.
	ActionCreateCard = "createCard"
	// ActionUpdateCard is a card changed: renamed, moved between lists, or closed.
	ActionUpdateCard = "updateCard"
)

// Config holds non-secret Trello integration config.
type Config struct {
	APIKey  string `json:"apiKey"`
	BoardID string `json:"boardId"`
}

// Client is a typed Trello API client.
type Client struct {
	baseURL    string
	apiKey     string
	token      string
	httpClient *http.Client
}

// NewClient creates a new Trello API client.
func NewClient(apiKey, token string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL:    DefaultBaseURL,
		apiKey:     apiKey,
		token:      token,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ClientOption tunes a Client at construction. Nothing but a test sets one today: the daemon uses
// Trello's real address and its own short-timeout HTTP client.
type ClientOption func(*Client)

// WithBaseURL points the client at another address, which is what lets a test run against a fake
// server. A trailing slash is dropped so the paths join cleanly.
func WithBaseURL(u string) ClientOption {
	return func(c *Client) {
		c.baseURL = strings.TrimSuffix(u, "/")
	}
}

// WithHTTPClient replaces the HTTP client every call is made through.
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// VerifySignature checks a Trello webhook delivery's X-Trello-Webhook header.
// Trello computes HMAC-SHA1 over (rawBody + callbackURL) with the secret key, base64 encoded.
func VerifySignature(secret string, callbackURL string, body []byte, signature string) bool {
	if secret == "" || signature == "" {
		return false
	}
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(body)
	mac.Write([]byte(callbackURL))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// Board represents a Trello board.
type Board struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Card represents a Trello card.
type Card struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Desc   string  `json:"desc"`
	IDList string  `json:"idList"`
	Closed bool    `json:"closed"`
	URL    string  `json:"url"`
	Labels []Label `json:"labels"`
}

// Label represents a Trello label.
type Label struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// List is one column on a board.
type List struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GetBoard fetches a Trello board by ID.
func (c *Client) GetBoard(ctx context.Context, boardID string) (Board, error) {
	u := fmt.Sprintf("%s/boards/%s?key=%s&token=%s", c.baseURL, url.PathEscape(boardID), url.QueryEscape(c.apiKey), url.QueryEscape(c.token))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Board{}, err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return Board{}, err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return Board{}, fmt.Errorf("trello API error %d: %s", res.StatusCode, string(body))
	}

	var board Board
	if err := json.NewDecoder(res.Body).Decode(&board); err != nil {
		return Board{}, err
	}
	return board, nil
}

// boardGet reads one of a board's sub-resources (its cards, its lists) and decodes the JSON array
// it answers into out. ListCards and ListLists are this call with a different resource and a
// different array type.
func (c *Client) boardGet(ctx context.Context, boardID, resource string, out any) error {
	u := fmt.Sprintf("%s/boards/%s/%s?key=%s&token=%s", c.baseURL, url.PathEscape(boardID), resource, url.QueryEscape(c.apiKey), url.QueryEscape(c.token))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("trello API error %d: %s", res.StatusCode, string(body))
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// ListCards fetches cards on a board.
func (c *Client) ListCards(ctx context.Context, boardID string) ([]Card, error) {
	var cards []Card
	if err := c.boardGet(ctx, boardID, "cards", &cards); err != nil {
		return nil, err
	}
	return cards, nil
}

// ListLists fetches a board's own lists (columns), open ones only.
func (c *Client) ListLists(ctx context.Context, boardID string) ([]List, error) {
	var lists []List
	if err := c.boardGet(ctx, boardID, "lists", &lists); err != nil {
		return nil, err
	}
	return lists, nil
}

// MoveCard moves a card to another list.
func (c *Client) MoveCard(ctx context.Context, cardID, listID string) error {
	v := url.Values{}
	v.Set("key", c.apiKey)
	v.Set("token", c.token)
	v.Set("idList", listID)

	u := fmt.Sprintf("%s/cards/%s?%s", c.baseURL, url.PathEscape(cardID), v.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, nil)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("trello API error %d: %s", res.StatusCode, string(body))
	}
	return nil
}

// CreateCard creates a new card on a list.
func (c *Client) CreateCard(ctx context.Context, listID, name, desc string) (Card, error) {
	v := url.Values{}
	v.Set("key", c.apiKey)
	v.Set("token", c.token)
	v.Set("idList", listID)
	v.Set("name", name)
	v.Set("desc", desc)

	u := fmt.Sprintf("%s/cards", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(v.Encode()))
	if err != nil {
		return Card{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return Card{}, err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return Card{}, fmt.Errorf("trello API error %d: %s", res.StatusCode, string(body))
	}

	var card Card
	if err := json.NewDecoder(res.Body).Decode(&card); err != nil {
		return Card{}, err
	}
	return card, nil
}

// The names of the checks this connection's test reports (docs/architecture.md section 18). They are
// the row labels a screen shows, so they are words rather than ids, and - like every connection's -
// the summary is named "Summary" so a row reads back what the test found without the list route
// knowing what this test asks (internal/integrations.summaryOf).
const (
	// CheckSummary is the one sentence a connection row shows.
	CheckSummary = "Summary"
	// CheckCredentials is the check that both halves of the API credential are there.
	CheckCredentials = "API key & token"
	// CheckBoardRead is the check that the configured board can be read.
	CheckBoardRead = "Board"
)

// TestConnection runs the Trello connection test (docs/architecture.md section 18, B8.2): are both
// halves of the credential there, and is the configured board readable. Nothing it does writes to
// Trello, so a person can run it as often as the cooldown allows without changing anything.
//
// The checks are collected in the order a screen shows them, with the summary first, and OK is
// decided by protocol.NewTestResult from the checks themselves - a warning does not make a result
// not OK. Nothing here is an error: a refused credential and an unreadable board are answers, not
// failures of the test, so the one error the caller can get is its own context ending.
func TestConnection(ctx context.Context, client *Client, boardID string, now time.Time) protocol.TestResult {
	checks := []protocol.TestCheck{credentialsCheck(client)}
	if checks[0].State == protocol.CheckStatePassed {
		checks = append(checks, boardCheck(ctx, client, boardID))
	}
	checks = append([]protocol.TestCheck{summaryCheck(checks,
		"Marshal can use this Trello board.",
		"Marshal can use Trello, with something to check.")}, checks...)
	return protocol.NewTestResult(ID, checks, now)
}

// credentialsCheck is whether both halves of the credential Marshal needs are there. A missing half
// is a failure: nothing can be asked of Trello without it, and the fix names which field to fill in.
func credentialsCheck(client *Client) protocol.TestCheck {
	check := protocol.TestCheck{Name: CheckCredentials}
	switch {
	case client == nil || client.apiKey == "":
		check.State = protocol.CheckStateFailed
		check.Message = "Marshal has no Trello API key saved."
		check.Fix = "Add the Trello API key and token in Settings, under Integrations."
	case client.token == "":
		check.State = protocol.CheckStateFailed
		check.Message = "Marshal has no Trello token saved."
		check.Fix = "Add the Trello API key and token in Settings, under Integrations."
	default:
		check.State = protocol.CheckStatePassed
		check.Message = "Marshal has a Trello API key and token."
	}
	return check
}

// boardCheck is whether the configured board can be read. It is only asked when the credential is
// there, because a board read with a missing credential would report the wrong problem.
func boardCheck(ctx context.Context, client *Client, boardID string) protocol.TestCheck {
	check := protocol.TestCheck{Name: CheckBoardRead}
	if strings.TrimSpace(boardID) == "" {
		check.State = protocol.CheckStateFailed
		check.Message = "Marshal is set up with no Trello board."
		check.Fix = "Choose the board Marshal should watch, in Settings under Integrations."
		return check
	}
	board, err := client.GetBoard(ctx, boardID)
	if err != nil {
		check.State = protocol.CheckStateFailed
		check.Message = fmt.Sprintf("Marshal could not read the Trello board %s.", boardID)
		check.Fix = "Check the board's id, and that the token may see this board, then test again."
		return check
	}
	check.State = protocol.CheckStatePassed
	check.Message = fmt.Sprintf("Marshal can read %q on Trello.", board.Name)
	return check
}

// summaryCheck is the one sentence the connection row shows, built from the checks themselves: the
// first failure if there is one, and otherwise the passing sentence. It mirrors the parent package's
// own summary (internal/integrations.test.go) without importing it, because a subpackage cannot
// import the package that builds it.
func summaryCheck(checks []protocol.TestCheck, works, partly string) protocol.TestCheck {
	for _, check := range checks {
		if check.State == protocol.CheckStateFailed {
			return protocol.TestCheck{
				Name:    CheckSummary,
				State:   protocol.CheckStateFailed,
				Message: check.Message,
				Fix:     check.Fix,
			}
		}
	}
	warned := 0
	for _, check := range checks {
		if check.State == protocol.CheckStateWarning {
			warned++
		}
	}
	summary := protocol.TestCheck{Name: CheckSummary, State: protocol.CheckStatePassed, Message: works}
	if warned > 0 {
		summary.State = protocol.CheckStateWarning
		summary.Message = partly
	}
	return summary
}

// WebhookEvent represents a received Trello webhook payload.
type WebhookEvent struct {
	Action struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Date string `json:"date"`
		Data struct {
			Card  Card  `json:"card"`
			Board Board `json:"board"`
			// ListAfter is set only when this updateCard action is a move between lists -
			// present beside ListBefore, absent for any other card edit.
			ListAfter *List `json:"listAfter,omitempty"`
		} `json:"data"`
	} `json:"action"`
	Model struct {
		ID string `json:"id"`
	} `json:"model"`
}

// Moved reports whether this delivery is a card moved into another list, and which one.
func (e WebhookEvent) Moved() (list List, ok bool) {
	if e.Action.Type != "updateCard" || e.Action.Data.ListAfter == nil {
		return List{}, false
	}
	return *e.Action.Data.ListAfter, true
}
