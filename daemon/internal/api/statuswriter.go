package api

import "net/http"

// statusWriter remembers the status a handler sent, for the access log and for the panic
// recovery, which needs to know whether an answer has started.
//
// It has Unwrap so that http.ResponseController and the WebSocket library can reach the real
// writer to take over the connection. Without it, upgrading to a WebSocket fails.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(status int) {
	if !w.wrote {
		w.status, w.wrote = status, true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(data []byte) (int, error) {
	if !w.wrote {
		w.status, w.wrote = http.StatusOK, true
	}
	return w.ResponseWriter.Write(data)
}

// Unwrap returns the writer this one wraps.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// answerStarted reports whether the handler has begun to answer.
func answerStarted(w http.ResponseWriter) bool {
	sw, ok := w.(*statusWriter)
	return ok && sw.wrote
}
