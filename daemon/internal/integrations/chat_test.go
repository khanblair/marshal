package integrations_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The two chat connections (B9.3, build-plan 9.5 and 9.6). Saving one writes the token to the
// keychain and the chat to the row; testing one asks the service over an in-process fake server, so
// no test ever reaches Telegram or Discord and no real bot token is ever used (hard rule 3).

// fakeTelegram answers the two Bot API calls the connection test makes.
func fakeTelegram(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"username":"marshal_bot"}}`))
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			_, _ = io.ReadAll(r.Body)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":1,"type":"private"},"text":"ok"}}`))
		default:
			http.Error(w, "no such method", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// fakeDiscord answers the two REST calls the connection test makes, reached through a transport that
// rewrites the real discord.com address to this in-process server.
func fakeDiscord(t *testing.T) *http.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/users/@me"):
			_, _ = w.Write([]byte(`{"id":"1","username":"marshal_bot"}`))
		case strings.Contains(r.URL.Path, "/channels/") && strings.HasSuffix(r.URL.Path, "/messages"):
			_, _ = io.ReadAll(r.Body)
			_, _ = w.Write([]byte(`{"id":"2","channel_id":"555","content":"ok"}`))
		default:
			http.Error(w, "no such endpoint", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse the fake server's address: %v", err)
	}
	return &http.Client{Transport: &rewriteTo{target: target, inner: http.DefaultTransport}}
}

// rewriteTo sends every request to a fixed address, so a library whose endpoints are constants can
// still be driven from an in-process server.
type rewriteTo struct {
	target *url.URL
	inner  http.RoundTripper
}

func (t *rewriteTo) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = t.target.Scheme
	clone.URL.Host = t.target.Host
	clone.Host = t.target.Host
	return t.inner.RoundTrip(clone)
}

func TestSavingATelegramConnectionStoresTheTokenAndTheChat(t *testing.T) {
	f := newFixture(t)
	if err := f.svc.SaveTelegram(context.Background(), protocol.SaveTelegramRequest{
		Token: "123:TESTTOKEN", ChatID: "555",
	}); err != nil {
		t.Fatalf("SaveTelegram: %v", err)
	}
	config, ref, found := f.row(integrations.TelegramID)
	if !found {
		t.Fatal("no Telegram row was written")
	}
	if ref != integrations.TelegramID {
		t.Errorf("the keychain reference is %q, want %q", ref, integrations.TelegramID)
	}
	if strings.Contains(config, "TESTTOKEN") {
		t.Error("the token was written to the row instead of the keychain")
	}
	raw, err := f.keys.Get(integrations.TelegramID)
	if err != nil {
		t.Fatalf("the token is not in the keychain: %v", err)
	}
	if !strings.Contains(raw, "TESTTOKEN") {
		t.Error("the keychain entry does not hold the token")
	}
}

func TestATelegramConnectionWithNoTokenOrChatIsRefused(t *testing.T) {
	f := newFixture(t)
	for _, req := range []protocol.SaveTelegramRequest{
		{Token: "", ChatID: "555"},
		{Token: "123:TESTTOKEN", ChatID: ""},
	} {
		if err := f.svc.SaveTelegram(context.Background(), req); err == nil {
			t.Errorf("SaveTelegram(%+v) was accepted, want a refusal", req)
		}
	}
	if _, _, found := f.row(integrations.TelegramID); found {
		t.Error("a refused save wrote a row")
	}
}

func TestTelegramConnectionTestPassesAgainstAFakeServer(t *testing.T) {
	srv := fakeTelegram(t)
	f := newFixture(t, func(o *integrations.Options) { o.TelegramBaseURL = srv.URL })
	if err := f.svc.SaveTelegram(context.Background(), protocol.SaveTelegramRequest{
		Token: "123:TESTTOKEN", ChatID: "555",
	}); err != nil {
		t.Fatalf("SaveTelegram: %v", err)
	}
	result, err := f.svc.Test(context.Background(), integrations.TelegramID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !result.OK {
		t.Fatalf("the Telegram test did not pass: %+v", result.Checks)
	}
}

func TestDiscordConnectionTestPassesAgainstAFakeServer(t *testing.T) {
	client := fakeDiscord(t)
	f := newFixture(t, func(o *integrations.Options) { o.DiscordHTTPClient = client })
	if err := f.svc.SaveDiscord(context.Background(), protocol.SaveDiscordRequest{
		Token: "TESTTOKEN", ChannelID: "555",
	}); err != nil {
		t.Fatalf("SaveDiscord: %v", err)
	}
	result, err := f.svc.Test(context.Background(), integrations.DiscordID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !result.OK {
		t.Fatalf("the Discord test did not pass: %+v", result.Checks)
	}
}

func TestAChatConnectionSavedWithNothingShowsTheRightKind(t *testing.T) {
	f := newFixture(t)
	list, err := f.svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := map[string]string{
		integrations.TelegramID: integrations.KindTelegram,
		integrations.DiscordID:  integrations.KindDiscord,
	}
	for _, row := range list {
		kind, ok := want[row.ID]
		if !ok {
			continue
		}
		if row.Kind != kind {
			t.Errorf("%s's kind is %q, want %q", row.ID, row.Kind, kind)
		}
		if row.Status != protocol.IntegrationStatusNone {
			t.Errorf("%s reads %q with nothing stored, want none", row.ID, row.Status)
		}
	}
}

func TestNtfySavesWithATopicAloneAndTestsAgainstAFakeServer(t *testing.T) {
	var published []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		published = append(published, string(body))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	f := newFixture(t, func(o *integrations.Options) { o.NtfyHTTPClient = srv.Client() })
	ctx := context.Background()
	if err := f.svc.SaveNtfy(ctx, protocol.SaveNtfyRequest{Server: srv.URL, Topic: "  "}); err == nil {
		t.Fatal("an ntfy connection with no topic was saved")
	}
	if err := f.svc.SaveNtfy(ctx, protocol.SaveNtfyRequest{Server: srv.URL, Topic: "marshal-x"}); err != nil {
		t.Fatalf("SaveNtfy: %v", err)
	}
	result, err := f.svc.Test(ctx, integrations.NtfyID)
	if err != nil || !result.OK {
		t.Fatalf("the ntfy test answered %+v (%v), want a pass", result, err)
	}
	if len(published) != 1 || !strings.Contains(published[0], `"topic":"marshal-x"`) {
		t.Errorf("the fake server saw %v, want one publish to the topic", published)
	}
	list, err := f.svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, row := range list {
		if row.ID == integrations.NtfyID && row.Status != protocol.IntegrationStatusConnected {
			t.Errorf("the row reads %q after saving, want connected with a topic alone", row.Status)
		}
	}
}

func TestDetectingTheTelegramChatAnswersItsIdOrHowToGetOne(t *testing.T) {
	answer := `{"ok":true,"result":[]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(answer))
	}))
	t.Cleanup(srv.Close)
	f := newFixture(t, func(o *integrations.Options) { o.TelegramBaseURL = srv.URL })
	ctx := context.Background()

	none, err := f.svc.DetectTelegramChat(ctx, protocol.DetectTelegramChatRequest{Token: "123:TESTTOKEN"})
	if err != nil || none.Found || none.Message == "" {
		t.Fatalf("with nobody written: %+v, %v", none, err)
	}
	answer = `{"ok":true,"result":[{"update_id":5,"message":{"chat":{"id":777,"type":"private","first_name":"Ada"}}}]}`
	found, err := f.svc.DetectTelegramChat(ctx, protocol.DetectTelegramChatRequest{Token: "123:TESTTOKEN"})
	if err != nil || !found.Found || found.ChatID != "777" || found.Name != "Ada" {
		t.Fatalf("with a message: %+v, %v", found, err)
	}
	// Nothing was saved by looking.
	if _, _, saved := f.row(integrations.TelegramID); saved {
		t.Error("detecting a chat wrote a connection")
	}
}
