package api_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// googleFake is where a stack's Google connections are pointed in a test.
type googleFake struct{ auth, token, calendar, clientID, clientSecret string }

// fakeGoogleServer answers Google's token endpoint and a one-calendar Calendar API. down makes the
// Calendar API fail.
func fakeGoogleServer(t *testing.T, down *atomic.Bool) googleFake {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"a","refresh_token":"r","token_type":"Bearer","expires_in":3600}`))
	})
	mux.HandleFunc("/calendar/v3/users/me/calendarList", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"id":"me@x.com","summary":"me@x.com","primary":true,"accessRole":"owner","selected":true}]}`))
	})
	mux.HandleFunc("/calendar/v3/calendars/", func(w http.ResponseWriter, _ *http.Request) {
		if down != nil && down.Load() {
			http.Error(w, `{"error":{"code":500}}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"id":"e1","summary":"Standup","location":"Room 2","htmlLink":"https://g/e1",
			"start":{"dateTime":"2026-09-30T09:00:00Z"},"end":{"dateTime":"2026-09-30T09:30:00Z"}}]}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return googleFake{auth: server.URL + "/auth", token: server.URL + "/token", calendar: server.URL + "/calendar/v3/"}
}

// withMarshalsGoogleClient gives the daemon Marshal's own Google client, so a person connects with one
// click and saves no client of their own.
func withMarshalsGoogleClient(fake googleFake) stackOption {
	fake.clientID, fake.clientSecret = "marshal-client", "marshal-secret"
	return func(c *stackConfig) { c.google = fake }
}

func withGoogle(fake googleFake) stackOption {
	return func(c *stackConfig) { c.google = fake }
}

// connectGoogle saves a client and walks the consent flow, ending with the callback a browser would
// hit: no token on it, because a browser visit carries none.
func connectGoogle(t *testing.T, st *stack) {
	t.Helper()
	st.do(http.MethodPut, "/v1/integrations/gcal", protocol.SaveGoogleCalendarRequest{ClientID: "id", ClientSecret: "secret"}).
		want(t, http.StatusOK)
	consent := decode[protocol.AuthorizeURL](t, st.do(http.MethodGet, "/v1/integrations/gcal/authorize", nil).want(t, http.StatusOK))
	u, err := url.Parse(consent.URL)
	if err != nil {
		t.Fatal(err)
	}
	r := st.doWith("", http.MethodGet, "/v1/integrations/gcal/callback?code=the-code&state="+u.Query().Get("state"), nil)
	if r.Status != http.StatusOK || !strings.Contains(string(r.Body), "connected") {
		t.Fatalf("the callback with no token = %d %q, want 200 and a connected page", r.Status, r.Body)
	}
}

func TestGoogleConsentCompletesThroughACallbackThatCarriesNoToken(t *testing.T) {
	st := newStack(t, withGoogle(fakeGoogleServer(t, nil)))
	connectGoogle(t, st)
	choices := decode[protocol.GoogleCalendarChoices](t, st.do(http.MethodGet, "/v1/integrations/gcal/calendars", nil).want(t, http.StatusOK))
	if len(choices.Calendars) != 1 || !choices.Calendars[0].Selected || !choices.Calendars[0].Primary {
		t.Errorf("calendars = %+v", choices)
	}
}

func TestTheGoogleCallbackRefusesAnUnknownStateAndAGoogleRefusal(t *testing.T) {
	st := newStack(t, withGoogle(fakeGoogleServer(t, nil)))
	st.do(http.MethodPut, "/v1/integrations/gcal", protocol.SaveGoogleCalendarRequest{ClientID: "id", ClientSecret: "s"}).want(t, http.StatusOK)
	if r := st.doWith("", http.MethodGet, "/v1/integrations/gcal/callback?code=x&state=made-up", nil); r.Status != http.StatusBadRequest {
		t.Errorf("an unknown state = %d, want 400", r.Status)
	}
	if r := st.doWith("", http.MethodGet, "/v1/integrations/gcal/callback?error=access_denied", nil); r.Status != http.StatusBadRequest ||
		!strings.Contains(string(r.Body), "access_denied") {
		t.Errorf("a refusal = %d %q, want 400 naming it", r.Status, r.Body)
	}
}

func TestChoosingCalendarsWithoutAConnectionIsRefusedInWords(t *testing.T) {
	st := newStack(t, withGoogle(fakeGoogleServer(t, nil)))
	r := st.do(http.MethodGet, "/v1/integrations/gcal/calendars", nil)
	if r.Status != http.StatusUnprocessableEntity || !strings.Contains(string(r.Body), "not connected") {
		t.Errorf("before connecting = %d %q, want a refusal that says so", r.Status, r.Body)
	}
}

func rangeQuery() string {
	return "/v1/calendar?start=1790000000000&end=1790900000000"
}

func TestTheCalendarListsGoogleEventsWithTheirDetail(t *testing.T) {
	st := newStack(t, withGoogle(fakeGoogleServer(t, nil)))
	connectGoogle(t, st)
	list := decode[protocol.CalendarList](t, st.do(http.MethodGet, rangeQuery(), nil).want(t, http.StatusOK))
	if !list.GoogleConnected || list.GoogleError != "" || len(list.Events) != 1 {
		t.Fatalf("calendar = %+v", list)
	}
	event := list.Events[0]
	if event.Title != "Standup" || event.Location != "Room 2" || event.URL != "https://g/e1" ||
		event.End == nil || event.Calendar != "me@x.com" || event.AllDay {
		t.Errorf("event = %+v", event)
	}
}

func TestTheCalendarKeepsItsOtherPartsWhenGoogleCannotBeRead(t *testing.T) {
	var down atomic.Bool
	st := newStack(t, withGoogle(fakeGoogleServer(t, &down)))
	connectGoogle(t, st)
	down.Store(true)
	r := st.do(http.MethodGet, rangeQuery(), nil)
	list := decode[protocol.CalendarList](t, r.want(t, http.StatusOK))
	if list.GoogleError == "" || len(list.Events) != 0 {
		t.Errorf("down: %+v, want the answer to say why Google's events are missing", list)
	}
	if list.Schedules == nil || list.DueCards == nil {
		t.Error("the schedules and due cards must still be there, never null")
	}
}

func TestTheCalendarSaysNotConnectedBeforeGoogleIsSetUp(t *testing.T) {
	st := newStack(t)
	list := decode[protocol.CalendarList](t, st.do(http.MethodGet, rangeQuery(), nil).want(t, http.StatusOK))
	if list.GoogleConnected || list.GoogleError != "" || len(list.Events) != 0 {
		t.Errorf("not connected: %+v", list)
	}
}

func TestTheClientInfoSaysWhetherOneClickIsAvailable(t *testing.T) {
	without := newStack(t, withGoogle(fakeGoogleServer(t, nil)))
	info := decode[protocol.GoogleClientInfo](t, without.do(http.MethodGet, "/v1/integrations/gcal/client", nil).want(t, http.StatusOK))
	if info.Bundled || info.Own {
		t.Errorf("a build with no client: %+v", info)
	}
	with := newStack(t, withMarshalsGoogleClient(fakeGoogleServer(t, nil)))
	info = decode[protocol.GoogleClientInfo](t, with.do(http.MethodGet, "/v1/integrations/gcal/client", nil).want(t, http.StatusOK))
	if !info.Bundled || info.Own {
		t.Errorf("a build with Marshal's client: %+v", info)
	}
}

func TestOneClickConnectsWithoutSavingAnyClient(t *testing.T) {
	st := newStack(t, withMarshalsGoogleClient(fakeGoogleServer(t, nil)))
	consent := decode[protocol.AuthorizeURL](t, st.do(http.MethodGet, "/v1/integrations/gcal/authorize", nil).want(t, http.StatusOK))
	u, err := url.Parse(consent.URL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("client_id") != "marshal-client" {
		t.Errorf("consent asked as %q, want Marshal's own client", u.Query().Get("client_id"))
	}
	r := st.doWith("", http.MethodGet, "/v1/integrations/gcal/callback?code=c&state="+u.Query().Get("state"), nil)
	if r.Status != http.StatusOK {
		t.Fatalf("callback = %d %q", r.Status, r.Body)
	}
	choices := decode[protocol.GoogleCalendarChoices](t, st.do(http.MethodGet, "/v1/integrations/gcal/calendars", nil).want(t, http.StatusOK))
	if len(choices.Calendars) != 1 {
		t.Errorf("calendars = %+v, want the one calendar read through Marshal's client", choices)
	}
}
