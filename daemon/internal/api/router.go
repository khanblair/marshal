package api

import (
	"net/http"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// router is the server's routes on the standard ServeMux, with two rules added: an address that
// does not exist and a method that an address does not accept are answered in the one error shape,
// and every route says up front whether it needs a token. Domain routes are added with public,
// protected, or stream, and so get the rules without repeating them.
type router struct {
	s   *Server
	mux *http.ServeMux
}

func newRouter(s *Server) *router {
	return &router{s: s, mux: http.NewServeMux()}
}

// public adds a route that needs no token. Only health is one.
func (r *router) public(pattern string, handler http.HandlerFunc) {
	r.mux.Handle(pattern, r.s.jsonBody(handler))
}

// protected adds a route that needs a valid token. The caller is in the request context, see
// Principal. The body rules apply: a size limit, and JSON only.
func (r *router) protected(pattern string, handler http.HandlerFunc) {
	r.mux.Handle(pattern, r.s.authenticated(r.s.jsonBody(handler)))
}

// uploadCeilingBytes is the most any upload route reads, whatever the route allows itself. A route
// applies its own, smaller limit, and this one only makes sure that a route which forgot to still
// cannot be made to read without end.
const uploadCeilingBytes = 32 << 20

// upload adds a route that needs a valid token and reads its body as it is, such as an image. It
// has the token check and none of the JSON rules: the handler checks the kind and the size of what
// it is sent.
func (r *router) upload(pattern string, handler http.HandlerFunc) {
	r.mux.Handle(pattern, r.s.authenticated(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req.Body = http.MaxBytesReader(w, req.Body, uploadCeilingBytes)
		handler(w, req)
	})))
}

// stream adds a route that takes over its connection, such as the WebSocket. It has no body
// rules, and no token check either: the handler reads the token from the place its protocol
// carries it and calls the same check.
func (r *router) stream(pattern string, handler http.HandlerFunc) {
	r.mux.Handle(pattern, handler)
}

// webhookCeilingBytes is the most any signed-webhook delivery reads, whatever the route allows
// itself. It matches the verifier's own limit, so a delivery that is too large to be believed is
// also too large to be read.
const webhookCeilingBytes = 1 << 20

// signedWebhook adds a route that carries no bearer token and is instead authorized by a signature
// over its raw body, such as a GitHub delivery (B6.1, architecture.md section 8). A webhook is the
// one kind of caller that cannot be given a token - GitHub does not have one - so the route takes
// the body rules off and puts a size limit on the body itself: the handler reads the raw bytes to
// check the signature before it reads them as anything else.
func (r *router) signedWebhook(pattern string, handler http.HandlerFunc) {
	r.mux.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req.Body = http.MaxBytesReader(w, req.Body, webhookCeilingBytes)
		handler(w, req)
	}))
}

// ServeHTTP sends a request to its route, or answers in the error shape when there is none.
func (r *router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	handler, pattern := r.mux.Handler(req)
	if pattern != "" {
		// Serve through the mux and not through handler, so the path values are set.
		r.mux.ServeHTTP(w, req)
		return
	}
	// No route matches. Before answering not_found, give the embedded web app a chance: it
	// serves any GET or HEAD that is not under /v1 or /hooks (webui.go), so the app's own
	// client-side routes work without a route of their own here.
	if r.s.serveWebUI(w, req) {
		return
	}
	// The standard library says whether the address exists for another method, but in plain
	// text, so it is asked into a writer that keeps only its status and its Allow header.
	probe := &probeWriter{header: http.Header{}}
	handler.ServeHTTP(probe, req)
	if probe.status == http.StatusMethodNotAllowed {
		allow := probe.header.Get("Allow")
		w.Header().Set("Allow", allow)
		r.s.writeError(w, protocol.MethodNotAllowed(strings.Split(allow, ", ")...))
		return
	}
	r.s.writeError(w, protocol.NewError(protocol.ErrorCodeNotFound,
		"Marshal has nothing at that address. Check the address and try again."))
}

// probeWriter catches the status and headers of the standard library's own 404 and 405 answers
// and drops their body.
type probeWriter struct {
	header http.Header
	status int
}

func (p *probeWriter) Header() http.Header         { return p.header }
func (p *probeWriter) Write(b []byte) (int, error) { return len(b), nil }
func (p *probeWriter) WriteHeader(status int)      { p.status = status }
