package api

import (
	"context"
	"crypto/rand"
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// requestIDHeader carries the id of a request back to the client, so a person can quote it and
// the log line for that request can be found.
const requestIDHeader = "X-Request-Id"

type contextKey int

const (
	requestIDKey contextKey = iota
	callerKey
)

// withRequestID gives every request an id made by the server. A header from the client is never
// trusted, so the id in the log is always one this daemon made.
func (s *Server) withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := protocol.NewID(s.now(), rand.Reader)
		if err != nil {
			// Without an id the request still works. The log line just has none.
			s.log.Error("make a request id", "error", err)
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// requestID returns the id that withRequestID gave the request, or "" when there is none.
func requestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}
