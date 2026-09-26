package api

import (
	"encoding/json"
	"fmt"

	"github.com/coder/websocket"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The messages a client sends on the event stream after its first Hello (docs/architecture.md
// 11.2). Besides a new Hello, which replaces what it follows, there are three about a card's
// terminal: terminal.input, terminal.resize, and terminal.snapshot. Each names a card, and the
// connection must follow that card's topic.
//
// A message the daemon cannot read, or that is out of bounds, is a mistake in the client. It is
// answered like a bad Hello: an error frame, then the connection closes, and the client fixes the
// mistake and connects again. A message that is well made but cannot be done now, because the card
// is not in the terminal view, is not a mistake: a keystroke in flight when the card went back to
// the chat is the ordinary way to meet it. It is answered with a terminal.refused frame and the
// connection stays open (see stream_terminal.go).

// clientMessage is one message the client sent after its first Hello: a protocol.Hello,
// protocol.TerminalInput, protocol.TerminalResize, or protocol.TerminalSnapshotRequest.
type clientMessage any

// parseMessage checks a message from the client, and reads it into its type. The errors are for the
// client, so they are plain sentences.
func (st *stream) parseMessage(kind websocket.MessageType, data []byte) (clientMessage, *protocol.Error) {
	if kind != websocket.MessageText {
		return nil, protocol.InvalidArgument("Marshal reads text messages on this connection. Send messages as JSON text.")
	}
	var head struct {
		Type protocol.FrameType `json:"type"`
	}
	if json.Unmarshal(data, &head) != nil {
		return nil, protocol.InvalidArgument("That message is not valid JSON. Send messages as JSON.")
	}
	switch head.Type {
	case protocol.FrameTypeHello:
		hello, perr := st.parseHello(kind, data)
		if perr != nil {
			return nil, perr
		}
		return hello, nil
	case protocol.FrameTypeTerminalInput:
		return terminalMessage(data, checkTerminalInput)
	case protocol.FrameTypeTerminalResize:
		return terminalMessage(data, checkTerminalResize)
	case protocol.FrameTypeTerminalSnapshot:
		return terminalMessage(data, checkTerminalSnapshot)
	default:
		return nil, protocol.InvalidArgument(
			"Marshal does not know that kind of message. Send a hello or a terminal message.")
	}
}

// terminalMessage reads a terminal message of the type its check takes, and runs the check.
func terminalMessage[T any](data []byte, check func(T) *protocol.Error) (clientMessage, *protocol.Error) {
	var msg T
	if json.Unmarshal(data, &msg) != nil {
		return nil, protocol.InvalidArgument("That terminal message is not valid. Check its fields and try again.")
	}
	if perr := check(msg); perr != nil {
		return nil, perr
	}
	return msg, nil
}

// checkTerminalCard refuses a card id that cannot be one.
func checkTerminalCard(cardID string) *protocol.Error {
	if !protocol.ValidID(cardID) {
		return protocol.InvalidArgument("That card id is not one Marshal knows. Check the card and try again.")
	}
	return nil
}

// checkTerminalInput holds terminal input to its rules: a card, exactly one of text and a named key,
// a key that exists, and text that is not larger than a message may carry.
func checkTerminalInput(msg protocol.TerminalInput) *protocol.Error {
	if perr := checkTerminalCard(msg.CardID); perr != nil {
		return perr
	}
	switch {
	case (msg.Data == "") == (msg.Key == ""):
		return protocol.InvalidArgument("Send either text or a key in a terminal input, not both and not neither.")
	case msg.Key != "" && !msg.Key.Valid():
		return protocol.InvalidArgument("That key is not one Marshal knows. Check the key and try again.")
	case len(msg.Data) > protocol.MaxTerminalInputBytes:
		return protocol.InvalidArgument(fmt.Sprintf(
			"That input is longer than %d KiB. Send it in smaller pieces.", protocol.MaxTerminalInputBytes>>bitsPerKiB))
	}
	return nil
}

// checkTerminalResize holds a terminal size to the protocol's limits.
func checkTerminalResize(msg protocol.TerminalResize) *protocol.Error {
	if perr := checkTerminalCard(msg.CardID); perr != nil {
		return perr
	}
	if msg.Cols < 1 || msg.Cols > protocol.MaxTerminalCols || msg.Rows < 1 || msg.Rows > protocol.MaxTerminalRows {
		return protocol.InvalidArgument(fmt.Sprintf(
			"The terminal must be 1 to %d columns wide and 1 to %d rows tall.", protocol.MaxTerminalCols, protocol.MaxTerminalRows))
	}
	return nil
}

// checkTerminalSnapshot holds a request for a screen to its rule: it names a card.
func checkTerminalSnapshot(msg protocol.TerminalSnapshotRequest) *protocol.Error {
	return checkTerminalCard(msg.CardID)
}
