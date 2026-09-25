package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/khanblair/marshal/daemon/internal/api"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// upgrade makes the request of a WebSocket client: the upgrade headers, the subprotocols it
// offers, and the origin a browser page would send. It is sent as a plain request, so a refusal
// can be read whole.
func (st *stack) upgrade(origin string, upgrade bool, protocols ...string) *http.Request {
	st.t.Helper()
	req := st.newRequest(http.MethodGet, "/v1/events", nil)
	if upgrade {
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Sec-WebSocket-Version", "13")
		req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	}
	if len(protocols) > 0 {
		req.Header.Set("Sec-WebSocket-Protocol", strings.Join(protocols, ", "))
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	return req
}

const notALiveConnection = "This address is for a live connection. Open it as a WebSocket and offer the marshal.v1 protocol."

func TestTheEventStreamRefusesBeforeUpgrading(t *testing.T) {
	st := newStack(t)
	bearer := protocol.BearerSubprotocolPrefix + st.token
	tests := []struct {
		name      string
		origin    string
		upgrade   bool
		protocols []string
		status    int
		code      protocol.ErrorCode
		message   string
	}{
		{"a page from another site", "http://evil.example", true, []string{"marshal.v1", bearer},
			http.StatusForbidden, protocol.ErrorCodeForbidden,
			"Marshal only connects to its own app. Open Marshal from the desktop app or from its own address."},
		{"an origin that cannot be read", "http://[::1", true, []string{"marshal.v1", bearer},
			http.StatusForbidden, protocol.ErrorCodeForbidden, ""},
		{"no token", "", true, []string{"marshal.v1"}, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized, unauthorizedMsg},
		{"a wrong token", "", true, []string{"marshal.v1", "bearer.wrong"}, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized, unauthorizedMsg},
		{"no marshal.v1 offered", "", true, []string{bearer}, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument, notALiveConnection},
		{"a plain request", "", false, []string{"marshal.v1", bearer}, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument, notALiveConnection},
		{"the dev server's page passes the origin check", "http://localhost:5173", false, []string{"marshal.v1", bearer},
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument, notALiveConnection},
		{"a page on the daemon's own address passes it too", "http://127.0.0.1:9", false, []string{"marshal.v1", bearer},
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument, notALiveConnection},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := st.upgrade(tc.origin, tc.upgrade, tc.protocols...)
			got := st.send(req).apiError(t, tc.status, tc.code)
			if tc.message != "" && got.Message != tc.message {
				t.Errorf("message = %q\nwant      %q", got.Message, tc.message)
			}
		})
	}
	// The token is never sent back in the answer, and never logged.
	st.mustNotLog(st.token)
}

// The token can also come in the Authorization header, for a client that can set one.
func TestTheEventStreamTakesTheTokenFromTheAuthorizationHeader(t *testing.T) {
	st := newStack(t)
	conn, resp, err := st.dialConn(&websocket.DialOptions{
		Subprotocols: []string{protocol.WebSocketSubprotocol},
		HTTPHeader:   http.Header{"Authorization": {"Bearer " + st.token}},
	})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("open the event stream: %v", err)
	}
	w := st.track(conn)
	w.hello(protocol.HomeTopic)
	st.addProject("small-repo")
	w.until(ofType(protocol.EventTypeProjectCreated))
}

// A normal daemon accepts the desktop app's web view and nothing else: not a page from the dev
// server, which only a dev daemon accepts.
func TestANormalDaemonOnlyAcceptsTheDesktopOrigin(t *testing.T) {
	st := newStack(t, normalDaemon())
	bearer := protocol.BearerSubprotocolPrefix + st.token
	tests := []struct {
		origin  string
		allowed bool
	}{
		{"http://localhost:5173", false},
		{"http://evil.example", false},
		{"tauri://localhost", true},
		{"http://tauri.localhost", true},
		{"", true},
	}
	for _, tc := range tests {
		t.Run("origin "+tc.origin, func(t *testing.T) {
			got := st.send(st.upgrade(tc.origin, false, "marshal.v1", bearer))
			if tc.allowed {
				got.apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument) // past the origin check
				return
			}
			got.apiError(t, http.StatusForbidden, protocol.ErrorCodeForbidden)
		})
	}
	// An origin that passes the check also gets through the WebSocket library's own check.
	conn, resp, err := st.dialConn(&websocket.DialOptions{
		Subprotocols: []string{protocol.WebSocketSubprotocol, bearer},
		HTTPHeader:   http.Header{"Origin": {"tauri://localhost"}},
	})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("the desktop app's origin was refused at the upgrade: %v", err)
	}
	st.track(conn)
}

func TestTheEventStreamRefusesWhatItCannotRead(t *testing.T) {
	st := newStack(t, withLimits(api.Limits{MaxHelloTopics: 2}))
	tests := []struct {
		name    string
		kind    websocket.MessageType
		data    string
		message string
	}{
		{"text that is not JSON", websocket.MessageText, "{nope", "That message is not valid JSON. Send a hello message as JSON."},
		{"a message that is not a hello", websocket.MessageText, `{"type":"events"}`,
			"Marshal expected a hello message. Send a message with the type hello."},
		{"a binary message", websocket.MessageBinary, "hello",
			"Marshal reads text messages on this connection. Send the hello as JSON text."},
		{"a topic that does not exist", websocket.MessageText, `{"type":"hello","subscribe":["project:Bad_ID"]}`,
			"That topic is not one Marshal knows. Check the topic and try again."},
		{"more topics than allowed", websocket.MessageText, `{"type":"hello","subscribe":["home","project:one","project:two"]}`,
			"That hello follows more than 2 topics. Follow fewer topics."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := st.dialRaw()
			w.sendRaw(tc.kind, tc.data)
			if got := w.expectRefusal(protocol.ErrorCodeInvalidArgument); got.Message != tc.message {
				t.Errorf("message = %q\nwant      %q", got.Message, tc.message)
			}
		})
	}
	t.Run("a later message that is not a hello", func(t *testing.T) {
		w := st.dial(protocol.HomeTopic)
		w.sendRaw(websocket.MessageText, `{"type":"goodbye"}`)
		for {
			// The first frame of a new connection is the resync. The refusal follows it.
			_, data, err := w.conn.Read(context.Background())
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if strings.Contains(string(data), `"type":"error"`) {
				break
			}
		}
	})
}

// A client that does not say hello in time is told why and closed.
func TestAClientThatDoesNotSayHelloIsClosed(t *testing.T) {
	st := newStack(t, withLimits(api.Limits{HelloTimeout: 100 * time.Millisecond}))
	w := st.dialRaw()
	ctx, cancel := context.WithTimeout(context.Background(), streamTimeout)
	defer cancel()
	_, _, err := w.conn.Read(ctx)
	var closed websocket.CloseError
	if got := websocket.CloseStatus(err); got != websocket.StatusPolicyViolation {
		t.Fatalf("the stream ended with %v (%v), want a policy violation", got, err)
	}
	if !strings.Contains(err.Error(), "Send a hello message first.") {
		t.Errorf("the close said %v (%T), want it to say to send a hello", err, closed)
	}
}

func TestTheEventStreamAllowsOnlyTheConfiguredNumberOfConnections(t *testing.T) {
	st := newStack(t, withLimits(api.Limits{MaxConnections: 1}))
	st.dial(protocol.HomeTopic)
	got := st.send(st.upgrade("", true, "marshal.v1", protocol.BearerSubprotocolPrefix+st.token)).
		apiError(t, http.StatusServiceUnavailable, protocol.ErrorCodeUnavailable)
	if got.Message != "Marshal has too many live connections open. Close one and try again." {
		t.Errorf("message = %q", got.Message)
	}
}

// A client that has its epoch and its last number is sent what it missed. One that is ahead of the
// daemon, or has an epoch the daemon does not know, is told to reload.
func TestReplayAndResync(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Missed while away")

	t.Run("what was missed is replayed on the same epoch", func(t *testing.T) {
		w := st.dialRaw()
		w.epoch = "test-epoch"
		w.hello(protocol.HomeTopic, protocol.ProjectTopic(project.ID))
		seen := w.until(ofType(protocol.EventTypeCardCreated))
		if len(seen) != 2 || seen[0].Type != protocol.EventTypeProjectCreated || len(w.resyncs) != 0 {
			t.Errorf("saw %s and %d resyncs, want project.created and card.created with no resync", describeEvents(seen), len(w.resyncs))
		}
		if got := dataOf[protocol.CardEventData](t, seen[1]).Card.ID; got != card.ID {
			t.Errorf("card.created is about %s, want %s", got, card.ID)
		}
	})
	t.Run("a position ahead of the daemon is a resync", func(t *testing.T) {
		w := st.dialRaw()
		w.epoch, w.lastSeq = "test-epoch", 9999
		w.hello(protocol.HomeTopic)
		ctx, cancel := context.WithTimeout(context.Background(), streamTimeout)
		defer cancel()
		if err := w.read(ctx); err != nil {
			t.Fatal(err)
		}
		if len(w.resyncs) != 1 || w.resyncs[0].Reason != protocol.ResyncReasonUnknownPosition {
			t.Errorf("resyncs = %+v, want unknown-position", w.resyncs)
		}
	})
}

// A later Hello with no epoch only changes the topics, for events from then on.
func TestALaterHelloChangesTheTopics(t *testing.T) {
	st := newStack(t)
	project, repo := st.addProject("small-repo")
	w := st.dialRaw()
	w.hello(protocol.ProjectTopic(project.ID))
	st.addCard(project.ID, "Seen at first")
	w.until(ofType(protocol.EventTypeCardCreated))

	w.sendRaw(websocket.MessageText, `{"type":"hello","subscribe":["home"],"sinceSeq":0,"epoch":""}`)
	// A home event proves the change was applied, because home was not followed before it.
	other := st.addProjectAt(cloneOf(t, st, repo))
	seen := w.until(func(ev protocol.Event) bool {
		return ev.Type == protocol.EventTypeProjectCreated && dataOf[protocol.ProjectEventData](t, ev).Project.ID == other.ID
	})
	st.addCard(project.ID, "No longer followed")
	st.do(http.MethodPatch, "/v1/projects/"+other.ID, `{"name":"Renamed"}`).want(t, http.StatusOK)
	seen = append(seen, w.until(ofType(protocol.EventTypeProjectUpdated))...)
	for _, ev := range seen {
		if ev.Type == protocol.EventTypeCardCreated {
			t.Errorf("a card event arrived for a topic that is no longer followed: %s", describeEvents(seen))
		}
	}
}

// cloneOf makes a second copy of a repository, as another project's folder.
func cloneOf(t *testing.T, st *stack, repo string) string {
	t.Helper()
	dest := t.TempDir() + "/second"
	if _, err := st.git.Run(context.Background(), t.TempDir(), "clone", "--quiet", repo, dest); err != nil {
		t.Fatalf("clone the repository: %v", err)
	}
	return dest
}

// Events go out in frames no bigger than the limits, in order, and none is lost.
func TestEventsAreSentInOrderInSmallFrames(t *testing.T) {
	for _, limits := range []api.Limits{
		{MaxBatchEvents: 2, FlushInterval: 5 * time.Millisecond},
		{MaxBatchBytes: 1},
	} {
		st := newStack(t, withLimits(limits))
		project, _ := st.addProject("small-repo")
		w := st.dialRaw()
		w.hello(protocol.ProjectTopic(project.ID))
		const cards = 7
		for i := range cards {
			st.addCard(project.ID, "Card "+string(rune('a'+i)))
		}
		var last uint64
		count := 0
		w.until(func(ev protocol.Event) bool {
			if ev.Seq <= last {
				t.Errorf("event %d came after %d", ev.Seq, last)
			}
			last, count = ev.Seq, count+1
			return count == cards
		})
	}
}

// Pings that the client answers keep a connection open, and a client that never answers is let go.
func TestPingsKeepAConnectionOpenAndCloseADeadOne(t *testing.T) {
	t.Run("a client that answers", func(t *testing.T) {
		st := newStack(t, withLimits(api.Limits{PingInterval: 20 * time.Millisecond, PongTimeout: 2 * time.Second}))
		w := st.dial(protocol.HomeTopic)
		time.Sleep(150 * time.Millisecond) // several pings go by, and the reader answers them below
		st.addProject("small-repo")
		w.until(ofType(protocol.EventTypeProjectCreated))
	})
	t.Run("a client that does not", func(t *testing.T) {
		st := newStack(t, withLimits(api.Limits{PingInterval: 20 * time.Millisecond, PongTimeout: 50 * time.Millisecond}))
		w := st.dial(protocol.HomeTopic)
		deadline := time.Now().Add(5 * time.Second)
		for !strings.Contains(st.logText(), "did not answer a ping") {
			if time.Now().After(deadline) {
				t.Fatalf("the daemon never let go of a client that does not answer pings:\n%s", st.logText())
			}
			time.Sleep(10 * time.Millisecond)
		}
		ctx, cancel := context.WithTimeout(context.Background(), streamTimeout)
		defer cancel()
		for {
			if _, _, err := w.conn.Read(ctx); err != nil {
				break
			}
		}
	})
}

// When the daemon stops, every open stream is closed with "going away", and the daemon does not
// have to wait for a client that stopped listening.
func TestStoppingTheServerClosesStreamsWithGoingAway(t *testing.T) {
	st := newStack(t)
	w := st.dial(protocol.HomeTopic)
	closed := make(chan error, 1)
	go func() {
		for {
			if _, _, err := w.conn.Read(context.Background()); err != nil {
				closed <- err
				return
			}
		}
	}()
	began := time.Now()
	st.stopServer()
	err := <-closed
	if got := websocket.CloseStatus(err); got != websocket.StatusGoingAway {
		t.Errorf("the stream ended with %v (%v), want going away", got, err)
	}
	if !strings.Contains(err.Error(), "Marshal is shutting down.") {
		t.Errorf("the close said %v, want it to say Marshal is shutting down", err)
	}
	if took := time.Since(began); took > 2*time.Second {
		t.Errorf("stopping took %v with one well-behaved client", took)
	}
}

// A shutdown must not wait for a client that has stopped reading. Closing a WebSocket is a
// handshake, and a client that never reads never answers it, so the daemon has to cut the raw
// connection once the close window passes. `st.stopServer` fails this test itself when `Serve`
// returns an error, which is what the old `CloseNow`-only cut produced ("some event streams did
// not close in time and were cut").
func TestStoppingTheServerCutsAClientThatStoppedReading(t *testing.T) {
	st := newStack(t)
	// The Hello goes out, and then this client never reads again.
	st.dial(protocol.HomeTopic)
	began := time.Now()
	st.stopServer()
	took := time.Since(began)
	if took > 3*time.Second {
		t.Errorf("stopping took %v with a client that stopped reading, want about the 2 second close window", took)
	}
}
