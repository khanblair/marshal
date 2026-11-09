package chatbot

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

type conversationTransport struct {
	target   *url.URL
	deadline chan time.Duration
}

func (transport conversationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if transport.deadline != nil {
		deadline, ok := request.Context().Deadline()
		if !ok {
			return nil, errors.New("thread lookup request has no deadline")
		}
		transport.deadline <- time.Until(deadline)
	}
	copy := request.Clone(request.Context())
	copy.URL.Scheme, copy.URL.Host, copy.Host = transport.target.Scheme, transport.target.Host, transport.target.Host
	return http.DefaultTransport.RoundTrip(copy)
}

func TestDiscordIncomingReadsReplyAndValidatedThreadMetadata(t *testing.T) {
	deadlines := make(chan time.Duration, 4)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"thread-1","type":11,"parent_id":"parent-1"}`))
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	bot, err := NewDiscord(DiscordConfig{
		Token: "fake-token", ChannelID: "parent-1",
		HTTPClient: &http.Client{Transport: conversationTransport{target: target, deadline: deadlines}},
	})
	if err != nil {
		t.Fatal(err)
	}
	in := bot.incoming(&discordgo.Message{
		ID: "message-3", ChannelID: "thread-1",
		MessageReference: &discordgo.MessageReference{MessageID: "message-2"},
	}, "hello")
	if in.ChatID != "thread-1" || in.MessageID != "message-3" || in.ReplyToID != "message-2" || in.ThreadID != "thread-1" {
		t.Fatalf("incoming metadata = %+v", in)
	}
	if !bot.Accepts(in) {
		t.Fatal("rejected a thread whose API metadata names the configured parent")
	}
	if bot.Accepts(Incoming{ChatID: "unrelated-thread"}) {
		t.Fatal("accepted an unrelated thread")
	}
	select {
	case remaining := <-deadlines:
		if remaining <= 0 || remaining > discordThreadLookupTimeout || remaining < discordThreadLookupTimeout-time.Second {
			t.Fatalf("thread lookup deadline has %s remaining, want at most %s", remaining, discordThreadLookupTimeout)
		}
	default:
		t.Fatal("thread lookup did not use the bounded HTTP request")
	}
}
