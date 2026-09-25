package api

import (
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// withRecovery turns a panic in a handler into an `internal` answer. The panic and its stack go
// to the log at error level and never to the client. A panic that is http.ErrAbortHandler is
// passed on, because that is how a handler asks the server to drop the connection quietly.
func (s *Server) withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			value := recover()
			if value == nil {
				return
			}
			if err, ok := value.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(value)
			}
			s.log.Error("a handler panicked",
				"method", r.Method,
				"path", r.URL.Path,
				"request_id", requestID(r.Context()),
				"panic", fmt.Sprint(value),
				"stack", string(debug.Stack()),
			)
			// Once the answer has started, the status is already sent and nothing more can be said.
			if !answerStarted(w) {
				s.writeError(w, protocol.Internal())
			}
		}()
		next.ServeHTTP(w, r)
	})
}
