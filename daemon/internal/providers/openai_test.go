package providers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// openAITestClient is a Client for the fake, with the SDK's own retrying turned off.
func openAITestClient(f *fakeProvider, id string) Client {
	return NewOpenAICompatible(id, Config{
		APIKey:  "test-key-not-real",
		BaseURL: f.server.URL,
		Retries: noRetries(),
	})
}

func TestAnOpenAICompatibleWholeAnswerIsRead(t *testing.T) {
	f := newFakeProvider(t)
	f.answerJSON(`{
		"id": "chatcmpl-1", "object": "chat.completion", "model": "gpt-5-mini",
		"choices": [{
			"index": 0, "finish_reason": "tool_calls",
			"message": {
				"role": "assistant",
				"content": "Reading the file.",
				"tool_calls": [{
					"id": "call_1", "type": "function",
					"function": {"name": "read_file", "arguments": "{\"path\":\"a.go\"}"}
				}]
			}
		}],
		"usage": {"prompt_tokens": 12, "completion_tokens": 7, "total_tokens": 19}
	}`)

	reply, err := openAITestClient(f, OpenAIID).Complete(context.Background(), Request{
		Model: "gpt-5-mini", Messages: userSays("read a.go"),
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(reply.Parts) != 2 {
		t.Fatalf("want 2 parts, got %d (%+v)", len(reply.Parts), reply.Parts)
	}
	if reply.Parts[0].Kind != PartText || reply.Parts[0].Text != "Reading the file." {
		t.Errorf("first part = %+v, want the text", reply.Parts[0])
	}
	tool := reply.Parts[1]
	if tool.Kind != PartToolUse || tool.ToolUseID != "call_1" || tool.ToolName != "read_file" {
		t.Errorf("second part = %+v, want the tool call", tool)
	}
	if string(tool.ToolInput) != `{"path":"a.go"}` {
		t.Errorf("tool input = %s, want the JSON the model sent", tool.ToolInput)
	}
	if reply.StopReason != StopToolUse {
		t.Errorf("stop reason = %q, want %q", reply.StopReason, StopToolUse)
	}
	if reply.Usage != (Usage{InputTokens: 12, OutputTokens: 7}) {
		t.Errorf("usage = %+v, want 12 in / 7 out", reply.Usage)
	}
}

func TestWhatIsSentToAnOpenAICompatibleProvider(t *testing.T) {
	f := newFakeProvider(t)
	f.answerJSON(`{"id":"1","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)

	_, err := openAITestClient(f, DeepSeekID).Complete(context.Background(), Request{
		Model:  "deepseek-chat",
		System: "You are a careful engineer.",
		Messages: []Message{
			{Role: RoleUser, Parts: []Part{{Kind: PartText, Text: "fix the bug"}}},
			{Role: RoleAssistant, Parts: []Part{
				{Kind: PartText, Text: "on it"},
				{Kind: PartToolUse, ToolUseID: "call_1", ToolName: "run", ToolInput: json.RawMessage(`{"cmd":"go test"}`)},
			}},
			{Role: RoleUser, Parts: []Part{
				{Kind: PartToolResult, ToolUseID: "call_1", Text: "ok"},
				{Kind: PartText, Text: "and again"},
			}},
		},
		Tools:     []Tool{{Name: "run", Description: "run a command", InputSchema: json.RawMessage(`{"type":"object","properties":{"cmd":{"type":"string"}},"required":["cmd"]}`)}},
		MaxTokens: 4096,
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	path, header := f.sentTo(t)
	if path != "/chat/completions" {
		t.Errorf("path = %q, want the chat-completions endpoint", path)
	}
	if got := header.Get("authorization"); got != "Bearer test-key-not-real" {
		t.Errorf("authorization = %q, want the key that was configured", got)
	}
	sent := f.sent(t)
	if sent["model"] != "deepseek-chat" {
		t.Errorf("model = %v, want deepseek-chat", sent["model"])
	}
	if sent["max_tokens"] != float64(4096) {
		t.Errorf("max_tokens = %v, want 4096", sent["max_tokens"])
	}
	if sent["stream"] != nil {
		t.Errorf("a non-streamed call must not ask to stream, got stream=%v", sent["stream"])
	}
	if sent["reasoning_effort"] != nil {
		t.Errorf("a request that asked for no thinking must not ask for an effort, got %v", sent["reasoning_effort"])
	}
	msgs, ok := sent["messages"].([]any)
	if !ok {
		t.Fatalf("messages = %v, want a list", sent["messages"])
	}
	// The system prompt is the first message: this API has no field of its own for it.
	if len(msgs) != 5 {
		t.Fatalf("messages = %v, want five (system, user, assistant, tool, user)", sent["messages"])
	}
	first := msgs[0].(map[string]any)
	if first["role"] != "system" || first["content"] != "You are a careful engineer." {
		t.Errorf("first message = %v, want the system prompt", first)
	}
	assistant := msgs[2].(map[string]any)
	if assistant["role"] != "assistant" || assistant["content"] != "on it" {
		t.Errorf("assistant message = %v", assistant)
	}
	calls, ok := assistant["tool_calls"].([]any)
	if !ok || len(calls) != 1 {
		t.Fatalf("assistant tool_calls = %v, want one", assistant["tool_calls"])
	}
	call := calls[0].(map[string]any)
	fn := call["function"].(map[string]any)
	if call["id"] != "call_1" || fn["name"] != "run" {
		t.Errorf("tool call = %v, want call_1 to run", call)
	}
	if fn["arguments"] != `{"cmd":"go test"}` {
		t.Errorf("tool arguments = %v, want the JSON that was passed in", fn["arguments"])
	}
	// A tool result is a message of its own, and the text the person added still follows it.
	result := msgs[3].(map[string]any)
	if result["role"] != "tool" || result["tool_call_id"] != "call_1" || result["content"] != "ok" {
		t.Errorf("tool message = %v, want the result of call_1", result)
	}
	last := msgs[4].(map[string]any)
	if last["role"] != "user" || last["content"] != "and again" {
		t.Errorf("last message = %v, want what the person added after the tool ran", last)
	}
	tools, ok := sent["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %v, want one", sent["tools"])
	}
	tool := tools[0].(map[string]any)
	if tool["type"] != "function" {
		t.Errorf("tool type = %v, want function", tool["type"])
	}
	toolFn := tool["function"].(map[string]any)
	if toolFn["name"] != "run" || toolFn["description"] != "run a command" {
		t.Errorf("tool = %v, want run with its description", toolFn)
	}
	params, _ := toolFn["parameters"].(map[string]any)
	if params["type"] != "object" || params["required"] == nil {
		t.Errorf("tool parameters = %v, want the JSON schema that was passed in", toolFn["parameters"])
	}
}

func TestAToolResultWithNothingToSayStillHasContent(t *testing.T) {
	f := newFakeProvider(t)
	f.answerJSON(`{"id":"1","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}]}`)

	_, err := openAITestClient(f, OllamaID).Complete(context.Background(), Request{
		Model: "llama3",
		Messages: []Message{
			{Role: RoleAssistant, Parts: []Part{{Kind: PartToolUse, ToolUseID: "call_1", ToolName: "run", ToolInput: json.RawMessage(`{}`)}}},
			{Role: RoleUser, Parts: []Part{{Kind: PartToolResult, ToolUseID: "call_1", IsError: true}}},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	msgs := f.sent(t)["messages"].([]any)
	result := msgs[1].(map[string]any)
	text, _ := result["content"].(string)
	// The message must not be empty, and a failure must be said in words: this API has nowhere to
	// flag a tool result as a failure.
	if !strings.HasPrefix(text, "the tool failed: ") {
		t.Fatalf("content = %q, want it to say the tool failed", text)
	}
	if len(text) == len("the tool failed: ") {
		t.Errorf("content = %q, want it to say what happened, not only that it failed", text)
	}
}

func TestOpenAICompatibleThinkingBecomesAnEffort(t *testing.T) {
	cases := []struct {
		mode string
		want string
	}{
		{"", ""},
		{"low", "low"},
		{"medium", "medium"},
		{"high", "high"},
		// The family has no word above high, so the most thinking levels both ask for high.
		{"extra-high", "high"},
		{"nonsense", ""},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			params, err := openAIParams(Request{Model: "gpt-5-mini", Thinking: tc.mode, Messages: userSays("hi")})
			if err != nil {
				t.Fatalf("openAIParams: %v", err)
			}
			if got := string(params.ReasoningEffort); got != tc.want {
				t.Errorf("effort = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAnOpenAICompatibleStreamedAnswerIsRead(t *testing.T) {
	f := newFakeProvider(t)
	f.answerSSE(
		`data: {"id":"1","object":"chat.completion.chunk","model":"gpt-5-mini","choices":[{"index":0,"delta":{"role":"assistant","content":"Hel"}}]}`,
		`data: {"id":"1","object":"chat.completion.chunk","model":"gpt-5-mini","choices":[{"index":0,"delta":{"content":"lo"}}]}`,
		`data: {"id":"1","object":"chat.completion.chunk","model":"gpt-5-mini","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{\"path\":"}}]}}]}`,
		`data: {"id":"1","object":"chat.completion.chunk","model":"gpt-5-mini","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"a.go\"}"}}]}}]}`,
		`data: {"id":"1","object":"chat.completion.chunk","model":"gpt-5-mini","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: {"id":"1","object":"chat.completion.chunk","model":"gpt-5-mini","choices":[],"usage":{"prompt_tokens":9,"completion_tokens":11,"total_tokens":20}}`,
		`data: [DONE]`,
	)

	stream, err := openAITestClient(f, OpenRouterID).Stream(context.Background(), Request{
		Model: "gpt-5-mini", Messages: userSays("read a.go"),
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer func() { _ = stream.Close() }()

	got := drain(t, stream)
	want := []Event{
		{Kind: EventText, Text: "Hel"},
		{Kind: EventText, Text: "lo"},
		{Kind: EventToolUse, ToolUseID: "call_1", ToolName: "read_file", ToolInput: json.RawMessage(`{"path":"a.go"}`)},
		{Kind: EventDone, StopReason: StopToolUse, Usage: Usage{InputTokens: 9, OutputTokens: 11}},
	}
	checkEvents(t, got, want)
}

func TestAnOpenAICompatibleStreamedCallAsksForUsage(t *testing.T) {
	f := newFakeProvider(t)
	f.answerSSE(`data: {"id":"1","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":"stop"}]}`, `data: [DONE]`)
	stream, err := openAITestClient(f, LMStudioID).Stream(context.Background(), Request{Model: "local-model", Messages: userSays("hi")})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer func() { _ = stream.Close() }()
	drain(t, stream)

	sent := f.sent(t)
	if sent["stream"] != true {
		t.Errorf("stream = %v, want true", sent["stream"])
	}
	options, ok := sent["stream_options"].(map[string]any)
	if !ok || options["include_usage"] != true {
		t.Errorf("stream_options = %v, want usage to be asked for", sent["stream_options"])
	}
}

func TestAnOpenAICompatibleProviderIsNamedByItsID(t *testing.T) {
	f := newFakeProvider(t)
	for _, id := range []string{OpenAIID, OpenRouterID, DeepSeekID, OllamaID, LMStudioID} {
		if got := openAITestClient(f, id).ID(); got != id {
			t.Errorf("ID() = %q, want %q", got, id)
		}
	}
}

func TestALocalProviderNeedsNoKey(t *testing.T) {
	f := newFakeProvider(t)
	f.answerJSON(`{"id":"1","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}]}`)

	// No key at all: an Ollama or LM Studio server has none to give.
	client := NewOpenAICompatible(OllamaID, Config{BaseURL: f.server.URL, Retries: noRetries()})
	if _, err := client.Complete(context.Background(), Request{Model: "llama3", Messages: userSays("hi")}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	sent := f.sent(t)
	if sent["model"] != "llama3" {
		t.Errorf("model = %v, want llama3", sent["model"])
	}
}

func TestWhatAnOpenAICompatibleProviderRefusesIsMapped(t *testing.T) {
	cases := []struct {
		status int
		want   error
	}{
		{http.StatusUnauthorized, ErrAuth},
		{http.StatusForbidden, ErrAuth},
		{http.StatusNotFound, ErrNoSuchModel},
		{http.StatusTooManyRequests, ErrRateLimited},
		{http.StatusServiceUnavailable, ErrUnavailable},
	}
	body := `{"error":{"message":"no","type":"invalid_request_error","code":"invalid"}}`
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			f := newFakeProvider(t)
			f.answerStatus(tc.status, body)
			_, err := openAITestClient(f, OpenAIID).Complete(context.Background(), Request{Model: "gpt-5-mini", Messages: userSays("hi")})
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want it to wrap %v", err, tc.want)
			}
		})
	}
}

func TestARefusedOpenAICompatibleKeyLooksLikeAnAuthErrorOnAStream(t *testing.T) {
	f := newFakeProvider(t)
	f.answerStatus(http.StatusUnauthorized, `{"error":{"message":"no","type":"invalid_request_error","code":"invalid"}}`)
	stream, err := openAITestClient(f, OpenAIID).Stream(context.Background(), Request{Model: "gpt-5-mini", Messages: userSays("hi")})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer func() { _ = stream.Close() }()
	if _, err := stream.Recv(); !errors.Is(err, ErrAuth) {
		t.Fatalf("Recv error = %v, want it to wrap ErrAuth", err)
	}
}

// drain reads a stream to its end, so a test can look at everything it said.
func drain(t *testing.T, stream Stream) []Event {
	t.Helper()
	var got []Event
	for {
		ev, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return got
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		got = append(got, ev)
	}
}

// checkEvents compares the events a stream said with the ones a test expects, ignoring the fields
// each kind of event does not use.
func checkEvents(t *testing.T, got, want []Event) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d events %+v, want %d %+v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i].Kind != want[i].Kind {
			t.Errorf("event %d kind = %q, want %q", i, got[i].Kind, want[i].Kind)
			continue
		}
		switch want[i].Kind {
		case EventText, EventThinking:
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
