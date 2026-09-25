package api

import (
	"mime"
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const jsonMediaType = "application/json"

// limitBody stops a request body from growing past the limit. A body over the limit reads as an
// error, and decodeJSON turns that into a plain answer. A declared length that is already too
// large is refused before anything is read.
func (s *Server) limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > s.limits.MaxBodyBytes {
			s.writeError(w, errBodyTooLarge(s.limits.MaxBodyBytes))
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, s.limits.MaxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

// requireJSON refuses a request that has a body that is not marked as JSON. A request without a
// body passes, so a GET or a DELETE needs no header. A length of 0 means no body, and -1 means a
// body of unknown length.
func (s *Server) requireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength != 0 {
			mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mediaType != jsonMediaType {
				s.writeError(w, protocol.InvalidArgument(
					"Marshal reads JSON only. Send the body as JSON with the header Content-Type: application/json."))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// jsonBody applies the rules for a route that reads JSON: the size limit and the content type.
func (s *Server) jsonBody(next http.Handler) http.Handler {
	return s.limitBody(s.requireJSON(next))
}
