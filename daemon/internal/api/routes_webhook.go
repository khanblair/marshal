package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	githubapp "github.com/khanblair/marshal/daemon/internal/integrations/github"
	"github.com/khanblair/marshal/daemon/internal/integrations/trello"
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

// trelloWebhook is POST /hooks/trello (B8.2, build-plan 8.2). Like GitHub's, it is unauthenticated:
// Trello cannot carry a bearer token, so the delivery is authorized by its own signature instead,
// and the signature is checked before the body is read as anything else. A delivery that does not
// verify is refused and never reaches the sync.
//
// The two halves a Trello signature needs - the secret and the callback URL - come from the stored
// connection, read from the keychain and the row on every delivery, so saving one in Settings takes
// effect without a restart. Both are required: Trello signs over the body followed by the callback
// URL, so without the URL a correct delivery and a forged one are indistinguishable, and Marshal
// says so rather than guessing.
//
// The failures are answered the way GitHub's are. A missing or wrong signature is a 401: it was not
// Trello, or the secret is not the one Trello signs with, and both are "check the secret". Anything
// else is the daemon's own failure and goes out in the one error shape.
func (s *Server) trelloWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(w, protocol.InvalidArgument(
			"This delivery is larger than Marshal will read.").
			With("max_bytes", strconv.Itoa(webhookCeilingBytes)))
		return
	}
	secret, callbackURL, err := s.integrations.TrelloWebhook(r.Context())
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	if secret == "" || callbackURL == "" {
		s.writeError(w, protocol.NewError(protocol.ErrorCodeUnauthorized,
			"Marshal has no Trello webhook saved yet, so it cannot check this delivery. Add the webhook "+
				"secret and callback URL in Settings, under Integrations."))
		return
	}
	signature := r.Header.Get(trello.SignatureHeader)
	if signature == "" {
		s.writeError(w, protocol.NewError(protocol.ErrorCodeUnauthorized,
			"This delivery has no Trello signature, so Marshal cannot tell it came from Trello.").
			With("header", trello.SignatureHeader))
		return
	}
	if !trello.VerifySignature(secret, callbackURL, body, signature) {
		s.writeError(w, protocol.NewError(protocol.ErrorCodeUnauthorized,
			"This delivery's signature does not match. Check that the webhook secret and the callback "+
				"URL are the same on Trello and in Marshal."))
		return
	}
	// A believed delivery is handed to the sync. An empty body is Trello's own "is this webhook
	// alive" call, which carries nothing to act on and is answered rather than refused.
	s.handleTrelloDelivery(w, r, body)
}

// handleTrelloDelivery applies one believed Trello delivery and answers the ack. The body is read as
// a Trello event; a body Marshal cannot read at all is a 400, because there is nothing to hand on and
// the sender is not Trello. Everything the event means - including "nothing" - is an outcome the sync
// names, and an outcome is never a refusal: a delivery Trello believes it delivered, and that Marshal
// refuses, is one Trello will send again and again.
func (s *Server) handleTrelloDelivery(w http.ResponseWriter, r *http.Request, body []byte) {
	if len(bytes.TrimSpace(body)) == 0 {
		s.writeJSON(w, http.StatusOK, webhookAck{OK: true})
		return
	}
	var event trello.WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		s.writeError(w, protocol.InvalidArgument("This delivery is not a Trello event Marshal can read."))
		return
	}
	outcome, err := s.integrations.HandleTrelloDelivery(r.Context(), event)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.log.Info("handled a Trello delivery", "action", event.Action.Type, "outcome", string(outcome))
	s.writeJSON(w, http.StatusOK, webhookAck{OK: true})
}
