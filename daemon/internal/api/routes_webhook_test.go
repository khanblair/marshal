package api_test

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"net/http"
	"sync"
	"testing"

	githubapp "github.com/khanblair/marshal/daemon/internal/integrations/github"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// webhookRecorder is the sink the stack's webhook receiver is given: it remembers every verified
// delivery that reached it, so a test can prove one arrived and that an unverified one did not. It
// also forwards to an inner sink when one is set, which is how the stack wires the CI monitor the
// way cmd/marshald does: a delivery the route verified reaches the monitor as well as the recorder.
type webhookRecorder struct {
	mu     sync.Mutex
	events []githubapp.Event
	inner  githubapp.Sink
}

func (r *webhookRecorder) Delivery(ctx context.Context, event githubapp.Event) error {
	r.mu.Lock()
	r.events = append(r.events, event)
	inner := r.inner
	r.mu.Unlock()
	if inner == nil {
		return nil
	}
	return inner.Delivery(ctx, event)
}

// setInner attaches the sink deliveries are forwarded to after they are recorded, which is the CI
// monitor in the stack.
func (r *webhookRecorder) setInner(inner githubapp.Sink) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inner = inner
}

// seen returns a copy of the deliveries that reached the sink.
func (r *webhookRecorder) seen() []githubapp.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]githubapp.Event(nil), r.events...)
}

// githubTestKey is a throwaway RSA key, made once for the whole test binary, so a stack can save a
// GitHub App connection without generating a key per test. It signs nothing that is ever sent: the
// App is only ever built from it, never called with it.
var githubTestKey = sync.OnceValue(func() []byte {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
})

// githubAppRequest is the body PUT /v1/integrations/github takes, with a real throwaway key so the
// daemon's own validation passes.
func githubAppRequest(secret string) protocol.SaveGitHubRequest {
	return protocol.SaveGitHubRequest{
		AppID: 42, InstallationID: 99,
		PrivateKey:    string(githubTestKey()),
		WebhookSecret: secret,
	}
}

// connectGitHub saves a GitHub App connection the way the Settings screen does, so a delivery can be
// signed with a secret the daemon actually holds.
func (st *stack) connectGitHub(t *testing.T, secret string) {
	t.Helper()
	if st.integrations == nil {
		t.Fatal("this stack has no connections service to save a GitHub App on")
	}
	if err := st.integrations.SaveGitHub(context.Background(), githubAppRequest(secret)); err != nil {
		t.Fatalf("save the GitHub App connection: %v", err)
	}
}

// webhookSignature is the signature GitHub would send for a body with a secret.
func webhookSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// deliver posts a webhook body to the daemon with the given headers set.
func (st *stack) deliver(t *testing.T, headers map[string]string, body []byte) reply {
	t.Helper()
	req := st.newRequest(http.MethodPost, "/hooks/github", body)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return st.send(req)
}

func TestAGitHubDeliveryNeedsASignatureAndNoToken(t *testing.T) {
	const secret = "s3cr3t"
	st := newStack(t, withWebhooks(secret))
	body := []byte(`{"zen":"Keep it logically awesome.","hook_id":1}`)

	// No bearer token at all: GitHub cannot carry one, and the route must still accept the delivery
	// because its own signature proves where it came from.
	got := st.deliver(t, map[string]string{
		githubapp.SignatureHeader: webhookSignature(secret, body),
		githubapp.EventHeader:     "ping",
		githubapp.DeliveryHeader:  "d-1",
	}, body).want(t, http.StatusOK)

	if string(got.Body) != `{"ok":true}`+"\n" && string(got.Body) != `{"ok":true}` {
		t.Fatalf("the ack body = %q, want {\"ok\":true}", got.Body)
	}
}

func TestAVerifiedDeliveryReachesTheSink(t *testing.T) {
	const secret = "s3cr3t"
	st := newStack(t, withWebhooks(secret))
	body := []byte(`{"action":"completed","check_run":{"conclusion":"failure"}}`)

	st.deliver(t, map[string]string{
		githubapp.SignatureHeader: webhookSignature(secret, body),
		githubapp.EventHeader:     "check_run",
		githubapp.DeliveryHeader:  "d-42",
	}, body).want(t, http.StatusOK)

	seen := st.webhookSink.seen()
	if len(seen) != 1 {
		t.Fatalf("the sink got %d deliveries, want 1", len(seen))
	}
	if seen[0].Kind != "check_run" || seen[0].Delivery != "d-42" || string(seen[0].Body) != string(body) {
		t.Fatalf("the sink got %+v, want the check_run delivery d-42 with its body", seen[0])
	}
}

func TestAGitHubDeliveryWithAWrongSignatureIsRefused(t *testing.T) {
	st := newStack(t, withWebhooks("s3cr3t"))
	body := []byte(`{"zen":"hi"}`)

	st.deliver(t, map[string]string{
		githubapp.SignatureHeader: webhookSignature("the-wrong-secret", body),
		githubapp.EventHeader:     "ping",
	}, body).apiError(t, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized)

	if got := st.webhookSink.seen(); len(got) != 0 {
		t.Fatalf("a delivery that did not verify reached the sink: %+v", got)
	}
}

func TestAGitHubDeliveryWithNoSignatureIsRefused(t *testing.T) {
	st := newStack(t, withWebhooks("s3cr3t"))
	body := []byte(`{"zen":"hi"}`)

	st.deliver(t, map[string]string{
		githubapp.EventHeader: "ping",
	}, body).apiError(t, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized)

	if got := st.webhookSink.seen(); len(got) != 0 {
		t.Fatalf("a delivery with no signature reached the sink: %+v", got)
	}
}

func TestADeliveryWithoutAnEventKindIsRefused(t *testing.T) {
	const secret = "s3cr3t"
	st := newStack(t, withWebhooks(secret))
	body := []byte(`{"zen":"hi"}`)

	got := st.deliver(t, map[string]string{
		githubapp.SignatureHeader: webhookSignature(secret, body),
		// No X-GitHub-Event: the delivery cannot be routed.
	}, body).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	if got.Details["header"] != githubapp.EventHeader {
		t.Fatalf("details.header = %q, want %q", got.Details["header"], githubapp.EventHeader)
	}
}

func TestADeliveryWithNoAppConnectedIsRefused(t *testing.T) {
	// The route exists on every daemon, because the secret is saved from Settings while the daemon
	// runs and a restart cannot be required to make the App's deliveries arrive. A daemon nobody has
	// connected GitHub to has no secret, so every delivery is refused rather than believed: the
	// address is there, and nothing is accepted through it.
	const secret = "s3cr3t"
	st := newStack(t)
	body := []byte(`{"zen":"hi"}`)

	st.deliver(t, map[string]string{
		githubapp.SignatureHeader: webhookSignature(secret, body),
		githubapp.EventHeader:     "ping",
	}, body).apiError(t, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized)

	if got := st.webhookSink.seen(); len(got) != 0 {
		t.Fatalf("a delivery reached the sink with no App connected: %+v", got)
	}
}

func TestThereIsNoWebhookRouteWithoutTheConnectionsService(t *testing.T) {
	// A daemon built without the connections service has no receiver and so no such address at all.
	st := newStack(t, withoutIntegrations())
	st.deliver(t, map[string]string{githubapp.EventHeader: "ping"}, []byte(`{}`)).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}
