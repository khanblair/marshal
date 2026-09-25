package api

import (
	"cmp"
	"time"
)

// The defaults for Limits. Each number has its unit in its name.
const (
	defaultMaxBodyBytes        = 1 << 20 // 1 MiB
	defaultReadTimeout         = 30 * time.Second
	defaultWriteTimeout        = 60 * time.Second
	defaultIdleTimeout         = 120 * time.Second
	defaultMaxConnections      = 64
	defaultHelloTimeout        = 5 * time.Second
	defaultFrameWriteTimeout   = 10 * time.Second
	defaultPingInterval        = 30 * time.Second
	defaultPongTimeout         = 10 * time.Second
	defaultFlushInterval       = 25 * time.Millisecond // inside the 16 to 50 ms window of section 11.2
	defaultMaxBatchEvents      = 100
	defaultMaxBatchBytes       = 256 << 10 // 256 KiB
	defaultClientMessageBytes  = 64 << 10  // 64 KiB
	defaultMaxHelloTopics      = 200
	defaultShutdownCloseWindow = 2 * time.Second
)

// Limits are the sizes and times the server works within. A zero field takes its default, so a
// test names only the ones it changes.
type Limits struct {
	// MaxBodyBytes is the largest request body a JSON route accepts. Default 1 MiB.
	MaxBodyBytes int64
	// ReadTimeout is how long a normal request may take to arrive. Default 30 seconds. A
	// WebSocket takes over the connection, so this does not end it.
	ReadTimeout time.Duration
	// WriteTimeout is how long a normal answer may take to send. Default 60 seconds.
	WriteTimeout time.Duration
	// IdleTimeout is how long a connection may wait for its next request. Default 120 seconds.
	IdleTimeout time.Duration
	// MaxConnections is how many event streams may be open at once. Default 64.
	MaxConnections int
	// HelloTimeout is how long a new stream may take to send its Hello. Default 5 seconds.
	HelloTimeout time.Duration
	// FrameWriteTimeout is how long one frame may take to reach a client before the client is
	// treated as stuck and disconnected. Default 10 seconds.
	FrameWriteTimeout time.Duration
	// PingInterval is how often an idle stream is pinged. Default 30 seconds.
	PingInterval time.Duration
	// PongTimeout is how long a client has to answer a ping. Default 10 seconds.
	PongTimeout time.Duration
	// FlushInterval is how long events are held to be sent together. Default 25 milliseconds.
	FlushInterval time.Duration
	// MaxBatchEvents is the most events in one frame. Default 100.
	MaxBatchEvents int
	// MaxBatchBytes is the most bytes in one frame, unless a single event is larger. Default 256 KiB.
	MaxBatchBytes int
	// ClientMessageBytes is the largest message a stream client may send. Default 64 KiB.
	ClientMessageBytes int64
	// MaxHelloTopics is the most topics one Hello may follow. Default 200.
	MaxHelloTopics int
	// ShutdownCloseWindow is how long a closing stream waits for its client to answer the close
	// before the connection is cut. Default 2 seconds.
	ShutdownCloseWindow time.Duration
}

// withDefaults fills every zero field.
func (l Limits) withDefaults() Limits {
	return Limits{
		MaxBodyBytes:        cmp.Or(l.MaxBodyBytes, defaultMaxBodyBytes),
		ReadTimeout:         cmp.Or(l.ReadTimeout, defaultReadTimeout),
		WriteTimeout:        cmp.Or(l.WriteTimeout, defaultWriteTimeout),
		IdleTimeout:         cmp.Or(l.IdleTimeout, defaultIdleTimeout),
		MaxConnections:      cmp.Or(l.MaxConnections, defaultMaxConnections),
		HelloTimeout:        cmp.Or(l.HelloTimeout, defaultHelloTimeout),
		FrameWriteTimeout:   cmp.Or(l.FrameWriteTimeout, defaultFrameWriteTimeout),
		PingInterval:        cmp.Or(l.PingInterval, defaultPingInterval),
		PongTimeout:         cmp.Or(l.PongTimeout, defaultPongTimeout),
		FlushInterval:       cmp.Or(l.FlushInterval, defaultFlushInterval),
		MaxBatchEvents:      cmp.Or(l.MaxBatchEvents, defaultMaxBatchEvents),
		MaxBatchBytes:       cmp.Or(l.MaxBatchBytes, defaultMaxBatchBytes),
		ClientMessageBytes:  cmp.Or(l.ClientMessageBytes, defaultClientMessageBytes),
		MaxHelloTopics:      cmp.Or(l.MaxHelloTopics, defaultMaxHelloTopics),
		ShutdownCloseWindow: cmp.Or(l.ShutdownCloseWindow, defaultShutdownCloseWindow),
	}
}
