package protocol

import (
	"encoding/json"
	"slices"
)

// WebSocketSubprotocol is the subprotocol the daemon answers with on /v1/events.
const WebSocketSubprotocol = "marshal.v1"

// BearerSubprotocolPrefix starts the second subprotocol a client offers. The token follows it
// ("bearer.<token>"). Browsers cannot set headers on a WebSocket and tokens must not be in URLs,
// so the token travels in the offered subprotocol list. The daemon never echoes it back and never
// logs the list.
const BearerSubprotocolPrefix = "bearer."

// ReplayBufferSize is how many recent events the daemon keeps to replay to a client that
// reconnects. A client that is further behind gets a Resync frame.
const ReplayBufferSize = 2000

// FrameType says which frame a message on the event stream is. Every frame has a "type" field,
// so a client can tell them apart with one switch. In TypeScript each frame interface has its own
// literal type, so `frame.type` narrows a ServerFrame.
type FrameType string

const (
	// FrameTypeHello is the first message a client sends. See Hello.
	FrameTypeHello FrameType = "hello"
	// FrameTypeEvents is a group of events from the daemon. See EventBatch.
	FrameTypeEvents FrameType = "events"
	// FrameTypeResync tells the client to reload. See Resync.
	FrameTypeResync FrameType = "resync"
	// FrameTypeError says the client did something wrong on the stream. See ErrorFrame.
	FrameTypeError FrameType = "error"
	// FrameTypeTerminalInput is a message from the client with what the person typed into a card's
	// terminal. See TerminalInput.
	FrameTypeTerminalInput FrameType = "terminal.input"
	// FrameTypeTerminalResize is a message from the client with the size of a card's terminal
	// view. See TerminalResize.
	FrameTypeTerminalResize FrameType = "terminal.resize"
	// FrameTypeTerminalSnapshot is a message from the client that asks for a card's recent
	// terminal screen. See TerminalSnapshotRequest.
	FrameTypeTerminalSnapshot FrameType = "terminal.snapshot"
	// FrameTypeTerminalScreen is a frame from the daemon with a card's recent terminal screen.
	// See TerminalScreen.
	FrameTypeTerminalScreen FrameType = "terminal.screen"
	// FrameTypeTerminalRefused is a frame from the daemon that says a terminal message cannot be
	// done now, and leaves the connection open. See TerminalRefusal.
	FrameTypeTerminalRefused FrameType = "terminal.refused"
)

// FrameTypeValues lists every frame type.
func FrameTypeValues() []FrameType {
	return []FrameType{
		FrameTypeHello, FrameTypeEvents, FrameTypeResync, FrameTypeError,
		FrameTypeTerminalInput, FrameTypeTerminalResize, FrameTypeTerminalSnapshot,
		FrameTypeTerminalScreen, FrameTypeTerminalRefused,
	}
}

// Valid reports whether t is a frame type.
func (t FrameType) Valid() bool { return slices.Contains(FrameTypeValues(), t) }

// Event is one thing that happened. Every event has a number that grows by one for each event in
// an epoch, so the daemon can replay what a client missed.
type Event struct {
	// Seq is the event's number within the epoch. It counts every event the daemon sends, not only
	// the ones a client subscribed to, so a client sees increasing numbers with gaps in them.
	Seq uint64 `json:"seq"`
	// Topic is the topic the event belongs to.
	Topic Topic `json:"topic"`
	// Type says what happened and how to read Data.
	Type EventType `json:"type"`
	// At is when it happened, by the daemon's clock.
	At Timestamp `json:"at"`
	// Data is the event's payload, already encoded, so the daemon encodes it once for all clients
	// and keeps it in the replay buffer. It is unknown in TypeScript: read it by Type.
	Data json.RawMessage `json:"data"`
}

// EventBatch is a frame from the daemon: events in order, sent together. The daemon groups
// events every 16 to 50 milliseconds. Epoch is a new random id each time the daemon starts. A
// client that sees an epoch it does not know must reload its snapshots.
type EventBatch struct {
	// Type is always "events". Encoding sets it, so a batch built without it is still right.
	Type FrameType `json:"type" tstype:"typeof FrameTypeEvents"`
	// Epoch identifies this run of the daemon.
	Epoch string `json:"epoch"`
	// Events are in increasing Seq order and are never empty.
	Events []Event `json:"events"`
}

// MarshalJSON writes the batch with its type set to "events".
func (b EventBatch) MarshalJSON() ([]byte, error) {
	type plain EventBatch // no methods, so this does not call MarshalJSON again
	b.Type = FrameTypeEvents
	return json.Marshal(plain(b))
}

// ResyncReason says why the daemon could not replay.
type ResyncReason string

const (
	// ResyncReasonEpochChanged means the client has no epoch or one from an earlier daemon run.
	ResyncReasonEpochChanged ResyncReason = "epoch-changed"
	// ResyncReasonTooFarBehind means the client's position is older than the replay buffer.
	ResyncReasonTooFarBehind ResyncReason = "too-far-behind"
	// ResyncReasonUnknownPosition means the client's position is newer than any event the daemon
	// has sent in this epoch.
	ResyncReasonUnknownPosition ResyncReason = "unknown-position"
)

// ResyncReasonValues lists every resync reason.
func ResyncReasonValues() []ResyncReason {
	return []ResyncReason{
		ResyncReasonEpochChanged, ResyncReasonTooFarBehind, ResyncReasonUnknownPosition,
	}
}

// Resync is a frame from the daemon that says it cannot replay what the client missed. The
// client reloads its snapshots. Events after Seq follow on the same connection, so nothing is
// lost between the frame and the reload.
//
//tygo:emit export type ServerFrame = EventBatch | Resync | ErrorFrame | TerminalScreen | TerminalRefusal
type Resync struct {
	// Type is always "resync". Encoding sets it.
	Type FrameType `json:"type" tstype:"typeof FrameTypeResync"`
	// Epoch is the daemon's current epoch. The client keeps it.
	Epoch string `json:"epoch"`
	// Reason is why the daemon could not replay.
	Reason ResyncReason `json:"reason"`
	// Seq is the newest event number the daemon has sent in this epoch, or 0 if none.
	Seq uint64 `json:"seq"`
}

// MarshalJSON writes the frame with its type set to "resync".
func (r Resync) MarshalJSON() ([]byte, error) {
	type plain Resync // no methods, so this does not call MarshalJSON again
	r.Type = FrameTypeResync
	return json.Marshal(plain(r))
}

// ErrorFrame is a frame from the daemon that says the client did something wrong on the stream,
// such as a topic that does not exist. The daemon closes the connection right after it, so the
// client fixes the mistake and connects again.
type ErrorFrame struct {
	// Type is always "error". Encoding sets it.
	Type FrameType `json:"type" tstype:"typeof FrameTypeError"`
	// Error is the same error shape that HTTP answers use.
	Error Error `json:"error"`
}

// MarshalJSON writes the frame with its type set to "error".
func (f ErrorFrame) MarshalJSON() ([]byte, error) {
	type plain ErrorFrame // no methods, so this does not call MarshalJSON again
	f.Type = FrameTypeError
	return json.Marshal(plain(f))
}

// Hello is the first message a client sends on the event stream, within 5 seconds of connecting.
// It says what to follow and where the client left off. Sending it again replaces the topics. A
// later Hello that carries an epoch also replays what the client missed since SinceSeq, in the
// same way as the first one. A later Hello with an empty epoch only changes the topics for
// events from then on.
type Hello struct {
	// Type is always "hello". The daemon refuses a first message with any other type.
	Type FrameType `json:"type" tstype:"typeof FrameTypeHello"`
	// Subscribe lists the topics to follow.
	Subscribe []Topic `json:"subscribe"`
	// SinceSeq is the Seq of the last event the client applied, or 0 on a first connection.
	SinceSeq uint64 `json:"sinceSeq"`
	// Epoch is the epoch that SinceSeq belongs to, or empty on a first connection.
	Epoch string `json:"epoch"`
}
