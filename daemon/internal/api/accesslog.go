package api

import "net/http"

// withAccessLog logs one line for each request when it ends: the method, the path without its
// query, the status, how long it took, and the request id. It never logs headers, bodies, or
// the query, because those can hold tokens and text the person typed. A WebSocket is logged when
// it closes, with the status 101 it was upgraded with.
func (s *Server) withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := s.now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			s.log.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"duration_ms", s.now().Sub(start).Milliseconds(),
				"request_id", requestID(r.Context()),
			)
		}()
		next.ServeHTTP(sw, r)
	})
}
