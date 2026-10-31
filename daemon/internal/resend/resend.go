// Package resend sends Marshal's email through Resend (https://resend.com). It is the one place that
// talks to Resend's API. Nothing here reads the keychain or the settings: the key and the sender are
// given in, so the integrations package decides where they live.
//
// Only two calls are made. Check reads the account's domains, which sends nothing. Send posts one
// email. A test that wants to know whether the key works uses Check, so pressing Test never sends mail.
package resend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultBaseURL is Resend's API. A test points the client at an in-process fake server instead.
const DefaultBaseURL = "https://api.resend.com"

// timeout bounds one request, so a service that never answers cannot hold a brief forever.
const timeout = 15 * time.Second

// errorBytes is how much of a refusal's text is kept for the person to read.
const errorBytes = 512

// Client calls Resend with one API key and sends from one address.
type Client struct {
	baseURL string
	key     string
	from    string
	http    *http.Client
}

// Config says how to reach Resend.
type Config struct {
	// BaseURL is Resend's API address. Empty uses DefaultBaseURL.
	BaseURL string
	// Key is the API key. It is sent only in the Authorization header and never shown back.
	Key string
	// From is the sender address, such as "Marshal <marshal@example.com>".
	From string
	// HTTPClient sends the requests. Nil uses a client with a time limit.
	HTTPClient *http.Client
}

// New makes a client. It checks what it can without a network call: a key and a sender are both
// needed, because Resend refuses a send without either.
func New(cfg Config) (*Client, error) {
	key := strings.TrimSpace(cfg.Key)
	from := strings.TrimSpace(cfg.From)
	if key == "" {
		return nil, errors.New("Resend needs an API key")
	}
	if from == "" {
		return nil, errors.New("Resend needs a sender address")
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	return &Client{baseURL: base, key: key, from: from, http: client}, nil
}

// APIError is a refusal from Resend, with the reason it gave. The key is never in it.
type APIError struct {
	// Status is the HTTP status Resend answered with.
	Status int
	// Message is Resend's own sentence, cut to a readable length.
	Message string
}

// Error says what Resend refused.
func (e *APIError) Error() string {
	return fmt.Sprintf("Resend answered %d: %s", e.Status, e.Message)
}

// Refused reports whether err is a refusal from Resend with the given status.
func Refused(err error, status int) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == status
}

// Restricted reports whether err is Resend saying the key may only send mail. Such a key is valid,
// but it cannot read the account's domains, which is all Check does.
func Restricted(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized &&
		strings.Contains(strings.ToLower(apiErr.Message), "restricted")
}

// Domain is one sending domain on the account.
type Domain struct {
	Name   string
	Status string
}

// Check reads the account's sending domains. It sends nothing, so it is what a connection test uses.
// It answers the domains, and an error for a key Resend refuses.
func (c *Client) Check(ctx context.Context) ([]Domain, error) {
	var body struct {
		Data []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := c.call(ctx, http.MethodGet, "/domains", nil, &body); err != nil {
		return nil, err
	}
	out := make([]Domain, 0, len(body.Data))
	for _, d := range body.Data {
		out = append(out, Domain{Name: d.Name, Status: d.Status})
	}
	return out, nil
}

// Send posts one email from the client's sender to one recipient. It answers the id Resend gave it.
func (c *Client) Send(ctx context.Context, to, subject, text string) (string, error) {
	payload := map[string]any{"from": c.from, "to": []string{to}, "subject": subject, "text": text}
	var answer struct {
		ID string `json:"id"`
	}
	if err := c.call(ctx, http.MethodPost, "/emails", payload, &answer); err != nil {
		if errors.Is(err, errAnswerUnreadable) {
			return "", nil
		}
		return "", err
	}
	return answer.ID, nil
}

// FromAddress is the sender address the client sends from, so a caller can check its domain.
func (c *Client) FromAddress() string { return c.from }

// call makes one request. The key goes only in the Authorization header. An error text that
// would carry the key, or a bare network error, is cut down to what the person needs.
func (c *Client) call(ctx context.Context, method, path string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode the Resend request: %w", err)
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("build the Resend request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("reach Resend: %w", redactErr(err, c.key))
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return &APIError{Status: res.StatusCode, Message: refusalOf(res.Body, c.key)}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: %w", errAnswerUnreadable, err)
	}
	return nil
}

// errAnswerUnreadable marks an answer Resend gave with a success status that could not be read. For a
// send the mail has already gone, so it is not a failure to report.
var errAnswerUnreadable = errors.New("read Resend's answer")

// refusalOf is Resend's sentence from a refusal, with the key removed if it ever appears.
func refusalOf(body io.Reader, key string) string {
	raw, _ := io.ReadAll(io.LimitReader(body, errorBytes))
	var parsed struct {
		Message string `json:"message"`
	}
	text := strings.TrimSpace(string(raw))
	if json.Unmarshal(raw, &parsed) == nil && parsed.Message != "" {
		text = parsed.Message
	}
	text = strings.ReplaceAll(text, key, "<key>")
	if text == "" {
		return "no reason given"
	}
	return text
}

// redactErr returns a network error with the key taken out of its text.
func redactErr(err error, key string) error {
	if !strings.Contains(err.Error(), key) {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), key, "<key>"))
}
