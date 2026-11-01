package api_test

import (
	"net/http"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A server with no monitor says the internet is reachable, because nothing could say otherwise.
func TestTheNetworkStatusReadsOnlineWithNoMonitor(t *testing.T) {
	st := newStack(t)
	r := st.do(http.MethodGet, "/v1/network", nil).want(t, http.StatusOK)
	sameShape(t, "network-status", r.Body)
	if got := decode[protocol.NetworkStatus](t, r); !got.Online {
		t.Errorf("status = %+v, want online with no monitor", got)
	}
}

// The route is behind the token like every domain route.
func TestTheNetworkStatusNeedsTheToken(t *testing.T) {
	st := newStack(t)
	st.doWith("", http.MethodGet, "/v1/network", nil).want(t, http.StatusUnauthorized)
}
