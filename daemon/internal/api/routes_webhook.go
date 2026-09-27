package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	githubapp "github.com/khanblair/marshal/daemon/internal/integrations/github"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// webhookAck is what POST /hooks/github answers with. It is deliberately not a protocol type and
// has no golden: nobody in Marshal reads it. GitHub needs only a 2xx to stop redelivering, and a
// small body makes a delivery that is being debugged by hand readable in `curl`.
type webhookAck struct {
	OK bool `json:"ok"`
}

// githubWebhook is POST /hooks/github (B6.1, build-plan 6.1). GitHub cannot carry a bearer token, so
// the route is unauthenticated (the signedWebhook category) and is authorized by the delivery's own
// signature over its raw body instead. The signature is checked before the body is read as anything
// else, and a delivery that does not verify is refused and never reaches the CI monitor.
//
// The three failures are answered differently on purpose. A missing or wrong signature is a 401: it
// was not GitHub, or the secret is not the same on both sides, and both are "check the secret". A
// delivery with no event header is a 400: it cannot be routed. Anything else is the daemon's own
// failure and goes out in the one error shape.
func (s *Server) githubWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(w, protocol.InvalidArgument(
			"This delivery is larger than Marshal will read.").
			With("max_bytes", strconv.Itoa(githubapp.MaxBodyBytes)))
		return
	}
	if err := s.webhooks.Receive(r.Context(), r.Header, body); err != nil {
		switch {
		case errors.Is(err, githubapp.ErrNoSecret):
			s.writeError(w, protocol.NewError(protocol.ErrorCodeUnauthorized,
				"Marshal has no webhook secret saved yet, so it cannot check this delivery. Save the GitHub App's webhook secret in Settings."))
		case errors.Is(err, githubapp.ErrNoSignature), errors.Is(err, githubapp.ErrBadSignature):
			s.writeError(w, protocol.NewError(protocol.ErrorCodeUnauthorized,
				"This delivery's signature does not match. Check that the webhook secret is the same on both sides."))
		case errors.Is(err, githubapp.ErrNoEvent):
			s.writeError(w, protocol.InvalidArgument(
				"This delivery does not say what kind of event it is.").
				With("header", githubapp.EventHeader))
		default:
			s.writeError(w, err)
		}
		return
	}
	s.writeJSON(w, http.StatusOK, webhookAck{OK: true})
}
