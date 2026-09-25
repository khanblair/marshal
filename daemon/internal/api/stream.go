package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/sync/errgroup"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// eventStream is GET /v1/events. It checks the origin, then the token, then makes room for the
// connection, and only then upgrades. Every refusal before the upgrade is in the error shape.
//
// The server's read and write timeouts do not end a WebSocket: net/http clears the connection's
// deadlines when a handler takes it over, and a test keeps that true.
func (s *Server) eventStream(w http.ResponseWriter, r *http.Request) {
	if !s.originAllowed(r) {
		s.writeError(w, errOriginRefused())
		return
	}
	offered := offeredSubprotocols(r)
	caller, err := s.auth.authenticate(r.Context(), streamToken(r, offered), r.RemoteAddr)
	if err != nil {
		s.denyAccess(w, err)
		return
	}
	if !isUpgrade(r) || !slices.Contains(offered, protocol.WebSocketSubprotocol) {
		s.writeError(w, protocol.InvalidArgument(
			"This address is for a live connection. Open it as a WebSocket and offer the marshal.v1 protocol."))
		return
	}
	st, err := s.hub.admit(caller, requestID(r.Context()))
	if err != nil {
		s.writeError(w, err)
		return
	}
	defer s.hub.release(st)
	// Only marshal.v1 is offered back. The client's other subprotocol carries its token and is
	// never echoed. The recorder keeps the raw connection so a shutdown can end a client that
	// stops reading (see hijack.go).
	recorder := &hijackRecorder{ResponseWriter: w}
	ws, err := websocket.Accept(recorder, r, &websocket.AcceptOptions{
		Subprotocols:   []string{protocol.WebSocketSubprotocol},
		OriginPatterns: s.originPatterns(),
	})
	if err != nil {
		st.log.Debug("the upgrade to a WebSocket was refused", "error", err)
		return
	}
	st.serve(context.WithoutCancel(r.Context()), ws, recorder.conn)
}

// serve runs a connection until it ends and logs how it ended.
func (st *stream) serve(ctx context.Context, ws *websocket.Conn, conn net.Conn) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	start := st.hub.now()
	st.attach(ws, conn)
	ws.SetReadLimit(st.hub.limits.ClientMessageBytes)
	st.log.Info("event stream opened")
	cause := st.run(ctx, ws)
	_ = ws.CloseNow() // Ends the connection if it is still open. It is safe after a close.
	st.log.Info("event stream closed",
		"duration_ms", st.hub.now().Sub(start).Milliseconds(),
		"reason", describeEnd(cause),
	)
}

// run reads the Hello, starts the pump, and runs the three goroutines of a connection until the
// first of them ends: the reader (which also lets pings be answered), the pump (which owns every
// write of events), and the pinger. Their group has one owner, this function, and one way to
// stop, the context.
func (st *stream) run(ctx context.Context, ws *websocket.Conn) error {
	hello, err := st.readHello(ctx, ws)
	if err != nil {
		return err
	}
	p, err := newPump(ctx, st, ws, hello)
	if err != nil {
		return err
	}
	defer p.close()
	hellos := make(chan protocol.Hello)
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return st.readLoop(groupCtx, ws, hellos) })
	group.Go(func() error { return p.run(groupCtx, hellos) })
	group.Go(func() error { return st.pingLoop(groupCtx, ws) })
	return group.Wait()
}

// readHello reads the first message, which must be a Hello and must arrive in time. When the time
// runs out the client is closed with a status, so it learns why.
func (st *stream) readHello(ctx context.Context, ws *websocket.Conn) (protocol.Hello, error) {
	limits := st.hub.limits
	expired := make(chan struct{})
	timer := time.AfterFunc(limits.HelloTimeout, func() {
		defer close(expired)
		st.closeWithStatus(ws, websocket.StatusPolicyViolation, "Send a hello message first.")
	})
	kind, data, err := ws.Read(ctx)
	if !timer.Stop() {
		<-expired // the close has started, so wait for it to finish
	}
	if err != nil {
		return protocol.Hello{}, fmt.Errorf("read the first message: %w", err)
	}
	hello, perr := st.parseHello(kind, data)
	if perr != nil {
		return protocol.Hello{}, st.refuse(ctx, ws, perr)
	}
	return hello, nil
}

// readLoop reads every later message. Each one is a new Hello, which it passes to the pump.
func (st *stream) readLoop(ctx context.Context, ws *websocket.Conn, hellos chan<- protocol.Hello) error {
	for {
		kind, data, err := ws.Read(ctx)
		if err != nil {
			return fmt.Errorf("read a message: %w", err)
		}
		hello, perr := st.parseHello(kind, data)
		if perr != nil {
			return st.refuse(ctx, ws, perr)
		}
		select {
		case hellos <- hello:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// parseHello checks a client message. The errors are for the client, so they are plain sentences.
func (st *stream) parseHello(kind websocket.MessageType, data []byte) (protocol.Hello, *protocol.Error) {
	var hello protocol.Hello
	switch {
	case kind != websocket.MessageText:
		return hello, protocol.InvalidArgument("Marshal reads text messages on this connection. Send the hello as JSON text.")
	case json.Unmarshal(data, &hello) != nil:
		return hello, protocol.InvalidArgument("That message is not valid JSON. Send a hello message as JSON.")
	case hello.Type != protocol.FrameTypeHello:
		return hello, protocol.InvalidArgument("Marshal expected a hello message. Send a message with the type hello.")
	case len(hello.Subscribe) > st.hub.limits.MaxHelloTopics:
		return hello, protocol.InvalidArgument(
			fmt.Sprintf("That hello follows more than %d topics. Follow fewer topics.", st.hub.limits.MaxHelloTopics))
	}
	for _, topic := range hello.Subscribe {
		if _, _, err := protocol.ParseTopic(topic); err != nil {
			return hello, protocol.InvalidArgument(
				"That topic is not one Marshal knows. Check the topic and try again.").With("topic", string(topic))
		}
	}
	return hello, nil
}

// refuse tells the client what it did wrong in an error frame, then closes with the policy
// violation status. The returned error ends the connection's goroutines.
func (st *stream) refuse(ctx context.Context, ws *websocket.Conn, perr *protocol.Error) error {
	// A client that is already gone cannot be told, and the close below still runs.
	if err := st.send(ctx, ws, protocol.ErrorFrame{Error: *perr}); err != nil {
		st.log.Debug("send an error frame", "error", err)
	}
	closeWithin(ws, st.rawConn(), st.hub.limits.ShutdownCloseWindow, websocket.StatusPolicyViolation, "Marshal could not use that message.")
	return perr
}

// rawConn is the connection behind the stream's WebSocket, or nil before it was attached.
func (st *stream) rawConn() net.Conn {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.conn
}

// closeWithStatus closes the stream's connection with a status, cutting it after the window.
func (st *stream) closeWithStatus(ws *websocket.Conn, code websocket.StatusCode, reason string) {
	closeWithin(ws, st.rawConn(), st.hub.limits.ShutdownCloseWindow, code, reason)
}

// send writes one frame as a text message. A client that takes longer than the frame timeout is
// treated as stuck: the library closes the connection when the write's context expires.
func (st *stream) send(ctx context.Context, ws *websocket.Conn, frame any) error {
	data, err := json.Marshal(frame)
	if err != nil {
		return fmt.Errorf("encode a frame: %w", err)
	}
	writeCtx, cancel := context.WithTimeout(ctx, st.hub.limits.FrameWriteTimeout)
	defer cancel()
	if err := ws.Write(writeCtx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("write a frame: %w", err)
	}
	return nil
}

// pingLoop pings the client on an interval and ends the connection when a pong does not come in
// time. The reader goroutine is what receives the pong.
func (st *stream) pingLoop(ctx context.Context, ws *websocket.Conn) error {
	limits := st.hub.limits
	ticker := time.NewTicker(limits.PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		pingCtx, cancel := context.WithTimeout(ctx, limits.PongTimeout)
		err := ws.Ping(pingCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("the client did not answer a ping: %w", err)
		}
	}
}

// describeEnd says in a few words why a connection ended, for the log. It never includes what
// the client sent.
func describeEnd(err error) string {
	var perr *protocol.Error
	switch {
	case err == nil:
		return "ended"
	case errors.As(err, &perr):
		return "the client sent something the daemon refused: " + string(perr.Code)
	case websocket.CloseStatus(err) == websocket.StatusNormalClosure,
		websocket.CloseStatus(err) == websocket.StatusGoingAway:
		return "the client closed the connection"
	case errors.Is(err, context.Canceled):
		return "the daemon stopped the stream"
	default:
		return err.Error()
	}
}
