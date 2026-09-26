package session

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A card's terminal (docs/architecture.md 4.3 and 11.2, checklist item B2.7).
//
// While a card is in the terminal view, its live session runs the agent's own interactive command in
// a pseudo-terminal. The pump publishes what the terminal prints as session.terminal_output events,
// which are live-only: they never enter the replay ring, so a viewer that opens late, or reconnects,
// asks for the screen instead (TerminalScreen). The screen it is given and the events that follow it
// must join without a byte repeated or missing, and a terminal cannot tolerate either, so the screen
// is kept here, in the same lock that numbers each event: the screen a viewer reads is complete
// through exactly one event number, and it applies only the events after it.

const (
	// terminalScreenBytes is how much of the latest output a terminal keeps for a viewer that opens
	// late. It matches the PTY adapter's own ring, which is what a viewer of the adapter alone gets.
	terminalScreenBytes = 256 << 10
	// terminalInputQueue is how many typed messages wait for the terminal's writer. A person types a
	// few keys a second, and a paste arrives as a few messages, so a full queue means the program is
	// not reading its input at all.
	terminalInputQueue = 64
)

// The sentences a person reads when a terminal call is refused, with the stable reason beside each.
const (
	messageTerminalNotActive = "This card has no terminal running. Switch it to terminal view first."
	messageTerminalBusy      = "The terminal is not keeping up with what you type. Wait a moment, then type again."
)

// terminalSession is what a live session keeps while its agent runs in a terminal.
type terminalSession struct {
	agent  agents.Terminal
	handle agents.SessionHandle
	// input holds what was typed and has not reached the program yet. The stream never writes to the
	// program itself, because a write blocks while the program does not read; a writer goroutine
	// drains this queue instead (Manager.pumpTerminalInput), and a full queue is refused.
	input chan []byte
	// done is closed when the session ends, and stops the writer.
	done     chan struct{}
	doneOnce sync.Once

	// mu keeps the screen and the event number it is complete through together. It is held while
	// an output event is published, so no viewer can read a screen that is ahead of, or behind, the
	// number it reports.
	mu      sync.Mutex
	screen  []byte
	through uint64
}

func newTerminalSession(agent agents.Terminal, handle agents.SessionHandle) *terminalSession {
	return &terminalSession{
		agent: agent, handle: handle,
		input: make(chan []byte, terminalInputQueue), done: make(chan struct{}),
	}
}

// close ends the writer. It is safe to call more than once.
func (t *terminalSession) close() {
	t.doneOnce.Do(func() { close(t.done) })
}

// enqueue puts typed bytes in the queue, and reports whether there was room.
func (t *terminalSession) enqueue(data []byte) bool {
	select {
	case t.input <- data:
		return true
	default:
		return false
	}
}

// publish sends a piece of output to the card's topic as a live-only event and adds it to the
// screen, as one step. The bytes are encoded before the lock is taken.
func (t *terminalSession) publish(bus *events.Bus, cardID string, data []byte) {
	payload := protocol.TerminalOutputEventData{CardID: cardID, Data: base64.StdEncoding.EncodeToString(data)}
	t.mu.Lock()
	defer t.mu.Unlock()
	seq := bus.PublishLive(string(protocol.CardTopic(cardID)), string(protocol.EventTypeSessionTerminalOutput), payload)
	t.screen = append(t.screen, data...)
	if extra := len(t.screen) - terminalScreenBytes; extra > 0 {
		t.screen = append(t.screen[:0], t.screen[extra:]...)
	}
	if seq != 0 { // The bus is closed when the daemon is shutting down, and nobody is left to hear it.
		t.through = seq
	}
}

// snapshot returns a copy of the screen and the number of the newest event it includes.
func (t *terminalSession) snapshot() (screen []byte, through uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]byte(nil), t.screen...), t.through
}

// terminalNotActive is the refusal for a call about a card that has no terminal running.
func terminalNotActive(cardID string) *protocol.Error {
	return protocol.Refused(messageTerminalNotActive).With("cardId", cardID).
		With("reason", string(protocol.TerminalRefusalReasonNotActive))
}

// terminalOf returns the live terminal of a card, or the refusal that says it has none.
func (m *Manager) terminalOf(cardID string) (*terminalSession, error) {
	ls := m.liveOf(cardID)
	if ls == nil || ls.term == nil {
		return nil, terminalNotActive(cardID)
	}
	return ls.term, nil
}

// TerminalInput queues what the person typed for a card's terminal. It never waits for the program:
// the bytes go to a bounded queue that a writer goroutine drains, so a program that stopped reading
// cannot hold up the event stream that carries the keys. A full queue is refused and the input is
// dropped, since keys that arrive late and out of order do more harm than keys that are lost. The
// bytes are never logged. A card with no terminal running is refused.
func (m *Manager) TerminalInput(cardID string, data []byte) error {
	term, err := m.terminalOf(cardID)
	if err != nil {
		return err
	}
	if !term.enqueue(data) {
		return protocol.Refused(messageTerminalBusy).With("cardId", cardID).
			With("reason", string(protocol.TerminalRefusalReasonBusy))
	}
	return nil
}

// TerminalResize tells a card's terminal the size of the view that draws it. The caller has checked
// the size against the protocol's limits. A terminal whose program already ended is refused.
func (m *Manager) TerminalResize(ctx context.Context, cardID string, cols, rows int) error {
	term, err := m.terminalOf(cardID)
	if err != nil {
		return err
	}
	if err := term.agent.Resize(ctx, term.handle, cols, rows); err != nil {
		if errors.Is(err, agents.ErrStopped) || errors.Is(err, agents.ErrUnknownSession) {
			return terminalNotActive(cardID)
		}
		return fmt.Errorf("resize the terminal of card %s: %w", cardID, err)
	}
	return nil
}

// TerminalScreen is the recent screen of a card's terminal, and the number of the newest output
// event it includes, for a viewer that opens the terminal view or reconnects. A card with no
// terminal running is refused.
func (m *Manager) TerminalScreen(cardID string) (protocol.TerminalScreen, error) {
	term, err := m.terminalOf(cardID)
	if err != nil {
		return protocol.TerminalScreen{}, err
	}
	cols, rows, err := term.agent.Size(term.handle)
	if err != nil {
		return protocol.TerminalScreen{}, terminalNotActive(cardID)
	}
	screen, through := term.snapshot()
	return protocol.TerminalScreen{
		CardID: cardID, Cols: cols, Rows: rows, ThroughSeq: through,
		Data: base64.StdEncoding.EncodeToString(screen),
	}, nil
}

// publishTerminalOutput publishes what a terminal printed. It is called by the pump for the
// session's own events, so it is the only writer of the screen.
func (m *Manager) publishTerminalOutput(ls *liveSession, ev agents.TerminalOutput) {
	if ls.term == nil {
		return
	}
	ls.term.publish(m.bus, ls.cardID, ev.Data)
}

// pumpTerminalInput writes queued input into a terminal until its session ends. The write can wait
// for a program that does not read, which is why it has a goroutine of its own, and ending the
// session closes the terminal and so lets it return.
func (m *Manager) pumpTerminalInput(ls *liveSession) {
	defer m.pumpWG.Done()
	term := ls.term
	for {
		select {
		case data := <-term.input:
			if err := term.agent.WriteRaw(m.ctx, term.handle, data); err != nil && !errors.Is(err, agents.ErrStopped) {
				// What was typed is never logged: it can hold a password.
				m.log.Warn("could not write to a terminal", "card_id", ls.cardID, "bytes", len(data), "error", err)
			}
		case <-term.done:
			return
		}
	}
}

// NotInTerminal is the refusal for a call about the terminal of a card that has none running, for
// the callers that know a card has no terminal without asking the manager.
func NotInTerminal(cardID string) *protocol.Error { return terminalNotActive(cardID) }
