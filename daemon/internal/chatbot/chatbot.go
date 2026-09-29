// Package chatbot sends Marshal's notices to a chat service and reads the person's answers back
// (B9.3, build-plan 9.5 and 9.6). Telegram and Discord are the two services, and they are the same
// shape on purpose: a notice is a short title, a body, a link, and the actions a person can take
// from the chat, and an answer is the action's own data coming back.
//
// It is deliberately thin, in the same way internal/tailnet is. Everything the daemon decides about
// a bot - which token to use, when to send, and whether a service is switched on - lives in the API
// layer and internal/integrations. This package holds nothing but how one service is talked to, so
// the API layer reaches a bot through an interface and is tested with a fake and no network.
//
// Nothing here ever reaches a real service in a test: each implementation takes the address it
// talks to, and the tests point it at an in-process fake server. That is why a real bot token is
// never needed to prove this half works (hard rule 3).
package chatbot

import (
	"context"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Kind is which chat service a bot talks to. It is the same word the connection's own id and the
// `integrations` row's kind use, so a bot, a row, and a keychain entry are never told apart.
type Kind string

const (
	// KindTelegram is a Telegram bot, reached through the Bot API.
	KindTelegram Kind = "telegram"
	// KindDiscord is a Discord bot, reached through the REST API.
	KindDiscord Kind = "discord"
	// KindNtfy is an ntfy topic. It only receives notices.
	KindNtfy Kind = "ntfy"
)

// checkConnectionFix is the fix a person reads when a chat service call failed for a reason that is
// not the connection's own fault - a timeout or an unreached service - and is the same sentence for
// both Telegram and Discord.
const checkConnectionFix = "Check this computer's connection, then test again."

// Valid reports whether k is a chat service Marshal knows.
func (k Kind) Valid() bool { return k == KindTelegram || k == KindDiscord || k == KindNtfy }

// Action is one thing a person can do from a notice, drawn as a button where the service has them
// and listed as text where it does not. Data is what comes back when it is pressed; it is opaque to
// the chat service and is Marshal's own.
type Action struct {
	// Label is the button's own words, kept short enough for a phone.
	Label string
	// Data is what Marshal receives back when the action is taken, such as "approve:abc123".
	Data string
}

// Notice is one message Marshal sends to a chat: what happened, and what can be done about it. It
// is the same shape whatever the service, so the routing half (B9.4) never knows which service a
// notice is going to.
type Notice struct {
	// Title is the one line that says what happened, such as "Card 42 needs you".
	Title string
	// Body is the detail under it. Empty when the title says everything.
	Body string
	// URL is where to look, when there is somewhere to look.
	URL string
	// Actions are what the person can do from the chat. Empty is an ordinary notice.
	Actions []Action
}

// Bot is one chat service, reached with one connection. Every method must be safe to call when the
// service is unreachable: an error is returned, never a panic, because a notice that cannot be
// delivered must not take the daemon with it.
type Bot interface {
	// Kind is which service this bot talks to.
	Kind() Kind
	// Notify sends one notice. It is the whole of the sending half.
	Notify(ctx context.Context, notice Notice) error
	// Start reads messages until ctx ends, handing each one to handle. It is the receive half, and
	// it only ever runs for a connection that is switched on: a test never calls it, so no test
	// reaches a real service (hard rule 3). A Telegram bot blocks until ctx ends; a Discord one
	// returns as soon as its gateway is open and keeps receiving on its own.
	Start(ctx context.Context, handle Handler) error
	// Test checks the connection and answers what to show the person, in the shape every other
	// connection test answers in (docs/architecture.md section 18).
	Test(ctx context.Context) (protocol.TestResult, error)
	// Close releases the bot's own resources. It is a no-op on a bot that holds none.
	Close() error
}

// Incoming is one message read from a chat and handed to the daemon's own command handling. It is
// the reading half's own shape, kept apart from a Notice so the two directions never get confused.
type Incoming struct {
	// ChatID is the chat the message came from, so a reply goes back to the same place.
	ChatID string
	// Text is what the person typed. It is empty for a voice note.
	Text string
	// Voice is true when the message was a voice note rather than text. The words are not read
	// here: Marshal asks the person to type instead, so a bot never has to trust a transcript.
	Voice bool
}

// Handler receives one incoming chat message. It is called on the receive loop's own goroutine, so
// it must not block for long; the daemon's handling answers back through the same bot's Notify.
type Handler func(ctx context.Context, in Incoming)

// render turns a notice into the plain text a chat shows. It is plain and not Markdown on purpose:
// a card title is a person's own words, and a title with an underscore or an asterisk in it must not
// fail to send because the words happened to look like formatting.
func render(n Notice) string {
	var b strings.Builder
	if n.Title != "" {
		b.WriteString(n.Title)
	}
	if n.Body != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(n.Body)
	}
	if n.URL != "" {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(n.URL)
	}
	if len(n.Actions) > 0 {
		lines := make([]string, 0, len(n.Actions))
		for _, action := range n.Actions {
			lines = append(lines, action.Label+" ("+action.Data+")")
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(strings.Join(lines, "\n"))
	}
	return b.String()
}
