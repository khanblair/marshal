package api

import (
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// listDevices is GET /v1/me/devices: the paired-devices list on the profile (B9.2, build-plan 9.2).
// It carries the revoked devices too, so the screen can say one was removed rather than watching
// a row vanish.
func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	caller, ok := Principal(r.Context())
	if !ok {
		s.writeError(w, errUnauthorized())
		return
	}
	list, err := s.devices.List(r.Context(), caller.UserID)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, list)
}

// createPairingCode is POST /v1/me/devices/pairing-code (B9.1, B9.2): it makes the short code the
// person reads off the desktop app and types into the device being paired. Only a device that is
// already signed in may ask for one, which is what makes the code a second factor rather than the
// only one.
func (s *Server) createPairingCode(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, s.devices.IssueCode())
}

// revokeDevice is DELETE /v1/me/devices/{id} (B9.1, B9.2): the device loses access at once, and
// pairing it again is the only way back. A device of someone else is not found rather than
// forbidden, so this answer says nothing about devices that are not this person's.
func (s *Server) revokeDevice(w http.ResponseWriter, r *http.Request) {
	caller, ok := Principal(r.Context())
	if !ok {
		s.writeError(w, errUnauthorized())
		return
	}
	id := r.PathValue("id")
	if err := s.devices.Revoke(r.Context(), caller.UserID, id); err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeNoContent(w)
}

// pairDevice is POST /v1/devices/pair. It is the one route in the daemon that mints a token with
// no token of its own, because a device being paired has none yet - which is the whole point of
// pairing. What authorizes it is the short-lived, single-use code read off a screen on a device
// that is already signed in (internal/devices).
//
// It is deliberately not a domain route: it is registered beside health, outside the token check,
// and it is refused by the Funnel handler along with every other address outside /hooks/, so it is
// reachable only on this machine and on the person's own tailnet (docs/architecture.md section
// 13).
func (s *Server) pairDevice(w http.ResponseWriter, r *http.Request) {
	var req protocol.PairDeviceRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	answer, err := s.devices.Pair(r.Context(), req)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, answer)
}
