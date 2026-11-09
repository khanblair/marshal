// Package previewrelay exposes loopback development servers through short-lived,
// per-card gateways. It deliberately has no dependency on the API or store layers.
package previewrelay

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	previewUnavailable    = "Preview unavailable"
	reopenPreview         = "Close this tab and reopen Preview in Marshal."
	contentSecurityPolicy = "Content-Security-Policy"
)

func (g *gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !g.validHost(r.Host) {
		writeHTML(w, http.StatusMisdirectedRequest, previewUnavailable, "This preview link is not valid for this gateway.", "")
		return
	}
	if !time.Now().Before(g.expiresAt) {
		writeHTML(w, http.StatusForbidden, "Preview expired", reopenPreview, "")
		return
	}
	if r.URL.Path == g.prefix+"bootstrap" {
		g.serveBootstrap(w, r)
		return
	}
	if r.URL.Path == g.prefix+"authorize" {
		g.serveAuthorize(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, previewPath) {
		http.NotFound(w, r)
		return
	}
	g.serveProxy(w, r)
}

func (g *gateway) validHost(host string) bool {
	if strings.EqualFold(host, g.publicHost) {
		return true
	}
	hostname, port, err := net.SplitHostPort(host)
	if err != nil {
		return false
	}
	if _, err = strconv.Atoi(port); err != nil || port != strconv.Itoa(g.listener.Addr().(*net.TCPAddr).Port) {
		return false
	}
	return isLoopbackHost(hostname)
}

func (g *gateway) serveBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeHTML(w, http.StatusMethodNotAllowed, previewUnavailable, "This preview link is not valid.", "")
		return
	}
	origin, err := parseOrigin(r.URL.Query().Get("origin"))
	if err != nil || !g.hasClientOrigin(origin) {
		writeHTML(w, http.StatusForbidden, previewUnavailable, "This preview link is not valid.", "")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set(contentSecurityPolicy, bootstrapCSP(origin))
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, bootstrapHTML)
}

func (g *gateway) serveAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeHTML(w, http.StatusMethodNotAllowed, previewUnavailable, "This preview link is not valid.", "")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != g.publicBase.Scheme+"://"+g.publicBase.Host {
		writeHTML(w, http.StatusForbidden, "Preview access denied", "The preview authorization request was not accepted.", "")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBootstrapBody)
	defer func() { _ = r.Body.Close() }()
	var payload authorizePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Ticket == "" || len(payload.Ticket) > 128 || !validRelativePath(payload.Path) {
		writeHTML(w, http.StatusForbidden, "Preview expired", reopenPreview, "")
		return
	}
	now := time.Now()
	g.mu.Lock()
	g.pruneLocked(now)
	t, ok := g.tickets[payload.Ticket]
	if ok {
		delete(g.tickets, payload.Ticket)
	}
	if !ok || subtle.ConstantTimeCompare([]byte(payload.Path), []byte(t.path)) != 1 || !now.Before(t.expires) {
		g.mu.Unlock()
		writeHTML(w, http.StatusForbidden, "Preview expired", reopenPreview, "")
		return
	}
	g.mu.Unlock()
	if !safeAuthorized(t.authorize, r.Context()) {
		writeHTML(w, http.StatusForbidden, "Preview access denied", "Your Marshal pairing is no longer authorized. Reopen Preview after reconnecting.", t.clientOrigin)
		return
	}
	secret, err := randomToken()
	if err != nil {
		writeHTML(w, http.StatusInternalServerError, previewUnavailable, "Marshal could not authorize this preview. Reopen it to try again.", t.clientOrigin)
		return
	}
	g.mu.Lock()
	g.pruneLocked(time.Now())
	if len(g.sessions) >= MaxAuthorizations {
		g.mu.Unlock()
		writeHTML(w, http.StatusTooManyRequests, "Preview busy", "Too many preview tabs are open. Close one and try again.", t.clientOrigin)
		return
	}
	expires := time.Now().Add(CapabilityTTL)
	if g.expiresAt.Before(expires) {
		expires = g.expiresAt
	}
	g.sessions[secret] = authorization{expires: expires, clientOrigin: t.clientOrigin, allowed: t.authorize}
	g.mu.Unlock()
	remaining := int(time.Until(expires).Seconds())
	if remaining < 1 {
		remaining = 1
	}
	http.SetCookie(w, &http.Cookie{
		Name: g.cookieName, Value: secret, Path: "/", HttpOnly: true,
		Secure: g.secureCookie, SameSite: http.SameSiteStrictMode,
		MaxAge: remaining, Expires: expires,
	})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set(contentSecurityPolicy, "default-src 'none'; frame-ancestors "+t.clientOrigin)
	redirect := t.path
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(authorizeResponse{Redirect: redirect})
}

func bootstrapCSP(origin string) string {
	return "default-src 'none'; script-src 'sha256-" + scriptHash() + "'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors " + origin
}

func writeHTML(w http.ResponseWriter, status int, title, message, origin string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if origin != "" {
		w.Header().Set(contentSecurityPolicy, "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors "+origin)
	} else {
		w.Header().Set(contentSecurityPolicy, "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'")
	}
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, "<!doctype html><html><head><meta charset=utf-8><meta name=viewport content=\"width=device-width,initial-scale=1\"><title>%s</title><style>body{font:16px system-ui,sans-serif;background:#f7f7f7;color:#222;margin:0;padding:2rem}main{max-width:32rem;margin:10vh auto;background:white;border:1px solid #ddd;border-radius:12px;padding:1.5rem}</style></head><body><main><h1>%s</h1><p>%s</p></main></body></html>", html.EscapeString(title), html.EscapeString(title), html.EscapeString(message))
}

const bootstrapScript = `(()=>{const raw=location.hash.slice(1);history.replaceState(null,"",location.pathname+location.search);const fail=()=>{document.body.innerHTML="<main><h1>Preview expired</h1><p>Close this tab and reopen Preview in Marshal.</p></main>"};try{const bytes=Uint8Array.from(atob(raw.replace(/-/g,"+").replace(/_/g,"/")),c=>c.charCodeAt(0));const payload=JSON.parse(new TextDecoder().decode(bytes));const endpoint=location.pathname.replace(/bootstrap$/,"authorize");fetch(endpoint,{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:JSON.stringify(payload)}).then(async r=>{if(!r.ok)throw Error();const body=await r.json();if(typeof body.redirect!=="string"||!body.redirect.startsWith("/")||body.redirect.startsWith("//"))throw Error();const next=new URL(body.redirect,location.origin);if(next.origin!==location.origin)throw Error();location.replace(next.href)}).catch(fail)}catch{fail()}})();`

func scriptHash() string {
	sum := sha256.Sum256([]byte(bootstrapScript))
	return base64.StdEncoding.EncodeToString(sum[:])
}

const bootstrapHTML = `<!doctype html><html><head><meta charset="utf-8"><meta name="referrer" content="no-referrer"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Opening preview</title></head><body><main><h1>Opening preview…</h1></main><script>` + bootstrapScript + `</script></body></html>`
