package previewrelay

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestRelativePathsAndOrigins(t *testing.T) {
	for _, path := range []string{"/", "/assets/app.js?v=1#part", "/nested/page"} {
		if !validRelativePath(path) {
			t.Errorf("rejected %q", path)
		}
	}
	for _, path := range []string{"", "//evil.test", "/a/../b", "/a/%2e%2e/b", "/a\\b", "/bad\npath", "http://evil.test", "/%zz", strings.Repeat("/", 8200)} {
		if validRelativePath(path) {
			t.Errorf("accepted %q", path)
		}
	}
	for raw, want := range map[string]string{"http://LOCALHOST:80/": "http://localhost", "https://Host.test:443": "https://host.test", "https://host.test:9443": "https://host.test:9443"} {
		got, err := parseOrigin(raw)
		if err != nil || got != want {
			t.Errorf("origin %q = %q %v", raw, got, err)
		}
	}
	if _, err := parsePublicBase("https://host.test/path"); err == nil {
		t.Fatal("accepted public path")
	}
	for _, host := range []string{"localhost", "127.0.0.1", "::1"} {
		if !isLoopbackHost(host) {
			t.Error(host)
		}
	}
	if isLoopbackHost("localhost.evil.test") {
		t.Fatal("trusted hostname suffix")
	}
	if safeAuthorized(nil, context.Background()) || safeAuthorized(func(context.Context) bool { panic("bad") }, context.Background()) {
		t.Fatal("unsafe authorization")
	}
}

func TestProxyHeadersAndRedirects(t *testing.T) {
	h := http.Header{}
	h.Add("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; frame-ancestors bad")
	h.Add("Content-Security-Policy", "script-src 'self'")
	h.Add("Set-Cookie", "marshal-preview-secret=bad; Path=/")
	h.Add("Set-Cookie", "app=good; Path=/")
	replaceFrameAncestors(h, "http://localhost:3210")
	stripGatewayCookies(h)
	if len(h.Values("Content-Security-Policy")) != 2 || len(h.Values("Set-Cookie")) != 1 {
		t.Fatal(h)
	}
	target, _ := url.Parse("http://localhost:3000")
	for raw, want := range map[string]string{"http://localhost:3000/login?q=1": "/login?q=1", "/next": "/next", "https://example.com/next": "https://example.com/next", "login": "login", "": ""} {
		h.Set("Location", raw)
		rewriteLocation(h, target, "")
		if h.Get("Location") != want {
			t.Errorf("redirect %q => %q", raw, h.Get("Location"))
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "http://localhost", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "keep-alive, Upgrade")
	if !isWebSocketUpgrade(req) {
		t.Fatal("upgrade not recognized")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	g := &gateway{}
	g.watchWebSocket(ctx, cancel, authorization{expires: time.Now().Add(-time.Second)})
}
