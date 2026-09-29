package protocol

import "time"

// TailnetStatus is the answer to GET /v1/tailnet: what the daemon's own node on the person's
// tailnet is doing (B9.1, B9.2, build-plan task 9.9). It is the state the tailnet and Funnel
// status screens draw, and what the profile's Tailscale identity is filled from.
//
// State is one of four words, in the order a node goes through them:
//
//   - "off": the daemon was not started to join a tailnet, so it is reachable on this machine
//     only. This is the default.
//   - "signing-in": the node exists but nobody has signed it in yet. The daemon prints a login
//     address as LoginURL; until it is opened, the node is not on the tailnet.
//   - "online": the node is on the tailnet and the daemon serves on it.
//   - "error": joining failed. Error says what happened, in words a person can act on.
type TailnetStatus struct {
	// Enabled is true when the daemon was started to join a tailnet.
	Enabled bool `json:"enabled"`
	// State is "off", "signing-in", "online", or "error".
	State string `json:"state"`
	// Hostname is the node's name on the tailnet.
	Hostname string `json:"hostname"`
	// DNSName is the node's full MagicDNS name, such as "marshal.tailnet-name.ts.net". Empty
	// until the node is online.
	DNSName string `json:"dnsName"`
	// IPs are the node's tailnet addresses, IPv4 first. Empty until the node is online.
	IPs []string `json:"ips"`
	// Identity is the Tailscale account this machine is signed in as, such as
	// "blair@example.com". Empty until the node is online.
	Identity string `json:"identity"`
	// LoginURL is the address a person opens to sign this node in, while State is "signing-in".
	// Empty at every other time.
	LoginURL string `json:"loginUrl"`
	// Funnel is true when the daemon was started to expose /hooks/* through Funnel. It says what
	// was asked for, not what Tailscale has allowed: Funnel also has to be turned on for the
	// tailnet, in the Tailscale admin console.
	Funnel bool `json:"funnel"`
	// Error is what went wrong, while State is "error". Empty at every other time.
	Error string `json:"error"`
	// ServerTime is the daemon's clock when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}

// NewTailnetStatus makes an answer stamped with the daemon's time.
func NewTailnetStatus(status TailnetStatus, now time.Time) TailnetStatus {
	status.ServerTime = NewTimestamp(now)
	return status
}

// TailnetPeer is one other machine on the tailnet and whether it is a Marshal daemon (B9.5).
//
// The Marshal field is why this type exists: Tailscale lists every machine on the network, and
// only Marshal's own health answer separates the daemons worth talking to from the rest. Address
// is the base URL another daemon would be reached at.
type TailnetPeer struct {
	Host    string `json:"host"`
	Address string `json:"address"`
	OS      string `json:"os,omitempty"`
	Online  bool   `json:"online"`
	Marshal bool   `json:"marshal"`
}

// TailnetPeerList is GET /v1/tailnet/peers (B9.5): every other machine on the tailnet, newest
// answers only - the list is read rather than kept, so there is nothing here but the machines.
type TailnetPeerList struct {
	Peers []TailnetPeer `json:"peers"`
	// ServerTime is the daemon's clock when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}

// NewTailnetPeers makes a peer list stamped with the daemon's time.
func NewTailnetPeers(peers []TailnetPeer, now time.Time) TailnetPeerList {
	return TailnetPeerList{Peers: peers, ServerTime: NewTimestamp(now)}
}
