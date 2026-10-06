package api

import (
	"mime"
	"net/http"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const jsonMediaType = "application/json"

// commentBodyBytes is the size a new comment's body may reach: files travel inside it as base64, so
// the 6 MiB a comment may keep in files (protocol.MaxCommentFileBytes) needs 8 MiB and some room.
const commentBodyBytes = 9 << 20

// googleBodyBytes is the size a request that makes a Google file may reach: Marshal saves up to 2 MB
// of content (integrations.maxContentBytes), and JSON escaping can make that larger on the wire.
const googleBodyBytes = 8 << 20

// bodyLimitOf is the size limit of this request's body. Only posting a comment, and making a Google
// file, may exceed the ordinary limit, and only up to what they need.
func (s *Server) bodyLimitOf(r *http.Request) int64 {
	if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/cards/") && strings.HasSuffix(r.URL.Path, "/comments") {
		return max(s.limits.MaxBodyBytes, commentBodyBytes)
	}
	if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/google/") {
		return max(s.limits.MaxBodyBytes, googleBodyBytes)
	}
	return s.limits.MaxBodyBytes
}

// limitBody stops a request body from growing past the limit. A body over the limit reads as an
// error, and decodeJSON turns that into a plain answer. A declared length that is already too
// large is refused before anything is read.
func (s *Server) limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := s.bodyLimitOf(r)
		if r.ContentLength > limit {
			s.writeError(w, errBodyTooLarge(limit))
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
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
