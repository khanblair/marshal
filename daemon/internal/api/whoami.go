package api

import (
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// whoami tells a client which device and user its token belongs to. It is how a client checks
// that a token still works.
func (s *Server) whoami(w http.ResponseWriter, r *http.Request) {
	caller, ok := Principal(r.Context())
	if !ok {
		// The route is protected, so this cannot happen. If it did, the answer must not be a guess.
		s.writeError(w, errUnauthorized())
		return
	}
	s.writeJSON(w, http.StatusOK, protocol.WhoAmI{
		DeviceID:   caller.DeviceID,
		DeviceKind: caller.DeviceKind,
		UserID:     caller.UserID,
		Mode:       string(s.settings.Mode),
		ServerTime: protocol.NewTimestamp(s.now()),
	})
}
