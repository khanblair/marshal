package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// reply is one answer of the daemon: the status, the headers, and the body as it arrived.
type reply struct {
	Status int
	Header http.Header
	Body   []byte
}

// do sends a request with the stack's own token. A nil body sends none, a string or []byte is
// sent as it is, and anything else is encoded as JSON. A body is marked as JSON.
func (st *stack) do(method, path string, body any) reply {
	st.t.Helper()
	return st.doWith(st.token, method, path, body)
}

// doWith is do with another token, or with none when the token is empty.
func (st *stack) doWith(token, method, path string, body any) reply {
	st.t.Helper()
	req := st.newRequest(method, path, body)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return st.send(req)
}

func (st *stack) newRequest(method, path string, body any) *http.Request {
	st.t.Helper()
	var reader io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(b)
	case []byte:
		reader = bytes.NewReader(b)
	default:
		data, err := json.Marshal(b)
		if err != nil {
			st.t.Fatalf("encode the request body: %v", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, st.base+path, reader)
	if err != nil {
		st.t.Fatalf("make the request: %v", err)
	}
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func (st *stack) send(req *http.Request) reply {
	st.t.Helper()
	resp, err := st.client.Do(req)
	if err != nil {
		st.t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		st.t.Fatalf("read the answer: %v", err)
	}
	return reply{Status: resp.StatusCode, Header: resp.Header, Body: data}
}

// want fails the test unless the status is the expected one, and shows the body when it is not.
func (r reply) want(t *testing.T, status int) reply {
	t.Helper()
	if r.Status != status {
		t.Fatalf("status = %d, want %d; body: %s", r.Status, status, r.Body)
	}
	return r
}

// decode reads the body into a value of the type. It refuses a field that the type does not have,
// so an answer that carries more than the wire type says fails here.
func decode[T any](t *testing.T, r reply) T {
	t.Helper()
	var out T
	decoder := json.NewDecoder(bytes.NewReader(r.Body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&out); err != nil {
		t.Fatalf("decode %s into %T: %v", r.Body, out, err)
	}
	return out
}

// apiError reads an error answer and checks that it has the one error shape and nothing else.
func (r reply) apiError(t *testing.T, status int, code protocol.ErrorCode) protocol.Error {
	t.Helper()
	r.want(t, status)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(r.Body, &raw); err != nil || len(raw) != 1 || raw["error"] == nil {
		t.Fatalf("an error answer is only {\"error\": ...}, got %s", r.Body)
	}
	got := decode[protocol.ErrorResponse](t, r).Error
	if got.Code != code {
		t.Fatalf("error code = %q, want %q; message: %s", got.Code, code, got.Message)
	}
	if got.Message == "" || strings.ContainsAny(got.Message, "\n") {
		t.Errorf("the message must be one plain sentence, got %q", got.Message)
	}
	if want := "application/json; charset=utf-8"; r.Header.Get("Content-Type") != want {
		t.Errorf("Content-Type = %q, want %q", r.Header.Get("Content-Type"), want)
	}
	return got
}

// sameShape compares the JSON of an answer with a golden file by its shape: the same keys at
// every level, the same kind of value under each, and, for a list, the shape of its first item
// on both sides when both have one. The values differ, because an answer has live ids and times.
// A null on either side stands for any value, because a field may be absent in one sample.
func sameShape(t *testing.T, goldenName string, body []byte) {
	t.Helper()
	path := testutil.TestdataPath(t, "golden", goldenName+".json")
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the golden file %s: %v", goldenName, err)
	}
	var wantValue, gotValue any
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("decode the golden file: %v", err)
	}
	if err := json.Unmarshal(body, &gotValue); err != nil {
		t.Fatalf("decode the answer: %v", err)
	}
	if diffs := shapeDiffs("$", gotValue, wantValue); len(diffs) > 0 {
		t.Errorf("the answer does not have the shape of %s.json:\n  %s\nanswer: %s", goldenName, strings.Join(diffs, "\n  "), body)
	}
}

func shapeDiffs(path string, got, want any) []string {
	if got == nil || want == nil {
		return nil
	}
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s is %T, want an object", path, got)}
		}
		return objectDiffs(path, g, w)
	case []any:
		g, ok := got.([]any)
		if !ok {
			return []string{fmt.Sprintf("%s is %T, want a list", path, got)}
		}
		if len(g) > 0 && len(w) > 0 {
			return shapeDiffs(path+"[0]", g[0], w[0])
		}
		return nil
	}
	if reflect.TypeOf(got) != reflect.TypeOf(want) {
		return []string{fmt.Sprintf("%s is %T, want %T", path, got, want)}
	}
	return nil
}

func objectDiffs(path string, got, want map[string]any) []string {
	var diffs []string
	keys := make([]string, 0, len(want)+len(got))
	seen := map[string]bool{}
	for k := range want {
		keys, seen[k] = append(keys, k), true
	}
	for k := range got {
		if !seen[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		g, inGot := got[k]
		w, inWant := want[k]
		switch {
		case !inGot:
			diffs = append(diffs, fmt.Sprintf("%s.%s is missing", path, k))
		case !inWant:
			diffs = append(diffs, fmt.Sprintf("%s.%s is not in the golden file", path, k))
		default:
			diffs = append(diffs, shapeDiffs(path+"."+k, g, w)...)
		}
	}
	return diffs
}

// wire is a client of the event stream. It follows what the daemon sends: the epoch, the newest
// sequence number, and the events, so a later Hello can ask for what was missed.
type wire struct {
	t       *testing.T
	conn    *websocket.Conn
	epoch   string
	lastSeq uint64
	queue   []protocol.Event
	resyncs []protocol.Resync
}

const streamTimeout = 15 * time.Second

// dial opens the event stream with the token in the subprotocol list, as a browser does, and
// sends the first Hello.
func (st *stack) dial(subscribe ...protocol.Topic) *wire {
	st.t.Helper()
	return st.dialWith(st.token, subscribe...)
}

func (st *stack) dialWith(token string, subscribe ...protocol.Topic) *wire {
	st.t.Helper()
	conn, resp, err := st.dialConn(&websocket.DialOptions{
		Subprotocols: []string{protocol.WebSocketSubprotocol, protocol.BearerSubprotocolPrefix + token},
	})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		st.t.Fatalf("open the event stream: %v", err)
	}
	w := st.track(conn)
	w.hello(subscribe...)
	return w
}

// dialConn opens the event stream with the options as they are, and gives the answer to the
// handshake, so a test can look at a refusal.
func (st *stack) dialConn(opts *websocket.DialOptions) (*websocket.Conn, *http.Response, error) {
	st.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), streamTimeout)
	defer cancel()
	//nolint:bodyclose // the caller closes the body, or there is none after an upgrade
	return websocket.Dial(ctx, "ws"+strings.TrimPrefix(st.base, "http")+"/v1/events", opts)
}

// track makes a client of a connection, and closes the connection when the test ends.
func (st *stack) track(conn *websocket.Conn) *wire {
	st.t.Helper()
	if got := conn.Subprotocol(); got != protocol.WebSocketSubprotocol {
		st.t.Errorf("the daemon answered with the subprotocol %q, want only %q", got, protocol.WebSocketSubprotocol)
	}
	w := &wire{t: st.t, conn: conn}
	st.wires = append(st.wires, w)
	st.t.Cleanup(func() { _ = conn.CloseNow() })
	return w
}

// dialRaw opens the event stream and sends nothing, so a test can send what it likes as the first
// message.
func (st *stack) dialRaw() *wire {
	st.t.Helper()
	conn, resp, err := st.dialConn(&websocket.DialOptions{
		Subprotocols: []string{protocol.WebSocketSubprotocol, protocol.BearerSubprotocolPrefix + st.token},
	})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		st.t.Fatalf("open the event stream: %v", err)
	}
	return st.track(conn)
}

// sendRaw writes one message as it is.
func (w *wire) sendRaw(kind websocket.MessageType, data string) {
	w.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), streamTimeout)
	defer cancel()
	if err := w.conn.Write(ctx, kind, []byte(data)); err != nil {
		w.t.Fatalf("send a message: %v", err)
	}
}

// expectRefusal reads what the daemon does with a message it will not take: an error frame that
// says what was wrong, and then a close with the policy violation status.
func (w *wire) expectRefusal(want protocol.ErrorCode) protocol.Error {
	w.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), streamTimeout)
	defer cancel()
	_, data, err := w.conn.Read(ctx)
	if err != nil {
		w.t.Fatalf("read the error frame: %v", err)
	}
	var frame protocol.ErrorFrame
	if err := json.Unmarshal(data, &frame); err != nil || frame.Type != protocol.FrameTypeError || frame.Error.Code != want {
		w.t.Fatalf("frame = %s, want an error frame with the code %s", data, want)
	}
	_, _, err = w.conn.Read(ctx)
	if got := websocket.CloseStatus(err); got != websocket.StatusPolicyViolation {
		w.t.Errorf("the stream closed with %v (%v), want a policy violation", got, err)
	}
	return frame.Error
}

// hello sends a Hello that follows the topics, from the position this client has reached.
func (w *wire) hello(subscribe ...protocol.Topic) {
	w.t.Helper()
	data, err := json.Marshal(protocol.Hello{
		Type: protocol.FrameTypeHello, Subscribe: subscribe, SinceSeq: w.lastSeq, Epoch: w.epoch,
	})
	if err != nil {
		w.t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), streamTimeout)
	defer cancel()
	if err := w.conn.Write(ctx, websocket.MessageText, data); err != nil {
		w.t.Fatalf("send the hello: %v", err)
	}
}

// read takes one frame and files what is in it.
func (w *wire) read(ctx context.Context) error {
	_, data, err := w.conn.Read(ctx)
	if err != nil {
		return fmt.Errorf("read a frame: %w", err)
	}
	var head struct {
		Type protocol.FrameType `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		w.t.Fatalf("a frame is not JSON: %s", data)
	}
	switch head.Type {
	case protocol.FrameTypeEvents:
		var batch protocol.EventBatch
		if err := json.Unmarshal(data, &batch); err != nil {
			w.t.Fatalf("decode a batch: %v", err)
		}
		w.epoch = batch.Epoch
		for _, ev := range batch.Events {
			w.lastSeq = max(w.lastSeq, ev.Seq)
			w.queue = append(w.queue, ev)
		}
	case protocol.FrameTypeResync:
		var resync protocol.Resync
		if err := json.Unmarshal(data, &resync); err != nil {
			w.t.Fatalf("decode a resync: %v", err)
		}
		w.epoch, w.lastSeq = resync.Epoch, max(w.lastSeq, resync.Seq)
		w.resyncs = append(w.resyncs, resync)
	default:
		w.t.Fatalf("the daemon sent a frame of type %q: %s", head.Type, data)
	}
	return nil
}

// until reads events, in the order they came, until one matches, and returns every event it
// read on the way, the matching one last. Events read earlier and not yet used come first.
func (w *wire) until(match func(protocol.Event) bool) []protocol.Event {
	w.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), streamTimeout)
	defer cancel()
	var seen []protocol.Event
	for {
		for len(w.queue) > 0 {
			ev := w.queue[0]
			w.queue = w.queue[1:]
			seen = append(seen, ev)
			if match(ev) {
				return seen
			}
		}
		if err := w.read(ctx); err != nil {
			w.t.Fatalf("waiting for an event: %v; saw %d events: %s", err, len(seen), describeEvents(seen))
		}
	}
}

func describeEvents(evs []protocol.Event) string {
	parts := make([]string, len(evs))
	for i, ev := range evs {
		parts[i] = fmt.Sprintf("%s@%s", ev.Type, ev.Topic)
	}
	return strings.Join(parts, ", ")
}

// ofType matches an event of a type.
func ofType(typ protocol.EventType) func(protocol.Event) bool {
	return func(ev protocol.Event) bool { return ev.Type == typ }
}

// stateOf matches the session.state_changed event that moves a card's session to a state.
func stateOf(cardID string, state protocol.SessionState) func(protocol.Event) bool {
	return func(ev protocol.Event) bool {
		if ev.Type != protocol.EventTypeSessionStateChanged {
			return false
		}
		var data protocol.SessionStateChangedEventData
		return json.Unmarshal(ev.Data, &data) == nil && data.CardID == cardID && data.State == state
	}
}

// messageTexts pulls the text of every message chunk out of the events, in order.
func messageTexts(events []protocol.Event) []string {
	var out []string
	for _, ev := range events {
		if ev.Type != protocol.EventTypeSessionOutput {
			continue
		}
		var data protocol.SessionOutputEventData
		if json.Unmarshal(ev.Data, &data) == nil && data.Kind == "message" {
			out = append(out, data.Text)
		}
	}
	return out
}

// dataOf decodes the payload of an event.
func dataOf[T any](t *testing.T, ev protocol.Event) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(ev.Data, &out); err != nil {
		t.Fatalf("decode the data of %s: %v", ev.Type, err)
	}
	return out
}
