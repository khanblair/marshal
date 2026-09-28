package mcpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The host is what lets an agent reach its card's server over HTTP (docs/architecture.md section
// 11.4): the agent runs the daemon's own `mcp` mode, and that forwards here. These tests drive a
// real MCP client over a real in-process HTTP server, through the same handshake an agent uses, so a
// secret that does not work or a card that cannot be found fails here and not in production.

// authTransport adds one Authorization header to every request, standing in for the card's secret
// the `mcp` command is given in its environment.
type authTransport struct {
	secret string
	base   http.RoundTripper
}

func (a authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	if a.secret != "" {
		clone.Header.Set("Authorization", "Bearer "+a.secret)
	}
	return a.base.RoundTrip(clone)
}

// hostClient connects a client to a hosted card's server the way the `mcp` command does.
func hostClient(t *testing.T, address, cardID, secret string) *mcp.ClientSession {
	t.Helper()
	ctx := t.Context()
	transport := &mcp.StreamableClientTransport{
		Endpoint:             address + PathPrefix + cardID,
		DisableStandaloneSSE: true,
		HTTPClient:           &http.Client{Transport: authTransport{secret: secret, base: http.DefaultTransport}},
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "mcp-command", Version: "0"}, nil)
	cs, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("connect to the hosted server: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestAHostServesACardsServerOverHTTP(t *testing.T) {
	f := newFixture(t)
	host := NewHost()
	secret, err := host.Add("card-1", f.server)
	if err != nil {
		t.Fatalf("host the server: %v", err)
	}
	httpSrv := httptest.NewServer(host)
	t.Cleanup(httpSrv.Close)

	cs := hostClient(t, httpSrv.URL, "card-1", secret)
	tools, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("list tools over HTTP: %v", err)
	}
	if len(tools.Tools) != f.server.ToolCount() {
		t.Errorf("the host serves %d tools, want the %d the server does", len(tools.Tools), f.server.ToolCount())
	}

	// A real read goes all the way through: the same tools, the same permission check, the same
	// answer a local agent gets.
	text, isError := call(t, cs, "board_status", map[string]any{})
	if isError {
		t.Fatalf("board_status was refused over HTTP: %s", text)
	}
	if !strings.Contains(text, "Ship the board") {
		t.Errorf("board_status answered %q, want the other cards", text)
	}
}

func TestAHostRefusesTheWrongSecret(t *testing.T) {
	f := newFixture(t)
	host := NewHost()
	if _, err := host.Add("card-1", f.server); err != nil {
		t.Fatalf("host the server: %v", err)
	}
	httpSrv := httptest.NewServer(host)
	t.Cleanup(httpSrv.Close)

	for _, tc := range []struct{ name, secret string }{
		{"no secret", ""},
		{"the wrong secret", "not-the-one"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A refusal is the connect itself: without a good secret the handshake never completes.
			transport := &mcp.StreamableClientTransport{
				Endpoint:             httpSrv.URL + PathPrefix + "card-1",
				DisableStandaloneSSE: true,
				HTTPClient:           &http.Client{Transport: authTransport{secret: tc.secret, base: http.DefaultTransport}},
			}
			client := mcp.NewClient(&mcp.Implementation{Name: "mcp-command", Version: "0"}, nil)
			cs, err := client.Connect(t.Context(), transport, nil)
			if err == nil {
				_ = cs.Close()
				t.Fatal("a client reached the server with no valid secret")
			}
		})
	}
}

func TestRemovingACardTakesItsServerAway(t *testing.T) {
	f := newFixture(t)
	host := NewHost()
	secret, err := host.Add("card-1", f.server)
	if err != nil {
		t.Fatalf("host the server: %v", err)
	}
	httpSrv := httptest.NewServer(host)
	t.Cleanup(httpSrv.Close)

	cs := hostClient(t, httpSrv.URL, "card-1", secret)
	if _, isError := call(t, cs, "board_status", map[string]any{}); isError {
		t.Fatal("the server was not reachable before it was removed")
	}

	// The session ending is what removes it. A new client cannot reach it after that, and the old
	// secret is worth nothing.
	host.Remove("card-1")
	if host.Has("card-1") {
		t.Error("the card is still hosted after Remove")
	}
	transport := &mcp.StreamableClientTransport{
		Endpoint:             httpSrv.URL + PathPrefix + "card-1",
		DisableStandaloneSSE: true,
		HTTPClient:           &http.Client{Transport: authTransport{secret: secret, base: http.DefaultTransport}},
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "mcp-command", Version: "0"}, nil)
	fresh, err := client.Connect(t.Context(), transport, nil)
	if err == nil {
		_ = fresh.Close()
		t.Fatal("a removed card's server was still reachable")
	}
}

func TestAHostKeepsOneSecretForACard(t *testing.T) {
	f := newFixture(t)
	host := NewHost()
	first, err := host.Add("card-1", f.server)
	if err != nil {
		t.Fatalf("host the server: %v", err)
	}
	// A resume asks again; the agent that already holds the first secret must not be stranded.
	second, err := host.Add("card-1", f.server)
	if err != nil {
		t.Fatalf("re-host the server: %v", err)
	}
	if first != second {
		t.Error("hosting a card again minted a second secret")
	}
	if host.Len() != 1 {
		t.Errorf("the host holds %d servers, want one", host.Len())
	}
}

func TestHostRefusesACardWithNoServer(t *testing.T) {
	host := NewHost()
	if _, err := host.Add("", newFixture(t).server); err == nil {
		t.Error("a server was hosted for no card")
	}
	if _, err := host.Add("card-1", nil); err == nil {
		t.Error("nothing was hosted as a server")
	}
}

func TestCardOfReadsTheAddress(t *testing.T) {
	for _, tc := range []struct {
		path string
		card string
		ok   bool
	}{
		{"/v1/mcp/card-1", "card-1", true},
		{"/v1/mcp/card-1/", "card-1", true},
		{"/v1/mcp/", "", false},
		{"/v1/mcp", "", false},
		{"/v1/mcp/card-1/extra", "", false},
		{"/v1/other/card-1", "", false},
	} {
		card, ok := cardOf(tc.path)
		if ok != tc.ok || card != tc.card {
			t.Errorf("cardOf(%q) = %q, %v; want %q, %v", tc.path, card, ok, tc.card, tc.ok)
		}
	}
}

func TestBearerReadsTheHeader(t *testing.T) {
	for _, tc := range []struct {
		header string
		token  string
		ok     bool
	}{
		{"Bearer abc", "abc", true},
		{"bearer abc", "abc", true},
		{"Bearer  abc ", "abc", true},
		{"Bearer ", "", false},
		{"abc", "", false},
		{"", "", false},
	} {
		token, ok := bearer(tc.header)
		if ok != tc.ok || token != tc.token {
			t.Errorf("bearer(%q) = %q, %v; want %q, %v", tc.header, token, ok, tc.token, tc.ok)
		}
	}
}

// A request with no card in the address, or to a card nobody hosted, is answered without ever
// reaching the MCP transport.
func TestHostAnswersAnUnknownCardWithoutAServer(t *testing.T) {
	host := NewHost()
	srv := httptest.NewServer(host)
	t.Cleanup(srv.Close)
	res, err := http.Get(srv.URL + PathPrefix + "nobody")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("an unknown card answered %d, want 401", res.StatusCode)
	}
}
