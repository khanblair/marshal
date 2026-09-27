package github_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	githubapp "github.com/khanblair/marshal/daemon/internal/integrations/github"
)

// sign makes the signature GitHub would send for a body with a secret: "sha256=<hex hmac>".
func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// headers builds the headers a GitHub delivery carries, with a signature over body.
func headers(secret, kind, delivery string, body []byte) http.Header {
	h := http.Header{}
	h.Set(githubapp.EventHeader, kind)
	h.Set(githubapp.DeliveryHeader, delivery)
	h.Set(githubapp.SignatureHeader, sign(secret, body))
	return h
}

// recordingSink remembers every event it is handed, so a test can prove what reached it.
type recordingSink struct {
	events []githubapp.Event
	err    error
}

func (s *recordingSink) Delivery(_ context.Context, event githubapp.Event) error {
	s.events = append(s.events, event)
	return s.err
}

func receiver(t *testing.T, secret string, sink githubapp.Sink) *githubapp.Receiver {
	t.Helper()
	v, err := githubapp.NewVerifier(secret)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	r, err := githubapp.NewReceiver(v, sink, nil)
	if err != nil {
		t.Fatalf("NewReceiver: %v", err)
	}
	return r
}

func TestAVerifiedDeliveryReachesTheSink(t *testing.T) {
	const secret = "s3cr3t"
	body := []byte(`{"action":"completed"}`)
	sink := &recordingSink{}
	recv := receiver(t, secret, sink)

	if err := recv.Receive(context.Background(),
		headers(secret, "check_run", "d-1", body), body); err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if len(sink.events) != 1 {
		t.Fatalf("the sink got %d events, want 1", len(sink.events))
	}
	got := sink.events[0]
	if got.Kind != "check_run" || got.Delivery != "d-1" || string(got.Body) != string(body) {
		t.Fatalf("the sink got %+v, want the check_run delivery d-1 with its body", got)
	}
}

func TestTheSignatureIsOverTheRawBodyNotAReparsedOne(t *testing.T) {
	const secret = "s3cr3t"
	// The same JSON, but with different whitespace. It is a different body, so the signature for
	// the first does not verify the second: this is why the route reads the body before parsing it.
	signed := []byte(`{"action": "completed"}`)
	resent := []byte(`{"action":"completed"}`)
	recv := receiver(t, secret, &recordingSink{})
	h := headers(secret, "check_run", "d-1", signed)

	if err := recv.Receive(context.Background(), h, resent); !errors.Is(err, githubapp.ErrBadSignature) {
		t.Fatalf("Receive with a changed body = %v, want ErrBadSignature", err)
	}
}

func TestADeliveryWithNoSignatureIsRefused(t *testing.T) {
	body := []byte(`{}`)
	h := http.Header{}
	h.Set(githubapp.EventHeader, "ping")
	recv := receiver(t, "s3cr3t", &recordingSink{})

	if err := recv.Receive(context.Background(), h, body); !errors.Is(err, githubapp.ErrNoSignature) {
		t.Fatalf("Receive with no signature = %v, want ErrNoSignature", err)
	}
}

func TestADeliverySignedWithAnotherSecretIsRefused(t *testing.T) {
	body := []byte(`{}`)
	h := headers("the-wrong-secret", "ping", "d-1", body)
	sink := &recordingSink{}
	recv := receiver(t, "s3cr3t", sink)

	if err := recv.Receive(context.Background(), h, body); !errors.Is(err, githubapp.ErrBadSignature) {
		t.Fatalf("Receive with a wrong secret = %v, want ErrBadSignature", err)
	}
	if len(sink.events) != 0 {
		t.Fatal("a delivery that did not verify reached the sink")
	}
}

func TestASignatureWithAnotherAlgorithmIsRefused(t *testing.T) {
	body := []byte(`{}`)
	h := http.Header{}
	h.Set(githubapp.EventHeader, "ping")
	h.Set(githubapp.SignatureHeader, "sha1="+hex.EncodeToString([]byte("nope")))
	recv := receiver(t, "s3cr3t", &recordingSink{})

	if err := recv.Receive(context.Background(), h, body); !errors.Is(err, githubapp.ErrBadSignature) {
		t.Fatalf("Receive with a sha1 signature = %v, want ErrBadSignature", err)
	}
}

func TestAMalformedSignatureIsRefused(t *testing.T) {
	body := []byte(`{}`)
	h := http.Header{}
	h.Set(githubapp.EventHeader, "ping")
	h.Set(githubapp.SignatureHeader, "sha256=not-hex")
	recv := receiver(t, "s3cr3t", &recordingSink{})

	if err := recv.Receive(context.Background(), h, body); !errors.Is(err, githubapp.ErrBadSignature) {
		t.Fatalf("Receive with a malformed signature = %v, want ErrBadSignature", err)
	}
}

func TestADeliveryWithNoEventKindIsRefused(t *testing.T) {
	const secret = "s3cr3t"
	body := []byte(`{}`)
	h := http.Header{}
	h.Set(githubapp.SignatureHeader, sign(secret, body))
	recv := receiver(t, secret, &recordingSink{})

	if err := recv.Receive(context.Background(), h, body); !errors.Is(err, githubapp.ErrNoEvent) {
		t.Fatalf("Receive with no event header = %v, want ErrNoEvent", err)
	}
}

func TestASinkFailureIsNotTheSendersProblem(t *testing.T) {
	const secret = "s3cr3t"
	body := []byte(`{}`)
	sink := &recordingSink{err: errors.New("the monitor is down")}
	recv := receiver(t, secret, sink)

	// A verified delivery that Marshal could not handle is still accepted: GitHub must not be told
	// to send it again over a failure on Marshal's side.
	if err := recv.Receive(context.Background(), headers(secret, "check_run", "d-1", body), body); err != nil {
		t.Fatalf("Receive = %v, want nil even when the sink failed", err)
	}
	if len(sink.events) != 1 {
		t.Fatalf("the sink got %d events, want 1 (the failure is the sink's, not a gate)", len(sink.events))
	}
}

func TestAReceiverWithNoSinkAcceptsAVerifiedDelivery(t *testing.T) {
	const secret = "s3cr3t"
	body := []byte(`{}`)
	recv := receiver(t, secret, nil)

	if err := recv.Receive(context.Background(), headers(secret, "ping", "d-1", body), body); err != nil {
		t.Fatalf("Receive = %v, want nil with no sink set", err)
	}
}

func TestAVerifierNeedsASecret(t *testing.T) {
	if _, err := githubapp.NewVerifier(""); err == nil {
		t.Fatal("NewVerifier with an empty secret should fail")
	}
	if _, err := githubapp.NewVerifier("   "); err == nil {
		t.Fatal("NewVerifier with a blank secret should fail")
	}
	if _, err := githubapp.NewVerifier("s3cr3t"); err != nil {
		t.Fatalf("NewVerifier with a secret: %v", err)
	}
}

func TestTheSecretIsUsedExactlyAsGiven(t *testing.T) {
	// A secret with a trailing space is a different secret. Trimming it would silently accept the
	// wrong sender, so the bytes are used as they were configured.
	body := []byte(`{}`)
	h := http.Header{}
	h.Set(githubapp.EventHeader, "ping")
	h.Set(githubapp.SignatureHeader, sign("s3cr3t ", body))
	recv := receiver(t, "s3cr3t", &recordingSink{})

	if err := recv.Receive(context.Background(), h, body); !errors.Is(err, githubapp.ErrBadSignature) {
		t.Fatalf("Receive where only the trailing space differs = %v, want ErrBadSignature", err)
	}
}

func TestAReceiverNeedsAVerifier(t *testing.T) {
	if _, err := githubapp.NewReceiver(nil, nil, nil); err == nil {
		t.Fatal("NewReceiver with no verifier should fail")
	}
}

func TestAVerifierFromASourceReadsTheSecretEachTime(t *testing.T) {
	// The daemon saves its webhook secret while it is running, so the verifier must read the secret
	// on every delivery and not hold the one it was built with.
	const secret = "s3cr3t"
	held := ""
	v, err := githubapp.NewVerifierFrom(func(context.Context) (string, error) { return held, nil })
	if err != nil {
		t.Fatalf("NewVerifierFrom: %v", err)
	}
	recv, err := githubapp.NewReceiver(v, &recordingSink{}, nil)
	if err != nil {
		t.Fatalf("NewReceiver: %v", err)
	}
	body := []byte(`{}`)

	// Nothing is saved yet, so a delivery cannot be checked and must not be believed.
	if err := recv.Receive(context.Background(), headers(secret, "ping", "d-1", body), body); !errors.Is(err, githubapp.ErrNoSecret) {
		t.Fatalf("Receive with no secret saved = %v, want ErrNoSecret", err)
	}
	held = secret
	if err := recv.Receive(context.Background(), headers(secret, "ping", "d-2", body), body); err != nil {
		t.Fatalf("Receive after the secret was saved: %v", err)
	}
	// A secret that is removed stops being trusted at once, without a restart.
	held = ""
	if err := recv.Receive(context.Background(), headers(secret, "ping", "d-3", body), body); !errors.Is(err, githubapp.ErrNoSecret) {
		t.Fatalf("Receive after the secret was removed = %v, want ErrNoSecret", err)
	}
}

func TestAVerifierFromNeedsASource(t *testing.T) {
	if _, err := githubapp.NewVerifierFrom(nil); err == nil {
		t.Fatal("NewVerifierFrom with no source should fail")
	}
}

func TestTheReceiverRemembersWhatItSaw(t *testing.T) {
	// The connection test asks the receiver whether a delivery has reached the route and whether it
	// verified, which are different problems: one is a blocked webhook, the other is a wrong secret.
	const secret = "s3cr3t"
	body := []byte(`{}`)
	sink := &recordingSink{}
	recv := receiver(t, secret, sink)

	if got := recv.Deliveries(); got.Accepted != 0 || got.Refused != 0 {
		t.Fatalf("a fresh receiver read %+v, want nothing seen", got)
	}
	// A delivery signed with the wrong secret is remembered as a refusal, with its kind.
	if err := recv.Receive(context.Background(), headers("wrong", "check_run", "d-1", body), body); err == nil {
		t.Fatal("a wrongly signed delivery should be refused")
	}
	got := recv.Deliveries()
	if got.Refused != 1 || got.Accepted != 0 || got.LastKind != "check_run" {
		t.Fatalf("after one refusal the reading is %+v, want one refusal of a check_run", got)
	}
	if !errors.Is(got.LastError, githubapp.ErrBadSignature) {
		t.Fatalf("the last refusal was %v, want ErrBadSignature", got.LastError)
	}
	// A delivery that verifies is remembered as accepted, and clears the last refusal.
	if err := recv.Receive(context.Background(), headers(secret, "ping", "d-2", body), body); err != nil {
		t.Fatalf("Receive: %v", err)
	}
	got = recv.Deliveries()
	if got.Accepted != 1 || got.Refused != 1 || got.LastKind != "ping" || got.LastError != nil {
		t.Fatalf("after one acceptance the reading is %+v, want one accepted ping and no last error", got)
	}
}

func TestTheReceiverCanBeGivenASinkAfterItIsBuilt(t *testing.T) {
	// The CI monitor does not exist yet when the receiver is built for the route, so it is attached
	// afterwards. Until then a verified delivery is accepted and remembered by nobody but the count.
	const secret = "s3cr3t"
	body := []byte(`{}`)
	recv := receiver(t, secret, nil)

	if err := recv.Receive(context.Background(), headers(secret, "ping", "d-1", body), body); err != nil {
		t.Fatalf("Receive with no sink: %v", err)
	}
	sink := &recordingSink{}
	recv.SetSink(sink)
	if err := recv.Receive(context.Background(), headers(secret, "check_run", "d-2", body), body); err != nil {
		t.Fatalf("Receive after a sink was set: %v", err)
	}
	if len(sink.events) != 1 || sink.events[0].Kind != "check_run" {
		t.Fatalf("the sink got %+v, want only the delivery made after it was set", sink.events)
	}
}

func TestThePingFixtureVerifies(t *testing.T) {
	// The recorded ping delivery (daemon/testdata/hooks/github/ping.json) is the shape GitHub sends
	// on install, and it is the fixture tools/hooks-replay sends. Its headers carry no signature -
	// the recording predates the secret - so this reads the fixture's own body bytes, signs them
	// with the recorded secret, and proves the verifier accepts the delivery the tool would send.
	// The signature vector is the same one tools/hooks-replay's own test asserts, so the sender and
	// the checker are proven against each other and not just against themselves.
	const secret = "s3cr3t"
	type recording struct {
		Headers map[string]string `json:"headers"`
		Body    json.RawMessage   `json:"body"`
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "hooks", "github", "ping.json"))
	if err != nil {
		t.Fatalf("read the ping fixture: %v", err)
	}
	var rec recording
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("read the ping fixture: %v", err)
	}
	if len(rec.Body) == 0 {
		t.Fatal("the ping fixture has no body")
	}
	if got := sign(secret, rec.Body); got != "sha256=79582579274f1b605ec5f9eb889ba26b1e9d6f4875b58474130cc4114e956bc4" {
		t.Fatalf("the fixture signed with %q = %s", secret, got)
	}
	h := http.Header{}
	h.Set(githubapp.EventHeader, rec.Headers["X-GitHub-Event"])
	h.Set(githubapp.DeliveryHeader, rec.Headers["X-GitHub-Delivery"])
	h.Set(githubapp.SignatureHeader, sign(secret, rec.Body))

	sink := &recordingSink{}
	recv := receiver(t, secret, sink)
	if err := recv.Receive(context.Background(), h, rec.Body); err != nil {
		t.Fatalf("Receive of the ping fixture: %v", err)
	}
	if len(sink.events) != 1 || sink.events[0].Kind != "ping" {
		t.Fatalf("the ping fixture did not reach the sink as a ping: %+v", sink.events)
	}
	if sink.events[0].Delivery != rec.Headers["X-GitHub-Delivery"] {
		t.Errorf("the delivery id = %q, want the fixture's own id", sink.events[0].Delivery)
	}
}
