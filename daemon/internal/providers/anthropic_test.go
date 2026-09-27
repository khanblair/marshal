package providers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// fakeAnthropic stands in for Anthropic's API. It records the last request it was sent and answers
// with whatever the test set up. It runs on the loopback interface, no real key is used, and no
// call ever leaves the machine.
type fakeAnthropic struct {
	server *httptest.Server
	mu     sync.Mutex
	last   []byte
	// send is extra headers the fake answers with, such as the rate-limit ones Anthropic sends.
	send   http.Header
	status int
	body   string
	stream bool
}

func newFakeAnthropic(t *testing.T) *fakeAnthropic {
	t.Helper()
	f := &fakeAnthropic{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.last = body
		status, resp, stream, send := f.status, f.body, f.stream, f.send
		f.mu.Unlock()
		for name, values := range send {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		if status != 0 {
			w.WriteHeader(status)
			io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"the key is not valid"}}`)
			return
		}
		if stream {
			w.Header().Set("content-type", "text/event-stream")
		} else {
			w.Header().Set("content-type", "application/json")
		}
		io.WriteString(w, resp)
	}))
	t.Cleanup(f.server.Close)
	return f
}

// answerJSON answers the next call with a whole non-streamed message body.
func (f *fakeAnthropic) answerJSON(body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.body, f.stream = 0, body, false
}

// answerJSONWith answers the next call with a whole body and extra headers, which is how a test
// makes Anthropic report its rate limits.
func (f *fakeAnthropic) answerJSONWith(headers map[string]string, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.body, f.stream = 0, body, false
	f.send = http.Header{}
	for name, value := range headers {
		f.send.Set(name, value)
	}
}

// answerSSE answers the next call with a stream of server-sent events.
func (f *fakeAnthropic) answerSSE(frames ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.body, f.stream = 0, joinFrames(frames), true
}

// answerStatus answers the next call with an error status and no body.
func (f *fakeAnthropic) answerStatus(status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.body, f.stream = status, "", false
}

// sent reads the last request body as a map, so a test can check what the adapter really sent.
func (f *fakeAnthropic) sent(t *testing.T) map[string]any {
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

// client returns a Client pointed at the fake, with the SDK's own retrying turned off so a test is
// quick and sees exactly one request.
func (f *fakeAnthropic) client() Client {
	noRetries := 0
	return NewAnthropic(Config{
		APIKey:  "test-key-not-real",
		BaseURL: f.server.URL,
		Retries: &noRetries,
	})
}

// joinFrames lays out server-sent event frames the way the SDK's decoder reads them.
func joinFrames(frames []string) string {
	var out string
	for _, fr := range frames {
		out += fr + "\n\n"
	}
	return out
}

func userSays(text string) []Message {
	return []Message{{Role: RoleUser, Parts: []Part{{Kind: PartText, Text: text}}}}
}

func TestAWholeAnswerIsRead(t *testing.T) {
	f := newFakeAnthropic(t)
	f.answerJSON(`{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-sonnet-5",
		"content": [
			{"type": "text", "text": "Reading the file."},
			{"type": "tool_use", "id": "toolu_1", "name": "read_file", "input": {"path": "a.go"}}
		],
		"stop_reason": "tool_use",
		"usage": {"input_tokens": 12, "output_tokens": 7}
	}`)

	reply, err := f.client().Complete(context.Background(), Request{Model: "claude-sonnet-5", Messages: userSays("read a.go")})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(reply.Parts) != 2 {
		t.Fatalf("want 2 parts, got %d (%+v)", len(reply.Parts), reply.Parts)
	}
	if reply.Parts[0].Kind != PartText || reply.Parts[0].Text != "Reading the file." {
		t.Errorf("first part = %+v, want the text block", reply.Parts[0])
	}
	tool := reply.Parts[1]
	if tool.Kind != PartToolUse || tool.ToolUseID != "toolu_1" || tool.ToolName != "read_file" {
		t.Errorf("second part = %+v, want the tool call", tool)
	}
	if string(tool.ToolInput) != `{"path": "a.go"}` && string(tool.ToolInput) != `{"path":"a.go"}` {
		t.Errorf("tool input = %s, want the JSON the model sent", tool.ToolInput)
	}
	if reply.StopReason != StopToolUse {
		t.Errorf("stop reason = %q, want %q", reply.StopReason, StopToolUse)
	}
	if reply.Usage != (Usage{InputTokens: 12, OutputTokens: 7}) {
		t.Errorf("usage = %+v, want 12 in / 7 out", reply.Usage)
	}
}

func TestWhatWasSent(t *testing.T) {
	f := newFakeAnthropic(t)
	f.answerJSON(`{"id":"m","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)

	_, err := f.client().Complete(context.Background(), Request{
		Model:  "claude-sonnet-5",
		System: "You are a careful engineer.",
		Messages: []Message{
			{Role: RoleUser, Parts: []Part{{Kind: PartText, Text: "fix the bug"}}},
			{Role: RoleAssistant, Parts: []Part{
				{Kind: PartText, Text: "on it"},
				{Kind: PartToolUse, ToolUseID: "toolu_1", ToolName: "run", ToolInput: json.RawMessage(`{"cmd":"go test"}`)},
			}},
			{Role: RoleUser, Parts: []Part{{Kind: PartToolResult, ToolUseID: "toolu_1", Text: "ok", IsError: false}}},
		},
		Tools:     []Tool{{Name: "run", Description: "run a command", InputSchema: json.RawMessage(`{"type":"object","properties":{"cmd":{"type":"string"}},"required":["cmd"]}`)}},
		MaxTokens: 4096,
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	sent := f.sent(t)
	if sent["model"] != "claude-sonnet-5" {
		t.Errorf("model = %v, want claude-sonnet-5", sent["model"])
	}
	if sent["max_tokens"] != float64(4096) {
		t.Errorf("max_tokens = %v, want 4096", sent["max_tokens"])
	}
	if sent["stream"] != nil {
		t.Errorf("a non-streamed call must not ask to stream, got stream=%v", sent["stream"])
	}
	if sys, ok := sent["system"].([]any); !ok || len(sys) != 1 {
		t.Fatalf("system = %v, want one text block", sent["system"])
	} else if blk := sys[0].(map[string]any); blk["text"] != "You are a careful engineer." {
		t.Errorf("system text = %v", blk["text"])
	}
	msgs, ok := sent["messages"].([]any)
	if !ok || len(msgs) != 3 {
		t.Fatalf("messages = %v, want three", sent["messages"])
	}
	if role := msgs[2].(map[string]any)["role"]; role != "user" {
		t.Errorf("the tool result message role = %v, want user", role)
	}
	tools, ok := sent["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %v, want one", sent["tools"])
	}
	tool := tools[0].(map[string]any)
	if tool["name"] != "run" || tool["description"] != "run a command" {
		t.Errorf("tool = %v, want run with its description", tool)
	}
	schema, _ := tool["input_schema"].(map[string]any)
	if schema["type"] != "object" || schema["required"] == nil {
		t.Errorf("tool schema = %v, want the JSON schema that was passed in", tool["input_schema"])
	}
}

func TestAStreamedAnswerIsRead(t *testing.T) {
	f := newFakeAnthropic(t)
	f.answerSSE(
		`event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5","content":[],"stop_reason":null,"usage":{"input_tokens":9,"output_tokens":0}}}`,
		`event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hel"}}`,
		`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lo"}}`,
		`event: content_block_stop
data: {"type":"content_block_stop","index":0}`,
		`event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"read_file","input":{}}}`,
		`event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}`,
		`event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"a.go\"}"}}`,
		`event: content_block_stop
data: {"type":"content_block_stop","index":1}`,
		`event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":11}}`,
		`event: message_stop
data: {"type":"message_stop"}`,
	)

	stream, err := f.client().Stream(context.Background(), Request{Model: "claude-sonnet-5", Messages: userSays("read a.go")})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer stream.Close()

	var got []Event
	for {
		ev, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		got = append(got, ev)
	}

	want := []Event{
		{Kind: EventText, Text: "Hel"},
		{Kind: EventText, Text: "lo"},
		{Kind: EventToolUse, ToolUseID: "toolu_1", ToolName: "read_file", ToolInput: json.RawMessage(`{"path":"a.go"}`)},
		{Kind: EventDone, StopReason: StopToolUse, Usage: Usage{InputTokens: 9, OutputTokens: 11}},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d events %+v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i].Kind != want[i].Kind {
			t.Errorf("event %d kind = %q, want %q", i, got[i].Kind, want[i].Kind)
			continue
		}
		switch want[i].Kind {
		case EventText:
			if got[i].Text != want[i].Text {
				t.Errorf("event %d text = %q, want %q", i, got[i].Text, want[i].Text)
			}
		case EventToolUse:
			if got[i].ToolUseID != want[i].ToolUseID || got[i].ToolName != want[i].ToolName {
				t.Errorf("event %d = %+v, want %+v", i, got[i], want[i])
			}
			if string(got[i].ToolInput) != string(want[i].ToolInput) {
				t.Errorf("event %d input = %s, want %s", i, got[i].ToolInput, want[i].ToolInput)
			}
		case EventDone:
			if got[i].StopReason != want[i].StopReason || got[i].Usage != want[i].Usage {
				t.Errorf("event %d = %+v, want %+v", i, got[i], want[i])
			}
		}
	}
}

func TestAStreamedCallAsksToStream(t *testing.T) {
	f := newFakeAnthropic(t)
	f.answerSSE(
		`event: message_start
data: {"type":"message_start","message":{"id":"m","type":"message","role":"assistant","model":"claude-sonnet-5","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`,
		`event: message_stop
data: {"type":"message_stop"}`,
	)
	stream, err := f.client().Stream(context.Background(), Request{Model: "claude-sonnet-5", Messages: userSays("hi")})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer stream.Close()
	for {
		if _, err := stream.Recv(); err != nil {
			break
		}
	}
	if asked := f.sent(t)["stream"]; asked != true {
		t.Errorf("stream = %v, want true", asked)
	}
}

func TestARejectedKeyIsAnAuthError(t *testing.T) {
	f := newFakeAnthropic(t)
	f.answerStatus(http.StatusUnauthorized)
	_, err := f.client().Complete(context.Background(), Request{Model: "claude-sonnet-5", Messages: userSays("hi")})
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("error = %v, want it to wrap ErrAuth", err)
	}
}

func TestAnUnavailableProviderIsReported(t *testing.T) {
	f := newFakeAnthropic(t)
	f.answerStatus(http.StatusServiceUnavailable)
	_, err := f.client().Complete(context.Background(), Request{Model: "claude-sonnet-5", Messages: userSays("hi")})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want it to wrap ErrUnavailable", err)
	}
}

func TestARefusedKeyLooksLikeAnAuthErrorOnAStream(t *testing.T) {
	f := newFakeAnthropic(t)
	f.answerStatus(http.StatusForbidden)
	stream, err := f.client().Stream(context.Background(), Request{Model: "claude-sonnet-5", Messages: userSays("hi")})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer stream.Close()
	_, err = stream.Recv()
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("Recv error = %v, want it to wrap ErrAuth", err)
	}
}

func TestThinkingBecomesABudget(t *testing.T) {
	cases := []struct {
		mode   string
		budget int64
	}{
		{"", 0},
		{"low", 2048},
		{"medium", 8192},
		{"high", 16384},
		{"extra-high", 32768},
		{"nonsense", 0},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			params, err := anthropicParams(Request{Model: "claude-sonnet-5", Thinking: tc.mode, Messages: userSays("hi")})
			if err != nil {
				t.Fatalf("anthropicParams: %v", err)
			}
			if tc.budget == 0 {
				if params.Thinking.OfEnabled != nil {
					t.Errorf("mode %q turned thinking on; it should leave the provider default alone", tc.mode)
				}
				return
			}
			if params.Thinking.OfEnabled == nil {
				t.Fatalf("mode %q did not turn thinking on", tc.mode)
			}
			if got := params.Thinking.OfEnabled.BudgetTokens; got != tc.budget {
				t.Errorf("budget = %d, want %d", got, tc.budget)
			}
			if params.MaxTokens <= tc.budget {
				t.Errorf("max_tokens = %d must stay above the budget %d", params.MaxTokens, tc.budget)
			}
		})
	}
}

func TestChatTitleThinkingIsSentAsABudget(t *testing.T) {
	f := newFakeAnthropic(t)
	f.answerJSON(`{"id":"m","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	_, err := f.client().Complete(context.Background(), Request{Model: "claude-sonnet-5", Thinking: "high", Messages: userSays("hi")})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	thinking, ok := f.sent(t)["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("thinking = %v, want a thinking object", f.sent(t)["thinking"])
	}
	if thinking["budget_tokens"] != float64(16384) {
		t.Errorf("budget_tokens = %v, want 16384", thinking["budget_tokens"])
	}
}
