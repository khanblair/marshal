package protocol

import (
	"encoding/json"
	"slices"
)

// The terminal view of a card (docs/architecture.md 4.3, 11.1, and 11.2, checklist item B2.7).
//
// A card's agent runs in one of two modes, and one process cannot serve both. In the chat view the
// daemon drives the agent's structured mode and the app draws messages and tool blocks. In the
// terminal view the daemon runs the agent's own interactive command in a pseudo-terminal, and the
// app draws the screen. Switching stops the current process and resumes the same agent session id
// in the other mode (POST /v1/cards/{id}/view).
//
// What the terminal view says on the event stream:
//
//   - The daemon sends the terminal's output as session.terminal_output events on card:<id>. They
//     are live-only: they take a sequence number like any event, but they are never kept in the
//     replay ring, because output that is replayed late or twice draws the wrong screen.
//   - The app sends three messages on the same connection: terminal.input, terminal.resize, and
//     terminal.snapshot. It may send them only for a card whose topic it follows.
//   - The answer to terminal.snapshot is a terminal.screen frame: the recent output, and the
//     sequence number it is complete through. The app paints it, and then applies only the output
//     events with a higher number. That is also what an app does after it reconnects, and after a
//     resync frame.
//   - A message for a card that is not in terminal view is answered with a terminal.refused frame.
//     Unlike an error frame it does not end the connection: it is the ordinary result of a keystroke
//     that was in flight when the card went back to the chat view.

// MaxTerminalInputBytes is the most one terminal.input message carries, as UTF-8 text. A larger
// paste is sent as several messages. JSON writes a control character as six bytes ("\u001b"), so
// even a message of nothing but control characters stays under the 64 KiB that the stream reads.
const MaxTerminalInputBytes = 8 << 10

// MaxTerminalCols is the widest terminal a client may ask for.
const MaxTerminalCols = 500

// MaxTerminalRows is the tallest terminal a client may ask for.
const MaxTerminalRows = 200

// TerminalKey is a key that the app names instead of sending the bytes a terminal sends for it. It
// is for the keys a phone's own keyboard lacks (the key bar of the design), so the app need not know
// escape sequences. Bytes gives the sequence. Any other key, and every letter with Ctrl held, goes as
// the characters in TerminalInput.Data.
type TerminalKey string

const (
	// TerminalKeyEnter is the Enter key.
	TerminalKeyEnter TerminalKey = "enter"
	// TerminalKeyEsc is the Escape key.
	TerminalKeyEsc TerminalKey = "esc"
	// TerminalKeyTab is the Tab key.
	TerminalKeyTab TerminalKey = "tab"
	// TerminalKeyShiftTab is Tab with Shift held, which moves back.
	TerminalKeyShiftTab TerminalKey = "shift-tab"
	// TerminalKeyBackspace is the Backspace key.
	TerminalKeyBackspace TerminalKey = "backspace"
	// TerminalKeyDelete is the Delete key.
	TerminalKeyDelete TerminalKey = "delete"
	// TerminalKeyUp is the up arrow.
	TerminalKeyUp TerminalKey = "up"
	// TerminalKeyDown is the down arrow.
	TerminalKeyDown TerminalKey = "down"
	// TerminalKeyLeft is the left arrow.
	TerminalKeyLeft TerminalKey = "left"
	// TerminalKeyRight is the right arrow.
	TerminalKeyRight TerminalKey = "right"
	// TerminalKeyHome is the Home key.
	TerminalKeyHome TerminalKey = "home"
	// TerminalKeyEnd is the End key.
	TerminalKeyEnd TerminalKey = "end"
	// TerminalKeyPageUp is the Page Up key.
	TerminalKeyPageUp TerminalKey = "page-up"
	// TerminalKeyPageDown is the Page Down key.
	TerminalKeyPageDown TerminalKey = "page-down"
	// TerminalKeyCtrlC is Ctrl and C, which interrupts the program in front.
	TerminalKeyCtrlC TerminalKey = "ctrl-c"
	// TerminalKeyCtrlD is Ctrl and D, which ends input.
	TerminalKeyCtrlD TerminalKey = "ctrl-d"
	// TerminalKeyCtrlL is Ctrl and L, which redraws the screen.
	TerminalKeyCtrlL TerminalKey = "ctrl-l"
	// TerminalKeyCtrlZ is Ctrl and Z, which suspends the program in front.
	TerminalKeyCtrlZ TerminalKey = "ctrl-z"
)

// TerminalKeyValues lists every named terminal key.
func TerminalKeyValues() []TerminalKey {
	return []TerminalKey{
		TerminalKeyEnter, TerminalKeyEsc, TerminalKeyTab, TerminalKeyShiftTab, TerminalKeyBackspace,
		TerminalKeyDelete, TerminalKeyUp, TerminalKeyDown, TerminalKeyLeft, TerminalKeyRight,
		TerminalKeyHome, TerminalKeyEnd, TerminalKeyPageUp, TerminalKeyPageDown,
		TerminalKeyCtrlC, TerminalKeyCtrlD, TerminalKeyCtrlL, TerminalKeyCtrlZ,
	}
}

// Valid reports whether k is a named terminal key.
func (k TerminalKey) Valid() bool { return slices.Contains(TerminalKeyValues(), k) }

// terminalKeyBytes are the bytes an xterm sends for each key. The arrows and Home and End are the
// normal-mode sequences ("\x1b[A", not the application-mode "\x1bOA"): the key bar sends the same
// bytes whatever mode the program asked for, and the agents that draw a screen read both. It is a
// function, not a package-level map, so the linter's ban on mutable globals has nothing to catch:
// the map it builds is thrown away after each call, which costs nothing next to a key press.
func terminalKeyBytes() map[TerminalKey]string {
	return map[TerminalKey]string{
		TerminalKeyEnter:     "\r",
		TerminalKeyEsc:       "\x1b",
		TerminalKeyTab:       "\t",
		TerminalKeyShiftTab:  "\x1b[Z",
		TerminalKeyBackspace: "\x7f",
		TerminalKeyDelete:    "\x1b[3~",
		TerminalKeyUp:        "\x1b[A",
		TerminalKeyDown:      "\x1b[B",
		TerminalKeyRight:     "\x1b[C",
		TerminalKeyLeft:      "\x1b[D",
		TerminalKeyHome:      "\x1b[H",
		TerminalKeyEnd:       "\x1b[F",
		TerminalKeyPageUp:    "\x1b[5~",
		TerminalKeyPageDown:  "\x1b[6~",
		TerminalKeyCtrlC:     "\x03",
		TerminalKeyCtrlD:     "\x04",
		TerminalKeyCtrlL:     "\x0c",
		TerminalKeyCtrlZ:     "\x1a",
	}
}

// Bytes returns the bytes a terminal sends for the key, or nil for a name that is not a key.
func (k TerminalKey) Bytes() []byte {
	sequence, ok := terminalKeyBytes()[k]
	if !ok {
		return nil
	}
	return []byte(sequence)
}

// TerminalInput is a message from the app: what the person typed into a card's terminal. It carries
// exactly one of Data and Key. Data is the text as a terminal emulator hands it over, so a control
// character (Ctrl and a letter, as "\u0003") and an escape sequence are only characters in it, and
// they reach the program as the same bytes. It is UTF-8, which a keyboard always produces, so it
// needs no encoding beyond JSON's. It is at most MaxTerminalInputBytes bytes.
type TerminalInput struct {
	// Type is always "terminal.input".
	Type FrameType `json:"type" tstype:"typeof FrameTypeTerminalInput"`
	// CardID is the card whose terminal it is. The connection must follow the card's topic.
	CardID string `json:"cardId"`
	// Data is text to type as it is. Leave it out when Key is set.
	Data string `json:"data,omitempty"`
	// Key is a named key. Leave it out when Data is set.
	Key TerminalKey `json:"key,omitempty"`
}

// TerminalResize is a message from the app: the size of the view that draws a card's terminal, in
// character cells. The program is told, and redraws. Columns are 1 to MaxTerminalCols and rows 1 to
// MaxTerminalRows.
type TerminalResize struct {
	// Type is always "terminal.resize".
	Type FrameType `json:"type" tstype:"typeof FrameTypeTerminalResize"`
	// CardID is the card whose terminal it is. The connection must follow the card's topic.
	CardID string `json:"cardId"`
	// Cols is the width of the view in cells.
	Cols int `json:"cols"`
	// Rows is the height of the view in cells.
	Rows int `json:"rows"`
}

// TerminalSnapshotRequest is a message from the app that asks for the recent screen of a card's
// terminal. The answer is a TerminalScreen frame. An app asks when it opens the terminal view, after
// it reconnects, and after a resync frame, because the output it missed is not replayed.
//
//tygo:emit export type ClientFrame = Hello | TerminalInput | TerminalResize | TerminalSnapshotRequest;
type TerminalSnapshotRequest struct {
	// Type is always "terminal.snapshot".
	Type FrameType `json:"type" tstype:"typeof FrameTypeTerminalSnapshot"`
	// CardID is the card whose terminal it is. The connection must follow the card's topic.
	CardID string `json:"cardId"`
}

// TerminalScreen is a frame from the daemon: the answer to a TerminalSnapshotRequest. Data is the
// last part of what the terminal printed, up to 256 KiB, oldest byte first, and it may start in the
// middle of an escape sequence. ThroughSeq is the sequence number of the newest session.terminal_output
// event that Data includes. The app clears its terminal, sets its size to Cols and Rows, paints Data,
// and then paints only the output events whose number is higher than ThroughSeq. Data is what one
// card's terminal printed from the moment its program started, so a snapshot of a terminal that has
// just been switched to is empty.
type TerminalScreen struct {
	// Type is always "terminal.screen". Encoding sets it.
	Type FrameType `json:"type" tstype:"typeof FrameTypeTerminalScreen"`
	// CardID is the card the screen belongs to.
	CardID string `json:"cardId"`
	// Cols and Rows are the size the terminal has now, which is the size Data was drawn for.
	Cols int `json:"cols"`
	Rows int `json:"rows"`
	// ThroughSeq is the number of the newest output event that Data includes, or 0 when the
	// program has printed nothing yet.
	ThroughSeq uint64 `json:"throughSeq"`
	// Data is the output, as base64 (the standard alphabet, with padding).
	Data string `json:"data"`
}

// MarshalJSON writes the frame with its type set to "terminal.screen".
func (f TerminalScreen) MarshalJSON() ([]byte, error) {
	type plain TerminalScreen // no methods, so this does not call MarshalJSON again
	f.Type = FrameTypeTerminalScreen
	return json.Marshal(plain(f))
}

// TerminalRefusalReason is why a message about a card's terminal was refused. The sentence a person
// reads comes with it; the reason is stable so a client can act on it.
type TerminalRefusalReason string

const (
	// TerminalRefusalReasonNotActive is a message for a card that has no terminal running: it is in
	// the chat view, or its agent is not running.
	TerminalRefusalReasonNotActive TerminalRefusalReason = "terminal_not_active"
	// TerminalRefusalReasonBusy is input for a terminal whose program is not reading what is sent.
	// The input that did not fit was dropped.
	TerminalRefusalReasonBusy TerminalRefusalReason = "terminal_busy"
)

// TerminalRefusalReasonValues lists every reason a terminal message can be refused.
func TerminalRefusalReasonValues() []TerminalRefusalReason {
	return []TerminalRefusalReason{TerminalRefusalReasonNotActive, TerminalRefusalReasonBusy}
}

// Valid reports whether r is a terminal refusal reason.
func (r TerminalRefusalReason) Valid() bool {
	return slices.Contains(TerminalRefusalReasonValues(), r)
}

// TerminalRefusal is a frame from the daemon: a terminal message was understood but cannot be done
// now. Error has the code "refused", a plain sentence, and the stable TerminalRefusalReason in
// details.reason. The connection stays open.
type TerminalRefusal struct {
	// Type is always "terminal.refused". Encoding sets it.
	Type FrameType `json:"type" tstype:"typeof FrameTypeTerminalRefused"`
	// CardID is the card the message was about.
	CardID string `json:"cardId"`
	// Error is the same error shape that HTTP answers use.
	Error Error `json:"error"`
}

// MarshalJSON writes the frame with its type set to "terminal.refused".
func (f TerminalRefusal) MarshalJSON() ([]byte, error) {
	type plain TerminalRefusal // no methods, so this does not call MarshalJSON again
	f.Type = FrameTypeTerminalRefused
	return json.Marshal(plain(f))
}

// TerminalOutputEventData is the payload of session.terminal_output: a piece of what a card's
// terminal printed. It is published on card:<id> and is live-only, so it is never replayed. Its
// event number is what orders it against the ThroughSeq of a TerminalScreen.
type TerminalOutputEventData struct {
	// CardID is the card whose terminal printed it.
	CardID string `json:"cardId"`
	// Data is the output, as base64 (the standard alphabet, with padding). It is raw bytes, escape
	// sequences included, and at most 16 KiB. A piece can end in the middle of a character or an
	// escape sequence, so the app joins pieces before it decodes text.
	Data string `json:"data"`
}

// SetViewRequest is the body of POST /v1/cards/{id}/view.
type SetViewRequest struct {
	// Mode is the view to show: chat or terminal.
	Mode CardViewMode `json:"mode"`
}

// CardView is the answer to POST /v1/cards/{id}/view: which view the card is in, and the state of
// the session that runs in it. A card is in the terminal view only while its agent runs in a
// terminal, so a card whose session stops is back in the chat view.
type CardView struct {
	// CardID is the card.
	CardID string `json:"cardId"`
	// Mode is the view the card is in now.
	Mode CardViewMode `json:"mode"`
	// Session is the state of the card's session. It is awake, or working in the chat view while a
	// turn runs.
	Session SessionState `json:"session"`
	// ServerTime is the daemon's time when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}

// ViewRefusalReason is why a switch of a card's view, or a message to a card that is in the
// terminal view, was refused. The sentence a person reads comes with it; the reason is stable so a
// client can act on it, the way HoldRefusalReason is for a refused pause. A refused switch leaves
// the card and its session exactly as they were, except ViewRefusalReasonCannotResume, which is the
// rule of docs/architecture.md section 5.3: the card needs the person.
type ViewRefusalReason string

const (
	// ViewRefusalReasonNoAgent is a switch of a card that has no agent running in this daemon: it
	// never started, its session slept or stopped, or a restart has not resumed it.
	ViewRefusalReasonNoAgent ViewRefusalReason = "view_no_agent"
	// ViewRefusalReasonTurnRunning is a switch while the agent is in the middle of a turn. Stopping
	// its process would cut the turn off, so the person waits for it or stops the card first.
	ViewRefusalReasonTurnRunning ViewRefusalReason = "view_turn_running"
	// ViewRefusalReasonHoldingMessages is a switch of a paused card that is still holding a message.
	// The waiting messages live in memory with the process, so a switch would lose them.
	ViewRefusalReasonHoldingMessages ViewRefusalReason = "view_holding_messages"
	// ViewRefusalReasonNoTerminal is a switch to the terminal view of an agent that has no terminal
	// mode Marshal can resume a session in.
	ViewRefusalReasonNoTerminal ViewRefusalReason = "view_no_terminal"
	// ViewRefusalReasonSwitching is a request for a card that is being started, resumed, woken, or
	// switched at that moment.
	ViewRefusalReasonSwitching ViewRefusalReason = "view_switching"
	// ViewRefusalReasonTerminalActive is a chat message for a card that is in the terminal view.
	// What is typed there goes to the terminal, and the card's chat does not see it.
	ViewRefusalReasonTerminalActive ViewRefusalReason = "view_terminal_active"
	// ViewRefusalReasonCannotResume is a switch whose new process could not pick the session up,
	// after the old one had stopped. The card moves to Needs you and its session stops.
	ViewRefusalReasonCannotResume ViewRefusalReason = "view_cannot_resume"
)

// ViewRefusalReasonValues lists every reason a view switch can be refused, in the order the session
// manager checks them.
func ViewRefusalReasonValues() []ViewRefusalReason {
	return []ViewRefusalReason{
		ViewRefusalReasonNoAgent, ViewRefusalReasonTurnRunning, ViewRefusalReasonHoldingMessages,
		ViewRefusalReasonNoTerminal, ViewRefusalReasonSwitching, ViewRefusalReasonTerminalActive,
		ViewRefusalReasonCannotResume,
	}
}

// Valid reports whether r is a view refusal reason.
func (r ViewRefusalReason) Valid() bool { return slices.Contains(ViewRefusalReasonValues(), r) }
