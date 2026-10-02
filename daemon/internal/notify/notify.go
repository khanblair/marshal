// Package notify decides which notices reach a person and where (B9.4, build-plan 9.7). It is the
// half between the daemon's events and the chat bots: an event type is routed to one or more
// channels, and notices that are not asking for an answer are grouped so a phone is not buzzed five
// times for five things that happened at once.
//
// It talks to no service itself. A Sender is handed in - the daemon's is internal/integrations,
// which builds a bot from what a person saved - so every rule here is proved with a fake sender, in
// process, with no token and no network.
//
// The five-second rule (B9.4: "an event reaches a phone notice in under 5 seconds") is what shapes
// the timing: a notice asking for an answer - an approval - is sent at once, and everything else
// waits at most one grouping window. Both are well inside five seconds, and the window is short
// enough that a grouped notice still feels immediate.
package notify

import (
	"context"
	"fmt"
	"log/slog"
	neturl "net/url"
	"strings"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/chatbot"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Channel names one place a notice can go. It is the same word as the connection's own id and the
// bot's own kind, so a route names the thing it is, with no translation table.
type Channel string

const (
	// ChannelTelegram is the person's Telegram chat.
	ChannelTelegram Channel = "telegram"
	// ChannelDiscord is the person's Discord channel.
	ChannelDiscord Channel = "discord"
	// ChannelNtfy is the person's ntfy topic. It only receives: there is nothing to answer from it.
	ChannelNtfy Channel = "ntfy"
)

// EventTypes are the kinds of thing notices are sent for, named the way the daemon's own events are
// (protocol.EventType). They are the keys a route is stored under.
const (
	// EventApproval is a decision waiting on the person. It is sent at once: a person who has to
	// answer should not also have to wait for a grouping window.
	EventApproval = "approval.requested"
	// EventCIFailed is CI turning red, on a card's branch or on a project's default branch.
	EventCIFailed = "ci.failed"
	// EventCardMerged is a card landing on the default branch.
	EventCardMerged = "card.merged"
	// EventCardDone is a card finishing: it reached the done column.
	EventCardDone = "card.done"
	// EventAgentStuck is a card whose agent cannot go on without the person: stuck, out of budget,
	// asking a question, or holding a plan to review. A permission request is EventApproval instead.
	EventAgentStuck = "agent.stuck"
	// EventNeedsYou is a standing notice: something the app lists for the person.
	EventNeedsYou = "card.needs-you"
)

// DefaultWindow is how long notices that are not asking for an answer wait to be grouped. It is
// short on purpose: long enough that a burst of events becomes one message, short enough that a
// single notice is still within arm's reach of the moment it happened.
const DefaultWindow = 1500 * time.Millisecond

// Event is one thing that happened, ready to be routed.
type Event struct {
	// Type is what kind of thing it is, one of the Event* words. An event type with no route goes
	// to the default channels rather than nowhere.
	Type string
	// Title is the one line a notice leads with.
	Title string
	// Body is the detail under it.
	Body string
	// URL is where to look.
	URL string
	// Actions are what the person can do from the chat. An event with actions is sent at once.
	Actions []chatbot.Action
	// CardID is the daemon's id of the card the event is about. Each channel turns it into the link
	// that channel can open (see linkFor); an event about no card carries none.
	CardID string
}

// actionable reports whether an event is asking for an answer, which is sent at once rather than
// grouped.
func (e Event) actionable() bool { return len(e.Actions) > 0 }

// noticeFor turns an event into the notice one channel shows, with the link that channel can open.
func (s *Service) noticeFor(e Event, channel Channel) chatbot.Notice {
	url := e.URL
	if url == "" {
		url = s.linkFor(channel, e.CardID)
	}
	return chatbot.Notice{Title: e.Title, Body: e.Body, URL: url, Actions: e.Actions}
}

// linkFor is where a notice about a card leads. The ntfy app opens the phone app through its own
// scheme; a chat cannot link a custom scheme, so it gets the daemon's page with the card asked for.
// Without a known address a chat gets no link at all, which is better than one that cannot open.
func (s *Service) linkFor(channel Channel, cardID string) string {
	if cardID == "" {
		return ""
	}
	if channel == ChannelNtfy {
		return "marshal://card/" + cardID
	}
	if s.link == nil {
		return ""
	}
	base := strings.TrimRight(s.link(), "/")
	if base == "" {
		return ""
	}
	return base + "/?open=" + neturl.QueryEscape("card/"+cardID)
}

// Sender sends one notice to one channel. The daemon's own is backed by a chat bot; a test's is a
// slice of what it was handed.
type Sender interface {
	Send(ctx context.Context, channel Channel, notice chatbot.Notice) error
}

// SenderFunc adapts a function to a Sender.
type SenderFunc func(ctx context.Context, channel Channel, notice chatbot.Notice) error

// Send calls the function.
func (f SenderFunc) Send(ctx context.Context, channel Channel, notice chatbot.Notice) error {
	return f(ctx, channel, notice)
}

// Options tunes a Service. Every field may be left out.
type Options struct {
	// Logger records a notice that could not be delivered. Nil means nothing is logged.
	Logger *slog.Logger
	// Window is how long a groupable notice waits. Below one means DefaultWindow.
	Window time.Duration
	// Routes overrides the routing table. Empty uses DefaultRoutes.
	Routes []Route
	// LinkBase answers the address a phone opens Marshal at, such as http://name.tail1.ts.net:47800,
	// or "" while the daemon does not know one. Nil means it never does, so notices carry no link.
	LinkBase func() string
}

// Route says which channels one kind of event goes to.
type Route struct {
	// Event is the event type.
	Event string
	// Channels are where it goes, in the order it is tried.
	Channels []Channel
}

// DefaultRoutes is where each event type goes out of the box: the two chat services for the things
// that want an answer, and Telegram alone for the quieter ones, so a busy channel does not become
// the place a person ignores.
func DefaultRoutes() []Route {
	return []Route{
		{Event: EventApproval, Channels: []Channel{ChannelTelegram, ChannelDiscord}},
		{Event: EventCIFailed, Channels: []Channel{ChannelTelegram}},
		{Event: EventCardMerged, Channels: []Channel{ChannelTelegram}},
		{Event: EventCardDone, Channels: []Channel{ChannelTelegram}},
		{Event: EventAgentStuck, Channels: []Channel{ChannelTelegram, ChannelDiscord}},
		{Event: EventNeedsYou, Channels: []Channel{ChannelTelegram, ChannelDiscord}},
	}
}

// Service routes events to channels and groups the ones that do not need an answer. It is safe for
// concurrent use.
type Service struct {
	sender Sender
	log    *slog.Logger
	window time.Duration
	link   func() string
	mu     sync.Mutex
	routes map[string][]Channel
	// defaultChannels are where an event type with no route of its own goes.
	defaultChannels []Channel
	// pending holds the groupable notices per channel until the next flush.
	pending map[Channel][]chatbot.Notice
	// sent is the ids of standing notices this router has already delivered, so the whole-list
	// payload of notice.created sends only what is new (bus.go).
	sent map[string]struct{}
	// quiet, when set, says notices that are not asking for an answer should wait: a calendar event is
	// on. It is asked only when something is waiting.
	quiet func(ctx context.Context) bool
	// ciStatus is the last default-branch state seen per project, so a red branch is told once and
	// not on every run that leaves it red.
	ciStatus map[string]protocol.CIState
}

// New builds a router over a sender. The sender is required: a router with nowhere to send is not a
// router.
func New(sender Sender, opts Options) (*Service, error) {
	if sender == nil {
		return nil, fmt.Errorf("notifications need somewhere to send")
	}
	log := opts.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	window := opts.Window
	if window <= 0 {
		window = DefaultWindow
	}
	routes := opts.Routes
	if len(routes) == 0 {
		routes = DefaultRoutes()
	}
	s := &Service{
		sender: sender, log: log, window: window, link: opts.LinkBase,
		routes:   make(map[string][]Channel, len(routes)),
		pending:  make(map[Channel][]chatbot.Notice),
		sent:     make(map[string]struct{}),
		ciStatus: make(map[string]protocol.CIState),
	}
	for _, route := range routes {
		s.routes[route.Event] = append([]Channel(nil), route.Channels...)
	}
	s.defaultChannels = []Channel{ChannelTelegram}
	return s, nil
}

// SetQuiet says when grouped notices are held back instead of sent. quiet is asked each time there
// is something to send, so it should be cheap; nil means never. A notice asking for an answer is
// never held. What waited is sent, as one message per channel, once quiet says no.
func (s *Service) SetQuiet(quiet func(ctx context.Context) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quiet = quiet
}

// maxHeld is the most notices one channel keeps while quiet. A long meeting never grows the list
// without bound: the oldest are dropped, and the message that is sent says how many.
const maxHeld = 100

// Window is how long a groupable notice waits before it is sent.
func (s *Service) Window() time.Duration { return s.window }

// SetRoute changes where one event type goes, replacing any route it had. No channels at all turns
// that event type off: it is stored as a route with no channels, which is deliberately different
// from having no route, because an unrouted type goes to the default channels.
func (s *Service) SetRoute(event string, channels ...Channel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if channels == nil {
		s.routes[event] = []Channel{}
		return
	}
	s.routes[event] = append([]Channel(nil), channels...)
}

// Routes answers the routing table, one row per event type that has a route of its own, in the
// order they were set.
func (s *Service) Routes() []Route {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Route, 0, len(s.routes))
	for event, channels := range s.routes {
		out = append(out, Route{Event: event, Channels: append([]Channel(nil), channels...)})
	}
	return out
}

// Notify routes one event. An event that is asking for an answer is sent at once, because a person
// who has to decide should not also wait; everything else waits for the next flush, where a burst of
// events becomes one message per channel.
func (s *Service) Notify(ctx context.Context, event Event) {
	channels := s.channelsFor(event.Type)
	if len(channels) == 0 {
		return
	}
	if event.actionable() {
		for _, channel := range channels {
			s.send(ctx, []Channel{channel}, s.noticeFor(event, channel))
		}
		return
	}
	s.mu.Lock()
	for _, channel := range channels {
		s.pending[channel] = append(s.pending[channel], s.noticeFor(event, channel))
	}
	s.mu.Unlock()
}

// Flush sends everything that has grouped so far, one message per channel. It is what the ticker
// calls, and it is exported so a test can flush without waiting out the window.
func (s *Service) Flush(ctx context.Context) {
	s.mu.Lock()
	waiting := false
	for _, notices := range s.pending {
		waiting = waiting || len(notices) > 0
	}
	quiet := s.quiet
	s.mu.Unlock()
	if waiting && quiet != nil && quiet(ctx) {
		s.mu.Lock()
		for channel, notices := range s.pending {
			if extra := len(notices) - maxHeld; extra > 0 {
				s.pending[channel] = notices[extra:]
			}
		}
		s.mu.Unlock()
		return
	}
	s.mu.Lock()
	pending := s.pending
	s.pending = make(map[Channel][]chatbot.Notice)
	s.mu.Unlock()
	for channel, notices := range pending {
		if len(notices) == 0 {
			continue
		}
		s.send(ctx, []Channel{channel}, group(notices))
	}
}

// Run flushes grouped notices on the window until ctx ends.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(s.window)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.Flush(context.WithoutCancel(ctx))
			return
		case <-ticker.C:
			s.Flush(ctx)
		}
	}
}

// channelsFor answers where one event type goes. A type with no route of its own goes to the
// default channels rather than nowhere, so a new event type is not silently dropped.
func (s *Service) channelsFor(event string) []Channel {
	s.mu.Lock()
	defer s.mu.Unlock()
	if channels, ok := s.routes[event]; ok {
		return append([]Channel(nil), channels...)
	}
	return append([]Channel(nil), s.defaultChannels...)
}

// send delivers one notice to each channel. A channel that refuses is logged and the rest are still
// tried: one broken service must not stop the other from reaching the person.
func (s *Service) send(ctx context.Context, channels []Channel, notice chatbot.Notice) {
	for _, channel := range channels {
		if err := s.sender.Send(ctx, channel, notice); err != nil {
			s.log.Warn("a notice could not be delivered", "channel", channel, "err", err)
		}
	}
}

// group folds several notices to one channel into a single message, so a burst of events is one
// buzz and not several. A single notice is sent as it is, because wrapping one thing in "1 thing
// happened" only makes it harder to read.
func group(notices []chatbot.Notice) chatbot.Notice {
	if len(notices) == 1 {
		return notices[0]
	}
	var body strings.Builder
	for i, notice := range notices {
		if i > 0 {
			body.WriteString("\n")
		}
		body.WriteString("- ")
		body.WriteString(notice.Title)
		if notice.URL != "" {
			body.WriteString(" ")
			body.WriteString(notice.URL)
		}
	}
	return chatbot.Notice{
		Title: fmt.Sprintf("%d things happened", len(notices)),
		Body:  body.String(),
	}
}
