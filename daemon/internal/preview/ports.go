package preview

// Picking a card's own port, and asking whether its dev server answers yet. Both have a real
// default and a seam, so a test runs two cards at once without a socket and reaches `running`
// without a server.

import (
	"context"
	"net"
	"net/http"
	"time"
)

// socketPicker picks a free port by asking the operating system for one: it binds port 0 on the
// loopback address, reads what it was given, and lets it go. Two cards picking at the same moment
// get different numbers, which is what lets two previews run at once.
type socketPicker struct{}

func (socketPicker) Pick() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		return 0, err
	}
	return port, nil
}

// probeTimeout bounds one question to a dev server. A dev server that does not answer in this long is
// not answering yet, which is not the same as not being there.
const probeTimeout = 3 * time.Second

// httpProber asks a dev server whether it answers. Any answer counts - a page that is still being
// built, or a page that refuses the request, both mean the server is up.
type httpProber struct{}

func (httpProber) Answers(ctx context.Context, url string) bool {
	if url == "" {
		return false
	}
	callCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(callCtx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	response, err := probeClient.Do(request)
	if err != nil {
		return false
	}
	_ = response.Body.Close()
	return true
}

// probeClient is one client for every probe, so a preview that is starting does not make a new
// connection pool four times a second.
var probeClient = &http.Client{Timeout: probeTimeout}
