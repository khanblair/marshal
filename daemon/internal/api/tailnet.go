package api

import (
	"context"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// TailnetNode is the daemon's own node on the person's tailnet (B9.1). The API layer defines it
// rather than importing internal/tailnet, so the API compiles, serves, and is tested with no
// Tailscale anywhere: a test hands it a fake and cmd/marshald hands it the real one.
//
// Every method must be safe on a node that is still signing in, and must never open anything on
// this machine's own interfaces - only the node's tailnet addresses.
type TailnetNode interface {
	// Up joins the tailnet and waits until the node is running. On a node nobody has signed in
	// yet it waits for the sign-in, which is why the daemon calls it in the background.
	Up(ctx context.Context) (protocol.TailnetStatus, error)
	// Watch keeps Status current until the context ends. The daemon starts it before Up, because
	// Up waits for a sign-in that may never come, and the screens have to show the sign-in address
	// in the meantime. It is a no-op on a node that answers.
	Watch(ctx context.Context)
	// Status is what the node knows right now, for the status screens. It answers at once, while
	// Up is still waiting, so a sign-in address can be shown while a person opens it.
	Status() protocol.TailnetStatus
	// Listen opens a listener on the tailnet alone, never on this machine's interfaces and never
	// on all of them (docs/architecture.md section 13).
	Listen(network, addr string) (net.Listener, error)
	// ListenFunnel opens a listener on the public internet through Tailscale Funnel. Tailscale
	// answers only on 443, 8443, and 10000, so the port is one of those three.
	ListenFunnel(network, addr string) (net.Listener, error)
	// Close ends the node and its listeners.
	Close() error
}

// HostTailscale is the Tailscale app on this computer, asked from outside it. The API defines it, as
// it does TailnetNode, so it is served and tested with no Tailscale anywhere. port is the port this
// daemon serves on, which a Serve rule has to hand to it to count.
type HostTailscale interface {
	Status(ctx context.Context, port int) protocol.TailnetHost
}

// TailnetPeers is the part of a node that can list the other machines on the tailnet (B9.5). It
// is a separate interface rather than a method on TailnetNode so that a node which does not
// answer peers still satisfies the status interface: a test fake and an unsigned-in node both
// stay valid, and the route simply lists nothing.
type TailnetPeers interface {
	// Peers lists the tailnet's other machines, marking the ones that answer Marshal's health.
	Peers(ctx context.Context, port int) []protocol.TailnetPeer
}

// tailnetAddr is the address the tailnet listener takes: the daemon's own port, with no host, so
// tsnet resolves it to the node's own tailnet address and nothing else.
//
// It is deliberately not built from an IP of this machine. A host of "" on a plain net.Listen
// would be every interface, which docs/architecture.md section 13 forbids; on a tailnet listener
// it is the node's own tailnet address, and that difference is the whole point.
func (s *Server) tailnetAddr() string {
	return net.JoinHostPort("", strconv.Itoa(s.settings.Port))
}

// funnelAddr is where Tailscale Funnel is opened. Funnel answers only on 443, 8443, and 10000,
// and 443 is the one that needs no port in a phone's address bar.
const funnelAddr = ":443"

// funnelAllowedPrefix is the one address the public internet may reach: the webhook routes
// (docs/architecture.md section 13: "Funnel exposes only /hooks/*"). Everything else is refused
// before the handler is asked to route it.
const funnelAllowedPrefix = "/hooks/"

// hooksOnly answers with the daemon's own routes for one path and with not_found for every other.
// The path is checked twice, once as it was sent and once cleaned, because a path that walks out
// of /hooks with ".." would otherwise be judged by the prefix and routed by the cleaned path.
func (s *Server) hooksOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.URL.Path
		if strings.HasPrefix(raw, funnelAllowedPrefix) && strings.HasPrefix(path.Clean(raw), funnelAllowedPrefix) {
			next.ServeHTTP(w, r)
			return
		}
		s.writeError(w, protocol.NewError(protocol.ErrorCodeNotFound,
			"Marshal has nothing at that address. Check the address and try again."))
	})
}

// FunnelHandler returns the routes the public internet may reach through Funnel: the webhook
// routes and nothing else, each still signature-verified the way Phase 8 built them. It is a
// handler of its own rather than the same one with a flag, because "which paths exist" is a rule
// that has to hold no matter who calls.
func (s *Server) FunnelHandler() http.Handler {
	return s.hooksOnly(s.handler())
}

// tailnetStatus answers GET /v1/tailnet (B9.1, build-plan task 9.9): whether this daemon was
// started to join a tailnet, what its node is doing, and whether /hooks/* was asked to be public.
// It answers at once with "off" on a daemon that was not, so the status screen never waits.
func (s *Server) tailnetStatus(w http.ResponseWriter, r *http.Request) {
	status := protocol.TailnetStatus{
		Enabled: s.tailnet != nil,
		State:   "off",
		Funnel:  s.funnel,
	}
	if s.tailnet != nil {
		status = s.tailnet.Status()
		status.Funnel = s.funnel
	}
	status.Port = s.settings.Port
	status.Host = protocol.EmptyTailnetHost()
	if s.hostTailscale != nil {
		status.Host = s.hostTailscale.Status(r.Context(), s.settings.Port)
	}
	s.writeJSON(w, http.StatusOK, protocol.NewTailnetStatus(status, s.now()))
}

// tailnetPeers answers GET /v1/tailnet/peers (B9.5): the other machines on this tailnet, and
// which of them are Marshal daemons. It answers with an empty list on a daemon that was not
// started to join a tailnet, because such a machine has no peers rather than having a fault.
//
// The list is read every time it is asked for rather than cached, because a machine joining or
// leaving the network is exactly the thing a person is looking at the screen to find out.
func (s *Server) tailnetPeers(w http.ResponseWriter, r *http.Request) {
	peers := []protocol.TailnetPeer{}
	if node, ok := s.tailnet.(TailnetPeers); ok {
		peers = node.Peers(r.Context(), s.settings.Port)
		if peers == nil {
			peers = []protocol.TailnetPeer{}
		}
	}
	s.writeJSON(w, http.StatusOK, protocol.NewTailnetPeers(peers, s.now()))
}

// tailnetOrigins lists the hosts besides the request's own host that name this daemon's own node:
// its MagicDNS name and its tailnet addresses. It is what lets a page opened over the tailnet
// reach the daemon's event stream even when it is addressed by a different name than the one it
// loaded from - and no other host is added to the list, so a page from anywhere else is still
// refused (stream_origin.go).
func (s *Server) tailnetOrigins() []string {
	if s.tailnet == nil {
		return nil
	}
	status := s.tailnet.Status()
	if status.DNSName == "" && len(status.IPs) == 0 {
		return nil
	}
	origins := make([]string, 0, len(status.IPs)*2+2)
	if status.DNSName != "" {
		origins = append(origins, status.DNSName, status.DNSName+":*")
	}
	for _, ip := range status.IPs {
		origins = append(origins, ip, ip+":*")
	}
	return origins
}
