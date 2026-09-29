package api_test

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/api"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// fakeTailnet stands in for a node on a tailnet: it records exactly which addresses the daemon
// asked it for and answers with a listener on this machine, so a test can see what the daemon
// would have bound without Tailscale, a tailnet, or a real second interface anywhere.
type fakeTailnet struct {
	mu       sync.Mutex
	status   protocol.TailnetStatus
	networks []string
	addrs    []string
	watched  bool
	upCalled bool
}

func newFakeTailnet() *fakeTailnet {
	return &fakeTailnet{status: protocol.TailnetStatus{
		Enabled: true, State: "online", Hostname: "marshal", DNSName: "marshal.example.ts.net",
		IPs: []string{"100.64.0.1"},
	}}
}

func (f *fakeTailnet) Up(context.Context) (protocol.TailnetStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upCalled = true
	return f.status, nil
}

func (f *fakeTailnet) Watch(context.Context) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.watched = true
}

func (f *fakeTailnet) Status() protocol.TailnetStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

func (f *fakeTailnet) Listen(network, addr string) (net.Listener, error) {
	return f.record(network, addr)
}

func (f *fakeTailnet) ListenFunnel(network, addr string) (net.Listener, error) {
	return f.record(network, addr)
}

func (f *fakeTailnet) record(network, addr string) (net.Listener, error) {
	f.mu.Lock()
	f.networks = append(f.networks, network)
	f.addrs = append(f.addrs, addr)
	f.mu.Unlock()
	return net.Listen(network, "127.0.0.1:0")
}

func (f *fakeTailnet) Close() error { return nil }

// askedFor is the addresses the daemon asked for, in order.
func (f *fakeTailnet) askedFor() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.addrs...)
}

// The one bind the daemon makes for itself is this machine, and only this machine: 127.0.0.1, not
// every interface. This is docs/architecture.md section 13 in a test - "the daemon listens on
// localhost and on its tailnet address only. Never on all interfaces."
func TestTheLoopbackBindIsThisMachineAndNothingElse(t *testing.T) {
	st := newStack(t)
	if got := api.Loopback(); got != "127.0.0.1" {
		t.Fatalf("the loopback address is %q, want 127.0.0.1", got)
	}
	server := api.New(st.settings, st.log, st.now, api.Deps{Devices: st.devices})
	listener, err := server.Listen()
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()
	host, _, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("read the address: %v", err)
	}
	if host != api.Loopback() {
		t.Errorf("the daemon is listening on %q, want %q and nothing else", host, api.Loopback())
	}
}

// Every other listener comes from the tailnet node, and the addresses the daemon asks it for are
// ones that name no interface at all: the node's own tailnet address, and Funnel's port. Neither
// is ever 0.0.0.0, ::, or any address of this machine - that is what makes the second bind safe to
// add beside the first.
func TestTheTailnetListenerIsAskedForOnTheNodeAddressOnly(t *testing.T) {
	st := newStack(t)
	node := newFakeTailnet()
	server := api.New(st.settings, st.log, st.now, api.Deps{
		Store: st.store, Bus: st.bus, Dev: st.dev, Tailnet: node, Funnel: true,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()

	// Wait for the node to have been asked for both listeners.
	deadline := time.Now().Add(5 * time.Second)
	for len(node.askedFor()) != 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if addrs := node.askedFor(); len(addrs) != 2 {
		t.Fatalf("the daemon asked for %v, want the tailnet and Funnel addresses", addrs)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("run: %v", err)
	}

	addrs := node.askedFor()
	// The tailnet listener takes the daemon's own port with no host: tsnet resolves that to the
	// node's own tailnet address, and a plain net.Listen with the same address would be every
	// interface - which is exactly why the two are not the same call.
	host, port, err := net.SplitHostPort(addrs[0])
	if err != nil {
		t.Fatalf("the tailnet address %q cannot be split: %v", addrs[0], err)
	}
	if host != "" {
		t.Errorf("the tailnet listener names %q; it must name no interface at all", host)
	}
	if port != strconv.Itoa(st.settings.Port) {
		t.Errorf("the tailnet listener is on port %q, want the daemon's own port %d", port, st.settings.Port)
	}
	if addrs[1] != api.FunnelAddress() {
		t.Errorf("Funnel is opened at %q, want %q", addrs[1], api.FunnelAddress())
	}
	// The node was joined before either was opened, so nothing can be serving on the tailnet of a
	// daemon that never got there.
	if !node.upCalled || !node.watched {
		t.Errorf("the node was up %v and watched %v, want both before serving", node.upCalled, node.watched)
	}
}

// There is exactly one plain TCP bind in the daemon's HTTP layer, and it is the loopback one.
// A second one would be a change to how the daemon binds, which docs/architecture.md section 13
// says has to be deliberate and reviewed.
func TestTheDaemonHasExactlyOnePlainBind(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	// \bnet\.Listen\( matches a call to net.Listen and not a method named Listen on something
	// whose name ends in "net", such as s.tailnet.Listen.
	bind := regexp.MustCompile(`\bnet\.Listen\(`)
	wildcard := regexp.MustCompile(`\bhttp\.ListenAndServe|net\.ListenConfig\(|0\.0\.0\.0`)
	count := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || path.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		count += len(bind.FindAll(src, -1))
		if at := wildcard.FindIndex(src); at != nil {
			t.Errorf("%s binds or names a wildcard address at byte %d", name, at[0])
		}
	}
	if count != 1 {
		t.Errorf("the API layer has %d plain binds, want exactly 1 (the loopback one)", count)
	}
}

// Only the webhook routes exist on the public listener. The UI, every /v1 route, the health check,
// and the pairing route are on this machine and on the tailnet, never on the public internet -
// docs/architecture.md section 13: "Funnel exposes only /hooks/*."
func TestFunnelServesOnlyTheWebhookRoutes(t *testing.T) {
	st := newStack(t, withTailnet(newFakeTailnet()), withFunnel())
	handler := st.server.FunnelHandler()

	// A path that walks out of /hooks with ".." is refused by the prefix check, not judged by the
	// prefix and then routed by the cleaned path.
	paths := []string{
		"/", "/index.html", "/v1/health", "/v1/me/devices", "/v1/devices/pair",
		"/v1/projects", "/hooks", "/hooks/../v1/me/devices", "/hooks/../../v1/health",
	}
	for _, target := range paths {
		t.Run(target, func(t *testing.T) {
			rec := serve(handler, http.MethodGet, target, nil)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("GET %s = %d, want 404; body: %s", target, rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "devices") || strings.Contains(rec.Body.String(), "health") {
				t.Errorf("the answer names an address that does not exist publicly: %s", rec.Body.String())
			}
		})
	}

	// The webhook routes are there, and they are still their own signature-verified selves: an
	// unsigned delivery is refused before it is read as anything else.
	for _, hook := range []string{"/hooks/github", "/hooks/trello"} {
		t.Run(hook, func(t *testing.T) {
			rec := serve(handler, http.MethodPost, hook, []byte(`{}`))
			if rec.Code == http.StatusNotFound {
				t.Fatalf("the public listener refuses %s entirely", hook)
			}
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("POST %s without a signature = %d, want 401", hook, rec.Code)
			}
		})
	}
}

// The public listener is not a second daemon: opening it does not change what the daemon answers
// on its own address. The same routes, the same answers.
func TestOpeningFunnelDoesNotChangeTheLocalDaemon(t *testing.T) {
	st := newStack(t, withTailnet(newFakeTailnet()), withFunnel())
	st.do(http.MethodGet, "/v1/me/devices", nil).want(t, http.StatusOK)
	st.do(http.MethodGet, "/v1/health", nil).want(t, http.StatusOK)
}

// A phone opens the page over the tailnet and its origin names the node's own MagicDNS name or one
// of its tailnet addresses: that is this daemon's own node, and it is accepted. A page from
// anywhere else is refused exactly as before.
func TestATailnetOriginIsAcceptedAndNoOther(t *testing.T) {
	st := newStack(t, withTailnet(newFakeTailnet()))
	bearer := protocol.BearerSubprotocolPrefix + st.token
	tests := []struct {
		name    string
		origin  string
		allowed bool
	}{
		{"the node's own MagicDNS name", "http://marshal.example.ts.net", true},
		{"the node's MagicDNS name on the daemon's port", "http://marshal.example.ts.net:47800", true},
		{"the node's tailnet address", "http://100.64.0.1", true},
		{"the node's tailnet address on a port", "http://100.64.0.1:47800", true},
		{"the dev server's page still passes", "http://localhost:5173", true},
		{"another name in the same tailnet", "http://other-node.example.ts.net", false},
		{"a page from another site", "http://evil.example", false},
		{"a name that only looks like the node", "http://xmarshal.example.ts.net", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := st.send(st.upgrade(tc.origin, false, "marshal.v1", bearer))
			if tc.allowed {
				got.apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
				return
			}
			got.apiError(t, http.StatusForbidden, protocol.ErrorCodeForbidden)
		})
	}
}

// Without a node there are no tailnet origins, so a daemon nobody asked to join a tailnet accepts
// exactly the origins it always did.
func TestADaemonWithNoTailnetAddsNoOrigin(t *testing.T) {
	st := newStack(t, normalDaemon())
	bearer := protocol.BearerSubprotocolPrefix + st.token
	for _, origin := range []string{"http://marshal.example.ts.net", "http://100.64.0.1"} {
		t.Run(origin, func(t *testing.T) {
			st.send(st.upgrade(origin, false, "marshal.v1", bearer)).
				apiError(t, http.StatusForbidden, protocol.ErrorCodeForbidden)
		})
	}
	st.send(st.upgrade("tauri://localhost", false, "marshal.v1", bearer)).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
}

// GET /v1/tailnet answers at once with "off" on a daemon that was not started to join one, so the
// status screen never waits on a node that does not exist.
func TestTheTailnetStatusSaysOffWhenThereIsNoNode(t *testing.T) {
	st := newStack(t)
	status := decode[protocol.TailnetStatus](t,
		st.do(http.MethodGet, "/v1/tailnet", nil).want(t, http.StatusOK))
	if status.Enabled || status.State != "off" || status.Funnel {
		t.Errorf("status = %+v, want a daemon that is off", status)
	}
	if status.ServerTime.Time().IsZero() {
		t.Error("the answer carries no server time")
	}
}

// With a node the answer is the node's own, plus whether Funnel was asked for - which is what the
// tailnet and Funnel status screens draw (build-plan task 9.9).
func TestTheTailnetStatusReportsTheNode(t *testing.T) {
	st := newStack(t, withTailnet(newFakeTailnet()), withFunnel())
	status := decode[protocol.TailnetStatus](t,
		st.do(http.MethodGet, "/v1/tailnet", nil).want(t, http.StatusOK))
	if !status.Enabled || !status.Funnel || status.State != "online" {
		t.Errorf("status = %+v, want an enabled node with Funnel asked for", status)
	}
	if status.DNSName != "marshal.example.ts.net" || status.Hostname != "marshal" {
		t.Errorf("status = %+v, want the node's own names", status)
	}
	if len(status.IPs) != 1 || status.IPs[0] != "100.64.0.1" {
		t.Errorf("ips = %v, want the node's tailnet address", status.IPs)
	}
}

// serve runs one request straight through a handler, which is how the public listener's own
// routing is checked without a listener of its own.
func serve(handler http.Handler, method, target string, body []byte) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	handler.ServeHTTP(rec, req)
	return rec
}
