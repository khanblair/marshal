package api

import (
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/netwatch"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// NetworkSource says whether this computer can reach the internet. The daemon's is the monitor in
// internal/netwatch, which only checks after a call failed.
type NetworkSource interface {
	Status() netwatch.Status
}

// NetworkStatusOf is a monitor's status as it goes on the wire, for the route and for the event.
func NetworkStatusOf(s netwatch.Status) protocol.NetworkStatus {
	return protocol.NetworkStatus{Online: s.Online, Since: protocol.NewTimestamp(s.Since)}
}

// networkStatus is GET /v1/network: whether the internet is reachable, and since when. A server with
// no monitor answers online since it started, because nothing could have said otherwise.
func (s *Server) networkStatus(w http.ResponseWriter, _ *http.Request) {
	if s.network == nil {
		s.writeJSON(w, http.StatusOK, protocol.NetworkStatus{Online: true, Since: protocol.NewTimestamp(s.now())})
		return
	}
	s.writeJSON(w, http.StatusOK, NetworkStatusOf(s.network.Status()))
}
