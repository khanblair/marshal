package api

import (
	"bufio"
	"net"
	"net/http"
)

// hijackRecorder is the http.ResponseWriter handed to the WebSocket upgrade. It does nothing to
// the answer itself; it remembers the raw connection that the upgrade hijacks, because that is the
// only thing that can end a connection whose close handshake is already waiting.
//
// Why this is needed: `(*websocket.Conn).Close` writes a close frame and then waits up to five
// seconds for the peer's. The library lets only the first close call do any work (`casClosing` in
// its `close.go`), so a later `CloseNow` just waits for that same handshake and cannot cut the
// connection. Closing the raw connection ends the wait at once, which is what a shutdown does to a
// client that has stopped reading.
type hijackRecorder struct {
	http.ResponseWriter
	conn net.Conn
}

// Hijack takes the connection over through the writer underneath, and keeps a copy of it. The
// writer underneath is the one the server gave the handler, which may itself be wrapped (the
// access log's `statusWriter`), so the search unwraps until it finds a real hijacker.
func (h *hijackRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := hijackerIn(h.ResponseWriter)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	conn, rw, err := hj.Hijack()
	if err == nil {
		h.conn = conn
	}
	return conn, rw, err
}

// Unwrap lets the WebSocket library and http.ResponseController reach the writer this one wraps,
// the same way statusWriter does.
func (h *hijackRecorder) Unwrap() http.ResponseWriter { return h.ResponseWriter }

// hijackerIn finds the http.Hijacker behind any number of wrappers that expose Unwrap.
func hijackerIn(w http.ResponseWriter) (http.Hijacker, bool) {
	for {
		switch typed := w.(type) {
		case http.Hijacker:
			return typed, true
		case interface{ Unwrap() http.ResponseWriter }:
			w = typed.Unwrap()
		default:
			return nil, false
		}
	}
}
