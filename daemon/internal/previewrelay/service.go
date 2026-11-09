// Package previewrelay exposes loopback development servers through short-lived,
// per-card gateways. It deliberately has no dependency on the API or store layers.
package previewrelay

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// CapabilityTTL limits each gateway lifetime.
	CapabilityTTL = 15 * time.Minute
	// BootstrapTicketTTL limits a one-use ticket before exchange.
	BootstrapTicketTTL = time.Minute
	// MaxGateways bounds listeners held by one daemon.
	MaxGateways = 4
	// MaxAuthorizations bounds pending tickets and browser sessions per gateway.
	MaxAuthorizations  = 32
	websocketCheckRate = 5 * time.Second
	maxBootstrapBody   = 16 << 10
	previewPath        = "/__marshal_preview__/"
)

func reservedPort(port int) bool { return port == 47800 || port == 47801 || port == 3210 }

// Connection is a short-lived browser connection to one preview gateway.
type Connection struct {
	URL       string
	SourceURL string
	ExpiresAt time.Time
}

// Expose makes a gateway available to a remote client and returns the public
// base URL and a cleanup function for only this exposure.
type Expose func(ctx context.Context, localPort int, secure bool) (publicBase string, cleanup func(), err error)

// Service owns isolated preview gateway listeners.
type Service struct {
	mu       sync.Mutex
	blocked  map[int]struct{}
	expose   Expose
	gateways map[string]*gateway
	closed   bool
}

type gateway struct {
	key          string
	upstream     *url.URL
	clientOrigin string
	publicBase   *url.URL
	publicHost   string
	prefix       string
	cookieName   string
	secureCookie bool
	listener     net.Listener
	server       *http.Server
	exposeClose  func()
	expiresAt    time.Time
	timer        *time.Timer

	mu       sync.Mutex
	tickets  map[string]ticket
	sessions map[string]authorization
	origins  map[string]struct{}
	closed   bool
}

type ticket struct {
	expires      time.Time
	path         string
	authorize    func(context.Context) bool
	clientOrigin string
}

type authorization struct {
	expires      time.Time
	clientOrigin string
	allowed      func(context.Context) bool
}

type bootstrapPayload struct {
	Ticket string `json:"ticket"`
	Path   string `json:"path"`
}

type authorizePayload struct {
	Ticket string `json:"ticket"`
	Path   string `json:"path"`
}

type authorizeResponse struct {
	Redirect string `json:"redirect"`
}

// New constructs an idle relay. It opens no listener until Open is called.
func New(blockedPorts []int) *Service {
	blocked := make(map[int]struct{}, len(blockedPorts))
	for _, port := range blockedPorts {
		if port > 0 && port <= 65535 {
			blocked[port] = struct{}{}
		}
	}
	return &Service{blocked: blocked, gateways: make(map[string]*gateway)}
}

// SetExposure configures the optional remote exposure mechanism.
func (s *Service) SetExposure(expose Expose) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expose = expose
}

// Open validates the upstream and client origin, then mints a one-time
// bootstrap ticket for a new or reusable per-card/upstream gateway.
func (s *Service) Open(ctx context.Context, cardID, sourceURL, clientOrigin string, authorized func(context.Context) bool) (Connection, error) {
	if err := ctx.Err(); err != nil {
		return Connection{}, err
	}
	if strings.TrimSpace(cardID) == "" || len(cardID) > 256 {
		return Connection{}, errors.New("invalid card ID")
	}
	if authorized == nil {
		return Connection{}, errors.New("authorization callback is required")
	}
	origin, err := parseOrigin(clientOrigin)
	if err != nil {
		return Connection{}, fmt.Errorf("invalid client origin: %w", err)
	}
	upstream, path, err := parseSourceURL(sourceURL)
	if err != nil {
		return Connection{}, err
	}
	key := cardID + "\x00" + upstream.Scheme + "://" + strings.ToLower(upstream.Host) + "\x00" + origin

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Connection{}, errors.New("preview relay is closed")
	}
	if err := ctx.Err(); err != nil {
		return Connection{}, err
	}

	port, _ := strconv.Atoi(upstream.Port())
	if s.blockedPort(port) {
		return Connection{}, errors.New("preview source uses a reserved Marshal port")
	}
	g := s.gateways[key]
	if g != nil && !time.Now().Before(g.expiresAt) {
		delete(s.gateways, key)
		g.shutdown()
		g = nil
	}
	if g == nil {
		if len(s.gateways) >= MaxGateways {
			return Connection{}, errors.New("preview gateway limit reached")
		}

		g, err = s.newGatewayLocked(ctx, key, upstream, origin)
		if err != nil {
			return Connection{}, err
		}
		s.gateways[key] = g
		g.timer = time.AfterFunc(time.Until(g.expiresAt), func() { s.remove(g) })
	} else {
		g.addClientOrigin(origin)
	}

	if err := g.checkCapacity(); err != nil {
		return Connection{}, err
	}
	secret, err := randomToken()
	if err != nil {
		return Connection{}, err
	}
	now := time.Now()
	g.mu.Lock()
	g.tickets[secret] = ticket{
		expires: now.Add(BootstrapTicketTTL), path: path,
		authorize: authorized, clientOrigin: origin,
	}
	g.mu.Unlock()

	fragmentJSON, _ := json.Marshal(bootstrapPayload{Ticket: secret, Path: path})
	fragment := base64.RawURLEncoding.EncodeToString(fragmentJSON)
	bootstrap := *g.publicBase
	bootstrap.Path = g.prefix + "bootstrap"
	bootstrap.RawPath = ""
	bootstrap.RawQuery = url.Values{"origin": []string{origin}}.Encode()
	bootstrap.Fragment = fragment
	return Connection{URL: bootstrap.String(), SourceURL: sourceURL, ExpiresAt: g.expiresAt}, nil
}

func (s *Service) newGatewayLocked(ctx context.Context, key string, upstream *url.URL, clientOrigin string) (*gateway, error) {
	network, address := "tcp4", "127.0.0.1:0"
	if clientHostname(clientOrigin) == "::1" {
		network, address = "tcp6", "[::1]:0"
	}
	listener, err := net.Listen(network, address)
	if err != nil {
		return nil, fmt.Errorf("open preview listener: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if reservedPort(port) || s.blockedPort(port) {
		_ = listener.Close()
		return nil, errors.New("preview listener selected a reserved port")
	}
	prefixToken, err := randomToken()
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	prefix := previewPath + prefixToken + "/"
	base, cleanup, err := s.gatewayAddress(ctx, port, clientOrigin)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}

	g := &gateway{
		key: key, upstream: upstream,
		clientOrigin: clientOrigin, publicBase: base, publicHost: strings.ToLower(base.Host),
		prefix: prefix, cookieName: "marshal-preview-" + prefixToken,
		secureCookie: base.Scheme == "https", listener: listener, exposeClose: cleanup,
		expiresAt: time.Now().Add(CapabilityTTL), tickets: make(map[string]ticket),
		sessions: make(map[string]authorization),
	}
	g.addClientOrigin(clientOrigin)
	g.server = &http.Server{
		Handler:           g,
		ReadHeaderTimeout: 5 * time.Second,
		ErrorLog:          log.New(io.Discard, "", 0),
	}
	go func() { _ = g.server.Serve(listener) }()
	return g, nil
}

func (s *Service) blockedPort(port int) bool {
	if reservedPort(port) {
		return true
	}
	if _, ok := s.blocked[port]; ok {
		return true
	}
	for _, g := range s.gateways {
		if g.listener.Addr().(*net.TCPAddr).Port == port {
			return true
		}
	}
	return false
}

func (g *gateway) checkCapacity() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.pruneLocked(time.Now())
	if len(g.tickets)+len(g.sessions) >= MaxAuthorizations {
		return errors.New("preview authorization limit reached")
	}
	return nil
}

func (g *gateway) addClientOrigin(origin string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.origins == nil {
		g.origins = make(map[string]struct{})
	}
	g.origins[origin] = struct{}{}
}

func (g *gateway) hasClientOrigin(origin string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, ok := g.origins[origin]
	return ok
}

func (g *gateway) pruneLocked(now time.Time) {
	for token, t := range g.tickets {
		if !now.Before(t.expires) || !now.Before(g.expiresAt) {
			delete(g.tickets, token)
		}
	}
	for token, a := range g.sessions {
		if !now.Before(a.expires) || !now.Before(g.expiresAt) {
			delete(g.sessions, token)
		}
	}
}

// Close tears down all listeners and their corresponding exposure routes.
func (s *Service) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	all := make([]*gateway, 0, len(s.gateways))
	for _, g := range s.gateways {
		all = append(all, g)
	}
	s.gateways = make(map[string]*gateway)
	s.mu.Unlock()
	for _, g := range all {
		g.shutdown()
	}
}

func (s *Service) remove(g *gateway) {
	s.mu.Lock()
	if s.gateways[g.key] != g {
		s.mu.Unlock()
		return
	}
	delete(s.gateways, g.key)
	s.mu.Unlock()
	g.shutdown()
}

func (g *gateway) shutdown() {
	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		return
	}
	g.closed = true
	if g.timer != nil {
		g.timer.Stop()
	}
	g.mu.Unlock()
	if g.server != nil {
		_ = g.server.Close()
	} else if g.listener != nil {
		_ = g.listener.Close()
	}
	if g.exposeClose != nil {
		g.exposeClose()
		g.exposeClose = nil
	}
}
