// Package previewrelay exposes loopback development servers through short-lived,
// per-card gateways. It deliberately has no dependency on the API or store layers.
package previewrelay

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

func (g *gateway) serveProxy(w http.ResponseWriter, r *http.Request) {
	secret := ""
	if cookie, err := r.Cookie(g.cookieName); err == nil {
		secret = cookie.Value
	}
	g.mu.Lock()
	g.pruneLocked(time.Now())
	auth, ok := g.sessions[secret]
	g.mu.Unlock()
	if !ok || !safeAuthorized(auth.allowed, r.Context()) {
		if ok {
			g.mu.Lock()
			delete(g.sessions, secret)
			g.mu.Unlock()
			http.SetCookie(w, &http.Cookie{Name: g.cookieName, Value: "", Path: "/", HttpOnly: true, Secure: g.secureCookie, SameSite: http.SameSiteStrictMode, MaxAge: -1})
		}
		writeHTML(w, http.StatusForbidden, "Preview access denied", "Preview access expired or is no longer authorized. Reopen Preview in Marshal.", g.clientOrigin)
		return
	}
	ctx := r.Context()
	cancel := func() {}
	if isWebSocketUpgrade(r) {
		var cancelCtx context.CancelFunc
		ctx, cancelCtx = context.WithCancel(ctx)
		cancel = cancelCtx
		go g.watchWebSocket(ctx, cancelCtx, auth)
	}
	defer cancel()
	proxy := g.reverseProxy()
	proxy.Rewrite = func(pr *httputil.ProxyRequest) {
		pr.SetURL(g.upstream)
		pr.Out.URL.Path = pr.In.URL.Path
		pr.Out.URL.RawPath = pr.In.URL.RawPath
		pr.Out.Host = g.upstream.Host
		if pr.Out.Header.Get("Origin") != "" {
			pr.Out.Header.Set("Origin", g.upstream.Scheme+"://"+g.upstream.Host)
		}
		pr.Out.Header.Del("Proxy-Authorization")
		pr.Out.Header.Del("Cookie")
		for _, c := range pr.In.Cookies() {
			if !strings.HasPrefix(strings.ToLower(c.Name), "marshal-preview-") {
				pr.Out.AddCookie(c)
			}
		}
		pr.Out.URL.RawQuery = pr.In.URL.RawQuery
		pr.Out.URL.ForceQuery = pr.In.URL.ForceQuery
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Del("X-Frame-Options")
		replaceFrameAncestors(resp.Header, auth.clientOrigin)
		stripGatewayCookies(resp.Header)
		rewriteLocation(resp.Header, g.upstream, "")
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, _ error) {
		writeHTML(w, http.StatusBadGateway, "Preview server unavailable", "The project preview server is not responding. Start or restart the project's development server, then reopen Preview.", auth.clientOrigin)
	}
	proxy.ServeHTTP(w, r.WithContext(ctx))
}

func (g *gateway) reverseProxy() *httputil.ReverseProxy {
	return &httputil.ReverseProxy{FlushInterval: -1, ErrorLog: log.New(io.Discard, "", 0)}
}

func (g *gateway) watchWebSocket(ctx context.Context, cancel context.CancelFunc, auth authorization) {
	ticker := time.NewTicker(websocketCheckRate)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !time.Now().Before(auth.expires) || !safeAuthorized(auth.allowed, ctx) {
				cancel()
				return
			}
		}
	}
}

func replaceFrameAncestors(h http.Header, origin string) {
	values := h.Values(contentSecurityPolicy)
	h.Del(contentSecurityPolicy)
	if len(values) == 0 {
		h.Add(contentSecurityPolicy, "frame-ancestors "+origin)
		return
	}
	for _, value := range values {
		parts := strings.Split(value, ";")
		out := make([]string, 0, len(parts)+1)
		found := false
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			fields := strings.Fields(trimmed)
			if len(fields) > 0 && strings.EqualFold(fields[0], "frame-ancestors") {
				if !found {
					out = append(out, " frame-ancestors "+origin)
					found = true
				}
				continue
			}
			if trimmed != "" {
				out = append(out, " "+trimmed)
			}
		}
		if !found {
			out = append(out, " frame-ancestors "+origin)
		}
		h.Add(contentSecurityPolicy, strings.Join(out, ";"))
	}
}

func stripGatewayCookies(h http.Header) {
	values := h.Values("Set-Cookie")
	h.Del("Set-Cookie")
	for _, value := range values {
		name, _, _ := strings.Cut(value, "=")
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "marshal-preview-") {
			continue
		}
		h.Add("Set-Cookie", value)
	}
}

func rewriteLocation(h http.Header, upstream *url.URL, appPrefix string) {
	location := h.Get("Location")
	if location == "" {
		return
	}
	u, err := url.Parse(location)
	if err != nil {
		return
	}
	if u.IsAbs() && (u.Scheme != upstream.Scheme || !strings.EqualFold(u.Host, upstream.Host)) {
		return
	}
	if u.IsAbs() {
		u.Scheme, u.Host = "", ""
	}
	if !strings.HasPrefix(u.Path, "/") {
		return
	}
	u.Path = appPrefix + u.Path
	h.Set("Location", u.String())
}
