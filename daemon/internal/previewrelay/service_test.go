package previewrelay

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"

	"github.com/coder/websocket"
	"sync/atomic"
	"testing"
	"time"
)

func allowPreview(context.Context) bool { return true }

func authorizePreview(t *testing.T, connection Connection) (*http.Client, string, *http.Cookie) {
	t.Helper()
	u, err := url.Parse(connection.URL)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(u.Fragment)
	if err != nil {
		t.Fatal(err)
	}
	var payload bootstrapPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	u.Fragment = ""
	bootstrap, err := http.Get(u.String())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(bootstrap.Body)
	_ = bootstrap.Body.Close()
	if bootstrap.StatusCode != http.StatusOK || !strings.Contains(string(body), "Opening preview") {
		t.Fatalf("bootstrap: %d %s", bootstrap.StatusCode, body)
	}
	u.Path = strings.TrimSuffix(u.Path, "bootstrap") + "authorize"
	u.RawQuery = ""
	response, err := http.Post(u.String(), "application/json", strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("authorize %d %s", response.StatusCode, data)
	}
	cookies := response.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].Path != "/" {
		t.Fatalf("gateway cookie: %+v", cookies)
	}
	var answer authorizeResponse
	if err := json.NewDecoder(response.Body).Decode(&answer); err != nil {
		t.Fatal(err)
	}
	base := u.Scheme + "://" + u.Host
	if answer.Redirect != payload.Path {
		t.Fatalf("redirect %q", answer.Redirect)
	}
	return &http.Client{Timeout: time.Second}, base, cookies[0]
}

func TestRelayProxiesRootAssetsAndRevokesPairing(t *testing.T) {
	var allowed atomic.Bool
	allowed.Store(true)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Cookie"), "marshal-preview-") {
			t.Error("gateway credential leaked upstream")
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("bearer reached project")
		}
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
		w.Header().Set("Set-Cookie", "project=ok; Path=/")
		_, _ = io.WriteString(w, r.URL.RequestURI())
	}))
	defer upstream.Close()
	service := New(nil)
	defer service.Close()
	connection, err := service.Open(context.Background(), "card", upstream.URL+"/start?q=one#part", "http://localhost:3210", func(context.Context) bool { return allowed.Load() })
	if err != nil {
		t.Fatal(err)
	}
	// The gateway listener uses the caller's localhost hostname, not the upstream's port.
	client, base, cookie := authorizePreview(t, connection)
	req, _ := http.NewRequest(http.MethodGet, base+"/_next/static/app.js?q=two", nil)
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: "project", Value: "ok"})
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != 200 || string(data) != "/_next/static/app.js?q=two" {
		t.Fatalf("asset: %d %s", response.StatusCode, data)
	}
	if response.Header.Get("X-Frame-Options") != "" || !strings.Contains(response.Header.Get("Content-Security-Policy"), "frame-ancestors http://localhost:3210") {
		t.Fatal("framing policy not isolated to Marshal")
	}
	allowed.Store(false)
	response, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("revoked pairing: %d", response.StatusCode)
	}
}

func TestRelayTicketIsSingleUseAndGatewayExpires(t *testing.T) {
	service := New(nil)
	defer service.Close()
	connection, err := service.Open(context.Background(), "card", "http://127.0.0.1:39871/", "http://127.0.0.1:3210", allowPreview)
	if err != nil {
		t.Fatal(err)
	}
	_, base, _ := authorizePreview(t, connection)
	u, _ := url.Parse(connection.URL)
	raw, _ := base64.RawURLEncoding.DecodeString(u.Fragment)
	endpoint := base + strings.TrimSuffix(u.Path, "bootstrap") + "authorize"
	response, err := http.Post(endpoint, "application/json", strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatalf("reused ticket: %d", response.StatusCode)
	}
	service.mu.Lock()
	for _, g := range service.gateways {
		g.mu.Lock()
		g.expiresAt = time.Now().Add(-time.Minute)
		g.mu.Unlock()
	}
	service.mu.Unlock()
	service.Close()
	service.Close()
	if _, err := service.Open(context.Background(), "card", "http://localhost:3000", "http://localhost:3210", allowPreview); err == nil {
		t.Fatal("opened closed relay")
	}
}

func TestRelayRejectsUnsafeSourcesAndOrigin(t *testing.T) {
	service := New([]int{49200})
	defer service.Close()
	for _, source := range []string{"https://localhost:3000", "http://example.com:3000", "http://127.0.0.1:80", "http://localhost:47800", "http://localhost:49200", "http://user:pass@localhost:3000", "http://localhost:3000/a/../b", "garbage"} {
		if _, err := service.Open(context.Background(), "card", source, "http://localhost:3210", allowPreview); err == nil {
			t.Errorf("accepted %s", source)
		}
	}
	for _, origin := range []string{"null", "javascript:bad", "http://host/a", "http://host?x=1", "http://host:99999", "http://host;script-src", "http://host'unsafe"} {
		if _, err := service.Open(context.Background(), "card", "http://localhost:3000", origin, allowPreview); err == nil {
			t.Errorf("accepted origin %q", origin)
		}
	}
	if _, err := service.Open(context.Background(), "", "http://localhost:3000", "http://localhost:3210", allowPreview); err == nil {
		t.Fatal("accepted empty card")
	}
	if _, err := service.Open(context.Background(), "card", "http://localhost:3000", "http://localhost:3210", nil); err == nil {
		t.Fatal("accepted missing authorization")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Open(ctx, "card", "http://localhost:3000", "http://localhost:3210", allowPreview); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestRemoteExposureIsPrivateAndCleanedUp(t *testing.T) {
	service := New(nil)
	var cleanups atomic.Int32
	service.SetExposure(func(_ context.Context, port int, secure bool) (string, func(), error) {
		if !secure || port < 1024 {
			t.Error("invalid exposure")
		}
		return "https://computer.test.ts.net:9443", func() { cleanups.Add(1) }, nil
	})
	connection, err := service.Open(context.Background(), "card", "http://localhost:3000", "https://computer.test.ts.net", allowPreview)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(connection.URL, "https://computer.test.ts.net:9443/") {
		t.Fatal(connection.URL)
	}
	service.Close()
	if cleanups.Load() != 1 {
		t.Fatalf("cleanup count %d", cleanups.Load())
	}
}

func TestGatewayRejectsWrongHostsAndBootstrapRequests(t *testing.T) {
	service := New(nil)
	defer service.Close()
	connection, err := service.Open(t.Context(), "card", "http://localhost:3000/", "http://localhost:3210", allowPreview)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(connection.URL)
	var g *gateway
	for _, candidate := range service.gateways {
		g = candidate
	}
	cases := []struct {
		method, path, host, body, origin string
		status                           int
	}{
		{"GET", "/", "evil.test", "", "", 421},
		{"POST", u.RequestURI(), u.Host, "", "", 405},
		{"GET", u.Path + "?origin=http://evil.test", u.Host, "", "", 403},
		{"GET", g.prefix + "authorize", u.Host, "", "", 405},
		{"POST", g.prefix + "authorize", u.Host, `{}`, "http://evil.test", 403},
		{"POST", g.prefix + "authorize", u.Host, `not json`, "", 403},
		{"POST", g.prefix + "authorize", u.Host, `{"ticket":"missing","path":"/"}`, "", 403},
		{"GET", previewPath + "unknown", u.Host, "", "", 404},
		{"GET", "/", u.Host, "", "", 403},
	}
	for _, test := range cases {
		req := httptest.NewRequest(test.method, "http://"+u.Host+test.path, strings.NewReader(test.body))
		req.Host = test.host
		if test.origin != "" {
			req.Header.Set("Origin", test.origin)
		}
		recorder := httptest.NewRecorder()
		g.ServeHTTP(recorder, req)
		if recorder.Code != test.status {
			t.Errorf("%s %s: %d want %d", test.method, test.path, recorder.Code, test.status)
		}
	}
}

func TestGatewayReportsAnUnavailableProjectAndBoundsAuthorizations(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	source := upstream.URL
	upstream.Close()
	service := New(nil)
	defer service.Close()
	connection, err := service.Open(t.Context(), "card", source, "http://127.0.0.1:3210", allowPreview)
	if err != nil {
		t.Fatal(err)
	}
	client, base, cookie := authorizePreview(t, connection)
	req, _ := http.NewRequest(http.MethodGet, base+"/", nil)
	req.AddCookie(cookie)
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusBadGateway {
		t.Fatal(response.StatusCode)
	}
	var g *gateway
	for _, candidate := range service.gateways {
		g = candidate
	}
	g.mu.Lock()
	for i := 0; i < MaxAuthorizations; i++ {
		g.tickets[strconv.Itoa(i)] = ticket{expires: time.Now().Add(time.Minute)}
	}
	g.mu.Unlock()
	if _, err := service.Open(t.Context(), "card", source, "http://127.0.0.1:3210", allowPreview); err == nil {
		t.Fatal("authorization limit bypassed")
	}
	g.mu.Lock()
	g.pruneLocked(time.Now().Add(CapabilityTTL + time.Minute))
	g.mu.Unlock()
}

func TestRelayCarriesWebSocketMessages(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer connection.CloseNow()
		kind, data, err := connection.Read(r.Context())
		if err == nil {
			_ = connection.Write(r.Context(), kind, data)
		}
	}))
	defer upstream.Close()
	service := New(nil)
	defer service.Close()
	connection, err := service.Open(t.Context(), "card", upstream.URL, "http://127.0.0.1:3210", allowPreview)
	if err != nil {
		t.Fatal(err)
	}
	_, base, cookie := authorizePreview(t, connection)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	headers := http.Header{}
	headers.Set("Cookie", cookie.String())
	socket, _, err := websocket.Dial(ctx, strings.Replace(base, "http:", "ws:", 1)+"/hmr", &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatal(err)
	}
	defer socket.CloseNow()
	if err := socket.Write(ctx, websocket.MessageText, []byte("reload")); err != nil {
		t.Fatal(err)
	}
	_, message, err := socket.Read(ctx)
	if err != nil || string(message) != "reload" {
		t.Fatalf("websocket: %s %v", message, err)
	}
}
