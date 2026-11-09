// Package previewrelay exposes loopback development servers through short-lived,
// per-card gateways. It deliberately has no dependency on the API or store layers.
package previewrelay

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func safeAuthorized(fn func(context.Context) bool, ctx context.Context) (allowed bool) {
	if fn == nil || ctx.Err() != nil {
		return false
	}
	defer func() {
		if recover() != nil {
			allowed = false
		}
	}()
	return fn(ctx)
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") && headerHasToken(r.Header, "Connection", "upgrade")
}

func headerHasToken(h http.Header, key, token string) bool {
	for _, line := range h.Values(key) {
		for _, value := range strings.Split(line, ",") {
			if strings.EqualFold(strings.TrimSpace(value), token) {
				return true
			}
		}
	}
	return false
}

func parseSourceURL(raw string) (*url.URL, string, error) {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Opaque != "" || !strings.EqualFold(u.Scheme, httpScheme) || u.User != nil || u.Host == "" {
		return nil, "", errors.New("preview source must be an HTTP URL to a loopback development server")
	}
	portText := u.Port()
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1024 || port > 65535 {
		return nil, "", errors.New("preview source port must be between 1024 and 65535")
	}
	hostname := u.Hostname()
	if !isLoopbackHost(hostname) {
		return nil, "", errors.New("preview source must use localhost or a literal loopback address")
	}
	if reservedPort(port) {
		return nil, "", errors.New("preview source uses a reserved Marshal port")
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	path += rawQueryAndFragment(u)
	if !validRelativePath(path) {
		return nil, "", errors.New("preview source path is invalid")
	}
	upstream := &url.URL{Scheme: httpScheme, Host: net.JoinHostPort(strings.ToLower(hostname), portText)}
	return upstream, path, nil
}

func rawQueryAndFragment(u *url.URL) string {
	var b strings.Builder
	if u.ForceQuery || u.RawQuery != "" {
		b.WriteByte('?')
		b.WriteString(u.RawQuery)
	}
	if u.Fragment != "" {
		b.WriteByte('#')
		b.WriteString(u.EscapedFragment())
	}
	return b.String()
}

func parseOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Opaque != "" || u.User != nil || u.Host == "" || (u.Scheme != httpScheme && u.Scheme != httpsScheme) || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("expected an http(s) origin")
	}
	if !validOriginHost(u.Hostname()) || strings.ContainsAny(u.Host, "\\\r\n\t ") || u.Port() == "" && strings.Contains(u.Host, ":") && !strings.HasPrefix(u.Host, "[") {
		return "", errors.New("invalid origin host")
	}
	if port := u.Port(); port != "" {
		p, parseErr := strconv.Atoi(port)
		if parseErr != nil || p < 1 || p > 65535 {
			return "", errors.New("invalid origin port")
		}
		if (u.Scheme == httpScheme && p == 80) || (u.Scheme == httpsScheme && p == 443) {
			u.Host = u.Hostname()
			if strings.Contains(u.Host, ":") {
				u.Host = "[" + u.Host + "]"
			}
		}
	}
	u.Host = strings.ToLower(u.Host)
	return u.Scheme + "://" + u.Host, nil
}

func parsePublicBase(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.User != nil || u.Host == "" || (u.Scheme != httpScheme && u.Scheme != httpsScheme) || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("expected an http(s) origin")
	}
	if _, err := parseOrigin(u.Scheme + "://" + u.Host); err != nil {
		return nil, err
	}
	u.Path, u.RawPath, u.RawQuery, u.Fragment = "", "", "", ""
	return u, nil
}

func isLoopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	return err == nil && isLoopbackHost(u.Hostname())
}

func clientHostname(origin string) string {
	u, _ := url.Parse(origin)
	return u.Hostname()
}

func originScheme(origin string) string {
	u, _ := url.Parse(origin)
	return u.Scheme
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func randomToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate preview capability: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func validRelativePath(value string) bool {
	if len(value) == 0 || len(value) > 8192 || value[0] != '/' || strings.HasPrefix(value, "//") || strings.ContainsAny(value, "\\\r\n\x00") {
		return false
	}
	u, err := url.Parse(value)
	if err != nil || u.IsAbs() || u.Host != "" || u.Scheme != "" || u.Opaque != "" {
		return false
	}
	decoded, err := url.PathUnescape(u.EscapedPath())
	if err != nil {
		return false
	}
	for _, segment := range strings.Split(decoded, "/") {
		if segment == ".." {
			return false
		}
	}
	return true
}

func validOriginHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if host == "" || len(host) > 253 {
		return false
	}
	for _, r := range host {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-') {
			return false
		}
	}
	return true
}
