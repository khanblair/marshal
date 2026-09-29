package tailnet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// peersHealthPath is the one public route every Marshal daemon answers, so it is how a machine on
// the tailnet tells a Marshal daemon from the other machines on the same network (B9.5).
const peersHealthPath = "/v1/health"

// peersProbeTimeout bounds one health probe. A machine that is asleep, off, or not a Marshal
// daemon has to answer before this or it is simply marked not one, which is the answer either way.
const peersProbeTimeout = 600 * time.Millisecond

// peersConcurrency caps how many probes are in the air at once, so listing the tailnet never
// becomes a burst of dozens of requests the moment a network is large.
const peersConcurrency = 8

// Peers lists the other machines on the tailnet and marks the ones that answer as a Marshal
// daemon. The probe is the whole point: Tailscale knows every machine on the network, and only
// Marshal's own health answer says which of them is another Marshal worth talking to (B9.5).
//
// It returns no peers and no error when the node is not up, because a machine that is not on the
// tailnet has no peers rather than having a problem.
func (n *Node) Peers(ctx context.Context, port int) []protocol.TailnetPeer {
	if n == nil {
		return nil
	}
	client, err := n.srv.LocalClient()
	if err != nil {
		return nil
	}
	st, err := client.Status(ctx)
	if err != nil || st == nil {
		return nil
	}

	out := make([]protocol.TailnetPeer, 0, len(st.Peer))
	for _, ps := range st.Peer {
		if ps == nil || len(ps.TailscaleIPs) == 0 || !ps.Online {
			continue
		}
		addr := netip.AddrPortFrom(ps.TailscaleIPs[0], uint16(port)).String()
		out = append(out, protocol.TailnetPeer{
			Host:    ps.HostName,
			Address: "http://" + addr,
			OS:      ps.OS,
			Online:  ps.Online,
		})
	}
	probeMarshal(ctx, out)
	return out
}

// probeMarshal fills in which of those machines are actually Marshal daemons. It runs the probes
// a few at a time under one context, and a peer it cannot reach in the time given is a peer it
// leaves unmarked rather than one that fails the whole list.
func probeMarshal(ctx context.Context, peers []protocol.TailnetPeer) {
	sem := make(chan struct{}, peersConcurrency)
	var wg sync.WaitGroup
	for i := range peers {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			peers[i].Marshal = isMarshal(ctx, peers[i].Address+peersHealthPath)
		}(i)
	}
	wg.Wait()
}

// isMarshal is the single health probe.
func isMarshal(ctx context.Context, url string) bool {
	ctx, cancel := context.WithTimeout(ctx, peersProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var body protocol.Health
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false
	}
	return body.Status == "ok"
}
