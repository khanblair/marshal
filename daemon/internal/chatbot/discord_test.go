package chatbot_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/chatbot"
	"github.com/khanblair/marshal/daemon/internal/protocol"
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
	// flags is the application's flags the fake answers with. Message Content Intent is on by default.
	flags int
	// noChannel makes the channel answer as Discord does for an id it has no channel for, and
	// asGuild makes that same number a server the bot is in.
	noChannel, asGuild bool
	// channelCode is the error code the channel answers with when the bot may not read it.
	channelCode int
}

// messageContentFlag is the application flag Discord sets when Message Content Intent is on.
const messageContentFlag = 1 << 19

// discordServer answers the two REST calls the bot makes. A refuse flag makes it answer the way
// Discord answers a token it does not like.
func discordServer(t *testing.T, refuse bool) (*httptest.Server, *discordCalls) {
	t.Helper()
	calls := &discordCalls{flags: messageContentFlag}
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
		case strings.HasSuffix(path, "/applications/@me"):
			discordOK(w, fmt.Sprintf(`{"id":"1","flags":%d}`, calls.flags))
		case strings.Contains(path, "/guilds/"):
			if !calls.asGuild {
				discordErrorCode(w, http.StatusNotFound, 10004)
				return
			}
			discordOK(w, `{"id":"555","name":"My server"}`)
		case strings.Contains(path, "/channels/"):
			if calls.noChannel {
				discordErrorCode(w, http.StatusNotFound, 10003)
				return
			}
			if calls.channelCode != 0 {
				discordErrorCode(w, http.StatusForbidden, calls.channelCode)
				return
			}
			discordOK(w, `{"id":"555","name":"general","type":0}`)
		default:
			http.Error(w, "no such endpoint", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, calls
}

func discordErrorCode(w http.ResponseWriter, status, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"message":"refused","code":%d}`, code)
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

// discordResult runs a connection test against the fake with one thing changed.
func discordResult(t *testing.T, change func(*discordCalls)) protocol.TestResult {
	t.Helper()
	srv, calls := discordServer(t, false)
	change(calls)
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
	return result
}

func discordCheck(t *testing.T, result protocol.TestResult, name string) protocol.TestCheck {
	t.Helper()
	for _, check := range result.Checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("the test reported no %q check: %+v", name, result.Checks)
	return protocol.TestCheck{}
}

func TestDiscordNamesTheChannelItSentTo(t *testing.T) {
	result := discordResult(t, func(*discordCalls) {})
	if got := discordCheck(t, result, chatbot.CheckChat).Message; !strings.Contains(got, "#general") {
		t.Errorf("the channel check says %q, want the channel's name", got)
	}
	if state := checkByName(t, result, chatbot.CheckTyping); state != "passed" {
		t.Errorf("the %q check is %q, want passed", chatbot.CheckTyping, state)
	}
}

func TestDiscordWarnsWhenTypedRepliesAreOffButStillWorks(t *testing.T) {
	result := discordResult(t, func(c *discordCalls) { c.flags = 0 })
	typing := discordCheck(t, result, chatbot.CheckTyping)
	if typing.State != protocol.CheckStateWarning || !strings.Contains(typing.Fix, "Message Content Intent") {
		t.Errorf("the typing check = %+v, want a warning that names Message Content Intent", typing)
	}
	if !result.OK {
		t.Error("a bot that cannot read typed replies still sends, so the result should be OK")
	}
	if state := checkByName(t, result, chatbot.CheckSummary); state != "warning" {
		t.Errorf("the summary is %q, want warning", state)
	}
}

func TestDiscordTellsAServerIDFromAChannelID(t *testing.T) {
	result := discordResult(t, func(c *discordCalls) { c.noChannel, c.asGuild = true, true })
	chat := discordCheck(t, result, chatbot.CheckChat)
	if chat.State != protocol.CheckStateFailed || !strings.Contains(chat.Message, "server's id") {
		t.Errorf("the channel check = %+v, want a failure that says it is a server's id", chat)
	}
	if !strings.Contains(chat.Fix, "Copy Channel ID") {
		t.Errorf("the fix = %q, want it to say how to copy a channel's id", chat.Fix)
	}
}

func TestDiscordSaysWhenThereIsNoSuchChannel(t *testing.T) {
	result := discordResult(t, func(c *discordCalls) { c.noChannel = true })
	chat := discordCheck(t, result, chatbot.CheckChat)
	if chat.State != protocol.CheckStateFailed || !strings.Contains(chat.Message, "no channel with that id") {
		t.Errorf("the channel check = %+v, want no channel with that id", chat)
	}
}

func TestDiscordSaysWhatToDoWhenTheBotCannotSeeOrSendToTheChannel(t *testing.T) {
	cases := map[int]string{50001: "cannot see that channel", 50013: "may not send messages"}
	for code, want := range cases {
		result := discordResult(t, func(c *discordCalls) { c.channelCode = code })
		if got := discordCheck(t, result, chatbot.CheckChat).Message; !strings.Contains(got, want) {
			t.Errorf("code %d: the channel check says %q, want %q", code, got, want)
		}
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

func TestDiscordAcceptsOnlyItsChannelAndNtfyAcceptsNothing(t *testing.T) {
	d, err := chatbot.NewDiscord(chatbot.DiscordConfig{Token: "TESTTOKEN", ChannelID: "555"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Accepts(chatbot.Incoming{ChatID: "555"}) || d.Accepts(chatbot.Incoming{ChatID: "556"}) {
		t.Error("Discord accepted the wrong channel")
	}
	empty, _ := chatbot.NewDiscord(chatbot.DiscordConfig{Token: "TESTTOKEN"})
	if empty.Accepts(chatbot.Incoming{}) {
		t.Error("Discord accepted a message when no channel was set")
	}
	n, err := chatbot.NewNtfy(chatbot.NtfyConfig{Topic: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if n.Accepts(chatbot.Incoming{ChatID: "t"}) {
		t.Error("ntfy accepted a message, but it only sends")
	}
}

func TestADiscordNoticeWithActionsCarriesButtons(t *testing.T) {
	srv, calls := discordServer(t, false)
	bot, err := chatbot.NewDiscord(chatbot.DiscordConfig{Token: "TESTTOKEN", ChannelID: "555", HTTPClient: discordClient(t, srv)})
	if err != nil {
		t.Fatal(err)
	}
	err = bot.Notify(context.Background(), chatbot.Notice{Title: "Needs you", Actions: []chatbot.Action{
		{Label: "Approve", Data: "approval:X:allow_once"}, {Label: "Reject", Data: "approval:X:reject_once"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	body := calls.sent[0]
	for _, want := range []string{`"custom_id":"approval:X:allow_once"`, `"label":"Reject"`, `"type":1`} {
		if !strings.Contains(body, want) {
			t.Errorf("the message has no %s:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Approve (approval") {
		t.Errorf("the actions were listed as text beside their buttons:\n%s", body)
	}
}

func TestADiscordButtonPressReachesTheHandlerAsTheTextItCarries(t *testing.T) {
	srv, _ := discordServer(t, false)
	bot, err := chatbot.NewDiscord(chatbot.DiscordConfig{Token: "TESTTOKEN", ChannelID: "555", HTTPClient: discordClient(t, srv)})
	if err != nil {
		t.Fatal(err)
	}
	var got chatbot.Incoming
	bot.InteractionForTest(context.Background(), "555", "approval:X:allow_once", func(_ context.Context, in chatbot.Incoming) { got = in })
	if got.Text != "approval:X:allow_once" || got.ChatID != "555" {
		t.Fatalf("incoming = %+v", got)
	}
}
