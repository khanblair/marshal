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
//
// client is one client for every probe an httpProber makes, so a preview that is starting does not
// make a new connection pool four times a second. It is a field rather than a package-level client
// so that nothing here is shared state between one Service and another.
type httpProber struct {
	client *http.Client
}

// newHTTPProber returns a prober with its own client, timed out the same way every probe is.
func newHTTPProber() httpProber {
	return httpProber{client: &http.Client{Timeout: probeTimeout}}
}

func (p httpProber) Answers(ctx context.Context, url string) bool {
	if url == "" {
		return false
	}
	callCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(callCtx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	response, err := p.client.Do(request)
	if err != nil {
		return false
	}
	_ = response.Body.Close()
	return true
}
