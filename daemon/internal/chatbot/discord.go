package chatbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Discord talks to a Discord bot through the REST API (B9.3, build-plan 9.6). One bot is one
// connection: a token and the channel notices go to.
type Discord struct {
	session   *discordgo.Session
	channelID string
	now       func() time.Time
}

// DiscordConfig says how to reach one Discord bot.
type DiscordConfig struct {
	// Token is the bot's own token, from the Discord developer portal. It is the secret half and is
	// only ever read from the keychain.
	Token string
	// ChannelID is the channel notices are sent to.
	ChannelID string
	// HTTPClient overrides how Discord's REST API is reached. Nil uses the real one. It exists so a
	// test can point a bot at an in-process fake server through a rewriting transport, and never
	// touch the network (hard rule 3). The gateway is not opened here at all, so the fake server
	// only ever has to answer REST calls.
	HTTPClient *http.Client
	// Now is the clock the test result is stamped with. Nil uses time.Now.
	Now func() time.Time
}

// NewDiscord makes a bot. The token is checked here rather than at the first send, so a connection
// saved with an empty token is refused where a person can see it. Nothing is sent and no request is
// made: the session is built and left closed, and opening the gateway is the receive half's own job.
func NewDiscord(cfg DiscordConfig) (*Discord, error) {
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("a Discord bot needs a token")
	}
	session, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("build the Discord bot: %w", err)
	}
	// Buttons need no privileged intent. Typed replies (approve <id>, new <project> <title>) need the
	// message content intent, which Start asks for only when the developer portal has it switched
	// on: Discord closes the gateway with 4014 for a privileged intent that is not.
	session.Identify.Intents |= discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages
	if cfg.HTTPClient != nil {
		session.Client = cfg.HTTPClient
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Discord{session: session, channelID: cfg.ChannelID, now: now}, nil
}

// Kind is which service this bot talks to.
func (d *Discord) Kind() Kind { return KindDiscord }

// Notify sends one notice to the channel.
func (d *Discord) Notify(_ context.Context, notice Notice) error {
	message := &discordgo.MessageSend{Content: render(notice)}
	if rows := discordButtons(notice.Actions); rows != nil {
		message.Content, message.Components = renderPlain(notice), rows
	}
	if _, err := d.session.ChannelMessageSendComplex(d.channelID, message); err != nil {
		return fmt.Errorf("send a Discord notice: %w", err)
	}
	return nil
}

// Test checks the connection and answers what to show the person. It makes a few REST calls and
// says what each proved: who the bot is (which proves the token), that the channel is one the bot
// can see and send to (which proves the channel id), and whether the bot may read typed replies
// (which is the Message Content Intent). The send really posts a short message, because a channel
// that cannot receive is exactly what the test is for.
func (d *Discord) Test(_ context.Context) (protocol.TestResult, error) {
	checks := make([]protocol.TestCheck, 0, 4)
	checks = append(checks, d.botCheck(), d.channelCheck())
	if typing, known := d.typingCheck(); known {
		checks = append(checks, typing)
	}
	checks = append([]protocol.TestCheck{summaryCheck(checks,
		"Discord is set up and Marshal can send notices to it.",
		"Discord works, with something to check.")}, checks...)
	return protocol.NewTestResult(string(KindDiscord), checks, d.now()), nil
}

// botCheck is who Discord says the bot is. Only a valid token can read it, so its answer is what
// proves the token.
func (d *Discord) botCheck() protocol.TestCheck {
	check := protocol.TestCheck{Name: CheckBot}
	me, err := d.session.User("@me")
	if err != nil {
		check.State = protocol.CheckStateFailed
		check.Message, check.Fix = discordFailure(err, "Marshal could not read the bot's own name.")
		return check
	}
	check.State = protocol.CheckStatePassed
	check.Message = fmt.Sprintf("Discord answered as %s.", me.Username)
	return check
}

// channelCheck is that the channel is one the bot can see, and that a message reaches it. Reading
// the channel first gives its name, so the row says where notices go, and tells a channel that does
// not exist from one the bot may not write in.
func (d *Discord) channelCheck() protocol.TestCheck {
	check := protocol.TestCheck{Name: CheckChat, State: protocol.CheckStateFailed}
	channel, err := d.session.Channel(d.channelID)
	if err != nil {
		check.Message, check.Fix = d.channelFailure(err)
		return check
	}
	if _, err := d.session.ChannelMessageSend(d.channelID, "Marshal is connected to this channel."); err != nil {
		check.Message, check.Fix = discordFailure(err, "Marshal could not send a message to the channel.")
		return check
	}
	check.State = protocol.CheckStatePassed
	check.Message = "Marshal sent a test message to " + channelWords(channel) + "."
	return check
}

// channelWords is how a channel is named in a sentence: #general, or "the channel" when Discord
// gives no name (a direct message has none).
func channelWords(channel *discordgo.Channel) string {
	if channel == nil || channel.Name == "" {
		return "the channel"
	}
	return "#" + channel.Name
}

// channelFailure explains a channel Marshal could not read. A number Discord has no channel for is
// most often a server's id, because a channel's link carries both numbers and the first is the
// server's, so a server answering to it is told apart and named.
func (d *Discord) channelFailure(err error) (message, fix string) {
	if restCode(err) == discordUnknownChannel {
		if _, guildErr := d.session.Guild(d.channelID); guildErr == nil {
			return "That number is a server's id, not a channel's.", channelIDFix
		}
	}
	return discordFailure(err, "Marshal could not read the channel.")
}

// typingCheck is whether the bot may read what a person types in the channel, which is the Message
// Content Intent in the developer portal. The second answer is false when Discord could not be
// asked, so a connection that works is never marked down for a question it did not answer.
func (d *Discord) typingCheck() (protocol.TestCheck, bool) {
	on, known := d.messageContentOn()
	if !known {
		return protocol.TestCheck{}, false
	}
	if on {
		return protocol.TestCheck{
			Name: CheckTyping, State: protocol.CheckStatePassed,
			Message: "Marshal can read replies typed in the channel.",
		}, true
	}
	return protocol.TestCheck{
		Name: CheckTyping, State: protocol.CheckStateWarning,
		Message: "Marshal can send notices and read the buttons, but not replies typed in the channel.",
		Fix: "On the bot's page in the Discord developer portal, switch on Message Content Intent under " +
			"Privileged Gateway Intents, save, and restart Marshal.",
	}, true
}

// The application flags Discord sets when Message Content Intent is switched on in the developer
// portal: the first for a bot in 100 or more servers, the second for a bot in fewer.
const (
	applicationFlagMessageContent        = 1 << 18
	applicationFlagMessageContentLimited = 1 << 19
)

// messageContentOn reads the bot's own application and answers whether Message Content Intent is
// on. The second answer is false when it could not be read.
func (d *Discord) messageContentOn() (on, known bool) {
	body, err := d.session.RequestWithBucketID("GET", discordgo.EndpointApplication("@me"), nil, discordgo.EndpointApplication(""))
	if err != nil {
		return false, false
	}
	var app struct {
		Flags int `json:"flags"`
	}
	if err := json.Unmarshal(body, &app); err != nil {
		return false, false
	}
	return app.Flags&(applicationFlagMessageContent|applicationFlagMessageContentLimited) != 0, true
}

// Start opens the gateway and hands every message to handle until ctx ends. It is the receive half,
// and it only ever runs for a connection that is switched on: a test never calls it, and its fake
// server is never asked for the gateway at all (hard rule 3). A message from a bot is ignored, so
// two bots in one channel cannot answer each other forever.
func (d *Discord) Start(ctx context.Context, handle Handler) error {
	d.session.AddHandler(func(_ *discordgo.Session, i *discordgo.InteractionCreate) {
		d.onInteraction(ctx, i, handle)
	})
	if on, _ := d.messageContentOn(); on {
		d.session.Identify.Intents |= discordgo.IntentMessageContent
	}
	d.session.AddHandler(func(_ *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author == nil || m.Author.Bot {
			return
		}
		handle(ctx, Incoming{ChatID: m.ChannelID, Text: m.Content})
	})
	if err := d.session.Open(); err != nil {
		return fmt.Errorf("open the Discord gateway: %w", err)
	}
	return nil
}

// Discord's limits on buttons: five to a row, five rows, and a custom id of at most 100 characters.
const (
	discordButtonsPerRow = 5
	discordMaxRows       = 5
	discordMaxCustomID   = 100
	discordMaxLabel      = 80
)

// discordButtons draws a notice's actions as buttons, or answers nil when there are none or one
// would not fit Discord's limits, in which case the actions are listed as text.
func discordButtons(actions []Action) []discordgo.MessageComponent {
	if len(actions) == 0 || len(actions) > discordButtonsPerRow*discordMaxRows {
		return nil
	}
	var rows []discordgo.MessageComponent
	var row discordgo.ActionsRow
	for _, action := range actions {
		if action.Data == "" || len(action.Data) > discordMaxCustomID {
			return nil
		}
		label := action.Label
		if len([]rune(label)) > discordMaxLabel {
			label = string([]rune(label)[:discordMaxLabel])
		}
		row.Components = append(row.Components, discordgo.Button{Label: label, Style: discordgo.SecondaryButton, CustomID: action.Data})
		if len(row.Components) == discordButtonsPerRow {
			rows = append(rows, row)
			row = discordgo.ActionsRow{}
		}
	}
	if len(row.Components) > 0 {
		rows = append(rows, row)
	}
	return rows
}

// onInteraction hands a pressed button to the receive loop as if the person had typed what the
// button carries, after telling Discord the press was seen so the button does not show a failure.
func (d *Discord) onInteraction(ctx context.Context, i *discordgo.InteractionCreate, handle Handler) {
	if i == nil || i.Type != discordgo.InteractionMessageComponent {
		return
	}
	data := i.MessageComponentData().CustomID
	_ = d.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})
	if data == "" {
		return
	}
	handle(ctx, Incoming{ChatID: i.ChannelID, Text: data})
}

// Accepts is true for a message from the channel this bot was set up with.
func (d *Discord) Accepts(in Incoming) bool {
	return d.channelID != "" && in.ChatID == d.channelID
}

// Close ends the bot. A session whose gateway was never opened has nothing to close, and a failure
// to close is never worth failing a person's request over.
func (d *Discord) Close() error {
	if d.session == nil {
		return nil
	}
	return d.session.Close()
}

// Discord's own error codes for a channel Marshal cannot use.
const (
	discordUnknownChannel    = 10003
	discordMissingAccess     = 50001
	discordMissingPermission = 50013
)

// channelIDFix is the fix for a channel id that is wrong, in the words a person can follow.
const channelIDFix = "Open the channel itself in Discord, then copy its id (right-click it, or long-press it " +
	"on a phone, and choose Copy Channel ID). Do not copy the server's id."

// restCode is the Discord error code inside a failed call, or 0 when there is none.
func restCode(err error) int {
	var rest *discordgo.RESTError
	if errors.As(err, &rest) && rest.Message != nil {
		return rest.Message.Code
	}
	return 0
}

// discordFailure turns a failed Discord call into the sentence and the fix a person reads. Discord
// answers a bad token with a 401 and a channel it will not serve with a 403 or a 404 whose code says
// which, so those are told apart from each other and from the network, which is not the connection's
// own fault.
func discordFailure(err error, what string) (message, fix string) {
	var rest *discordgo.RESTError
	switch {
	case errors.As(err, &rest) && rest.Response != nil && rest.Response.StatusCode == http.StatusUnauthorized:
		return "Discord refused the bot token.",
			"Copy the token again from the bot's page in the Discord developer portal (Reset Token shows a new one), and save it again."
	case restCode(err) == discordUnknownChannel:
		return "Discord has no channel with that id.", channelIDFix
	case restCode(err) == discordMissingAccess:
		return "The bot cannot see that channel.",
			"Add the bot to the server with its invite link. For a private channel, also give the bot (or its role) access to that channel."
	case restCode(err) == discordMissingPermission:
		return "The bot can see the channel but may not send messages in it.",
			"In the channel's permissions, allow the bot (or its role) View Channel and Send Messages."
	case errors.As(err, &rest) && rest.Response != nil &&
		(rest.Response.StatusCode == http.StatusForbidden || rest.Response.StatusCode == http.StatusNotFound):
		return what + " Discord refused it.",
			"Check that the bot is in the server and can see the channel, and that the id is the channel's, not the server's."
	case errors.As(err, &rest) && rest.Response != nil && rest.Response.StatusCode == http.StatusTooManyRequests:
		return "Discord is asking Marshal to slow down.",
			"Wait a moment, then test again."
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "Discord did not answer in time.", checkConnectionFix
	default:
		return what + " Marshal could not reach Discord.", checkConnectionFix
	}
}
