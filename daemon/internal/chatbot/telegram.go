package chatbot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	tgbotmodels "github.com/go-telegram/bot/models"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Telegram talks to a Telegram bot through the Bot API (B9.3, build-plan 9.5). One bot is one
// connection: a token and the chat notices go to.
type Telegram struct {
	bot    *tgbot.Bot
	chatID any
	// chat is the chat as it was configured, kept as typed to check who a message came from.
	chat string
	now  func() time.Time

	// handle is the receive loop's own handler, set once by Start and read by the update handler.
	// The mutex is here because Start is called from the daemon's goroutine while an update may
	// already be arriving on the library's own.
	mu     sync.Mutex
	handle Handler
}

// TelegramConfig says how to reach one Telegram bot.
type TelegramConfig struct {
	// Token is the bot's token, the one BotFather hands out. It is the secret half and is only ever
	// read from the keychain.
	Token string
	// ChatID is the chat notices are sent to: the person's own chat, or a group's. A numeric id is
	// sent as a number, because that is what the Bot API expects; anything else, such as
	// "@my_channel", is sent as it is.
	ChatID string
	// BaseURL overrides where the Bot API is reached. Empty uses Telegram. It exists so a test can
	// point a bot at an in-process fake server and never touch the network (hard rule 3).
	BaseURL string
	// Now is the clock the test result is stamped with. Nil uses time.Now.
	Now func() time.Time
}

// NewTelegram makes a bot. The token is checked here rather than at the first send, so a connection
// saved with an empty token is refused where a person can see it. Nothing is sent and no request is
// made: the bot is built without asking Telegram who it is, which is what the connection test is
// for, so saving a token never depends on the network.
func NewTelegram(cfg TelegramConfig) (*Telegram, error) {
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("a Telegram bot needs a token")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	t := &Telegram{chatID: telegramChatID(cfg.ChatID), chat: strings.TrimSpace(cfg.ChatID), now: now}
	// The default handler catches every update that has no more specific one, which is every
	// message including a voice note. Registering only a text handler would silently drop a voice
	// note, and a dropped message looks exactly like a broken bot.
	opts := []tgbot.Option{tgbot.WithSkipGetMe(), tgbot.WithDefaultHandler(t.onUpdate)}
	if cfg.BaseURL != "" {
		opts = append(opts, tgbot.WithServerURL(cfg.BaseURL))
	}
	bot, err := tgbot.New(cfg.Token, opts...)
	if err != nil {
		return nil, fmt.Errorf("build the Telegram bot: %w", err)
	}
	t.bot = bot
	return t, nil
}

// Start reads messages until ctx ends, handing each to handle. It is the receive half, and it only
// ever runs for a connection that is switched on: a test never calls it, so no test reaches a real
// service (hard rule 3). A voice note is handed over as one, with no text, so the daemon answers it
// without ever trusting a transcript.
func (t *Telegram) Start(ctx context.Context, handle Handler) error {
	t.mu.Lock()
	t.handle = handle
	t.mu.Unlock()
	t.bot.Start(ctx)
	return nil
}

// onUpdate hands one update to the receive loop's handler. An update the library sends that is not a
// message at all is ignored, because there is nothing for a person to have said.
func (t *Telegram) onUpdate(ctx context.Context, _ *tgbot.Bot, update *tgbotmodels.Update) {
	if update.CallbackQuery != nil {
		t.onCallback(ctx, update.CallbackQuery)
		return
	}
	message := update.Message
	if message == nil {
		return
	}
	t.mu.Lock()
	handle := t.handle
	t.mu.Unlock()
	if handle == nil {
		return
	}
	handle(ctx, Incoming{
		ChatID:   strconv.FormatInt(message.Chat.ID, 10),
		ChatName: chatName(message.Chat.Username),
		Text:     message.Text,
		Voice:    message.Voice != nil,
	})
}

// Accepts is true for a message from the chat this bot was set up with: the same numeric id, or the
// channel name it was given. A message from any other chat is not acted on.
func (t *Telegram) Accepts(in Incoming) bool {
	if t.chat == "" {
		return false
	}
	return in.ChatID == t.chat || (in.ChatName != "" && strings.EqualFold(in.ChatName, t.chat))
}

// telegramChatID turns a typed chat into what the Bot API expects: a number when the person gave a
// number, and the string as it is when they gave a channel name.
func telegramChatID(given string) any {
	trimmed := strings.TrimSpace(given)
	if id, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return id
	}
	return trimmed
}

// Kind is which service this bot talks to.
func (t *Telegram) Kind() Kind { return KindTelegram }

// Notify sends one notice to the chat.
func (t *Telegram) Notify(ctx context.Context, notice Notice) error {
	params := &tgbot.SendMessageParams{ChatID: t.chatID, Text: render(notice)}
	if keyboard := telegramKeyboard(notice.Actions); keyboard != nil {
		params.Text, params.ReplyMarkup = renderPlain(notice), keyboard
	}
	if _, err := t.bot.SendMessage(ctx, params); err != nil {
		return fmt.Errorf("send a Telegram notice: %w", err)
	}
	return nil
}

// Test checks the connection and answers what to show the person. It makes two calls and says what
// each proved: who the bot is (which proves the token), and that a message reaches the chat (which
// proves the chat id). The second one really sends a short message, because a chat id that cannot
// receive is exactly what the test is for; the message it sends says so.
func (t *Telegram) Test(ctx context.Context) (protocol.TestResult, error) {
	checks := make([]protocol.TestCheck, 0, 3)
	checks = append(checks, t.botCheck(ctx))
	checks = append(checks, t.chatCheck(ctx))
	checks = append([]protocol.TestCheck{summaryCheck(checks,
		"Telegram is set up and Marshal can send notices to it.",
		"Telegram works, with something to check.")}, checks...)
	return protocol.NewTestResult(string(KindTelegram), checks, t.now()), nil
}

// botCheck is who Telegram says the bot is. Only a valid token can read it, so its answer is what
// proves the token.
func (t *Telegram) botCheck(ctx context.Context) protocol.TestCheck {
	check := protocol.TestCheck{Name: CheckBot}
	me, err := t.bot.GetMe(ctx)
	if err != nil {
		check.State = protocol.CheckStateFailed
		check.Message, check.Fix = telegramFailure(err, "Marshal could not read the bot's own name.")
		return check
	}
	check.State = protocol.CheckStatePassed
	check.Message = fmt.Sprintf("Telegram answered as @%s.", me.Username)
	return check
}

// chatCheck is that a message reaches the chat. It sends a short, obvious test message so a person
// who has the chat open sees it prove itself.
func (t *Telegram) chatCheck(ctx context.Context) protocol.TestCheck {
	check := protocol.TestCheck{Name: CheckChat}
	if _, err := t.bot.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID: t.chatID,
		Text:   "Marshal is connected to this chat.",
	}); err != nil {
		check.State = protocol.CheckStateFailed
		check.Message, check.Fix = telegramFailure(err, "Marshal could not send a message to the chat.")
		return check
	}
	check.State = protocol.CheckStatePassed
	check.Message = "Marshal sent a test message to the chat."
	return check
}

// Close is a no-op: a Telegram bot holds an HTTP client and nothing that needs releasing.
func (t *Telegram) Close() error { return nil }

// telegramFailure turns a failed Telegram call into the sentence and the fix a person reads. An
// unauthorized answer is the token; a forbidden or bad request is usually the chat id; a rate limit
// is Telegram asking Marshal to slow down; anything else is the network, which is not the
// connection's own fault.
func telegramFailure(err error, what string) (message, fix string) {
	switch {
	case errors.Is(err, tgbot.ErrorUnauthorized):
		return "Telegram refused the bot token.",
			"Check the token BotFather gave you, and save it again."
	case errors.Is(err, tgbot.ErrorForbidden), errors.Is(err, tgbot.ErrorBadRequest),
		errors.Is(err, tgbot.ErrorNotFound):
		return what + " Telegram refused it.",
			"Check that the bot is in the chat, and that the chat id is right."
	case errors.Is(err, tgbot.ErrorTooManyRequests):
		return "Telegram is asking Marshal to slow down.",
			"Wait a moment, then test again."
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "Telegram did not answer in time.", checkConnectionFix
	default:
		return what + " Marshal could not reach Telegram.", checkConnectionFix
	}
}

// onCallback hands a pressed button to the receive loop as if the person had typed what the button
// carries, and tells Telegram the press was seen so the button stops spinning.
func (t *Telegram) onCallback(ctx context.Context, query *tgbotmodels.CallbackQuery) {
	_, _ = t.bot.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
	message := query.Message.Message
	if message == nil || query.Data == "" {
		return
	}
	t.mu.Lock()
	handle := t.handle
	t.mu.Unlock()
	if handle == nil {
		return
	}
	handle(ctx, Incoming{
		ChatID:   strconv.FormatInt(message.Chat.ID, 10),
		ChatName: chatName(message.Chat.Username),
		Text:     query.Data,
	})
}

// maxTelegramCallbackBytes is the longest text Telegram lets a button carry back.
const maxTelegramCallbackBytes = 64

// telegramKeyboard draws a notice's actions as one row of buttons each, or answers nil when there
// are none or one would not fit Telegram's limit, in which case the actions are listed as text.
func telegramKeyboard(actions []Action) *tgbotmodels.InlineKeyboardMarkup {
	if len(actions) == 0 {
		return nil
	}
	rows := make([][]tgbotmodels.InlineKeyboardButton, 0, len(actions))
	for _, action := range actions {
		if action.Data == "" || len(action.Data) > maxTelegramCallbackBytes {
			return nil
		}
		rows = append(rows, []tgbotmodels.InlineKeyboardButton{{Text: action.Label, CallbackData: action.Data}})
	}
	return &tgbotmodels.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// chatName is a chat's public name with its @, or empty when it has none.
func chatName(username string) string {
	if username == "" {
		return ""
	}
	return "@" + username
}
