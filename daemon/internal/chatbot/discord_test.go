package chatbot_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/chatbot"
)

// Talking to Discord (B9.3, build-plan 9.6). Discord's REST endpoints are the real discord.com
// addresses in the library, so the fake server is reached through a transport that rewrites every
// request to the in-process server. Nothing leaves this machine, and the token is made up, so no
// test ever reaches Discord and no real bot token is ever used (hard rule 3).

// rewriteTransport sends every request to target instead of the host in the address. It is how a
// test points a library whose endpoints are constants at an in-process fake server.
type rewriteTransport struct {
	target *url.URL
	inner  http.RoundTripper
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = t.target.Scheme
	clone.URL.Host = t.target.Host
	clone.Host = t.target.Host
	return t.inner.RoundTrip(clone)
}

// discordCalls counts what the bot asked the fake server for.
type discordCalls struct {
	users    int
	messages int
	sent     []string
	token    string
}

// discordServer answers the two REST calls the bot makes. A refuse flag makes it answer the way
// Discord answers a token it does not like.
func discordServer(t *testing.T, refuse bool) (*httptest.Server, *discordCalls) {
	t.Helper()
	calls := &discordCalls{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.token = r.Header.Get("Authorization")
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/users/@me"):
			calls.users++
			if refuse {
				discordError(w, http.StatusUnauthorized)
				return
			}
			discordOK(w, `{"id":"1","username":"marshal_bot"}`)
		case strings.Contains(path, "/channels/") && strings.HasSuffix(path, "/messages"):
			calls.messages++
			body, _ := io.ReadAll(r.Body)
			calls.sent = append(calls.sent, string(body))
			if refuse {
				discordError(w, http.StatusForbidden)
				return
			}
			discordOK(w, `{"id":"2","channel_id":"555","content":"ok"}`)
		default:
			http.Error(w, "no such endpoint", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, calls
}

func discordOK(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

func discordError(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"message":"refused","code":0}`))
}

// discordClient builds the HTTP client that sends a bot's requests to the fake server.
func discordClient(t *testing.T, srv *httptest.Server) *http.Client {
	t.Helper()
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse the fake server's address: %v", err)
	}
	return &http.Client{Transport: &rewriteTransport{target: target, inner: http.DefaultTransport}}
}

func TestDiscordConnectionTestPassesWhenTheBotAndChannelAnswer(t *testing.T) {
	srv, calls := discordServer(t, false)
	bot, err := chatbot.NewDiscord(chatbot.DiscordConfig{
		Token: "TESTTOKEN", ChannelID: "555", HTTPClient: discordClient(t, srv),
	})
	if err != nil {
		t.Fatalf("NewDiscord: %v", err)
	}
	result, err := bot.Test(context.Background())
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !result.OK {
		t.Fatalf("the connection test did not pass: %+v", result.Checks)
	}
	if calls.users != 1 || calls.messages != 1 {
		t.Errorf("the fake server saw users %d and messages %d, want 1 and 1", calls.users, calls.messages)
	}
	if !strings.HasPrefix(calls.token, "Bot ") {
		t.Errorf("the bot sent %q as its Authorization header, want a Bot token", calls.token)
	}
	if state := checkByName(t, result, chatbot.CheckBot); state != "passed" {
		t.Errorf("the %q check is %q, want passed", chatbot.CheckBot, state)
	}
}

func TestDiscordConnectionTestFailsOnATokenDiscordRefuses(t *testing.T) {
	srv, _ := discordServer(t, true)
	bot, err := chatbot.NewDiscord(chatbot.DiscordConfig{
		Token: "TESTTOKEN", ChannelID: "555", HTTPClient: discordClient(t, srv),
	})
	if err != nil {
		t.Fatalf("NewDiscord: %v", err)
	}
	result, err := bot.Test(context.Background())
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if result.OK {
		t.Fatal("the connection test passed a token Discord refused")
	}
}

func TestDiscordNoticeReachesTheChannel(t *testing.T) {
	srv, calls := discordServer(t, false)
	bot, err := chatbot.NewDiscord(chatbot.DiscordConfig{
		Token: "TESTTOKEN", ChannelID: "555", HTTPClient: discordClient(t, srv),
	})
	if err != nil {
		t.Fatalf("NewDiscord: %v", err)
	}
	notice := chatbot.Notice{
		Title:   "Card 42 needs you",
		Body:    "The agent is waiting for approval.",
		Actions: []chatbot.Action{{Label: "Approve", Data: "approve:abc123"}},
	}
	if err := bot.Notify(context.Background(), notice); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if len(calls.sent) != 1 {
		t.Fatalf("the channel received %d messages, want 1", len(calls.sent))
	}
	for _, want := range []string{notice.Title, notice.Body, "approve:abc123"} {
		if !strings.Contains(calls.sent[0], want) {
			t.Errorf("the notice sent does not contain %q:\n%s", want, calls.sent[0])
		}
	}
}
