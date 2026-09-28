package providers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// fakeProvider stands in for a provider's API. It records the last request it was sent and answers
// with whatever the test set up. It runs on the loopback interface, no real key is ever used, and
// no call leaves the machine.
type fakeProvider struct {
	server *httptest.Server
	mu     sync.Mutex
	last   []byte
	path   string
	header http.Header
	// send is extra headers the fake answers with, such as the rate-limit ones a provider sends.
	send   http.Header
	status int
	body   string
	stream bool
}

// newFakeProvider starts a fake. Its URL is what a test gives an adapter as the base URL.
func newFakeProvider(t *testing.T) *fakeProvider {
	t.Helper()
	f := &fakeProvider{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.last, f.path, f.header = body, r.URL.Path, r.Header.Clone()
		status, resp, stream, send := f.status, f.body, f.stream, f.send
		f.mu.Unlock()
		for name, values := range send {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		if status != 0 {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, resp)
			return
		}
		if stream {
			w.Header().Set("content-type", "text/event-stream")
		} else {
			w.Header().Set("content-type", "application/json")
		}
		_, _ = io.WriteString(w, resp)
	}))
	t.Cleanup(f.server.Close)
	return f
}

// answerJSON answers the next call with a whole non-streamed body.
func (f *fakeProvider) answerJSON(body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.body, f.stream = 0, body, false
}

// answerSSE answers the next call with a stream of server-sent events.
func (f *fakeProvider) answerSSE(frames ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.body, f.stream = 0, joinFrames(frames), true
}

// answerJSONWith answers the next call with a whole body and extra headers, which is how a test
// makes a provider report its rate limits.
func (f *fakeProvider) answerJSONWith(headers map[string]string, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.body, f.stream = 0, body, false
	f.send = http.Header{}
	for name, value := range headers {
		f.send.Set(name, value)
	}
}

// answerStatus answers the next call with an error status and the body that provider sends with it.
func (f *fakeProvider) answerStatus(status int, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.body, f.stream = status, body, false
}

// sent reads the last request body as a map, so a test can check what the adapter really sent.
func (f *fakeProvider) sent(t *testing.T) map[string]any {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.last) == 0 {
		t.Fatal("no request was sent")
	}
	var out map[string]any
	if err := json.Unmarshal(f.last, &out); err != nil {
		t.Fatalf("the request body was not JSON: %v (%s)", err, f.last)
	}
	return out
}

// sentTo is the path the last request went to, and the headers it carried. The path says which
// endpoint an adapter chose, which is how a test proves a provider's own URL was used.
func (f *fakeProvider) sentTo(t *testing.T) (string, http.Header) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.path == "" {
		t.Fatal("no request was sent")
	}
	return f.path, f.header
}

// sentNothing reports whether the fake was never called, which a test asserts about a call Marshal
// must not make at all.
func (f *fakeProvider) sentNothing() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.last) == 0
}

// noRetries turns the SDK's own retrying off, so a test is quick and sees exactly one request.
func noRetries() *int {
	zero := 0
	return &zero
}
