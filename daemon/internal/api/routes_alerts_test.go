package api_test

import (
	"net/http"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Where each kind of alert goes, over the API (B9.4). The choices are the notification router's; the
// routes only read and write them, and refuse a choice the screen does not offer.

func TestAlertSettingsStartAtTheDefaultsAndKeepAChange(t *testing.T) {
	st := newStack(t)
	fresh := decode[protocol.AlertSettings](t, st.do(http.MethodGet, "/v1/settings/alerts", nil).want(t, http.StatusOK))
	if len(fresh.Routes) != 5 || len(fresh.Channels) != 3 {
		t.Fatalf("a fresh daemon answers %d alerts and %d channels, want 5 and 3", len(fresh.Routes), len(fresh.Channels))
	}
	change := protocol.SaveAlertSettingsRequest{Routes: []protocol.AlertRouteChoice{
		{Event: "card.done", Channels: []string{"ntfy"}},
	}}
	saved := decode[protocol.AlertSettings](t, st.do(http.MethodPut, "/v1/settings/alerts", change).want(t, http.StatusOK))
	again := decode[protocol.AlertSettings](t, st.do(http.MethodGet, "/v1/settings/alerts", nil).want(t, http.StatusOK))
	for _, got := range []protocol.AlertSettings{saved, again} {
		for _, route := range got.Routes {
			if route.Event == "card.done" && (len(route.Channels) != 1 || route.Channels[0] != "ntfy") {
				t.Errorf("a card finishing goes to %v, want ntfy", route.Channels)
			}
		}
	}
}

func TestAnAlertChoiceTheScreenDoesNotOfferIsRefused(t *testing.T) {
	st := newStack(t)
	st.do(http.MethodPut, "/v1/settings/alerts", protocol.SaveAlertSettingsRequest{Routes: []protocol.AlertRouteChoice{
		{Event: "card.done", Channels: []string{"fax"}},
	}}).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
}
