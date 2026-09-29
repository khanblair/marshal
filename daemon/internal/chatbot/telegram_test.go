package chatbot_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/chatbot"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Talking to Telegram (B9.3, build-plan 9.5). Every test points the bot at an in-process fake server
// and uses a made-up token, so no test ever reaches Telegram and no real bot token is ever used
// (hard rule 3).

// telegramCalls counts what the bot asked the fake server for.
type telegramCalls struct {
	getMe       int
	sendMessage int
	answered    int
	sent        []string
	raw         []string
}

// telegramServer answers the two Bot API calls the bot makes, in the shape Telegram answers them.
// A refuse flag makes it answer the way Telegram answers a token it does not like.
func telegramServer(t *testing.T, refuse bool) (*httptest.Server, *telegramCalls) {
	t.Helper()
	calls := &telegramCalls{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		switch method {
		case "getMe":
			calls.getMe++
			if refuse {
				telegramError(w, http.StatusUnauthorized, 401, "Unauthorized")
				return
			}
			telegramOK(w, `{"id":1,"is_bot":true,"first_name":"Marshal","username":"marshal_bot"}`)
		case "sendMessage":
			calls.sendMessage++
			body, _ := io.ReadAll(r.Body)
			calls.sent = append(calls.sent, telegramText(body))
			calls.raw = append(calls.raw, string(body))
			if refuse {
				telegramError(w, http.StatusBadRequest, 400, "Bad Request: chat not found")
				return
			}
			telegramOK(w, `{"message_id":1,"date":0,"chat":{"id":1,"type":"private"},"text":"ok"}`)
		case "answerCallbackQuery":
			calls.answered++
			telegramOK(w, `true`)
		default:
			http.Error(w, "no such method", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, calls
}

// telegramOK writes a successful Bot API answer around result.
func telegramOK(w http.ResponseWriter, result string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true,"result":` + result + `}`))
}

// telegramError writes a refused Bot API answer the way Telegram does, with the HTTP status and the
// envelope's own error code agreeing.
func telegramError(w http.ResponseWriter, status, code int, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"ok":false,"error_code":` + strconv.Itoa(code) + `,"description":"` + description + `"}`))
}

// telegramText reads the text out of either a form-encoded or a JSON sendMessage body.
func telegramText(body []byte) string {
	text := string(body)
	if i := strings.Index(text, `"text":"`); i >= 0 {
		rest := text[i+len(`"text":"`):]
		if j := strings.Index(rest, `"`); j >= 0 {
			return rest[:j]
		}
	}
	return text
}

func TestTelegramConnectionTestPassesWhenTheBotAndChatAnswer(t *testing.T) {
	srv, calls := telegramServer(t, false)
	bot, err := chatbot.NewTelegram(chatbot.TelegramConfig{
		Token: "123:TESTTOKEN", ChatID: "555", BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("NewTelegram: %v", err)
	}
	result, err := bot.Test(context.Background())
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !result.OK {
		t.Fatalf("the connection test did not pass: %+v", result.Checks)
	}
	if calls.getMe != 1 || calls.sendMessage != 1 {
		t.Errorf("the fake server saw getMe %d and sendMessage %d, want 1 and 1", calls.getMe, calls.sendMessage)
	}
	if state := checkByName(t, result, chatbot.CheckBot); state != protocol.CheckStatePassed {
		t.Errorf("the %q check is %q, want passed", chatbot.CheckBot, state)
	}
	if state := checkByName(t, result, chatbot.CheckChat); state != protocol.CheckStatePassed {
		t.Errorf("the %q check is %q, want passed", chatbot.CheckChat, state)
	}
}

func TestTelegramConnectionTestFailsOnATokenTelegramRefuses(t *testing.T) {
	srv, _ := telegramServer(t, true)
	bot, err := chatbot.NewTelegram(chatbot.TelegramConfig{
		Token: "123:TESTTOKEN", ChatID: "555", BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("NewTelegram: %v", err)
	}
	result, err := bot.Test(context.Background())
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if result.OK {
		t.Fatal("the connection test passed a token Telegram refused")
	}
	if summary := checkByName(t, result, chatbot.CheckSummary); summary != protocol.CheckStateFailed {
		t.Errorf("the summary check is %q, want failed", summary)
	}
}

func TestTelegramNoticeReachesTheChat(t *testing.T) {
	srv, calls := telegramServer(t, false)
	bot, err := chatbot.NewTelegram(chatbot.TelegramConfig{
		Token: "123:TESTTOKEN", ChatID: "555", BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("NewTelegram: %v", err)
	}
	notice := chatbot.Notice{
		Title:   "Card 42 needs you",
		Body:    "The agent is waiting for approval.",
		URL:     "https://marshal.local/cards/42",
		Actions: []chatbot.Action{{Label: "Approve", Data: "approve:abc123"}},
	}
	if err := bot.Notify(context.Background(), notice); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if len(calls.sent) != 1 {
		t.Fatalf("the chat received %d messages, want 1", len(calls.sent))
	}
	// The whole request is read, because the button's own text comes before the message's in it.
	text := calls.raw[0]
	for _, want := range []string{notice.Title, notice.Body, notice.URL} {
		if !strings.Contains(text, want) {
			t.Errorf("the notice sent does not contain %q:\n%s", want, text)
		}
	}
	// The action is a button that carries its data back, not a line of text to type.
	if strings.Contains(text, "Approve (approve:abc123)") {
		t.Errorf("the action was listed as text beside its button:\n%s", text)
	}
	if !strings.Contains(calls.raw[0], `"callback_data":"approve:abc123"`) || !strings.Contains(calls.raw[0], `"text":"Approve"`) {
		t.Errorf("the notice has no button for the action:\n%s", calls.raw[0])
	}
}

func TestATelegramNoticeWithAnActionThatCannotBeAButtonListsItAsText(t *testing.T) {
	srv, calls := telegramServer(t, false)
	bot, err := chatbot.NewTelegram(chatbot.TelegramConfig{Token: "123:TESTTOKEN", ChatID: "555", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	long := "approval:" + strings.Repeat("x", 70)
	if err := bot.Notify(context.Background(), chatbot.Notice{Title: "Needs you", Actions: []chatbot.Action{{Label: "Approve", Data: long}}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(calls.raw[0], "Approve ("+long+")") || strings.Contains(calls.raw[0], "callback_data") {
		t.Errorf("an action too long for a button was not listed as text:\n%s", calls.raw[0])
	}
}

func TestATelegramButtonPressReachesTheHandlerAsTheTextItCarriesAndIsAnswered(t *testing.T) {
	srv, calls := telegramServer(t, false)
	bot, err := chatbot.NewTelegram(chatbot.TelegramConfig{Token: "123:TESTTOKEN", ChatID: "555", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan chatbot.Incoming, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The receive loop is not started (it would call Telegram); the update handler is what is under test.
	bot.HandleUpdateForTest(ctx, func(_ context.Context, in chatbot.Incoming) { got <- in },
		`{"update_id":1,"callback_query":{"id":"cb1","from":{"id":9,"is_bot":false,"first_name":"A"},"data":"approval:X:allow_once","message":{"message_id":3,"date":1700000000,"chat":{"id":555,"type":"private"}}}}`)
	var in chatbot.Incoming
	select {
	case in = <-got:
	case <-time.After(3 * time.Second):
		t.Fatal("the press never reached the handler")
	}
	if in.Text != "approval:X:allow_once" || in.ChatID != "555" {
		t.Fatalf("incoming = %+v", in)
	}
	if calls.answered != 1 {
		t.Fatalf("the press was answered %d times, want 1", calls.answered)
	}
}

func TestTelegramAcceptsOnlyTheChatItWasSetUpWith(t *testing.T) {
	number, _ := chatbot.NewTelegram(chatbot.TelegramConfig{Token: "1:T", ChatID: " 555 "})
	channel, _ := chatbot.NewTelegram(chatbot.TelegramConfig{Token: "1:T", ChatID: "@My_Channel"})
	none, _ := chatbot.NewTelegram(chatbot.TelegramConfig{Token: "1:T", ChatID: ""})
	cases := []struct {
		name string
		bot  chatbot.Bot
		in   chatbot.Incoming
		want bool
	}{
		{"its own chat by number", number, chatbot.Incoming{ChatID: "555"}, true},
		{"another chat", number, chatbot.Incoming{ChatID: "556"}, false},
		{"a channel by its name", channel, chatbot.Incoming{ChatID: "-100", ChatName: "@my_channel"}, true},
		{"another channel", channel, chatbot.Incoming{ChatID: "-101", ChatName: "@other"}, false},
		{"a chat with no name against a channel", channel, chatbot.Incoming{ChatID: "-100"}, false},
		{"anything when no chat was set", none, chatbot.Incoming{ChatID: ""}, false},
	}
	for _, tc := range cases {
		if got := tc.bot.Accepts(tc.in); got != tc.want {
			t.Errorf("%s: Accepts = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// checkByName finds one check's state, or fails when the test did not report it.
func checkByName(t *testing.T, result protocol.TestResult, name string) protocol.CheckState {
	t.Helper()
	for _, check := range result.Checks {
		if check.Name == name {
			return check.State
		}
	}
	t.Fatalf("the test reported no %q check: %+v", name, result.Checks)
	return ""
}

// telegramUpdates serves getUpdates with a fixed answer and status, on the path a bot's own token gives.
func telegramUpdates(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot123:TESTTOKEN/getUpdates" {
			http.Error(w, "no such method", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDetectingTheChatPicksTheNewestUpdateAndNamesIt(t *testing.T) {
	srv := telegramUpdates(t, http.StatusOK, `{"ok":true,"result":[
		{"update_id":10,"message":{"chat":{"id":111,"type":"private","first_name":"Old","last_name":"Chat"}}},
		{"update_id":12,"message":{"chat":{"id":-1002,"type":"supergroup","title":"Marshal alerts"}}},
		{"update_id":11,"callback_query":{"id":"x"}}]}`)
	got, err := chatbot.DetectTelegramChat(context.Background(), nil, srv.URL, "123:TESTTOKEN")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "-1002" || got.Name != "Marshal alerts" || got.Kind != "supergroup" {
		t.Fatalf("chat = %+v", got)
	}
}

func TestDetectingTheChatOfAPersonUsesTheirName(t *testing.T) {
	srv := telegramUpdates(t, http.StatusOK, `{"ok":true,"result":[{"update_id":1,"message":{"chat":{"id":42,"type":"private","first_name":"Ada","last_name":"Okafor"}}}]}`)
	got, err := chatbot.DetectTelegramChat(context.Background(), nil, srv.URL, "123:TESTTOKEN")
	if err != nil || got.ID != "42" || got.Name != "Ada Okafor" || got.Kind != "private" {
		t.Fatalf("chat = %+v, %v", got, err)
	}
}

func TestDetectingTheChatSaysWhyItFoundNone(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"nobody wrote yet", http.StatusOK, `{"ok":true,"result":[]}`, chatbot.ErrNoTelegramChat},
		{"a token Telegram refuses", http.StatusUnauthorized, `{"ok":false,"error_code":401,"description":"Unauthorized"}`, chatbot.ErrTelegramTokenRefused},
		{"a bot read elsewhere", http.StatusConflict, `{"ok":false,"error_code":409,"description":"Conflict"}`, chatbot.ErrTelegramBusy},
	}
	for _, tc := range cases {
		srv := telegramUpdates(t, tc.status, tc.body)
		if _, err := chatbot.DetectTelegramChat(context.Background(), nil, srv.URL, "123:TESTTOKEN"); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
	if _, err := chatbot.DetectTelegramChat(context.Background(), nil, "http://unused", "  "); !errors.Is(err, chatbot.ErrTelegramTokenRefused) {
		t.Errorf("an empty token: err = %v", err)
	}
}
