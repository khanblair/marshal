package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// geminiTestClient is a Client for the fake. A Gemini client cannot be built without a key, so even
// a test against a fake uses a made-up one.
func geminiTestClient(t *testing.T, f *fakeProvider) Client {
	t.Helper()
	client, err := NewGemini(context.Background(), Config{
		APIKey:  "test-key-not-real",
		BaseURL: f.server.URL,
		Retries: noRetries(),
	})
	if err != nil {
		t.Fatalf("NewGemini: %v", err)
	}
	return client
}

func TestAGeminiWholeAnswerIsRead(t *testing.T) {
	f := newFakeProvider(t)
	f.answerJSON(`{
		"candidates": [{
			"index": 0,
			"finishReason": "STOP",
			"content": {"role": "model", "parts": [
				{"text": "Reading the file."},
				{"functionCall": {"name": "read_file", "id": "call_1", "args": {"path": "a.go"}}}
			]}
		}],
		"usageMetadata": {"promptTokenCount": 12, "candidatesTokenCount": 7, "totalTokenCount": 19},
		"modelVersion": "gemini-3-pro"
	}`)

	reply, err := geminiTestClient(t, f).Complete(context.Background(), Request{
		Model: "gemini-3-pro", Messages: userSays("read a.go"),
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
		t.Errorf("tool input = %s, want the arguments as JSON", tool.ToolInput)
	}
	// Gemini ends a turn that asked for a tool with its ordinary stop reason, so the answer says
	// what the turn actually did.
	if reply.StopReason != StopToolUse {
		t.Errorf("stop reason = %q, want %q", reply.StopReason, StopToolUse)
	}
	if reply.Usage != (Usage{InputTokens: 12, OutputTokens: 7}) {
		t.Errorf("usage = %+v, want 12 in / 7 out", reply.Usage)
	}
}

func TestAGeminiAnswerThatOnlyTalksEndsTheTurn(t *testing.T) {
	f := newFakeProvider(t)
	f.answerJSON(`{"candidates":[{"index":0,"finishReason":"STOP","content":{"role":"model","parts":[{"text":"All done."}]}}]}`)

	reply, err := geminiTestClient(t, f).Complete(context.Background(), Request{Model: "gemini-3-pro", Messages: userSays("hi")})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if reply.StopReason != StopEndTurn {
		t.Errorf("stop reason = %q, want %q", reply.StopReason, StopEndTurn)
	}
}

func TestAGeminiRefusalIsReportedAsOne(t *testing.T) {
	f := newFakeProvider(t)
	// No candidates at all is the provider declining to answer, not a failure of Marshal's.
	f.answerJSON(`{"promptFeedback":{"blockReason":"SAFETY"}}`)

	reply, err := geminiTestClient(t, f).Complete(context.Background(), Request{Model: "gemini-3-pro", Messages: userSays("hi")})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if reply.StopReason != StopRefusal {
		t.Errorf("stop reason = %q, want %q", reply.StopReason, StopRefusal)
	}
}

func TestWhatIsSentToGemini(t *testing.T) {
	f := newFakeProvider(t)
	f.answerJSON(`{"candidates":[{"index":0,"finishReason":"STOP","content":{"role":"model","parts":[{"text":"ok"}]}}]}`)

	_, err := geminiTestClient(t, f).Complete(context.Background(), Request{
		Model:  "gemini-3-pro",
		System: "You are a careful engineer.",
		Messages: []Message{
			{Role: RoleUser, Parts: []Part{{Kind: PartText, Text: "fix the bug"}}},
			{Role: RoleAssistant, Parts: []Part{
				{Kind: PartText, Text: "on it"},
				{Kind: PartToolUse, ToolUseID: "call_1", ToolName: "run", ToolInput: json.RawMessage(`{"cmd":"go test"}`)},
			}},
			// The result does not name its tool itself: Gemini's function responses carry the
			// name, so the adapter reads it off the call it answers.
			{Role: RoleUser, Parts: []Part{{Kind: PartToolResult, ToolUseID: "call_1", Text: "ok"}}},
		},
		Tools:     []Tool{{Name: "run", Description: "run a command", InputSchema: json.RawMessage(`{"type":"object","properties":{"cmd":{"type":"string"}},"required":["cmd"]}`)}},
		MaxTokens: 4096,
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	path, header := f.sentTo(t)
	if path != "/v1beta/models/gemini-3-pro:generateContent" {
		t.Errorf("path = %q, want the Gemini generateContent endpoint for the model asked for", path)
	}
	if got := header.Get("x-goog-api-key"); got != "test-key-not-real" {
		t.Errorf("x-goog-api-key = %q, want the key that was configured", got)
	}
	sent := f.sent(t)
	// The SDK puts the generation knobs in a "generationConfig" object and the prompt and
	// tools at the top level, so maxOutputTokens and thinkingConfig are one level down.
	config, _ := sent["generationConfig"].(map[string]any)
	if config == nil {
		t.Fatalf("generationConfig = %v, want the token limit", sent["generationConfig"])
	}
	if config["maxOutputTokens"] != float64(4096) {
		t.Errorf("maxOutputTokens = %v, want 4096", config["maxOutputTokens"])
	}
	if config["thinkingConfig"] != nil {
		t.Errorf("a request that asked for no thinking must not ask for one, got %v", config["thinkingConfig"])
	}
	system, ok := sent["systemInstruction"].(map[string]any)
	if !ok {
		t.Fatalf("systemInstruction = %v, want the system prompt", sent["systemInstruction"])
	}
	parts, _ := system["parts"].([]any)
	if len(parts) != 1 || parts[0].(map[string]any)["text"] != "You are a careful engineer." {
		t.Errorf("systemInstruction = %v, want the system prompt", system)
	}
	contents, ok := sent["contents"].([]any)
	if !ok || len(contents) != 3 {
		t.Fatalf("contents = %v, want three turns", sent["contents"])
	}
	if role := contents[0].(map[string]any)["role"]; role != "user" {
		t.Errorf("first role = %v, want user", role)
	}
	if role := contents[1].(map[string]any)["role"]; role != "model" {
		t.Errorf("assistant role = %v, want model", role)
	}
	assistantParts := contents[1].(map[string]any)["parts"].([]any)
	if len(assistantParts) != 2 {
		t.Fatalf("assistant parts = %v, want text and a function call", assistantParts)
	}
	call, _ := assistantParts[1].(map[string]any)["functionCall"].(map[string]any)
	if call["name"] != "run" || call["id"] != "call_1" {
		t.Errorf("functionCall = %v, want the call that was passed in", call)
	}
	if args, _ := call["args"].(map[string]any); args["cmd"] != "go test" {
		t.Errorf("functionCall args = %v, want the arguments that were passed in", call["args"])
	}
	resultParts := contents[2].(map[string]any)["parts"].([]any)
	response, _ := resultParts[0].(map[string]any)["functionResponse"].(map[string]any)
	if response["name"] != "run" {
		t.Errorf("functionResponse name = %v, want run, read from the call it answers", response["name"])
	}
	if output, _ := response["response"].(map[string]any); output["output"] != "ok" {
		t.Errorf("functionResponse = %v, want what the tool printed", response)
	}
	tools, ok := sent["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %v, want one", sent["tools"])
	}
	declarations, _ := tools[0].(map[string]any)["functionDeclarations"].([]any)
	if len(declarations) != 1 {
		t.Fatalf("functionDeclarations = %v, want one", tools[0])
	}
	declaration := declarations[0].(map[string]any)
	if declaration["name"] != "run" || declaration["description"] != "run a command" {
		t.Errorf("declaration = %v, want run with its description", declaration)
	}
	schema, _ := declaration["parametersJsonSchema"].(map[string]any)
	if schema["type"] != "object" || schema["required"] == nil {
		t.Errorf("parametersJsonSchema = %v, want the JSON schema that was passed in", declaration["parametersJsonSchema"])
	}
}

func TestGeminiThinkingBecomesALevel(t *testing.T) {
	cases := []struct {
		mode string
		want string
	}{
		{"", ""},
		{"low", "LOW"},
		{"medium", "MEDIUM"},
		{"high", "HIGH"},
		// The API has no level above high, so the two most thinking levels both ask for high.
		{"extra-high", "HIGH"},
		{"nonsense", ""},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			config, _, err := geminiParams(Request{Model: "gemini-3-pro", Thinking: tc.mode, Messages: userSays("hi")})
			if err != nil {
				t.Fatalf("geminiParams: %v", err)
			}
			if tc.want == "" {
				if config.ThinkingConfig != nil {
					t.Errorf("mode %q asked for thinking; it should leave the provider default alone", tc.mode)
				}
				return
			}
			if config.ThinkingConfig == nil {
				t.Fatalf("mode %q did not ask for thinking", tc.mode)
			}
			if got := string(config.ThinkingConfig.ThinkingLevel); got != tc.want {
				t.Errorf("level = %q, want %q", got, tc.want)
			}
			// The thoughts are asked for, so the app has something to show while the model works.
			if !config.ThinkingConfig.IncludeThoughts {
				t.Error("the thoughts were not asked for")
			}
		})
	}
}

func TestAGeminiStreamedAnswerIsRead(t *testing.T) {
	f := newFakeProvider(t)
	f.answerSSE(
		`data: {"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"Hel"}]}}]}`,
		`data: {"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"lo"}]}}]}`,
		`data: {"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"thinking","thought":true}]}}]}`,
		`data: {"candidates":[{"index":0,"content":{"role":"model","parts":[{"functionCall":{"name":"read_file","id":"call_1","args":{"path":"a.go"}}}]}}]}`,
		`data: {"candidates":[{"index":0,"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":9,"candidatesTokenCount":11,"thoughtsTokenCount":4}}`,
	)

	stream, err := geminiTestClient(t, f).Stream(context.Background(), Request{Model: "gemini-3-pro", Messages: userSays("read a.go")})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer func() { _ = stream.Close() }()

	got := drain(t, stream)
	want := []Event{
		{Kind: EventText, Text: "Hel"},
		{Kind: EventText, Text: "lo"},
		{Kind: EventThinking, Text: "thinking"},
		{Kind: EventToolUse, ToolUseID: "call_1", ToolName: "read_file", ToolInput: json.RawMessage(`{"path":"a.go"}`)},
		{Kind: EventDone, StopReason: StopToolUse, Usage: Usage{InputTokens: 9, OutputTokens: 15}},
	}
	checkEvents(t, got, want)

	path, _ := f.sentTo(t)
	if !strings.HasSuffix(path, ":streamGenerateContent") {
		t.Errorf("path = %q, want the streaming endpoint", path)
	}
}

func TestAGeminiThoughtSignatureIsSentBack(t *testing.T) {
	f := newFakeProvider(t)
	f.answerJSON(`{"candidates":[{"index":0,"finishReason":"STOP","content":{"role":"model","parts":[{"text":"ok"}]}}]}`)

	signature := base64.StdEncoding.EncodeToString([]byte("a signature from the model"))
	_, err := geminiTestClient(t, f).Complete(context.Background(), Request{
		Model: "gemini-3-pro",
		Messages: []Message{
			{Role: RoleAssistant, Parts: []Part{{Kind: PartThinking, Text: "hmm", Signature: signature}}},
			{Role: RoleUser, Parts: []Part{{Kind: PartText, Text: "go on"}}},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	contents := f.sent(t)["contents"].([]any)
	parts := contents[0].(map[string]any)["parts"].([]any)
	part := parts[0].(map[string]any)
	if part["thought"] != true {
		t.Errorf("part = %v, want it to be marked as a thought", part)
	}
	// The signature goes back with the part it belongs to, which is what the API asks for. Gemini's
	// signature is bytes, and base64 is how bytes travel in the one string field Marshal has.
	if part["thoughtSignature"] != signature {
		t.Errorf("thoughtSignature = %v, want %q", part["thoughtSignature"], signature)
	}
}

func TestAGeminiToolResultThatFailedSaysSo(t *testing.T) {
	f := newFakeProvider(t)
	f.answerJSON(`{"candidates":[{"index":0,"finishReason":"STOP","content":{"role":"model","parts":[{"text":"ok"}]}}]}`)

	_, err := geminiTestClient(t, f).Complete(context.Background(), Request{
		Model: "gemini-3-pro",
		Messages: []Message{
			{Role: RoleUser, Parts: []Part{{Kind: PartToolResult, ToolName: "run", ToolUseID: "call_1", Text: "it broke", IsError: true}}},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	contents := f.sent(t)["contents"].([]any)
	parts := contents[0].(map[string]any)["parts"].([]any)
	response, _ := parts[0].(map[string]any)["functionResponse"].(map[string]any)
	if response["name"] != "run" {
		t.Errorf("functionResponse name = %v, want run", response["name"])
	}
	result, _ := response["response"].(map[string]any)
	if result["error"] != "it broke" {
		t.Errorf("functionResponse = %v, want the failure to be named as one", response)
	}
}

func TestAGeminiKeyThatIsNotAcceptedIsAnAuthError(t *testing.T) {
	f := newFakeProvider(t)
	f.answerStatus(http.StatusUnauthorized, `{"error":{"code":401,"message":"API key not valid","status":"UNAUTHENTICATED"}}`)

	_, err := geminiTestClient(t, f).Complete(context.Background(), Request{Model: "gemini-3-pro", Messages: userSays("hi")})
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("error = %v, want it to wrap ErrAuth", err)
	}
}

func TestWhatGeminiRefusesIsMapped(t *testing.T) {
	cases := []struct {
		status int
		code   int
		want   error
	}{
		{http.StatusUnauthorized, http.StatusUnauthorized, ErrAuth},
		{http.StatusForbidden, http.StatusForbidden, ErrAuth},
		{http.StatusNotFound, http.StatusNotFound, ErrNoSuchModel},
		{http.StatusTooManyRequests, http.StatusTooManyRequests, ErrRateLimited},
		{http.StatusServiceUnavailable, http.StatusServiceUnavailable, ErrUnavailable},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			f := newFakeProvider(t)
			f.answerStatus(tc.status, `{"error":{"code":`+strconv.Itoa(tc.code)+`,"message":"no","status":"FAILED_PRECONDITION"}}`)
			_, err := geminiTestClient(t, f).Complete(context.Background(), Request{Model: "gemini-3-pro", Messages: userSays("hi")})
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want it to wrap %v", err, tc.want)
			}
		})
	}
}

func TestAGeminiStreamRefusalIsAnAuthError(t *testing.T) {
	f := newFakeProvider(t)
	f.answerStatus(http.StatusForbidden, `{"error":{"code":403,"message":"no","status":"PERMISSION_DENIED"}}`)
	stream, err := geminiTestClient(t, f).Stream(context.Background(), Request{Model: "gemini-3-pro", Messages: userSays("hi")})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer func() { _ = stream.Close() }()
	if _, err := stream.Recv(); !errors.Is(err, ErrAuth) {
		t.Fatalf("Recv error = %v, want it to wrap ErrAuth", err)
	}
}

func TestAGeminiProviderWithoutAKeyIsRefusedAtSetup(t *testing.T) {
	f := newFakeProvider(t)
	_, err := NewGemini(context.Background(), Config{BaseURL: f.server.URL})
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("error = %v, want it to wrap ErrAuth", err)
	}
}

func TestAGeminiStreamIsAbandonedWithoutLeavingAnythingBehind(t *testing.T) {
	f := newFakeProvider(t)
	f.answerSSE(
		`data: {"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"one"}]}}]}`,
		`data: {"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"two"}]}}]}`,
	)
	stream, err := geminiTestClient(t, f).Stream(context.Background(), Request{Model: "gemini-3-pro", Messages: userSays("hi")})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if ev, err := stream.Recv(); err != nil || ev.Text != "one" {
		t.Fatalf("first event = %+v (%v), want the first piece of text", ev, err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Reading after giving up reports the cancellation, not a finished answer.
	if _, err := stream.Recv(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Recv after Close = %v, want the cancellation that was asked for", err)
	}
}
