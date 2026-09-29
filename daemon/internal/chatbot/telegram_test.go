package chatbot_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

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
	sent        []string
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
			if refuse {
				telegramError(w, http.StatusBadRequest, 400, "Bad Request: chat not found")
				return
			}
			telegramOK(w, `{"message_id":1,"date":0,"chat":{"id":1,"type":"private"},"text":"ok"}`)
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
	text := calls.sent[0]
	for _, want := range []string{notice.Title, notice.Body, notice.URL, "Approve (approve:abc123)"} {
		if !strings.Contains(text, want) {
			t.Errorf("the notice sent does not contain %q:\n%s", want, text)
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
