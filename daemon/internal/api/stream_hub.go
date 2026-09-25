package api

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// hub keeps track of the open event streams. net/http does not track a connection once a handler
// has taken it over, so the hub does two jobs that the server cannot: it refuses more streams than
// the limit, and at shutdown it closes every stream and waits until each handler has finished.
type hub struct {
	bus    *events.Bus
	limits Limits
	log    *slog.Logger
	now    func() time.Time

	mu       sync.Mutex // guards streams and closing
	streams  map[*stream]struct{}
	closing  bool
	handlers sync.WaitGroup // one for each admitted stream, and one for each close in progress
}

func newHub(bus *events.Bus, limits Limits, log *slog.Logger, now func() time.Time) *hub {
	return &hub{bus: bus, limits: limits, log: log, now: now, streams: make(map[*stream]struct{})}
}

// admit makes room for one stream, or says why there is none. Every admitted stream must be
// released.
func (h *hub) admit(caller Caller, requestID string) (*stream, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closing {
		return nil, protocol.Unavailable("Marshal is shutting down. Open the app again in a moment.")
	}
	if len(h.streams) >= h.limits.MaxConnections {
		return nil, protocol.Unavailable("Marshal has too many live connections open. Close one and try again.")
	}
	st := &stream{
		hub:    h,
		caller: caller,
		log:    h.log.With("device_id", caller.DeviceID, "request_id", requestID),
	}
	h.streams[st] = struct{}{}
	h.handlers.Add(1)
	return st, nil
}

func (h *hub) release(st *stream) {
	h.mu.Lock()
	delete(h.streams, st)
	h.mu.Unlock()
	h.handlers.Done()
}

// closeAll refuses new streams and closes every open one with the status. The closes run side by
// side, so many clients cost the time of one, and each is cut after the close window.
func (h *hub) closeAll(code websocket.StatusCode, reason string) {
	h.mu.Lock()
	h.closing = true
	open := make([]*stream, 0, len(h.streams))
	for st := range h.streams {
		open = append(open, st)
	}
	h.mu.Unlock()
	for _, st := range open {
		st.requestClose(code, reason)
	}
}

// wait blocks until every handler and every close has finished. When the context ends first, the
// connections that are left are cut, which ends their handlers at once.
func (h *hub) wait(ctx context.Context) error {
	finished := make(chan struct{})
	go func() {
		h.handlers.Wait()
		close(finished)
	}()
	select {
	case <-finished:
		return nil
	case <-ctx.Done():
	}
	h.mu.Lock()
	for st := range h.streams {
		st.cut()
	}
	h.mu.Unlock()
	<-finished
	return errors.New("some event streams did not close in time and were cut")
}

// stream is one connected client. The hub knows it from admission until the handler ends.
type stream struct {
	hub    *hub
	caller Caller
	log    *slog.Logger

	mu       sync.Mutex // guards the three fields below
	ws       *websocket.Conn
	closeReq *closeRequest
}

// closeRequest is a status that the hub asked the stream to close with.
type closeRequest struct {
	code   websocket.StatusCode
	reason string
}

// attach gives the stream its connection. If the hub asked for a close before the connection
// existed, the close happens now.
func (st *stream) attach(ws *websocket.Conn) {
	st.mu.Lock()
	st.ws = ws
	req := st.closeReq
	st.mu.Unlock()
	if req != nil {
		closeWithin(ws, st.hub.limits.ShutdownCloseWindow, req.code, req.reason)
	}
}

// requestClose closes the stream's connection with a status, without waiting for it.
func (st *stream) requestClose(code websocket.StatusCode, reason string) {
	st.mu.Lock()
	if st.closeReq == nil {
		st.closeReq = &closeRequest{code: code, reason: reason}
	}
	ws := st.ws
	st.mu.Unlock()
	if ws == nil {
		return // attach will see the request
	}
	st.hub.handlers.Add(1)
	go func() {
		defer st.hub.handlers.Done()
		closeWithin(ws, st.hub.limits.ShutdownCloseWindow, code, reason)
	}()
}

// cut closes the connection at once, without a close handshake.
func (st *stream) cut() {
	st.mu.Lock()
	ws := st.ws
	st.mu.Unlock()
	if ws != nil {
		_ = ws.CloseNow()
	}
}

// closeWithin closes a WebSocket with a status and waits for the client to answer, but only for
// the window. After that the connection is cut, so a client that never answers cannot hold up a
// shutdown. An error from the close is not reported: the connection is going away either way.
func closeWithin(ws *websocket.Conn, window time.Duration, code websocket.StatusCode, reason string) {
	timer := time.AfterFunc(window, func() { _ = ws.CloseNow() })
	defer timer.Stop()
	_ = ws.Close(code, reason)
}
