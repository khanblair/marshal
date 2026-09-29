package chatbot

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

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// DefaultNtfyServer is ntfy's own public server. A person who runs their own gives its address.
const DefaultNtfyServer = "https://ntfy.sh"

// ntfyTimeout bounds one publish, so a server that never answers cannot hold a notice forever.
const ntfyTimeout = 10 * time.Second

// Ntfy publishes notices to a topic on an ntfy server (B9.3, docs/mobile.md section 4). It only
// sends: ntfy has nothing for a person to answer from, so a notice that wants an answer is still
// sent to it, and the answer is given in the app the notice's link opens.
type Ntfy struct {
	server string
	topic  string
	token  string
	client *http.Client
	now    func() time.Time
}

// NtfyConfig says how to reach one ntfy topic.
type NtfyConfig struct {
	// Server is the ntfy server's address. Empty uses DefaultNtfyServer.
	Server string
	// Topic is where notices go. On a public server it is the only thing keeping a stranger from
	// reading them, so it should be long and unguessable.
	Topic string
	// Token is an access token for a server that needs one. Empty is fine for an open topic.
	Token string
	// HTTPClient sends the requests. Nil uses a client with a short time limit. A test hands one in
	// that points at an in-process fake server (hard rule 3).
	HTTPClient *http.Client
	// Now is the clock the test result is stamped with. Nil uses time.Now.
	Now func() time.Time
}

// NewNtfy makes a publisher. Nothing is sent: the topic is checked here so a connection saved with
// none is refused where a person can see it, and the server is asked only by the connection test.
func NewNtfy(cfg NtfyConfig) (*Ntfy, error) {
	topic := strings.TrimSpace(cfg.Topic)
	if topic == "" {
		return nil, errors.New("ntfy needs a topic")
	}
	server := strings.TrimRight(strings.TrimSpace(cfg.Server), "/")
	if server == "" {
		server = DefaultNtfyServer
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: ntfyTimeout}
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Ntfy{server: server, topic: topic, token: strings.TrimSpace(cfg.Token), client: client, now: now}, nil
}

// Kind is which service this bot talks to.
func (n *Ntfy) Kind() Kind { return KindNtfy }

// ntfyMessage is one publish, in the JSON form ntfy takes at its root, which carries any text as it
// is where a header would need encoding.
type ntfyMessage struct {
	Topic   string `json:"topic"`
	Title   string `json:"title,omitempty"`
	Message string `json:"message"`
	Click   string `json:"click,omitempty"`
}

// Notify publishes one notice. Its link is the address tapping the notice opens.
func (n *Ntfy) Notify(ctx context.Context, notice Notice) error {
	message := notice.Body
	if message == "" {
		message = notice.Title
	}
	return n.publish(ctx, ntfyMessage{Topic: n.topic, Title: notice.Title, Message: message, Click: notice.URL})
}

func (n *Ntfy) publish(ctx context.Context, message ntfyMessage) error {
	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("write an ntfy notice: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, n.server+"/", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build an ntfy request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if n.token != "" {
		request.Header.Set("Authorization", "Bearer "+n.token)
	}
	response, err := n.client.Do(request)
	if err != nil {
		return fmt.Errorf("publish an ntfy notice: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode/100 != 2 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<10))
		return &ntfyStatusError{status: response.StatusCode}
	}
	return nil
}

// ntfyStatusError is a publish the server answered with something other than success.
type ntfyStatusError struct{ status int }

func (e *ntfyStatusError) Error() string { return fmt.Sprintf("ntfy answered %d", e.status) }

// Start does nothing and returns at once: ntfy has no messages to read.
func (n *Ntfy) Start(context.Context, Handler) error { return nil }

// Accepts is false: ntfy only sends, so nothing that arrives from it is ever acted on.
func (n *Ntfy) Accepts(Incoming) bool { return false }

// Close is a no-op: the publisher holds an HTTP client and nothing that needs releasing.
func (n *Ntfy) Close() error { return nil }

// Test publishes a short, obvious message and says what came of it. One publish proves both the
// server and the topic, because ntfy answers success only once the message is accepted.
func (n *Ntfy) Test(ctx context.Context) (protocol.TestResult, error) {
	check := protocol.TestCheck{Name: CheckTopic}
	err := n.publish(ctx, ntfyMessage{Topic: n.topic, Title: "Marshal", Message: "Marshal is connected to this topic."})
	if err != nil {
		check.State = protocol.CheckStateFailed
		check.Message, check.Fix = ntfyFailure(err)
	} else {
		check.State = protocol.CheckStatePassed
		check.Message = "Marshal sent a test message to the topic."
	}
	checks := []protocol.TestCheck{check}
	checks = append([]protocol.TestCheck{summaryCheck(checks,
		"ntfy is set up and Marshal can send notices to it.",
		"ntfy works, with something to check.")}, checks...)
	return protocol.NewTestResult(string(KindNtfy), checks, n.now()), nil
}

// ntfyFailure turns a failed publish into the sentence and the fix a person reads.
func ntfyFailure(err error) (message, fix string) {
	var status *ntfyStatusError
	switch {
	case errors.As(err, &status) && (status.status == http.StatusUnauthorized || status.status == http.StatusForbidden):
		return "ntfy refused the access token.",
			"Check the token from your ntfy account, or leave it empty for an open topic."
	case errors.As(err, &status) && status.status == http.StatusTooManyRequests:
		return "ntfy is asking Marshal to slow down.", "Wait a moment, then test again."
	case status != nil:
		return "ntfy did not accept the message.", "Check the server address and the topic, then test again."
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "ntfy did not answer in time.", checkConnectionFix
	default:
		return "Marshal could not reach the ntfy server.", checkConnectionFix
	}
}
