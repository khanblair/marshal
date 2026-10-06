package integrations_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// fakeGoogle is a Google that issues tokens and answers Calendar reads.
type fakeGoogle struct {
	server *httptest.Server

	// invalidGrant makes the token endpoint refuse every refresh.
	invalidGrant atomic.Bool
	// down makes the Calendar API fail with a 500.
	down atomic.Bool
	// tokenCalls and eventCalls count what Marshal asked for.
	tokenCalls, eventCalls atomic.Int32
	// lastVerifier is the PKCE verifier the last code exchange carried.
	lastVerifier atomic.Value
	// revokeCalls counts how many times Marshal asked to end an access, lastRevoked is the token it
	// sent, and revokeFails makes the endpoint answer with a 500.
	revokeCalls atomic.Int32
	lastRevoked atomic.Value
	revokeFails atomic.Bool
	// workBody, when set, is what the Work calendar answers with instead of its usual one event.
	workBody atomic.Value
	// scope, when set, is the scope list the token endpoint reports as granted. Google leaves it out
	// when nothing was unticked, and so does the fake until a test sets it.
	scope atomic.Value
}

func newFakeGoogle(t *testing.T) *fakeGoogle {
	t.Helper()
	g := &fakeGoogle{}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		g.tokenCalls.Add(1)
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		if g.invalidGrant.Load() && r.Form.Get("grant_type") == "refresh_token" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`))
			return
		}
		if r.Form.Get("grant_type") == "authorization_code" {
			g.lastVerifier.Store(r.Form.Get("code_verifier"))
		}
		granted := ""
		if scope, _ := g.scope.Load().(string); scope != "" {
			granted = `,"scope":"` + scope + `"`
		}
		_, _ = w.Write([]byte(`{"access_token":"access-` + r.Form.Get("grant_type") + `","refresh_token":"refresh-1","token_type":"Bearer","expires_in":1` + granted + `}`))
	})
	mux.HandleFunc("/revoke", func(w http.ResponseWriter, r *http.Request) {
		g.revokeCalls.Add(1)
		_ = r.ParseForm()
		g.lastRevoked.Store(r.Form.Get("token"))
		if g.revokeFails.Load() {
			http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/calendar/v3/users/me/calendarList", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[
			{"id":"me@x.com","summary":"me@x.com","primary":true,"accessRole":"owner","selected":true},
			{"id":"work","summary":"Work","accessRole":"owner","selected":true},
			{"id":"holidays","summary":"Holidays","accessRole":"reader","selected":false}]}`))
	})
	mux.HandleFunc("/calendar/v3/calendars/", func(w http.ResponseWriter, r *http.Request) {
		g.eventCalls.Add(1)
		if g.down.Load() {
			http.Error(w, `{"error":{"code":500}}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/work/") && g.workBody.Load() != nil:
			_, _ = w.Write([]byte(g.workBody.Load().(string)))
		case strings.Contains(r.URL.Path, "/work/"):
			_, _ = w.Write([]byte(`{"items":[{"id":"w1","summary":"Design review","start":{"dateTime":"2026-10-02T14:00:00Z"},"end":{"dateTime":"2026-10-02T15:00:00Z"}}]}`))
		case strings.Contains(r.URL.Path, "/holidays/"):
			_, _ = w.Write([]byte(`{"items":[{"id":"h1","summary":"Public holiday","start":{"date":"2026-10-09"},"end":{"date":"2026-10-10"}}]}`))
		default:
			_, _ = w.Write([]byte(`{"items":[{"id":"m1","summary":"Standup","start":{"dateTime":"2026-10-02T09:00:00Z"},"end":{"dateTime":"2026-10-02T09:15:00Z"}}]}`))
		}
	})
	g.server = httptest.NewServer(mux)
	t.Cleanup(g.server.Close)
	return g
}

// google builds a fixture whose Google is the fake, with the OAuth client already saved.
func newGoogleFixture(t *testing.T, mutate ...func(*integrations.Options)) (*fixture, *fakeGoogle) {
	t.Helper()
	g := newFakeGoogle(t)
	f := newFixture(t, append([]func(*integrations.Options){func(o *integrations.Options) {
		o.GCalRedirectURL = "http://127.0.0.1:47801/v1/integrations/gcal/callback"
		o.GoogleAuthURL = g.server.URL + "/auth"
		o.GoogleTokenURL = g.server.URL + "/token"
		o.GCalBaseURL = g.server.URL + "/calendar/v3/"
		o.GoogleRevokeURL = g.server.URL + "/revoke"
	}}, mutate...)...)
	if err := f.svc.SaveGoogleCalendar(context.Background(), protocol.SaveGoogleCalendarRequest{
		ClientID: "client-id", ClientSecret: "client-secret",
	}); err != nil {
		t.Fatal(err)
	}
	return f, g
}

// grant runs a whole consent flow the way a browser would: open the URL, then hand the callback the
// code and the state from it.
func (f *fixture) grant(authorize func(context.Context) (string, error)) string {
	f.t.Helper()
	raw, err := authorize(context.Background())
	if err != nil {
		f.t.Fatalf("authorize: %v", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		f.t.Fatal(err)
	}
	id, err := f.svc.FinishGoogle(context.Background(), "the-code", u.Query().Get("state"))
	if err != nil {
		f.t.Fatalf("FinishGoogle: %v", err)
	}
	return id
}

func TestTheConsentUrlAsksOnlyForCalendarAndCarriesAPKCEChallenge(t *testing.T) {
	f, _ := newGoogleFixture(t)
	raw, err := f.svc.AuthorizeGoogleCalendar(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	q := mustQuery(t, raw)
	if q.Get("scope") != "https://www.googleapis.com/auth/calendar.readonly" {
		t.Errorf("scope = %q, want the calendar scope alone", q.Get("scope"))
	}
	if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
		t.Errorf("challenge = %q (%s), want PKCE S256", q.Get("code_challenge"), q.Get("code_challenge_method"))
	}
	if q.Get("access_type") != "offline" || q.Get("state") == "" || q.Get("client_id") != "client-id" {
		t.Errorf("query = %v", q)
	}
	gmail, err := f.svc.AuthorizeGmail(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if scope := mustQuery(t, gmail).Get("scope"); scope != "https://www.googleapis.com/auth/gmail.readonly" {
		t.Errorf("gmail scope = %q, want Gmail's own alone", scope)
	}
}

func mustQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}

func TestFinishingStoresTheTokenAndSendsThePKCEVerifier(t *testing.T) {
	f, g := newGoogleFixture(t)
	if id := f.grant(f.svc.AuthorizeGoogleCalendar); id != integrations.GCalID {
		t.Errorf("granted %q, want gcal", id)
	}
	if v, _ := g.lastVerifier.Load().(string); len(v) < 43 {
		t.Errorf("verifier sent to Google = %q, want a PKCE verifier", v)
	}
	if _, err := f.svc.GoogleCalendarClient(context.Background()); err != nil {
		t.Errorf("a granted connection must give a client: %v", err)
	}
}

func TestAConsentLinkWorksOnce(t *testing.T) {
	f, _ := newGoogleFixture(t)
	raw, _ := f.svc.AuthorizeGoogleCalendar(context.Background())
	state := mustQuery(t, raw).Get("state")
	if _, err := f.svc.FinishGoogle(context.Background(), "code", state); err != nil {
		t.Fatal(err)
	}
	_, err := f.svc.FinishGoogle(context.Background(), "code", state)
	var refusal *protocol.Error
	if !errors.As(err, &refusal) {
		t.Fatalf("a used link gave %v, want a refusal", err)
	}
	if _, err := f.svc.FinishGoogle(context.Background(), "code", "made-up"); err == nil {
		t.Error("an unknown state must be refused")
	}
}

func TestGmailKeepsItsOwnTokenAndCalendarNeverNeedsIt(t *testing.T) {
	f, _ := newGoogleFixture(t)
	if _, err := f.svc.GmailClient(context.Background()); !errors.Is(err, integrations.ErrNotConnected) {
		t.Fatalf("Gmail before its own consent gave %v, want ErrNotConnected", err)
	}
	f.grant(f.svc.AuthorizeGoogleCalendar)
	if _, err := f.svc.GmailClient(context.Background()); !errors.Is(err, integrations.ErrNotConnected) {
		t.Fatalf("Calendar's grant must not cover Gmail, got %v", err)
	}
	if id := f.grant(f.svc.AuthorizeGmail); id != integrations.GmailID {
		t.Errorf("granted %q, want gmail", id)
	}
	if _, err := f.svc.GmailClient(context.Background()); err != nil {
		t.Errorf("Gmail after its own consent: %v", err)
	}
	if _, err := f.svc.GoogleCalendarClient(context.Background()); err != nil {
		t.Errorf("Calendar must still work: %v", err)
	}
}

func TestChangingTheClientDropsGmailsToken(t *testing.T) {
	f, _ := newGoogleFixture(t)
	f.grant(f.svc.AuthorizeGmail)
	if err := f.svc.SaveGoogleCalendar(context.Background(), protocol.SaveGoogleCalendarRequest{
		ClientID: "another", ClientSecret: "secret",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.GmailClient(context.Background()); !errors.Is(err, integrations.ErrNotConnected) {
		t.Errorf("a token from the old client must be gone, got %v", err)
	}
}

var (
	from = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	to   = time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
)

func titles(events []string) string { return strings.Join(events, ",") }

func eventTitles(t *testing.T, f *fixture) ([]string, protocol.GoogleReading) {
	t.Helper()
	events, reading := f.svc.GoogleEvents(context.Background(), from, to)
	var out []string
	for _, e := range events {
		out = append(out, e.Title)
	}
	return out, reading
}

func TestEventsFollowWhatIsTickedInGoogleUntilThePersonChooses(t *testing.T) {
	f, _ := newGoogleFixture(t)
	if got, reading := eventTitles(t, f); reading.Connected || len(got) != 0 {
		t.Fatalf("before consent: %v %+v, want nothing and not connected", got, reading)
	}
	f.grant(f.svc.AuthorizeGoogleCalendar)
	got, reading := eventTitles(t, f)
	if !reading.Connected || reading.Error != "" || titles(got) != "Standup,Design review" {
		t.Fatalf("events = %v %+v, want the two ticked calendars", got, reading)
	}
	choices, err := f.svc.GoogleCalendars(context.Background())
	if err != nil || choices.Chosen || len(choices.Calendars) != 3 {
		t.Fatalf("choices = %+v, %v", choices, err)
	}
	if err := f.svc.SetGoogleCalendars(context.Background(), []string{"holidays"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := eventTitles(t, f); titles(got) != "Public holiday" {
		t.Errorf("after choosing: %v, want only the holiday calendar", got)
	}
	if err := f.svc.SetGoogleCalendars(context.Background(), []string{"nope"}); err == nil {
		t.Error("a calendar Google does not list must be refused")
	}
	if err := f.svc.SetGoogleCalendars(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if got, reading := eventTitles(t, f); len(got) != 0 || !reading.Connected {
		t.Errorf("none chosen: %v %+v, want no events and still connected", got, reading)
	}
}

func TestEventsAreReusedBrieflyAndShownStaleWhenGoogleIsDown(t *testing.T) {
	f, g := newGoogleFixture(t)
	f.grant(f.svc.AuthorizeGoogleCalendar)
	eventTitles(t, f)
	calls := g.eventCalls.Load()
	eventTitles(t, f)
	if g.eventCalls.Load() != calls {
		t.Errorf("a second read inside the minute reached Google again (%d calls)", g.eventCalls.Load())
	}
	g.down.Store(true)
	if err := f.svc.SetGoogleCalendars(context.Background(), []string{"work"}); err != nil {
		t.Fatal(err)
	}
	// Choosing dropped the cache, so with Google down there is nothing to show: an honest error.
	got, reading := eventTitles(t, f)
	if len(got) != 0 || !reading.Connected || reading.Error == "" || reading.Stale {
		t.Errorf("down with nothing cached: %v %+v", got, reading)
	}
}

func TestARevokedGrantAsksToReconnectInsteadOfFailing(t *testing.T) {
	f, g := newGoogleFixture(t)
	f.grant(f.svc.AuthorizeGoogleCalendar)
	time.Sleep(1100 * time.Millisecond) // the fake's tokens last one second, so the next read refreshes
	g.invalidGrant.Store(true)
	got, reading := eventTitles(t, f)
	if len(got) != 0 || reading.Connected || !strings.Contains(reading.Error, "Reconnect") {
		t.Errorf("revoked: %v %+v, want not connected with a reconnect sentence", got, reading)
	}
	if _, err := f.svc.GoogleCalendarClient(context.Background()); !errors.Is(err, integrations.ErrNeedsReconnect) {
		t.Errorf("client error = %v, want ErrNeedsReconnect", err)
	}
}

func TestLastEventsAreShownWhenGoogleGoesDownAfterTheyWereRead(t *testing.T) {
	now := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	f, g := newGoogleFixture(t, func(o *integrations.Options) { o.Now = func() time.Time { return now } })
	f.grant(f.svc.AuthorizeGoogleCalendar)
	if got, _ := eventTitles(t, f); titles(got) != "Standup,Design review" {
		t.Fatalf("first read = %v", got)
	}
	now = now.Add(2 * time.Minute) // the cached read is no longer fresh
	g.down.Store(true)
	got, reading := eventTitles(t, f)
	if titles(got) != "Standup,Design review" || !reading.Stale || !reading.Connected || reading.Error == "" {
		t.Errorf("down after a good read: %v %+v, want the last events, marked stale, with a sentence", got, reading)
	}
	now = now.Add(48 * time.Hour) // far too old to show
	if got, reading := eventTitles(t, f); len(got) != 0 || reading.Stale {
		t.Errorf("a day-old read must not be shown: %v %+v", got, reading)
	}
}

func TestTheEventThatIsOnNowIsFoundAndNothingIsHeldWhenGoogleIsUnreadable(t *testing.T) {
	now := time.Date(2026, 10, 2, 9, 10, 0, 0, time.UTC) // inside the fake's Standup, 09:00 to 09:15
	f, g := newGoogleFixture(t, func(o *integrations.Options) { o.Now = func() time.Time { return now } })
	if _, on := f.svc.EventNow(context.Background(), now); on {
		t.Fatal("an event was on before Google Calendar was connected")
	}
	f.grant(f.svc.AuthorizeGoogleCalendar)
	event, on := f.svc.EventNow(context.Background(), now)
	if !on || event.Title != "Standup" {
		t.Fatalf("at 09:10 = %+v, %v, want Standup", event, on)
	}
	if _, on := f.svc.EventNow(context.Background(), now.Add(time.Hour)); on {
		t.Error("an event was on at 10:10, after Standup and before Design review")
	}
	g.down.Store(true)
	if err := f.svc.SetGoogleCalendars(context.Background(), []string{"work"}); err != nil {
		t.Fatal(err)
	}
	if _, on := f.svc.EventNow(context.Background(), now); on {
		t.Error("an unreadable Google must never count as an event being on")
	}
}

// newBundledFixture is a daemon whose build has Marshal's own Google client, and whose owner saved
// none of their own.
func newBundledFixture(t *testing.T) (*fixture, *fakeGoogle) {
	t.Helper()
	g := newFakeGoogle(t)
	f := newFixture(t, func(o *integrations.Options) {
		o.GCalRedirectURL = "http://127.0.0.1:47801/v1/integrations/gcal/callback"
		o.GoogleAuthURL = g.server.URL + "/auth"
		o.GoogleTokenURL = g.server.URL + "/token"
		o.GCalBaseURL = g.server.URL + "/calendar/v3/"
		o.GoogleClientID, o.GoogleClientSecret = "marshal-client-id", "marshal-client-secret"
	})
	return f, g
}

func TestWithMarshalsOwnClientOneClickConnectsWithNothingPasted(t *testing.T) {
	f, _ := newBundledFixture(t)
	info, err := f.svc.GoogleClientInfo(context.Background())
	if err != nil || !info.Bundled || info.Own {
		t.Fatalf("info = %+v, %v, want Marshal's own client and none of the person's", info, err)
	}
	raw, err := f.svc.AuthorizeGoogleCalendar(context.Background())
	if err != nil {
		t.Fatalf("authorize with no client saved: %v", err)
	}
	if got := mustQuery(t, raw).Get("client_id"); got != "marshal-client-id" {
		t.Errorf("consent asked as %q, want Marshal's own client", got)
	}
	f.grant(f.svc.AuthorizeGoogleCalendar)
	rows, err := f.svc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var status protocol.IntegrationStatus
	for _, row := range rows {
		if row.ID == integrations.GCalID {
			status = row.Status
		}
	}
	if status != protocol.IntegrationStatusConnected {
		t.Errorf("the Google Calendar row reads %q after the grant, want connected", status)
	}
	if got, reading := eventTitles(t, f); !reading.Connected || titles(got) != "Standup,Design review" {
		t.Errorf("events = %v %+v, want the calendar read through Marshal's client", got, reading)
	}
}

func TestAPersonsOwnClientIsUsedInsteadOfMarshalsAndDropsTheOldGrant(t *testing.T) {
	f, _ := newBundledFixture(t)
	f.grant(f.svc.AuthorizeGoogleCalendar)
	if err := f.svc.SaveGoogleCalendar(context.Background(), protocol.SaveGoogleCalendarRequest{
		ClientID: "my-own-id", ClientSecret: "my-own-secret",
	}); err != nil {
		t.Fatal(err)
	}
	info, _ := f.svc.GoogleClientInfo(context.Background())
	if !info.Own || !info.Bundled {
		t.Errorf("info = %+v, want their own client in use and Marshal's still known", info)
	}
	if _, err := f.svc.GoogleCalendarClient(context.Background()); !errors.Is(err, integrations.ErrNotConnected) {
		t.Errorf("a grant made through Marshal's client must not carry over to theirs, got %v", err)
	}
	raw, _ := f.svc.AuthorizeGoogleCalendar(context.Background())
	if got := mustQuery(t, raw).Get("client_id"); got != "my-own-id" {
		t.Errorf("consent asked as %q, want their own client", got)
	}
}

func TestWithNoClientAtAllConnectingIsRefusedAndThePasteFormIsTheWay(t *testing.T) {
	f, _ := newGoogleFixtureWithoutClient(t)
	info, err := f.svc.GoogleClientInfo(context.Background())
	if err != nil || info.Bundled || info.Own {
		t.Fatalf("info = %+v, %v, want no client at all", info, err)
	}
	if _, err := f.svc.AuthorizeGoogleCalendar(context.Background()); !errors.Is(err, integrations.ErrNoGoogleClient) {
		t.Errorf("authorize with no client gave %v, want ErrNoGoogleClient", err)
	}
}

func newGoogleFixtureWithoutClient(t *testing.T) (*fixture, *fakeGoogle) {
	t.Helper()
	g := newFakeGoogle(t)
	f := newFixture(t, func(o *integrations.Options) {
		o.GCalRedirectURL = "http://127.0.0.1:47801/v1/integrations/gcal/callback"
		o.GoogleAuthURL = g.server.URL + "/auth"
		o.GoogleTokenURL = g.server.URL + "/token"
		o.NoBundledGoogleClient = true
	})
	return f, g
}

func TestGmailAlsoConnectsThroughMarshalsClient(t *testing.T) {
	f, _ := newBundledFixture(t)
	if id := f.grant(f.svc.AuthorizeGmail); id != integrations.GmailID {
		t.Fatalf("granted %q, want gmail", id)
	}
	if _, err := f.svc.GmailClient(context.Background()); err != nil {
		t.Errorf("Gmail through Marshal's client: %v", err)
	}
}

func TestAnEventMarkedAvailableDoesNotMakeThePersonBusy(t *testing.T) {
	now := time.Date(2026, 10, 2, 10, 5, 0, 0, time.UTC)
	f, g := newGoogleFixture(t, func(o *integrations.Options) { o.Now = func() time.Time { return now } })
	g.workBody.Store(`{"items":[
		{"id":"a","summary":"Office hours","transparency":"transparent","start":{"dateTime":"2026-10-02T10:00:00Z"},"end":{"dateTime":"2026-10-02T11:00:00Z"}},
		{"id":"b","summary":"Working from home","eventType":"workingLocation","start":{"dateTime":"2026-10-02T10:00:00Z"},"end":{"dateTime":"2026-10-02T11:00:00Z"}},
		{"id":"c","summary":"Review","start":{"dateTime":"2026-10-02T10:30:00Z"},"end":{"dateTime":"2026-10-02T11:00:00Z"}}]}`)
	f.grant(f.svc.AuthorizeGoogleCalendar)
	if err := f.svc.SetGoogleCalendars(context.Background(), []string{"work"}); err != nil {
		t.Fatal(err)
	}
	if event, on := f.svc.EventNow(context.Background(), now); on {
		t.Errorf("at 10:05 only free events are on, but %q held alerts", event.Title)
	}
	if event, on := f.svc.EventNow(context.Background(), now.Add(30*time.Minute)); !on || event.Title != "Review" {
		t.Errorf("at 10:35 = %+v, %v, want Review", event, on)
	}
}

func TestDisconnectingTheLastGoogleConnectionEndsTheAccessAtGoogle(t *testing.T) {
	f, g := newGoogleFixture(t)
	f.grant(f.svc.AuthorizeGoogleCalendar)
	if err := f.svc.Remove(context.Background(), integrations.GCalID); err != nil {
		t.Fatal(err)
	}
	if g.revokeCalls.Load() != 1 || g.lastRevoked.Load() != "refresh-1" {
		t.Errorf("revoke calls = %d, token = %v, want one call with the refresh token", g.revokeCalls.Load(), g.lastRevoked.Load())
	}
	if _, err := f.svc.GoogleCalendarClient(context.Background()); !errors.Is(err, integrations.ErrNotConnected) {
		t.Errorf("Calendar after Disconnect gave %v, want ErrNotConnected", err)
	}
}

func TestDisconnectingOneOfTwoGoogleConnectionsLeavesTheGrantAlone(t *testing.T) {
	f, g := newGoogleFixture(t)
	f.grant(f.svc.AuthorizeGoogleCalendar)
	f.grant(f.svc.AuthorizeGmail)
	if err := f.svc.Remove(context.Background(), integrations.GCalID); err != nil {
		t.Fatal(err)
	}
	if g.revokeCalls.Load() != 0 {
		t.Fatalf("revoked while Gmail still used the grant: Google ends every scope for one revoked token")
	}
	if _, err := f.svc.GmailClient(context.Background()); err != nil {
		t.Errorf("Gmail stopped working after Calendar was removed: %v", err)
	}
	if err := f.svc.Remove(context.Background(), integrations.GmailID); err != nil {
		t.Fatal(err)
	}
	if g.revokeCalls.Load() != 1 {
		t.Errorf("revoke calls = %d after the last connection went, want 1", g.revokeCalls.Load())
	}
}

func TestAGoogleThatCannotBeToldDoesNotStopTheDisconnect(t *testing.T) {
	f, g := newGoogleFixture(t)
	f.grant(f.svc.AuthorizeGoogleCalendar)
	g.revokeFails.Store(true)
	if err := f.svc.Remove(context.Background(), integrations.GCalID); err != nil {
		t.Fatalf("Disconnect failed because Google answered 500: %v", err)
	}
	if _, err := f.svc.GoogleCalendarClient(context.Background()); !errors.Is(err, integrations.ErrNotConnected) {
		t.Errorf("the token stayed on this computer: %v", err)
	}
}

func TestGmailReadsConnectedOnlyWithALabelAndItsOwnGrant(t *testing.T) {
	f, _ := newGoogleFixture(t)
	ctx := context.Background()
	gmail := func() protocol.Integration {
		list, err := f.svc.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range list {
			if row.ID == integrations.GmailID {
				return row
			}
		}
		t.Fatal("no Gmail row")
		return protocol.Integration{}
	}
	if got := gmail(); got.Status != protocol.IntegrationStatusNone {
		t.Fatalf("before anything: %v", got.Status)
	}
	save := protocol.SaveGmailRequest{Label: "marshal", ProjectID: "marshal-sample"}
	if err := f.svc.SaveGmail(ctx, save); err != nil {
		t.Fatal(err)
	}
	if got := gmail(); got.Status != protocol.IntegrationStatusNone || !strings.Contains(got.Detail, "Grant access") {
		t.Errorf("label saved, not granted: %v %q, want not connected and told to grant", got.Status, got.Detail)
	}
	f.grant(f.svc.AuthorizeGmail)
	if got := gmail(); got.Status != protocol.IntegrationStatusConnected {
		t.Errorf("label saved and granted: %v, want connected", got.Status)
	}
	if err := f.svc.Remove(ctx, integrations.GmailID); err != nil {
		t.Fatal(err)
	}
	f.grant(f.svc.AuthorizeGmail)
	if got := gmail(); got.Status != protocol.IntegrationStatusNone {
		t.Errorf("granted with no label: %v, want not connected", got.Status)
	}
	if err := f.svc.SaveGmail(ctx, save); err != nil {
		t.Fatal(err)
	}
	if got := gmail(); got.Status != protocol.IntegrationStatusConnected {
		t.Errorf("granted first, label after: %v, want connected", got.Status)
	}
}
