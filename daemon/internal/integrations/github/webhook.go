// Package github is Marshal's GitHub App integration (docs/architecture.md section 8,
// docs/backend-checklist.md B6.1 and B6.7, build-plan 6.1-6.3). Phase 5 opened pull requests with a
// personal access token through the github.Client interface; this package is the second
// implementation behind that same interface - a real GitHub App - plus the other half of being an
// App, which is receiving its deliveries.
//
// It speaks no GitHub HTTP itself. The client it builds is internal/github's, whose transport
// ghinstallation fills with a fresh installation token; every request shape is still written in
// internal/github, in one place. What lives here is the App's own two things: turning an App's key
// into a client, and verifying and accepting a webhook delivery.
//
// Nothing here creates a real GitHub App: the owner does that (docs/backend-checklist.md section 3).
// Every test drives a recorded delivery or a fake server, never a live installation.
package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
)

// The headers a GitHub delivery carries. GitHub signs the exact bytes it sends, so the signature
// covers the raw body and nothing else about it.
const (
	// SignatureHeader is the HMAC of the raw body: "sha256=<hex>". It is the one thing that proves
	// a delivery came from GitHub and not from anyone who can reach the daemon's port.
	SignatureHeader = "X-Hub-Signature-256"
	// EventHeader names the kind of event: "ping", "check_run", "workflow_run", "pull_request",
	// "push", and the rest of the events the App subscribes to.
	EventHeader = "X-GitHub-Event"
	// DeliveryHeader is the delivery's own id. A redelivery keeps it, so a log line and a retry can
	// be lined up with each other.
	DeliveryHeader = "X-GitHub-Delivery"
)

// MaxBodyBytes is the most a delivery may read. GitHub's own payloads are far smaller; this only
// makes sure a delivery cannot be made to read without end. It matches the ceiling the router puts
// on a signed-webhook route.
const MaxBodyBytes = 1 << 20

// The ways a delivery fails to prove itself. They are different answers: a missing signature is a
// misconfigured sender, a wrong one is not GitHub or the wrong secret, and a delivery with no event
// kind cannot be routed at all.
var (
	// ErrNoSignature means the delivery carried no signature header at all.
	ErrNoSignature = errors.New("this delivery has no signature")
	// ErrBadSignature means the signature did not match the body: not GitHub, or the wrong secret.
	ErrBadSignature = errors.New("this delivery's signature does not match")
	// ErrNoEvent means the delivery did not say what kind of event it is.
	ErrNoEvent = errors.New("this delivery does not say what kind of event it is")
	// ErrNoSecret means Marshal has no webhook secret saved, so it cannot check the delivery at
	// all. It is refused rather than accepted: a verifier with no secret would believe anything.
	ErrNoSecret = errors.New("no webhook secret is saved")
)

// SecretSource answers the webhook secret the App was set up with. It is read fresh on every
// delivery and not held, so a secret the owner saves while the daemon is running takes effect
// without a restart, and a secret the owner removes stops being trusted at once.
type SecretSource func(ctx context.Context) (string, error)

// Verifier checks a delivery against the webhook secret the App was set up with. It is safe for use
// by many goroutines.
type Verifier struct {
	source SecretSource
}

// NewVerifier builds a Verifier over one webhook secret, for a caller that already holds it. The
// secret is kept exactly as given: an HMAC is over the bytes GitHub has, so a secret with a trailing
// space must keep it. An empty secret is refused - a verifier with no secret would accept anything,
// which is worse than no route at all.
func NewVerifier(secret string) (*Verifier, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("a webhook secret is required")
	}
	return &Verifier{source: func(context.Context) (string, error) { return secret, nil }}, nil
}

// NewVerifierFrom builds a Verifier over a source read on every delivery, which is what the daemon
// uses: the secret is saved through Settings, not at startup. A nil source is refused, and a source
// that answers an empty secret is refused at verify time with ErrNoSecret.
func NewVerifierFrom(source SecretSource) (*Verifier, error) {
	if source == nil {
		return nil, errors.New("a webhook verifier needs a source for the webhook secret")
	}
	return &Verifier{source: source}, nil
}

// Verify checks the signature header against the exact body bytes. It must be called before the
// body is read as JSON or anything else, because the signature is over the bytes as GitHub sent
// them and any re-encoding would change them. The comparison is constant time.
func (v *Verifier) Verify(ctx context.Context, signature string, body []byte) error {
	secret, err := v.source(ctx)
	if err != nil {
		return fmt.Errorf("read the webhook secret: %w", err)
	}
	if secret == "" {
		return ErrNoSecret
	}
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return ErrNoSignature
	}
	algo, hexSignature, ok := strings.Cut(signature, "=")
	if !ok || !strings.EqualFold(algo, "sha256") {
		return ErrBadSignature
	}
	want, err := hex.DecodeString(hexSignature)
	if err != nil {
		return ErrBadSignature
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	if !hmac.Equal(mac.Sum(nil), want) {
		return ErrBadSignature
	}
	return nil
}

// Event is a verified delivery: what kind it is, its id, and the raw body. The body is the exact
// bytes GitHub sent, left as bytes so a handler can decode whichever shape its event needs.
type Event struct {
	// Kind is the X-GitHub-Event value, such as "check_run".
	Kind string
	// Delivery is the X-GitHub-Delivery id, which a redelivery keeps.
	Delivery string
	// Body is the raw body, exactly as delivered.
	Body []byte
}

// Sink receives a verified event. The CI monitor is one (build-plan 6.2). A daemon that has nothing
// to do with deliveries yet sets no sink, and the receiver then accepts a verified delivery and
// remembers nothing - which is the honest answer for an event no part of Marshal handles.
type Sink interface {
	// Delivery handles one verified delivery. An error means Marshal could not handle it, not that
	// the delivery was bad; it is logged, and the delivery is still accepted.
	Delivery(ctx context.Context, event Event) error
}

// DeliveryReading is what one receiver has seen: how many deliveries verified, how many were
// refused, and what the newest of each was. It is how the GitHub connection test tells "GitHub never
// reached us" from "GitHub reached us and the secret does not match", which are different problems
// with different fixes.
type DeliveryReading struct {
	// Accepted is how many deliveries verified and reached the sink.
	Accepted int
	// Refused is how many deliveries were turned away.
	Refused int
	// LastKind is the event kind of the newest delivery of any sort, empty when there has been none.
	LastKind string
	// LastError is why the newest delivery was refused, nil when there has been none or the newest
	// was accepted.
	LastError error
}

// deliveryLog is a receiver's own count of what it saw. It is kept inside the receiver so nothing
// has to be wired up for the count to exist, and it is guarded because deliveries arrive on many
// goroutines.
type deliveryLog struct {
	mu        sync.Mutex
	accepted  int
	refused   int
	lastKind  string
	lastError error
}

func (d *deliveryLog) accept(kind string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.accepted++
	d.lastKind = kind
	d.lastError = nil
}

func (d *deliveryLog) refuse(kind string, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.refused++
	if kind != "" {
		d.lastKind = kind
	}
	d.lastError = err
}

func (d *deliveryLog) reading() DeliveryReading {
	d.mu.Lock()
	defer d.mu.Unlock()
	return DeliveryReading{
		Accepted:  d.accepted,
		Refused:   d.refused,
		LastKind:  d.lastKind,
		LastError: d.lastError,
	}
}

// Receiver verifies deliveries and hands them to the sink. It is the whole of the daemon's webhook
// handling that is not HTTP: the route reads the body, this decides whether to believe it.
type Receiver struct {
	verifier *Verifier
	sink     Sink
	log      *slog.Logger
	seen     deliveryLog
}

// NewReceiver builds a Receiver. The verifier is required: a receiver that cannot check a signature
// would accept anything. The sink may be nil, for a daemon with no handler yet, and the log may be
// nil, for a test.
func NewReceiver(verifier *Verifier, sink Sink, log *slog.Logger) (*Receiver, error) {
	if verifier == nil {
		return nil, errors.New("a webhook receiver needs a verifier")
	}
	return &Receiver{verifier: verifier, sink: sink, log: log}, nil
}

// SetSink replaces what verified deliveries are handed to. It is how the CI monitor is attached once
// it exists, on a receiver that was built earlier. It is called once, while the daemon starts and
// before the route is served, so it is not used to swap a sink under live deliveries.
func (r *Receiver) SetSink(sink Sink) {
	r.sink = sink
}

// Deliveries is what this receiver has seen. A connection test reads it to report whether a
// delivery has reached the route and whether it verified.
func (r *Receiver) Deliveries() DeliveryReading { return r.seen.reading() }

// Receive verifies one delivery and hands it to the sink. A delivery with a missing or wrong
// signature is refused and never reaches the sink. A verified delivery whose kind the sink does not
// know is the sink's business to ignore; a verified delivery with no event header at all cannot be
// routed and is refused. A sink error is logged, not returned - the delivery was good, and GitHub
// must not be told to send it again over a failure on Marshal's side.
func (r *Receiver) Receive(ctx context.Context, headers http.Header, body []byte) error {
	kind := strings.TrimSpace(headers.Get(EventHeader))
	if err := r.verifier.Verify(ctx, headers.Get(SignatureHeader), body); err != nil {
		r.seen.refuse(kind, err)
		return err
	}
	if kind == "" {
		err := ErrNoEvent
		r.seen.refuse("", err)
		return err
	}
	r.seen.accept(kind)
	event := Event{
		Kind:     kind,
		Delivery: strings.TrimSpace(headers.Get(DeliveryHeader)),
		Body:     body,
	}
	if r.sink == nil {
		return nil
	}
	if err := r.sink.Delivery(ctx, event); err != nil && r.log != nil {
		r.log.Warn("a webhook delivery could not be handled",
			"event", event.Kind, "delivery", event.Delivery, "error", err)
	}
	return nil
}
