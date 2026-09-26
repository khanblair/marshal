package api_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/agents/pty"
	"github.com/khanblair/marshal/daemon/internal/api"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The terminal view over HTTP and the event stream (checklist item B2.7): the route that switches a
// card between the chat view and the terminal view, and the three messages and the frames that let
// the app type into the terminal, size it, and paint it. The chat side is the real stub agent behind
// the real ACP adapter; the terminal side is the real PTY adapter running a tiny shell script (the
// one the stub daemon runs), or the PTY test helper. No real coding agent is ever started.

// stubTerminalScript is what the stub daemon runs in a card's terminal (cmd/marshald/terminals.go):
// it says which session it was given, then echoes what is typed.
const stubTerminalScript = `printf 'terminal session %s\n' "$1"; exec cat`

// terminalRegistry makes the terminals of a test from an adapter configuration, for the default card
// agent.
func terminalRegistry(t *testing.T, cfg pty.Config) *agents.Registry {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the terminal view is tested on macOS and Linux")
	}
	cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := agents.NewRegistry()
	factory := func() (agents.Agent, error) { return pty.New(cfg) }
	if err := reg.Register(protocol.AgentKindClaude, factory); err != nil {
		t.Fatalf("register the terminal: %v", err)
	}
	return reg
}

// shellTerminals is the stub daemon's terminal: it prints the session id it was resumed with and
// echoes what is typed, so a test can tell that the same session came back and that input arrived.
func shellTerminals(t *testing.T) *agents.Registry {
	t.Helper()
	args := func(sessionID string) []string { return []string{"-c", stubTerminalScript, "sh", sessionID} }
	return terminalRegistry(t, pty.Config{Path: "/bin/sh", StartArgs: args, ResumeArgs: args})
}

// helperTerminals runs the PTY test helper in a mode, whatever the session id.
func helperTerminals(t *testing.T, mode ...string) *agents.Registry {
	t.Helper()
	args := func(string) []string { return mode }
	return terminalRegistry(t, pty.Config{Path: testutil.TerminalHelper(t), StartArgs: args, ResumeArgs: args})
}

// send writes a message of the client as JSON.
func (w *wire) send(msg any) {
	w.t.Helper()
	data, err := json.Marshal(msg)
	if err != nil {
		w.t.Fatalf("encode a message: %v", err)
	}
	w.sendRaw(websocket.MessageText, string(data))
}

// snapshot asks for the screen of a card's terminal.
func (w *wire) snapshot(cardID string) {
	w.t.Helper()
	w.send(protocol.TerminalSnapshotRequest{Type: protocol.FrameTypeTerminalSnapshot, CardID: cardID})
}

// typeText types text into a card's terminal.
func (w *wire) typeText(cardID, text string) {
	w.t.Helper()
	w.send(protocol.TerminalInput{Type: protocol.FrameTypeTerminalInput, CardID: cardID, Data: text})
}

// pressKey presses a named key in a card's terminal.
func (w *wire) pressKey(cardID string, key protocol.TerminalKey) {
	w.t.Helper()
	w.send(protocol.TerminalInput{Type: protocol.FrameTypeTerminalInput, CardID: cardID, Key: key})
}

// resize tells a card's terminal the size of the view.
func (w *wire) resize(cardID string, cols, rows int) {
	w.t.Helper()
	w.send(protocol.TerminalResize{Type: protocol.FrameTypeTerminalResize, CardID: cardID, Cols: cols, Rows: rows})
}

// nextScreen reads frames until a terminal screen arrives, and returns it with its text. The events
// that come first are kept for until.
func (w *wire) nextScreen() (protocol.TerminalScreen, []byte) {
	w.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), streamTimeout)
	defer cancel()
	for len(w.screens) == 0 {
		if err := w.read(ctx); err != nil {
			w.t.Fatalf("waiting for a terminal screen: %v", err)
		}
	}
	screen := w.screens[0]
	w.screens = w.screens[1:]
	data, err := base64.StdEncoding.DecodeString(screen.Data)
	if err != nil {
		w.t.Fatalf("the screen is not base64: %v", err)
	}
	return screen, data
}

// nextRefusal reads frames until a terminal refusal arrives, and returns it.
func (w *wire) nextRefusal() protocol.TerminalRefusal {
	w.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), streamTimeout)
	defer cancel()
	for len(w.refusals) == 0 {
		if err := w.read(ctx); err != nil {
			w.t.Fatalf("waiting for a terminal refusal: %v", err)
		}
	}
	refusal := w.refusals[0]
	w.refusals = w.refusals[1:]
	return refusal
}

// untilScreenHas asks for the screen until it holds the text, and returns it. The program prints when
// it starts, which is a moment after the switch answered.
func (w *wire) untilScreenHas(cardID, text string) (protocol.TerminalScreen, []byte) {
	w.t.Helper()
	deadline := time.Now().Add(streamTimeout)
	for {
		w.snapshot(cardID)
		screen, data := w.nextScreen()
		if bytes.Contains(data, []byte(text)) {
			return screen, data
		}
		if time.Now().After(deadline) {
			w.t.Fatalf("the screen never held %q, it held %q", text, data)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// outputs are the terminal output events of one card that a client has read, in order.
type outputs struct {
	t      *testing.T
	cardID string
	events []protocol.Event
}

// text is the output the events carry, joined.
func (o *outputs) text() []byte {
	o.t.Helper()
	var out []byte
	for _, ev := range o.events {
		out = append(out, o.decode(ev)...)
	}
	return out
}

// through is the output of the events whose number is not above seq: what a screen complete through
// seq holds.
func (o *outputs) through(seq uint64) []byte {
	o.t.Helper()
	var out []byte
	for _, ev := range o.events {
		if ev.Seq <= seq {
			out = append(out, o.decode(ev)...)
		}
	}
	return out
}

func (o *outputs) decode(ev protocol.Event) []byte {
	o.t.Helper()
	data := dataOf[protocol.TerminalOutputEventData](o.t, ev)
	piece, err := base64.StdEncoding.DecodeString(data.Data)
	if err != nil {
		o.t.Fatalf("an output event is not base64: %v", err)
	}
	return piece
}

// add files the output events among events that were read.
func (o *outputs) add(evs []protocol.Event) {
	for _, ev := range evs {
		if ev.Type == protocol.EventTypeSessionTerminalOutput && dataOf[protocol.TerminalOutputEventData](o.t, ev).CardID == o.cardID {
			o.events = append(o.events, ev)
		}
	}
}

// untilHas reads events until the output holds the text.
func (o *outputs) untilHas(w *wire, text string) {
	o.t.Helper()
	for !bytes.Contains(o.text(), []byte(text)) {
		o.add(w.until(ofType(protocol.EventTypeSessionTerminalOutput)))
	}
}

// untilSeq reads events until one with a number of at least seq has been read.
func (o *outputs) untilSeq(w *wire, seq uint64) {
	o.t.Helper()
	if len(o.events) > 0 && o.events[len(o.events)-1].Seq >= seq {
		return
	}
	o.add(w.until(func(ev protocol.Event) bool { return ev.Seq >= seq }))
}

// startedTerminalCard adds a card, starts it with the stub agent, and waits for it to be awake.
func startedTerminalCard(t *testing.T, st *stack, title string) (protocol.Card, *wire) {
	t.Helper()
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, title)
	w := st.dial(protocol.CardTopic(card.ID))
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).want(t, http.StatusOK)
	w.until(stateOf(card.ID, protocol.SessionStateAwake))
	return card, w
}

// switchView asks for a view and returns the answer.
func (st *stack) switchView(cardID string, mode protocol.CardViewMode) reply {
	st.t.Helper()
	return st.do(http.MethodPost, "/v1/cards/"+cardID+"/view", protocol.SetViewRequest{Mode: mode})
}

// getCard reads a card through the API.
func (st *stack) getCard(cardID string) protocol.Card {
	st.t.Helper()
	return decode[protocol.Card](st.t, st.do(http.MethodGet, "/v1/cards/"+cardID, nil).want(st.t, http.StatusOK))
}

// The whole thing: a card is talked to in the chat, switched to the terminal, typed into, painted from
// a snapshot that joins the live events exactly, and switched back, with the same conversation on
// both sides.
func TestACardSwitchesToTheTerminalAndBackWithTheSameSession(t *testing.T) {
	st := newStack(t, withTerminals(shellTerminals(t)))
	card, w := startedTerminalCard(t, st, "Terminal over HTTP")
	base := "/v1/cards/" + card.ID
	row, err := st.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil || row.AgentSessionID == "" {
		t.Fatalf("the started card's session = %+v, %v", row, err)
	}

	// A first turn in the chat, so the agent has a conversation to keep.
	st.do(http.MethodPost, base+"/messages", protocol.SendMessageRequest{Text: "What is blocked right now?"}).
		want(t, http.StatusNoContent)
	w.until(stateOf(card.ID, protocol.SessionStateWorking))
	w.until(stateOf(card.ID, protocol.SessionStateAwake))

	begun := time.Now()
	answer := st.switchView(card.ID, protocol.CardViewModeTerminal).want(t, http.StatusOK)
	if took := time.Since(begun); took > 3*time.Second {
		t.Errorf("the switch took %v, want under the 3 seconds of architecture.md 4.3", took)
	}
	sameShape(t, "card-view", answer.Body)
	if view := decode[protocol.CardView](t, answer); view.CardID != card.ID || view.Mode != protocol.CardViewModeTerminal ||
		view.Session != protocol.SessionStateAwake {
		t.Errorf("the answer = %+v", view)
	}
	inTerminal := st.getCard(card.ID)
	if inTerminal.ViewMode != protocol.CardViewModeTerminal || inTerminal.Session == nil ||
		*inTerminal.Session != protocol.SessionStateAwake || inTerminal.State != protocol.CardStateWorking {
		t.Errorf("the card = view %q, session %v, state %q", inTerminal.ViewMode, inTerminal.Session, inTerminal.State)
	}

	// The terminal was resumed with the id the chat had, which the script prints.
	first, _ := w.untilScreenHas(card.ID, "terminal session "+row.AgentSessionID)
	if first.Cols != 120 || first.Rows != 32 || first.CardID != card.ID {
		t.Errorf("the screen = %+v", first)
	}

	// What is typed reaches the program: the terminal echoes it once, and the program prints it
	// again. A key by name is the bytes a terminal sends.
	out := &outputs{t: t, cardID: card.ID}
	w.typeText(card.ID, "hello\r")
	out.untilHas(w, "hello\r\nhello")
	w.typeText(card.ID, "k9")
	w.pressKey(card.ID, protocol.TerminalKeyEnter)
	out.untilHas(w, "k9\r\nk9")

	// The size reaches the terminal, and the next screen says so.
	w.resize(card.ID, 100, 40)
	w.snapshot(card.ID)
	screen, painted := w.nextScreen()
	if screen.Cols != 100 || screen.Rows != 40 {
		t.Errorf("the screen after a resize = %dx%d, want 100x40", screen.Cols, screen.Rows)
	}

	// The screen is exactly what the events up to its number carried: painted first and joined with
	// the live events after it, nothing repeats and nothing is missing.
	out.untilSeq(w, screen.ThroughSeq)
	if want := out.through(screen.ThroughSeq); !bytes.Equal(painted, want) {
		t.Errorf("the screen is %q, but the output through event %d is %q", painted, screen.ThroughSeq, want)
	}

	// A chat message for a card in the terminal has no place.
	refused := st.do(http.MethodPost, base+"/messages", protocol.SendMessageRequest{Text: "hello?"}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if refused.Details["reason"] != "view_terminal_active" {
		t.Errorf("the refusal = %+v", refused)
	}

	// Back to the chat: the same session id, and the agent remembers the turn it had before.
	back := decode[protocol.CardView](t, st.switchView(card.ID, protocol.CardViewModeChat).want(t, http.StatusOK))
	if back.Mode != protocol.CardViewModeChat || back.Session != protocol.SessionStateAwake {
		t.Errorf("the answer for the way back = %+v", back)
	}
	after, err := st.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil || after.AgentSessionID != row.AgentSessionID || after.ViewMode != "chat" || after.State != "awake" {
		t.Errorf("the session after the round trip = %+v (%v), want the same id %q in the chat view", after, err, row.AgentSessionID)
	}
	if got := st.getCard(card.ID); got.ViewMode != protocol.CardViewModeChat {
		t.Errorf("the card after the way back is in the %q view", got.ViewMode)
	}
	st.do(http.MethodPost, base+"/messages", protocol.SendMessageRequest{Text: "And now?"}).want(t, http.StatusNoContent)
	turn := w.until(stateOf(card.ID, protocol.SessionStateWorking))
	turn = append(turn, w.until(stateOf(card.ID, protocol.SessionStateAwake))...)
	if answered := strings.Join(messageTexts(turn), ""); !strings.Contains(answered, "Turn 2. I remember 1 earlier turns.") {
		t.Errorf("the agent answered %q after the round trip, want it to remember the first turn", answered)
	}

	// The terminal is gone, and a keystroke still on its way is refused without ending the stream.
	w.typeText(card.ID, "late")
	if refusal := w.nextRefusal(); refusal.Error.Details["reason"] != "terminal_not_active" || refusal.CardID != card.ID {
		t.Errorf("the refusal = %+v", refusal)
	}
	w.snapshot(card.ID)
	if refusal := w.nextRefusal(); refusal.Error.Details["reason"] != "terminal_not_active" {
		t.Errorf("the refusal of the snapshot = %+v", refusal)
	}
}

// Refusals of the view route: the stable reasons and sentences, and nothing changes.
func TestTheViewRouteRefusals(t *testing.T) {
	st := newStack(t, withTerminals(shellTerminals(t)))
	project, _ := st.addProject("small-repo")
	idle := st.addCard(project.ID, "Never started")
	const missing = "01M3C107JB041061050R3GG28A"

	t.Run("a card that does not exist, and an id that cannot be one", func(t *testing.T) {
		st.switchView(missing, protocol.CardViewModeTerminal).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
		st.do(http.MethodPost, "/v1/cards/not-an-id/view", protocol.SetViewRequest{Mode: protocol.CardViewModeChat}).
			apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	})
	t.Run("a body that is not one", func(t *testing.T) {
		for body, message := range map[string]string{
			`{"mode":"hologram"}`:   "The view must be chat or terminal.",
			`{}`:                    "The view must be chat or terminal.",
			`{"mode":"chat","x":1}`: `The field "x" is not one Marshal knows. Remove it and try again.`,
			``:                      "The request body is empty. Send a JSON object.",
		} {
			var sent any = body
			if body == "" {
				sent = nil
			}
			got := st.do(http.MethodPost, "/v1/cards/"+idle.ID+"/view", sent).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
			if got.Message != message {
				t.Errorf("body %q: message = %q, want %q", body, got.Message, message)
			}
		}
	})
	t.Run("a card that never started", func(t *testing.T) {
		got := st.switchView(idle.ID, protocol.CardViewModeTerminal).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
		if got.Details["reason"] != "view_no_agent" || got.Message != "This card has no agent running. Start the card first." {
			t.Errorf("the refusal = %+v", got)
		}
		if card := st.getCard(idle.ID); card.ViewMode != protocol.CardViewModeChat || card.Session != nil {
			t.Errorf("the card after a refusal = %+v", card)
		}
	})
	t.Run("no token", func(t *testing.T) {
		st.doWith("", http.MethodPost, "/v1/cards/"+idle.ID+"/view", protocol.SetViewRequest{Mode: protocol.CardViewModeChat}).
			apiError(t, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized)
	})
}

// An agent with no terminal mode is refused with its own sentence, and the chat process is not
// touched.
func TestAnAgentWithNoTerminalViewIsRefused(t *testing.T) {
	st := newStack(t)
	card, w := startedTerminalCard(t, st, "No terminal here")
	got := st.switchView(card.ID, protocol.CardViewModeTerminal).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got.Details["reason"] != "view_no_terminal" || got.Message != "This agent has no terminal view. Stay in the chat view." {
		t.Errorf("the refusal = %+v", got)
	}
	// The chat process is still running: the card can talk.
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/messages", protocol.SendMessageRequest{Text: "still here"}).want(t, http.StatusNoContent)
	w.until(stateOf(card.ID, protocol.SessionStateWorking))
}

// A terminal that cannot start after the chat process was stopped follows section 5.3, and the answer
// says so with its own reason.
func TestASwitchThatCannotStartATerminalMovesTheCardToNeedsYou(t *testing.T) {
	broken := terminalRegistry(t, pty.Config{
		Path: "/nonexistent/interactive-agent", ResumeArgs: func(id string) []string { return []string{id} },
	})
	st := newStack(t, withTerminals(broken))
	card, w := startedTerminalCard(t, st, "Terminal will not start")
	got := st.switchView(card.ID, protocol.CardViewModeTerminal).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got.Details["reason"] != "view_cannot_resume" || got.Message != "Marshal could not pick this session back up. The card now needs you." {
		t.Errorf("the refusal = %+v", got)
	}
	w.until(stateOf(card.ID, protocol.SessionStateStopped))
	if after := st.getCard(card.ID); after.State != protocol.CardStateNeeds || after.ViewMode != protocol.CardViewModeChat ||
		after.Session == nil || *after.Session != protocol.SessionStateStopped {
		t.Errorf("the card = %+v, want it to need the person in the chat view with a stopped session", after)
	}
	// Start brings the same conversation back, as it does for any stopped session.
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).want(t, http.StatusOK)
	w.until(stateOf(card.ID, protocol.SessionStateAwake))
}

// Switching starts a process, so the answer gets the longer time that starting a card gets: the
// limits here are far shorter than the switch back to the chat, which resumes a slow agent.
func TestASlowSwitchOutlastsTheUsualLimits(t *testing.T) {
	limits := api.Limits{WriteTimeout: 300 * time.Millisecond, ReadTimeout: 100 * time.Millisecond}
	st := newStack(t, withLimits(limits), withTerminals(shellTerminals(t)),
		withAgent(fakeFactory(fakeAgent{resumeDelay: 900 * time.Millisecond})))
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Slow to switch")
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).want(t, http.StatusOK)
	st.switchView(card.ID, protocol.CardViewModeTerminal).want(t, http.StatusOK)

	began := time.Now()
	st.switchView(card.ID, protocol.CardViewModeChat).want(t, http.StatusOK)
	if took := time.Since(began); took < 800*time.Millisecond {
		t.Fatalf("the switch took %v, the test does not prove anything", took)
	}
}

// A message on the stream that is malformed or out of bounds is a mistake in the client, and is
// answered like a bad hello: an error frame with a plain sentence, then the connection closes.
func TestAMalformedTerminalMessageClosesTheConnection(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Followed")
	other := "01M3C107JB041061050R3GG28A"
	tests := []struct {
		name    string
		kind    websocket.MessageType
		data    string
		message string
	}{
		{"input for a card id that cannot be one", websocket.MessageText, `{"type":"terminal.input","cardId":"nope","data":"x"}`,
			"That card id is not one Marshal knows. Check the card and try again."},
		{"input with neither text nor a key", websocket.MessageText, `{"type":"terminal.input","cardId":"` + card.ID + `"}`,
			"Send either text or a key in a terminal input, not both and not neither."},
		{"input with both", websocket.MessageText, `{"type":"terminal.input","cardId":"` + card.ID + `","data":"x","key":"esc"}`,
			"Send either text or a key in a terminal input, not both and not neither."},
		{"input with a key that does not exist", websocket.MessageText, `{"type":"terminal.input","cardId":"` + card.ID + `","key":"f13"}`,
			"That key is not one Marshal knows. Check the key and try again."},
		{"input that is too long", websocket.MessageText,
			`{"type":"terminal.input","cardId":"` + card.ID + `","data":"` + strings.Repeat("a", protocol.MaxTerminalInputBytes+1) + `"}`,
			"That input is longer than 8 KiB. Send it in smaller pieces."},
		{"input that is not text", websocket.MessageText, `{"type":"terminal.input","cardId":"` + card.ID + `","data":7}`,
			"That terminal message is not valid. Check its fields and try again."},
		{"a size of no columns", websocket.MessageText, `{"type":"terminal.resize","cardId":"` + card.ID + `","cols":0,"rows":24}`,
			"The terminal must be 1 to 500 columns wide and 1 to 200 rows tall."},
		{"a size that is too wide", websocket.MessageText, `{"type":"terminal.resize","cardId":"` + card.ID + `","cols":501,"rows":24}`,
			"The terminal must be 1 to 500 columns wide and 1 to 200 rows tall."},
		{"a size that is too tall", websocket.MessageText, `{"type":"terminal.resize","cardId":"` + card.ID + `","cols":80,"rows":201}`,
			"The terminal must be 1 to 500 columns wide and 1 to 200 rows tall."},
		{"a size with a negative row count", websocket.MessageText, `{"type":"terminal.resize","cardId":"` + card.ID + `","cols":80,"rows":-1}`,
			"The terminal must be 1 to 500 columns wide and 1 to 200 rows tall."},
		{"a snapshot of a card id that cannot be one", websocket.MessageText, `{"type":"terminal.snapshot","cardId":""}`,
			"That card id is not one Marshal knows. Check the card and try again."},
		{"a card whose topic is not followed", websocket.MessageText, `{"type":"terminal.input","cardId":"` + other + `","data":"x"}`,
			"Follow a card's topic before you use its terminal. Add the card to the topics of a hello first."},
		{"a resize for a card that is not followed", websocket.MessageText, `{"type":"terminal.resize","cardId":"` + other + `","cols":80,"rows":24}`,
			"Follow a card's topic before you use its terminal. Add the card to the topics of a hello first."},
		{"a snapshot for a card that is not followed", websocket.MessageText, `{"type":"terminal.snapshot","cardId":"` + other + `"}`,
			"Follow a card's topic before you use its terminal. Add the card to the topics of a hello first."},
		{"a kind of message that does not exist", websocket.MessageText, `{"type":"terminal.bogus","cardId":"` + card.ID + `"}`,
			"Marshal does not know that kind of message. Send a hello or a terminal message."},
		{"text that is not JSON", websocket.MessageText, `{nope`, "That message is not valid JSON. Send messages as JSON."},
		{"a binary message", websocket.MessageBinary, `hello`,
			"Marshal reads text messages on this connection. Send messages as JSON text."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := st.dial(protocol.CardTopic(card.ID))
			w.sendRaw(tc.kind, tc.data)
			if got := w.expectRefusalAfterHello(protocol.ErrorCodeInvalidArgument); got.Message != tc.message {
				t.Errorf("message = %q\nwant      %q", got.Message, tc.message)
			}
		})
	}
	t.Run("a terminal message as the first message", func(t *testing.T) {
		w := st.dialRaw()
		w.sendRaw(websocket.MessageText, `{"type":"terminal.snapshot","cardId":"`+card.ID+`"}`)
		got := w.expectRefusal(protocol.ErrorCodeInvalidArgument)
		if got.Message != "Marshal expected a hello message. Send a message with the type hello." {
			t.Errorf("message = %q", got.Message)
		}
	})
}

// expectRefusalAfterHello is expectRefusal for a client that already said hello: the first frame is
// the resync of a new connection, and the error frame follows it.
func (w *wire) expectRefusalAfterHello(want protocol.ErrorCode) protocol.Error {
	w.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), streamTimeout)
	defer cancel()
	for {
		_, data, err := w.conn.Read(ctx)
		if err != nil {
			w.t.Fatalf("read the error frame: %v", err)
		}
		var frame protocol.ErrorFrame
		if json.Unmarshal(data, &frame) != nil || frame.Type != protocol.FrameTypeError {
			continue
		}
		if frame.Error.Code != want {
			w.t.Fatalf("frame = %s, want an error frame with the code %s", data, want)
		}
		_, _, err = w.conn.Read(ctx)
		if got := websocket.CloseStatus(err); got != websocket.StatusPolicyViolation {
			w.t.Errorf("the stream closed with %v (%v), want a policy violation", got, err)
		}
		return frame.Error
	}
}

// A message that is well made but cannot be done now is not a mistake: the card may just have gone
// back to the chat while a key was on its way. It is answered with a terminal.refused frame, and the
// connection stays open, which the events that follow prove.
func TestATerminalMessageForACardWithNoTerminalKeepsTheConnectionOpen(t *testing.T) {
	st := newStack(t, withTerminals(shellTerminals(t)))
	project, _ := st.addProject("small-repo")
	idle := st.addCard(project.ID, "Never started")
	w := st.dial(protocol.CardTopic(idle.ID), protocol.ProjectTopic(project.ID))

	for name, send := range map[string]func(){
		"input":    func() { w.typeText(idle.ID, "x") },
		"a key":    func() { w.pressKey(idle.ID, protocol.TerminalKeyCtrlC) },
		"a resize": func() { w.resize(idle.ID, 80, 24) },
		"snapshot": func() { w.snapshot(idle.ID) },
	} {
		send()
		refusal := w.nextRefusal()
		if refusal.CardID != idle.ID || refusal.Error.Code != protocol.ErrorCodeRefused ||
			refusal.Error.Details["reason"] != "terminal_not_active" ||
			refusal.Error.Message != "This card has no terminal running. Switch it to terminal view first." {
			t.Errorf("%s: the refusal = %+v", name, refusal)
		}
	}
	// The connection is alive: an event still reaches it.
	st.addCard(project.ID, "Made after the refusals")
	w.until(ofType(protocol.EventTypeCardCreated))

	// A card that does not exist is the same refusal, not a mistake in the client.
	missing := "01M3C107JB041061050R3GG28A"
	w.hello(protocol.CardTopic(idle.ID), protocol.CardTopic(missing))
	w.snapshot(missing)
	if refusal := w.nextRefusal(); refusal.CardID != missing {
		t.Errorf("the refusal is about %s, want %s", refusal.CardID, missing)
	}
}

// A server with no session manager has no terminals, and answers a terminal message the same way.
func TestATerminalMessageWithoutASessionServiceIsRefused(t *testing.T) {
	st := newStack(t, withoutSessions())
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "No sessions")
	w := st.dial(protocol.CardTopic(card.ID))
	w.typeText(card.ID, "x")
	w.nextRefusal()
	w.snapshot(card.ID)
	if refusal := w.nextRefusal(); refusal.Error.Details["reason"] != "terminal_not_active" {
		t.Errorf("the refusal = %+v", refusal)
	}
}

// Terminal output is live only. A client that reconnects is not replayed it, and asks for the screen,
// which holds everything it missed, complete through a number it can compare with the events.
func TestAReconnectingClientAsksForTheScreenInsteadOfBeingReplayedOutput(t *testing.T) {
	st := newStack(t, withTerminals(shellTerminals(t)))
	card, w := startedTerminalCard(t, st, "Reconnects")
	st.switchView(card.ID, protocol.CardViewModeTerminal).want(t, http.StatusOK)
	out := &outputs{t: t, cardID: card.ID}
	w.typeText(card.ID, "seen by the first client\r")
	out.untilHas(w, "seen by the first client\r\nseen by the first client")
	epoch := w.epoch

	// A second client that says it has seen nothing is replayed the ring, which has no output in it.
	second := st.dialRaw()
	second.epoch, second.lastSeq = epoch, 0
	second.hello(protocol.CardTopic(card.ID))
	// It asks for the screen, and gets what it missed. The events that were replayed come before the
	// frame on the connection, so they have all been read once the screen has.
	screen, data := second.untilScreenHas(card.ID, "seen by the first client\r\nseen by the first client")
	if !bytes.Contains(data, []byte("terminal session ")) {
		t.Errorf("the screen %q does not start with what the program said first", data)
	}
	replayed := 0
	for _, ev := range second.queue {
		replayed++
		if ev.Type == protocol.EventTypeSessionTerminalOutput {
			t.Fatalf("the output event %d was replayed", ev.Seq)
		}
	}
	if replayed == 0 {
		t.Error("nothing was replayed, so the test does not prove that output is left out of it")
	}
	// Output after the screen is live, and a screen with a number: the events before it are dropped.
	second.typeText(card.ID, "after\r")
	newer := &outputs{t: t, cardID: card.ID}
	newer.untilHas(second, "after\r\nafter")
	for _, ev := range newer.events {
		if ev.Seq <= screen.ThroughSeq {
			continue
		}
		if bytes.Contains(newer.decode(ev), []byte("seen by the first client")) {
			t.Errorf("event %d, after the screen (through %d), repeats what the screen holds", ev.Seq, screen.ThroughSeq)
		}
	}
}

// What is typed and the size of the view reach the program: the helper prints the size it has when
// it starts, and again each time it is told.
func TestTheSizeOfTheViewReachesTheProgram(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the helper reads its terminal size with ioctl, which the standard library has only on Unix")
	}
	st := newStack(t, withTerminals(helperTerminals(t, "size")))
	card, w := startedTerminalCard(t, st, "Sized")
	st.switchView(card.ID, protocol.CardViewModeTerminal).want(t, http.StatusOK)
	out := &outputs{t: t, cardID: card.ID}
	out.untilHas(w, "size: 120x32")
	w.resize(card.ID, 100, 40)
	out.untilHas(w, "size: 100x40")
	w.resize(card.ID, 1, 1)
	out.untilHas(w, "size: 1x1")
}

// A card in the terminal view is resumed there by a daemon that starts again, on the same session id,
// and the board says so before and after.
func TestATerminalSurvivesADaemonRestart(t *testing.T) {
	st := newStack(t, withTerminals(shellTerminals(t)))
	card, w := startedTerminalCard(t, st, "Restarts in the terminal")
	row, err := st.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatal(err)
	}
	st.switchView(card.ID, protocol.CardViewModeTerminal).want(t, http.StatusOK)
	w.untilScreenHas(card.ID, "terminal session "+row.AgentSessionID)

	st.restart()
	// Before anything resumes it, the card already says which view it was left in.
	if got := st.getCard(card.ID); got.ViewMode != protocol.CardViewModeTerminal || got.Session == nil || *got.Session != protocol.SessionStateAwake {
		t.Errorf("the card after the restart = view %q, session %v, want the terminal view and an awake session", got.ViewMode, got.Session)
	}
	if err := st.mgr.RestoreAll(context.Background()); err != nil {
		t.Fatalf("RestoreAll: %v", err)
	}
	again := st.dial(protocol.CardTopic(card.ID))
	again.untilScreenHas(card.ID, "terminal session "+row.AgentSessionID)
	again.typeText(card.ID, "back\r")
	out := &outputs{t: t, cardID: card.ID}
	out.untilHas(again, "back\r\nback")
	if got := st.getCard(card.ID); got.ViewMode != protocol.CardViewModeTerminal {
		t.Errorf("the card after the restore is in the %q view", got.ViewMode)
	}
}

// A program that prints a great deal, watched by a client that never reads, does not hold up the
// terminal, the daemon, or a client that does read: the one that reads still sees the end of it.
func TestABurstFromATerminalDoesNotStallAnotherClient(t *testing.T) {
	st := newStack(t, withTerminals(helperTerminals(t, "burst", "6000000", "hold")))
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Prints a lot")
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).want(t, http.StatusOK)
	// Nobody reads on this connection, so its queue on the daemon fills and overflows.
	st.dial(protocol.CardTopic(card.ID))
	reader := st.dial(protocol.CardTopic(card.ID))
	st.switchView(card.ID, protocol.CardViewModeTerminal).want(t, http.StatusOK)

	begun := time.Now()
	var tail []byte
	sawEnd := func() bool { return bytes.Contains(tail, []byte("BURST-END")) }
	for !sawEnd() && len(reader.resyncs) == 0 {
		for _, ev := range reader.until(ofType(protocol.EventTypeSessionTerminalOutput)) {
			if ev.Type != protocol.EventTypeSessionTerminalOutput {
				continue
			}
			piece, err := base64.StdEncoding.DecodeString(dataOf[protocol.TerminalOutputEventData](t, ev).Data)
			if err != nil {
				t.Fatalf("an output event is not base64: %v", err)
			}
			tail = append(tail, piece...)
			if len(tail) > 128<<10 {
				tail = append([]byte(nil), tail[len(tail)-64<<10:]...)
			}
		}
	}
	if !sawEnd() {
		// The reader fell behind as well, which the rule allows: it was told to reload, and it asks
		// for the screen, which holds the end of the burst.
		reader.untilScreenHas(card.ID, "BURST-END")
	}
	if took := time.Since(begun); took > streamTimeout {
		t.Errorf("the burst took %v to reach a client that reads", took)
	}
	st.do(http.MethodGet, "/v1/health", nil).want(t, http.StatusOK)
	if got := st.getCard(card.ID); got.ViewMode != protocol.CardViewModeTerminal {
		t.Errorf("the card is in the %q view", got.ViewMode)
	}
}
