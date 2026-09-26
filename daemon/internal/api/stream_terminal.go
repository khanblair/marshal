package api

import (
	"context"
	"errors"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
)

// terminalControl is what the stream needs from the session manager to serve a card's terminal.
// None of its calls waits for the program that runs in the terminal, so the pump, which is the only
// writer of frames, can make them.
type terminalControl interface {
	TerminalInput(cardID string, data []byte) error
	TerminalResize(ctx context.Context, cardID string, cols, rows int) error
	TerminalScreen(cardID string) (protocol.TerminalScreen, error)
}

var _ terminalControl = (*session.Manager)(nil)

// onMessage applies one message from the client: a new Hello, or one of the three terminal
// messages. The pump owns the topics the connection follows, so it is where a terminal message is
// checked against them, and where the frames that answer one are written, in order with the events.
func (p *pump) onMessage(ctx context.Context, msg clientMessage) error {
	switch m := msg.(type) {
	case protocol.Hello:
		return p.replace(ctx, m)
	case protocol.TerminalInput:
		data := m.Key.Bytes()
		if m.Key == "" {
			data = []byte(m.Data)
		}
		return p.onTerminal(ctx, m.CardID, func(t terminalControl) error { return t.TerminalInput(m.CardID, data) })
	case protocol.TerminalResize:
		return p.onTerminal(ctx, m.CardID, func(t terminalControl) error {
			return t.TerminalResize(ctx, m.CardID, m.Cols, m.Rows)
		})
	case protocol.TerminalSnapshotRequest:
		return p.sendScreen(ctx, m.CardID)
	}
	return nil
}

// onTerminal runs a terminal call for a card the connection follows. A connection that does not
// follow the card made a mistake, which ends it like any bad message. A call that the terminal
// refuses is answered on the connection, which stays open.
func (p *pump) onTerminal(ctx context.Context, cardID string, call func(terminalControl) error) error {
	if err := p.checkFollows(ctx, cardID); err != nil {
		return err
	}
	if p.st.hub.terminals == nil {
		return p.refuseTerminal(ctx, cardID, session.NotInTerminal(cardID))
	}
	if err := call(p.st.hub.terminals); err != nil {
		return p.refuseTerminal(ctx, cardID, err)
	}
	return nil
}

// sendScreen answers a request for a card's screen. Events that are waiting go out first, so the
// order the client sees is the order the pump handled things in: everything before the screen is
// something the screen includes or the client had not asked for yet, and everything after it is
// newer or is dropped by the number the screen carries.
func (p *pump) sendScreen(ctx context.Context, cardID string) error {
	if err := p.checkFollows(ctx, cardID); err != nil {
		return err
	}
	if p.st.hub.terminals == nil {
		return p.refuseTerminal(ctx, cardID, session.NotInTerminal(cardID))
	}
	if err := p.flush(ctx); err != nil {
		return err
	}
	screen, err := p.st.hub.terminals.TerminalScreen(cardID)
	if err != nil {
		return p.refuseTerminal(ctx, cardID, err)
	}
	return p.st.send(ctx, p.ws, screen)
}

// checkFollows refuses a terminal message for a card whose topic the connection does not follow. It
// is a mistake in the client, so it is answered like one: an error frame, and the connection closes.
func (p *pump) checkFollows(ctx context.Context, cardID string) error {
	if _, follows := p.topics[protocol.CardTopic(cardID)]; follows {
		return nil
	}
	return p.st.refuse(ctx, p.ws, protocol.InvalidArgument(
		"Follow a card's topic before you use its terminal. Add the card to the topics of a hello first.").
		With("topic", string(protocol.CardTopic(cardID))))
}

// refuseTerminal answers a terminal message that cannot be done now with a terminal.refused frame.
// An error that is not an answer already is the daemon's own trouble: it is logged, and the client is
// told the terminal is not there, which is what it can act on.
func (p *pump) refuseTerminal(ctx context.Context, cardID string, err error) error {
	var answer *protocol.Error
	if !errors.As(err, &answer) {
		p.st.log.Warn("a terminal call failed", "card_id", cardID, "error", err)
		answer = session.NotInTerminal(cardID)
	}
	return p.st.send(ctx, p.ws, protocol.TerminalRefusal{CardID: cardID, Error: *answer})
}
