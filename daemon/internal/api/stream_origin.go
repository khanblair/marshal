package api

import (
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// originPatterns lists the origins besides the request's own host that may open a stream. A dev
// daemon accepts a page from the local dev server on any port. A normal daemon accepts the
// desktop app's web view, whose origin is tauri://localhost on macOS and Linux and
// http://tauri.localhost on Windows.
//
// A daemon with a tailnet node also accepts the origins that are its own node's names: its MagicDNS
// name and its tailnet addresses. That is the case a phone opens - the page loaded from one of
// those names, and the stream may be opened to another of them - and it adds no host that is not
// this daemon's own node, so a page from anywhere else is still refused.
//
// The rule a page served over the tailnet usually hits is not one of these at all: it is the
// request's own host in originAllowed, which passes any page the daemon itself served. These
// patterns are the exceptions to that, for the pages served somewhere else.
//
// A request with no Origin header is not from a browser page and passes the check: a page cannot
// leave the header out.
func (s *Server) originPatterns() []string {
	var patterns []string
	if s.settings.Dev() {
		patterns = []string{"localhost:*", "127.0.0.1:*"}
	} else {
		patterns = []string{"tauri.localhost", "tauri://localhost"}
	}
	return append(patterns, s.tailnetOrigins()...)
}

// originAllowed applies the same rule as the WebSocket library does when it accepts a connection:
// no Origin passes, an origin with the request's own host passes, and an origin that matches a
// pattern passes. The check is made here first so a refusal is answered in the error shape and
// before any token is looked at. A pattern with "://" is matched against "scheme://host", and any
// other pattern against the host with its port. Matching ignores case.
func (s *Server) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	if strings.EqualFold(r.Host, parsed.Host) {
		return true
	}
	for _, pattern := range s.originPatterns() {
		target := parsed.Host
		if strings.Contains(pattern, "://") {
			target = parsed.Scheme + "://" + parsed.Host
		}
		if matched, err := path.Match(strings.ToLower(pattern), strings.ToLower(target)); err == nil && matched {
			return true
		}
	}
	return false
}

func errOriginRefused() *protocol.Error {
	return protocol.Forbidden("Marshal only connects to its own app. Open Marshal from the desktop app or from its own address.")
}

// offeredSubprotocols lists what the client offered in Sec-WebSocket-Protocol. The list holds the
// client's token, so it is read and never logged.
func offeredSubprotocols(r *http.Request) []string {
	var offered []string
	for _, line := range r.Header.Values("Sec-WebSocket-Protocol") {
		for part := range strings.SplitSeq(line, ",") {
			if part = strings.TrimSpace(part); part != "" {
				offered = append(offered, part)
			}
		}
	}
	return offered
}

// streamToken finds the client's token. A browser cannot set a header on a WebSocket, so it
// offers the token as the subprotocol "bearer.<token>". A client that can set headers may send
// the usual Authorization header instead.
func streamToken(r *http.Request, offered []string) string {
	for _, name := range offered {
		if token, found := strings.CutPrefix(name, protocol.BearerSubprotocolPrefix); found {
			return token
		}
	}
	return bearerToken(r.Header.Get("Authorization"))
}

// isUpgrade reports whether the request asks to switch to a WebSocket.
func isUpgrade(r *http.Request) bool {
	return headerHasToken(r.Header, "Connection", "upgrade") && headerHasToken(r.Header, "Upgrade", "websocket")
}

func headerHasToken(h http.Header, key, token string) bool {
	for _, line := range h.Values(key) {
		for part := range strings.SplitSeq(line, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}
