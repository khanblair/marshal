package api

import (
	"context"
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// AlertSettings is the part of the notification router the alert settings routes need: where each
// kind of alert goes, and a way to change it. The API layer defines it rather than importing
// internal/notify, so the routes are tested with a fake and no router.
type AlertSettings interface {
	// Get answers where every kind of alert goes now, and which channels can be used.
	Get(ctx context.Context) (protocol.AlertSettings, error)
	// Save keeps the choices, applies them, and answers the settings as they now are.
	Save(ctx context.Context, req protocol.SaveAlertSettingsRequest) (protocol.AlertSettings, error)
}

// getAlertSettings is GET /v1/settings/alerts: which channel each kind of alert goes to.
func (s *Server) getAlertSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.alerts.Get(r.Context())
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, settings)
}

// setAlertSettings is PUT /v1/settings/alerts: change where some alerts go. A choice of an alert or
// a channel Marshal does not offer is refused, and nothing is changed.
func (s *Server) setAlertSettings(w http.ResponseWriter, r *http.Request) {
	var req protocol.SaveAlertSettingsRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	saved, err := s.alerts.Save(r.Context(), req)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, saved)
}
