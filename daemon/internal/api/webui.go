package api

import (
	"io/fs"
	"net/http"
	"strings"
)

// reservedPrefixes are the address spaces the web UI is never served under, even if a caller
// somehow reaches this handler for one of them (every route above already answers its own
// prefix; this is a second, cheap guard against ever serving a page in their place).
var reservedPrefixes = []string{"/v1/", "/hooks/"}

// serveWebUI answers a request from the embedded web app when nothing above matched it, and says
// whether it did. A path that names a real file gets that file; anything else, including a
// client-side route like /board or /cards/{id}, gets index.html, the way a single-page app's
// server always must. It answers only GET and HEAD, and only while the server was given a web UI
// to serve (deps.WebUI, unset unless a build copied the web app in first).
func (s *Server) serveWebUI(w http.ResponseWriter, req *http.Request) bool {
	if s.webUI == nil || (req.Method != http.MethodGet && req.Method != http.MethodHead) {
		return false
	}
	for _, prefix := range reservedPrefixes {
		if strings.HasPrefix(req.URL.Path, prefix) {
			return false
		}
	}
	name := strings.TrimPrefix(req.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	if info, err := fs.Stat(s.webUI, name); err == nil && !info.IsDir() {
		http.ServeFileFS(w, req, s.webUI, name)
		return true
	}
	// No file at that exact path: a client-side route, or a typo. Either way the single page
	// answers, the same 200 a browser needs to run the app and let it show its own not-found
	// state, rather than a server 404 that never runs the app at all.
	http.ServeFileFS(w, req, s.webUI, "index.html")
	return true
}
