// Package tailnet joins a daemon to the person's Tailscale tailnet from inside the process, with
// no separate Tailscale install (B9.1, build-plan task 9.1).
//
// It is deliberately a thin wrapper. Everything the daemon decides about the tailnet - whether to
// join at all, which port to serve on, and whether to expose /hooks/* publicly through Funnel -
// lives in the API layer, so this file holds nothing but the tsnet node and the facts about it.
// The daemon binds loopback first and this listener second, additively: a tailnet that cannot be
// joined never takes the local daemon down (docs/architecture.md section 13).
package tailnet

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tsnet"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The states a node goes through, the same words protocol.TailnetStatus.State documents.
const (
	// StateSigningIn means the node exists but nobody has signed it in yet.
	StateSigningIn = "signing-in"
	// StateOnline means the node is on the tailnet and the daemon serves on it.
	StateOnline = "online"
	// StateError means joining failed.
	StateError = "error"
)

// statusInterval is how often the node's own status is copied into the answer the screens read.
// It is short enough that a sign-in shows up while the person is still looking at the screen, and
// long enough that polling costs nothing.
const statusInterval = 3 * time.Second

// Config says how to join the tailnet.
type Config struct {
	// Dir is where the node's own state lives. It is a folder under the daemon's data folder, so
	// signing in once survives a restart and nothing is written outside it.
	Dir string
	// Hostname is the node's name on the tailnet. Empty leaves the daemon's own program name.
	Hostname string
	// AuthKey signs the node in without a person opening a browser. Empty means the person signs
	// in through the address the node prints, which is the usual case on a first run.
	AuthKey string
	// Logger receives what the node is doing. It never receives the node's keys.
	Logger *slog.Logger
}

// Node is the daemon's own node on the tailnet. It satisfies the interface the API layer serves
// through, so the API never depends on tsnet.
type Node struct {
	srv      *tsnet.Server
	log      *slog.Logger
	hostname string

	mu     sync.Mutex
	status protocol.TailnetStatus
}

// New makes a node. Nothing is joined until Up is called, so a daemon starts and serves on
// loopback whether or not the tailnet can be reached.
func New(cfg Config) *Node {
	log := cfg.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	n := &Node{log: log, hostname: cfg.Hostname}
	n.status = protocol.TailnetStatus{Enabled: true, State: StateSigningIn, Hostname: cfg.Hostname}
	n.srv = &tsnet.Server{
		Dir:      cfg.Dir,
		Hostname: cfg.Hostname,
		AuthKey:  cfg.AuthKey,
		// The node's own chatter is not a person's business; the sign-in address is read out of
		// the status instead of out of a log line, and the rest is kept at debug level.
		Logf: func(format string, args ...any) {
			log.Debug("tailnet", "message", fmt.Sprintf(format, args...))
		},
	}
	return n
}

// Watch keeps Status current until ctx ends. It is started before Up, because Up does not return
// while a sign-in is still needed, and the status screens have to be able to show the sign-in
// address in the meantime.
func (n *Node) Watch(ctx context.Context) {
	ticker := time.NewTicker(statusInterval)
	defer ticker.Stop()
	for {
		n.refresh(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// refresh reads the node's own status once and copies the parts the screens show.
func (n *Node) refresh(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	client, err := n.srv.LocalClient()
	if err != nil {
		n.fail(fmt.Errorf("reach the tailnet node: %w", err))
		return
	}
	status, err := client.Status(ctx)
	if err != nil {
		n.fail(fmt.Errorf("read the tailnet node's status: %w", err))
		return
	}
	n.record(status)
}

// Up joins the tailnet and waits until the node is running, which is what makes the daemon's
// tailnet listener possible. On a node nobody has signed in yet it does not return until the
// person has opened the sign-in address: Watch is what tells them the address.
func (n *Node) Up(ctx context.Context) (protocol.TailnetStatus, error) {
	status, err := n.srv.Up(ctx)
	if err != nil {
		// A cancelled context is the daemon shutting down, not a tailnet that failed.
		if ctx.Err() == nil {
			n.fail(fmt.Errorf("join the tailnet: %w", err))
		}
		return n.Status(), err
	}
	n.record(status)
	n.log.Info("joined the tailnet", "hostname", n.Status().Hostname, "dns_name", n.Status().DNSName)
	return n.Status(), nil
}

// Status is what the node knows right now, for the status screens.
func (n *Node) Status() protocol.TailnetStatus {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.status
}

// Listen opens a listener on the tailnet alone. tsnet resolves an address with no host to the
// node's own tailnet address, so this never opens anything on this machine's interfaces and never
// on all of them.
func (n *Node) Listen(network, addr string) (net.Listener, error) {
	listener, err := n.srv.Listen(network, addr)
	if err != nil {
		return nil, fmt.Errorf("listen on the tailnet at %s: %w", addr, err)
	}
	return listener, nil
}

// ListenFunnel opens a listener on the public internet through Tailscale Funnel. Tailscale only
// answers on 443, 8443, and 10000, and only for the node's own name, so the caller passes one of
// those ports and the daemon decides which paths are served on it.
func (n *Node) ListenFunnel(network, addr string) (net.Listener, error) {
	listener, err := n.srv.ListenFunnel(network, addr)
	if err != nil {
		return nil, fmt.Errorf("open Funnel at %s: %w", addr, err)
	}
	return listener, nil
}

// Close ends the node and its listeners.
func (n *Node) Close() error {
	if n.srv == nil {
		return nil
	}
	return n.srv.Close()
}

// record copies what the node says into the answer the screens read. The sign-in address is only
// present while one is needed, so a node that has since come up does not keep showing a stale one.
func (n *Node) record(status *ipnstate.Status) {
	report := protocol.TailnetStatus{
		Enabled:  true,
		State:    StateSigningIn,
		Hostname: n.hostname,
		LoginURL: status.AuthURL,
	}
	if status.Self != nil {
		if status.Self.HostName != "" {
			report.Hostname = status.Self.HostName
		}
		report.DNSName = strings.TrimSuffix(status.Self.DNSName, ".")
	}
	if status.Self != nil {
		if profile, ok := status.User[status.Self.UserID]; ok {
			report.Identity = profile.LoginName
		}
	}
	if status.BackendState == ipn.Running.String() || len(status.TailscaleIPs) > 0 {
		report.State = StateOnline
		report.IPs = nodeIPs(status.TailscaleIPs)
	}
	n.mu.Lock()
	n.status = report
	n.mu.Unlock()
}

// fail records that the node is not doing what it should.
func (n *Node) fail(err error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	// A node that is already online is not taken down by one failed poll: a status read that
	// times out says nothing about whether the daemon is serving on the tailnet.
	if n.status.State == StateOnline {
		return
	}
	n.status.State = StateError
	n.status.Error = err.Error()
}

// nodeIPs lists the node's addresses, IPv4 first, so the screen can show the address to type when
// a name does not resolve.
func nodeIPs(addrs []netip.Addr) []string {
	ips := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		if addr.Is4() {
			ips = append(ips, addr.String())
		}
	}
	for _, addr := range addrs {
		if addr.Is6() {
			ips = append(ips, addr.String())
		}
	}
	return ips
}
