// Package googlefiles is Marshal's thin REST client for Google Drive, Docs, Sheets and Slides: the
// folder, the files Marshal makes, and reading a doc, sheet or deck into markdown. It calls the
// REST APIs directly, because the generated packages would add much to the binary for few calls.
package googlefiles

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// The scopes Marshal asks for. drive.file lets Marshal make files and see the ones it made. The
// read-only scopes let it read any doc, sheet or deck a person points it to. None of them can edit
// or delete a file the person already had.
const (
	ScopeDriveFile      = "https://www.googleapis.com/auth/drive.file"
	ScopeDocsReadonly   = "https://www.googleapis.com/auth/documents.readonly"
	ScopeSheetsReadonly = "https://www.googleapis.com/auth/spreadsheets.readonly"
	ScopeSlidesReadonly = "https://www.googleapis.com/auth/presentations.readonly"
)

// API names one of the four Google APIs, each of which has its own host.
type API string

const (
	// APIDrive is the Drive API.
	APIDrive API = "drive"
	// APIDocs is the Docs API.
	APIDocs API = "docs"
	// APISheets is the Sheets API.
	APISheets API = "sheets"
	// APISlides is the Slides API.
	APISlides API = "slides"
)

// hostOf is where the real API lives. A test gives New one base URL for all four instead, and the
// paths tell them apart.
func hostOf(api API) string {
	switch api {
	case APIDocs:
		return "https://docs.googleapis.com"
	case APISheets:
		return "https://sheets.googleapis.com"
	case APISlides:
		return "https://slides.googleapis.com"
	}
	return "https://www.googleapis.com"
}

const (
	// maxAnswerBytes is the most of one answer that is read. A very long document is JSON of many
	// megabytes, and this stops a runaway answer from filling memory.
	maxAnswerBytes = 32 << 20
	// maxErrorText is how much of an answer that is not Google's own error shape is kept.
	maxErrorText = 200
	// maxTries is how many times one call is tried when Google says it is busy.
	maxTries = 3
	// firstRetryDelay is how long the first retry waits.
	firstRetryDelay = 300 * time.Millisecond
	// fieldsParam is the query parameter that picks which parts of an answer Google sends.
	fieldsParam = "fields"
)

// ErrUnreachable means Google could not be reached or did not finish answering: a network failure,
// and not an answer from Google.
var ErrUnreachable = errors.New("could not reach Google")

// ErrTooLarge means Google's answer was bigger than Marshal reads.
var ErrTooLarge = errors.New("the answer from Google is larger than Marshal reads")

// Client talks to the four APIs with one HTTP client, which is normally an OAuth2 client for a live
// token. It holds no state, so it is safe for concurrent use.
type Client struct {
	http *http.Client
	base string
	// retryDelay is how long the first retry waits; each one waits twice as long as the one before.
	retryDelay time.Duration
}

// New builds a Client. baseURL overrides where all four APIs are reached, for a test; empty means
// the real hosts.
func New(httpClient *http.Client, baseURL string) *Client {
	return &Client{http: httpClient, base: strings.TrimRight(baseURL, "/"), retryDelay: firstRetryDelay}
}

// endpoint is the address of one path on one API, with the query when there is one.
func (c *Client) endpoint(api API, path string, query url.Values) string {
	host := c.base
	if host == "" {
		host = hostOf(api)
	}
	target := host + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	return target
}

// APIError is Google's own refusal of a call, with the reason it gave.
type APIError struct {
	// API is the API that refused the call.
	API API
	// Status is the HTTP status.
	Status int
	// Reason is Google's short word for what went wrong, such as "accessNotConfigured". It is empty
	// when Google gave none.
	Reason string
	// Message is Google's own sentence.
	Message string

	reasons []string
}

func (e *APIError) Error() string {
	text := fmt.Sprintf("Google answered %d", e.Status)
	if e.Reason != "" {
		text += " (" + e.Reason + ")"
	}
	if e.Message != "" {
		text += ": " + e.Message
	}
	return text
}

// Unauthorized says Google does not accept the token.
func (e *APIError) Unauthorized() bool { return e.Status == http.StatusUnauthorized }

// NotFound says Google has no such file, or none this token may see.
func (e *APIError) NotFound() bool { return e.Status == http.StatusNotFound }

// NotEnabled says the API is turned off in the Google Cloud project Marshal signs in with.
func (e *APIError) NotEnabled() bool {
	if e.Status != http.StatusForbidden {
		return false
	}
	if e.has("accessNotConfigured") || e.has("SERVICE_DISABLED") {
		return true
	}
	text := strings.ToLower(e.Message)
	return strings.Contains(text, "has not been used") || strings.Contains(text, "is disabled")
}

// Temporary says trying again may work: Google is busy or broken for a moment.
func (e *APIError) Temporary() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= http.StatusInternalServerError ||
		e.Status == http.StatusForbidden && (e.has("rateLimitExceeded") || e.has("userRateLimitExceeded"))
}

func (e *APIError) has(reason string) bool { return slices.Contains(e.reasons, reason) }

// errorBody is Google's error shape. Drive puts the reason in errors, and Docs, Sheets and Slides
// put it in details, so both are read.
type errorBody struct {
	Error struct {
		Message string `json:"message"`
		Status  string `json:"status"`
		Errors  []struct {
			Reason string `json:"reason"`
		} `json:"errors"`
		Details []struct {
			Reason string `json:"reason"`
		} `json:"details"`
	} `json:"error"`
}

// answerError reads a refused call's body into an APIError. A body that is not Google's shape still
// makes one, with the start of the text as its message.
func answerError(api API, status int, data []byte) *APIError {
	apiErr := &APIError{API: api, Status: status}
	var body errorBody
	if json.Unmarshal(data, &body) != nil || body.Error.Message == "" && body.Error.Status == "" {
		apiErr.Message = shorten(strings.TrimSpace(string(data)))
		return apiErr
	}
	apiErr.Message = body.Error.Message
	for _, item := range body.Error.Errors {
		apiErr.reasons = append(apiErr.reasons, item.Reason)
	}
	for _, item := range body.Error.Details {
		if item.Reason != "" {
			apiErr.reasons = append(apiErr.reasons, item.Reason)
		}
	}
	apiErr.reasons = slices.DeleteFunc(apiErr.reasons, func(r string) bool { return r == "" })
	if body.Error.Status != "" {
		apiErr.reasons = append(apiErr.reasons, body.Error.Status)
	}
	apiErr.Reason = pickReason(apiErr.reasons)
	return apiErr
}

// pickReason chooses the reason worth showing: one that names a turned-off API or a busy Google
// first, and otherwise the first Google gave.
func pickReason(reasons []string) string {
	for _, want := range []string{"accessNotConfigured", "SERVICE_DISABLED", "rateLimitExceeded", "userRateLimitExceeded"} {
		if slices.Contains(reasons, want) {
			return want
		}
	}
	if len(reasons) > 0 {
		return reasons[0]
	}
	return ""
}

func shorten(text string) string {
	if len(text) <= maxErrorText {
		return text
	}
	return text[:maxErrorText]
}

// withRetry runs one Google call, and tries it again when Google says it is busy or broken for a
// moment (429, 5xx, and a quota 403), up to three times. Any other answer returns at once, and so
// does a network failure: waiting would not change what a person sees.
func (c *Client) withRetry(ctx context.Context, call func() error) error {
	delay := c.retryDelay
	var err error
	for try := 1; ; try++ {
		err = call()
		var apiErr *APIError
		if err == nil || try == maxTries || !errors.As(err, &apiErr) || !apiErr.Temporary() {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
}

// request is one call to Google. The body is a byte slice so every try sends a whole body, and not
// what the first try left of a reader.
type request struct {
	api         API
	method      string
	target      string
	contentType string
	body        []byte
}

// call sends one request and decodes a good answer into out, when out is not nil.
func (c *Client) call(ctx context.Context, send request, out any) error {
	return c.withRetry(ctx, func() error {
		var reader io.Reader
		if send.body != nil {
			reader = bytes.NewReader(send.body)
		}
		req, err := http.NewRequestWithContext(ctx, send.method, send.target, reader)
		if err != nil {
			return fmt.Errorf("build the request to Google: %w", err)
		}
		if send.contentType != "" {
			req.Header.Set("Content-Type", send.contentType)
		}
		req.Header.Set("Accept", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrUnreachable, err)
		}
		defer func() { _ = resp.Body.Close() }()
		return answer(send.api, resp, out)
	})
}

// answer reads Google's reply: a refusal becomes an APIError, and a good answer is decoded into out.
func answer(api API, resp *http.Response, out any) error {
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAnswerBytes+1))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
	if len(data) > maxAnswerBytes {
		return ErrTooLarge
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return answerError(api, resp.StatusCode, data)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("read Google's answer: %w", err)
	}
	return nil
}

// getJSON is a GET whose answer is JSON.
func (c *Client) getJSON(ctx context.Context, api API, path string, query url.Values, out any) error {
	return c.call(ctx, request{api: api, method: http.MethodGet, target: c.endpoint(api, path, query)}, out)
}

// postJSON is a POST with a JSON body.
func (c *Client) postJSON(ctx context.Context, api API, path string, query url.Values, in, out any) error {
	raw, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("write the request to Google: %w", err)
	}
	send := request{api: api, method: http.MethodPost, target: c.endpoint(api, path, query), contentType: "application/json; charset=UTF-8", body: raw}
	return c.call(ctx, send, out)
}
