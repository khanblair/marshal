package tailnet

import (
	"net/netip"
	"testing"
	"time"

	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A node nobody has signed in to says so, and says nothing it does not know: no addresses, no
// account, no error. That is the state the status screen draws while a person opens the sign-in
// address.
func TestANodeThatHasNotJoinedSaysSigningIn(t *testing.T) {
	n := New(Config{Hostname: "marshal"})
	status := n.Status()
	if !status.Enabled || status.State != StateSigningIn || status.Hostname != "marshal" {
		t.Errorf("status = %+v, want an enabled node that is signing in as marshal", status)
	}
	if status.DNSName != "" || status.Identity != "" || len(status.IPs) != 0 || status.Error != "" {
		t.Errorf("status = %+v, want nothing claimed before anything has happened", status)
	}
}

// The status the node copies is the node's own: its name, its full name, its addresses IPv4 first,
// and the account it is signed in as.
func TestAStatusIsCopiedFromTheNode(t *testing.T) {
	n := New(Config{Hostname: "marshal"})
	n.record(&ipnstate.Status{
		BackendState: "Running",
		AuthURL:      "",
		TailscaleIPs: []netip.Addr{
			netip.MustParseAddr("fd7a:115c:a1e0::1"),
			netip.MustParseAddr("100.64.0.7"),
		},
		Self: &ipnstate.PeerStatus{
			HostName: "marshal", DNSName: "marshal.example.ts.net.",
			UserID:       tailcfg.UserID(1),
			TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.7")},
		},
		User: map[tailcfg.UserID]tailcfg.UserProfile{
			tailcfg.UserID(1): {LoginName: "blair@example.com"},
		},
	})
	status := n.Status()
	if status.State != StateOnline {
		t.Errorf("state = %q, want %q", status.State, StateOnline)
	}
	if status.DNSName != "marshal.example.ts.net" {
		t.Errorf("dnsName = %q; the trailing dot Tailscale puts on it is not part of a name a person reads", status.DNSName)
	}
	if status.Identity != "blair@example.com" {
		t.Errorf("identity = %q, want the account the node joined as", status.Identity)
	}
	if len(status.IPs) != 2 || status.IPs[0] != "100.64.0.7" {
		t.Errorf("ips = %v, want IPv4 first", status.IPs)
	}
	if status.LoginURL != "" || status.Error != "" {
		t.Errorf("status = %+v, want no sign-in address and no error on a node that is online", status)
	}
}

// A node waiting for a sign-in shows the address a person has to open, and nothing that says it
// failed: it has not failed, it has asked.
func TestANodeWaitingForASignInShowsTheAddress(t *testing.T) {
	n := New(Config{Hostname: "marshal"})
	n.record(&ipnstate.Status{
		BackendState: "NeedsLogin",
		AuthURL:      "https://login.tailscale.com/a/abc123",
	})
	status := n.Status()
	if status.State != StateSigningIn {
		t.Errorf("state = %q, want %q", status.State, StateSigningIn)
	}
	if status.LoginURL != "https://login.tailscale.com/a/abc123" {
		t.Errorf("loginUrl = %q", status.LoginURL)
	}
	if status.Error != "" {
		t.Errorf("error = %q, want none: waiting is not failing", status.Error)
	}
	// Once it is running, the address is not carried along as if it were still needed.
	n.record(&ipnstate.Status{BackendState: "Running"})
	if status := n.Status(); status.LoginURL != "" {
		t.Errorf("loginUrl = %q, want it cleared once the node has joined", status.LoginURL)
	}
}

// A node that failed says what went wrong, and a node that is already online is not taken down by
// one failed poll: a status read that times out says nothing about whether the daemon is serving
// on the tailnet.
func TestAFailedPollDoesNotTakeAnOnlineNodeDown(t *testing.T) {
	n := New(Config{})
	n.record(&ipnstate.Status{BackendState: "Running", TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.9")}})
	n.fail(errTest)
	if status := n.Status(); status.State != StateOnline {
		t.Errorf("state = %q, want an online node to stay online through one failed poll", status.State)
	}

	cold := New(Config{})
	cold.fail(errTest)
	status := cold.Status()
	if status.State != StateError || status.Error == "" {
		t.Errorf("status = %+v, want a node that failed to say what went wrong", status)
	}
}

var errTest = errFail("could not reach the node")

type errFail string

func (e errFail) Error() string { return string(e) }

// nodeIPs puts IPv4 first, because that is the address a person types when a name does not
// resolve, and drops anything that is not an address at all.
func TestTheAddressesAreListedIPv4First(t *testing.T) {
	ips := nodeIPs([]netip.Addr{
		netip.MustParseAddr("fd7a:115c:a1e0::1"),
		netip.MustParseAddr("100.64.0.7"),
		netip.MustParseAddr("100.101.102.103"),
	})
	want := []string{"100.64.0.7", "100.101.102.103", "fd7a:115c:a1e0::1"}
	if len(ips) != len(want) {
		t.Fatalf("ips = %v, want %v", ips, want)
	}
	for i := range want {
		if ips[i] != want[i] {
			t.Errorf("ips[%d] = %q, want %q", i, ips[i], want[i])
		}
	}
	if got := nodeIPs(nil); len(got) != 0 {
		t.Errorf("nodeIPs(nil) = %v, want none", got)
	}
}

// The answer the API sends is stamped with the daemon's clock, so a client can tell a fresh answer
// from one it is holding.
func TestTheStatusCarriesTheDaemonClock(t *testing.T) {
	status := protocol.NewTailnetStatus(protocol.TailnetStatus{State: StateOnline},
		time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC))
	if status.ServerTime.Time().IsZero() {
		t.Error("the answer carries no server time")
	}
	if !status.ServerTime.Time().Equal(time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("serverTime = %v, want the clock it was stamped with", status.ServerTime.Time())
	}
	if status.State != StateOnline {
		t.Errorf("state = %q, want the state that was passed in", status.State)
	}
}
