package chatbot_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/chatbot"
)

// Publishing to ntfy (B9.3, docs/mobile.md section 4). Every test points the publisher at an
// in-process fake server, so no test reaches ntfy and no real topic or token is ever used.

type ntfyPublish struct {
	Topic, Title, Message, Click string
	auth                         string
}

func ntfyServer(t *testing.T, status int) (*httptest.Server, *[]ntfyPublish) {
	t.Helper()
	var got []ntfyPublish
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var one ntfyPublish
		_ = json.Unmarshal(body, &one)
		one.auth = r.Header.Get("Authorization")
		got = append(got, one)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestANoticeIsPublishedWithItsLinkAsWhatTappingOpens(t *testing.T) {
	srv, got := ntfyServer(t, http.StatusOK)
	bot, err := chatbot.NewNtfy(chatbot.NtfyConfig{Server: srv.URL + "/", Topic: "marshal-x", Token: "tk_test"})
	if err != nil {
		t.Fatalf("NewNtfy: %v", err)
	}
	if err := bot.Notify(context.Background(), chatbot.Notice{Title: "Fix login needs you", Body: "It is stuck.", URL: "marshal://card/c1"}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	want := ntfyPublish{Topic: "marshal-x", Title: "Fix login needs you", Message: "It is stuck.", Click: "marshal://card/c1", auth: "Bearer tk_test"}
	if len(*got) != 1 || (*got)[0] != want {
		t.Errorf("published %+v, want %+v", *got, want)
	}
}

func TestATitleOnlyNoticeIsTheMessageAndAnOpenTopicSendsNoToken(t *testing.T) {
	srv, got := ntfyServer(t, http.StatusOK)
	bot, _ := chatbot.NewNtfy(chatbot.NtfyConfig{Server: srv.URL, Topic: "open"})
	if err := bot.Notify(context.Background(), chatbot.Notice{Title: "Finished: x"}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if (*got)[0].Message != "Finished: x" || (*got)[0].auth != "" {
		t.Errorf("published %+v, want the title as the message and no Authorization", (*got)[0])
	}
}

func TestATopicIsRequired(t *testing.T) {
	if _, err := chatbot.NewNtfy(chatbot.NtfyConfig{Topic: "  "}); err == nil {
		t.Error("a publisher with no topic was built")
	}
}

func TestTheConnectionTestPassesAndSaysWhatToFixWhenRefused(t *testing.T) {
	ok, _ := ntfyServer(t, http.StatusOK)
	pass, _ := chatbot.NewNtfy(chatbot.NtfyConfig{Server: ok.URL, Topic: "t"})
	result, err := pass.Test(context.Background())
	if err != nil || !result.OK || result.ConnectionID != "ntfy" {
		t.Fatalf("the test answered %+v (%v), want a passing ntfy result", result, err)
	}
	for status, want := range map[int]string{
		http.StatusUnauthorized:    "ntfy refused the access token.",
		http.StatusTooManyRequests: "ntfy is asking Marshal to slow down.",
		http.StatusBadRequest:      "ntfy did not accept the message.",
	} {
		srv, _ := ntfyServer(t, status)
		bot, _ := chatbot.NewNtfy(chatbot.NtfyConfig{Server: srv.URL, Topic: "t"})
		result, _ := bot.Test(context.Background())
		summary := ""
		for _, check := range result.Checks {
			if check.Name == chatbot.CheckSummary {
				summary = check.Message
			}
		}
		if result.OK || !strings.Contains(summary, want) {
			t.Errorf("status %d: OK=%v summary %q, want a failure saying %q", status, result.OK, summary, want)
		}
	}
}

func TestAServerThatCannotBeReachedIsTheNetworksFaultNotTheTopics(t *testing.T) {
	srv, _ := ntfyServer(t, http.StatusOK)
	url := srv.URL
	srv.Close()
	bot, _ := chatbot.NewNtfy(chatbot.NtfyConfig{Server: url, Topic: "t"})
	result, _ := bot.Test(context.Background())
	if result.OK {
		t.Fatal("a server that is gone passed")
	}
	if !strings.Contains(result.Checks[0].Message, "could not reach the ntfy server") {
		t.Errorf("the summary is %q", result.Checks[0].Message)
	}
}
