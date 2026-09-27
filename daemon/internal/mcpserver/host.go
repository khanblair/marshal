package mcpserver

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// PathPrefix is where a Host is mounted on the daemon's own listener. The one segment after it is
// the card the request speaks for, so /v1/mcp/<cardID> reaches that card's server.
const PathPrefix = "/v1/mcp/"

// secretBytes is how much randomness a card's secret carries. It is the only thing standing between
// a local process that guesses an address and every tool the daemon serves, so it is a full-length
// token rather than a counter.
const secretBytes = 32

// Host serves the live per-card servers to agents over the daemon's own loopback listener
// (docs/architecture.md section 11.4). One server is built per agent session (see New), and this is
// what holds it while the session lives and lets the agent reach it.
//
// # Why the daemon serves them and not a child process
//
// A server's tools act on the whole daemon: they read the board through the projects service, the
// card's note and claims through the memory module, and, for ask_agent, put a question to another
// card's *live* session through the session manager. None of that is reachable from a process the
// agent starts on its own, so the server runs where the services are. The agent is given a stdio
// command that is this daemon again in its `mcp` mode, and that command forwards to here.
//
// # Reaching this from a CLI agent
//
// A CLI agent is told, over ACP, to run a command: the daemon's own executable as `mcp`, with the
// address and the card in its arguments and the card's secret in its environment (see internal/mcpattach).
// That command speaks MCP over stdio to the agent and this same protocol over HTTP to the daemon, so
// the agent never holds a daemon token and never sees a URL.
//
// # The secret
//
// Each card's server is reachable only with the secret minted when it was added, checked in constant
// time on every request. The secret lives exactly as long as the server: Add answers one, Remove
// takes the server away, and a card that never attached has nothing to guess. The listener is
// loopback-only, so the secret is a second lock rather than the only one.
type Host struct {
	mu      sync.RWMutex
	live    map[string]*hosted
	logger  *slog.Logger
	handler http.Handler
}

// hosted is one card's live server and the secret that reaches it.
type hosted struct {
	server *Server
	secret string
}

// HostOption changes how a Host is built.
type HostOption func(*Host)

// WithHostLogger sets where the transport notes a rejected or failed request. The default logs
// nothing.
func WithHostLogger(logger *slog.Logger) HostOption {
	return func(h *Host) { h.logger = logger }
}

// NewHost builds a host with nothing attached.
func NewHost(opts ...HostOption) *Host {
	h := &Host{live: make(map[string]*hosted)}
	for _, opt := range opts {
		opt(h)
	}
	h.handler = mcp.NewStreamableHTTPHandler(h.serverForRequest, &mcp.StreamableHTTPOptions{
		Logger: h.logger,
	})
	return h
}

// Add makes a card's server reachable and answers the secret that reaches it. A card that already
// has a server keeps the one it has and answers the same secret, so a resume does not strand the
// agent that was already told the old one.
func (h *Host) Add(cardID string, s *Server) (string, error) {
	if cardID == "" {
		return "", errors.New("mcpserver: a card id is required to host a server")
	}
	if s == nil {
		return "", errors.New("mcpserver: there is no server to host")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if have, ok := h.live[cardID]; ok {
		return have.secret, nil
	}
	secret, err := newSecret()
	if err != nil {
		return "", err
	}
	h.live[cardID] = &hosted{server: s, secret: secret}
	return secret, nil
}

// Remove takes a card's server away, so nothing reaches it any more. It does nothing for a card that
// was never added, which is what lets it be called for every session that ends.
func (h *Host) Remove(cardID string) {
	h.mu.Lock()
	delete(h.live, cardID)
	h.mu.Unlock()
}

// Has says whether a card's server is currently hosted.
func (h *Host) Has(cardID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.live[cardID]
	return ok
}

// Len is how many cards' servers are hosted, for a test or a health line.
func (h *Host) Len() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.live)
}

// ServeHTTP is the daemon's own endpoint: it checks the card's secret and hands the request to the
// MCP transport. It answers 401 for a missing or wrong secret, without saying whether the card
// exists.
func (h *Host) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	cardID, ok := cardOf(req.URL.Path)
	if !ok {
		http.Error(w, "no card in the address", http.StatusNotFound)
		return
	}
	secret, ok := bearer(req.Header.Get("Authorization"))
	if !ok || !h.authorized(cardID, secret) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	h.handler.ServeHTTP(w, req)
}

// serverForRequest answers the card's server to the MCP transport. The secret was already checked in
// ServeHTTP, so this only has to find the server, and answers nil when the card's session has ended
// between the two calls.
func (h *Host) serverForRequest(req *http.Request) *mcp.Server {
	cardID, ok := cardOf(req.URL.Path)
	if !ok {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	have, ok := h.live[cardID]
	if !ok {
		return nil
	}
	return have.server.impl
}

// authorized answers whether the secret is the one minted for the card, in constant time.
func (h *Host) authorized(cardID, secret string) bool {
	h.mu.RLock()
	have, ok := h.live[cardID]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(have.secret), []byte(secret)) == 1
}

// cardOf reads the card out of the address, and reports false when there is none.
func cardOf(path string) (string, bool) {
	rest, ok := strings.CutPrefix(path, PathPrefix)
	if !ok {
		return "", false
	}
	rest = strings.TrimSuffix(rest, "/")
	if rest == "" || strings.Contains(rest, "/") {
		return "", false
	}
	return rest, true
}

// bearer reads a bearer token out of an Authorization header.
func bearer(header string) (string, bool) {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

// newSecret mints the token one card's server is reached with.
func newSecret() (string, error) {
	buf := make([]byte, secretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
