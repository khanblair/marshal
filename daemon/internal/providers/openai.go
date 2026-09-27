package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/packages/ssestream"
	"github.com/openai/openai-go/shared"
)

// openAICompatibleClient is one provider that speaks the OpenAI chat-completions API, behind the
// Client interface. OpenRouter, DeepSeek, Ollama, and LM Studio all speak it, so one adapter serves
// them all and only the base URL and the id differ (docs/library-docs.md section 2.4).
type openAICompatibleClient struct {
	id  string
	sdk *openai.Client
}

// NewOpenAICompatible returns a Client for any provider that speaks the OpenAI chat-completions
// API. id is the provider's own id - OpenAIID, OpenRouterID, DeepSeekID, OllamaID, or LMStudioID -
// and Config carries the address and, for the providers that take one, the key. A local provider
// (Config.BaseURL pointing at the machine) is used with an empty key.
func NewOpenAICompatible(id string, cfg Config) Client {
	opts := []option.RequestOption{}
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	if cfg.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(cfg.HTTPClient))
	}
	if cfg.Retries != nil {
		opts = append(opts, option.WithMaxRetries(*cfg.Retries))
	}
	c := openai.NewClient(opts...)
	return &openAICompatibleClient{id: id, sdk: &c}
}

// ID is the provider's own id, as the caller named it.
func (c *openAICompatibleClient) ID() string { return c.id }

// Complete makes one non-streamed call and returns the whole answer. The response is asked for as
// well as the completion, so the rate-limit headers (ratelimit.go) can be read and handed to a
// connection test; a provider that sends none leaves Reply.RateLimit nil.
func (c *openAICompatibleClient) Complete(ctx context.Context, req Request) (Reply, error) {
	params, err := openAIParams(req)
	if err != nil {
		return Reply{}, err
	}
	var raw *http.Response
	completion, err := c.sdk.Chat.Completions.New(ctx, params, option.WithResponseInto(&raw))
	if err != nil {
		return Reply{}, mapOpenAIError(err)
	}
	reply, err := openAIReply(completion)
	if err != nil {
		return Reply{}, err
	}
	reply.RateLimit = openAIRateLimit(raw, time.Now())
	return reply, nil
}

// Stream makes one call and returns its events as they arrive.
func (c *openAICompatibleClient) Stream(ctx context.Context, req Request) (Stream, error) {
	params, err := openAIParams(req)
	if err != nil {
		return nil, err
	}
	// Usage arrives only when it is asked for, and only on the last chunk. Nothing is sent until
	// the first Recv, so an error surfaces there rather than here.
	params.StreamOptions = openai.ChatCompletionStreamOptionsParam{IncludeUsage: param.NewOpt(true)}
	return &openAIStream{sdk: c.sdk.Chat.Completions.NewStreaming(ctx, params)}, nil
}

// openAIParams turns a Request into the SDK's parameters. It fails when something the caller passed
// cannot be expressed, such as a tool whose schema or a tool call whose arguments are not JSON.
func openAIParams(req Request) (openai.ChatCompletionNewParams, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	msgs, err := openAIMessages(req.Messages)
	if err != nil {
		return openai.ChatCompletionNewParams{}, err
	}
	// The chat-completions API has no system field: the system prompt is the first message.
	if req.System != "" {
		msgs = append([]openai.ChatCompletionMessageParamUnion{openai.SystemMessage(req.System)}, msgs...)
	}
	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(req.Model),
		Messages: msgs,
	}
	// max_tokens, not max_completion_tokens: this adapter serves the whole compatible family, and
	// max_tokens is the spelling all of it implements. See the report's rulings.
	params.MaxTokens = param.NewOpt(maxTokens)
	// Providers that do not know this field ignore it, so it is only sent when a mode asked for
	// thinking at all.
	if effort := openAIEffort(req.Thinking); effort != "" {
		params.ReasoningEffort = openai.ReasoningEffort(effort)
	}
	tools, err := openAITools(req.Tools)
	if err != nil {
		return openai.ChatCompletionNewParams{}, err
	}
	params.Tools = tools
	return params, nil
}

// openAIMessages turns the conversation into the SDK's message list. This family has no blocks
// inside a message: a turn's parts become several messages, in the order the API expects them.
func openAIMessages(in []Message) ([]openai.ChatCompletionMessageParamUnion, error) {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(in))
	for _, m := range in {
		if m.Role == RoleAssistant {
			msg, err := openAIAssistantMessage(m)
			if err != nil {
				return nil, err
			}
			out = append(out, msg)
			continue
		}
		// On a user turn, each tool result is a message of its own that answers one tool call,
		// and anything the person said follows them as one user message.
		var said strings.Builder
		for _, p := range m.Parts {
			switch p.Kind {
			case PartText:
				said.WriteString(p.Text)
			case PartToolResult:
				out = append(out, openai.ToolMessage(toolResultText(p), p.ToolUseID))
			case PartThinking:
				// This family has nowhere to put reasoning the model did not send, so it is
				// dropped rather than sent back as something else.
			default:
				return nil, fmt.Errorf("a message part of kind %q cannot be sent to an OpenAI-compatible provider", p.Kind)
			}
		}
		if said.Len() > 0 {
			out = append(out, openai.UserMessage(said.String()))
		}
	}
	return out, nil
}

// openAIAssistantMessage turns one assistant turn into one message: its text, and the tool calls it
// asked for together in the same message, which is how this API spells an assistant turn.
func openAIAssistantMessage(m Message) (openai.ChatCompletionMessageParamUnion, error) {
	var said strings.Builder
	calls := make([]openai.ChatCompletionMessageToolCallParam, 0, len(m.Parts))
	for _, p := range m.Parts {
		switch p.Kind {
		case PartText:
			said.WriteString(p.Text)
		case PartThinking:
			// As above: this family does not take reasoning back.
		case PartToolUse:
			args, err := toolArguments(p)
			if err != nil {
				return openai.ChatCompletionMessageParamUnion{}, err
			}
			calls = append(calls, openai.ChatCompletionMessageToolCallParam{
				ID: p.ToolUseID,
				Function: openai.ChatCompletionMessageToolCallFunctionParam{
					Name:      p.ToolName,
					Arguments: args,
				},
			})
		default:
			return openai.ChatCompletionMessageParamUnion{}, fmt.Errorf("a message part of kind %q cannot be sent to an OpenAI-compatible provider", p.Kind)
		}
	}
	msg := &openai.ChatCompletionAssistantMessageParam{ToolCalls: calls}
	if said.Len() > 0 {
		msg.Content = openai.ChatCompletionAssistantMessageParamContentUnion{OfString: param.NewOpt(said.String())}
	}
	return openai.ChatCompletionMessageParamUnion{OfAssistant: msg}, nil
}

// toolArguments renders a tool call's arguments as the JSON text this API carries them in. A call
// the model made with no arguments is sent as an empty object.
func toolArguments(p Part) (string, error) {
	if len(p.ToolInput) == 0 {
		return "{}", nil
	}
	if !json.Valid(p.ToolInput) {
		return "", fmt.Errorf("the arguments of the call to tool %q are not JSON: %s", p.ToolName, p.ToolInput)
	}
	return string(p.ToolInput), nil
}

// toolResultText renders what a tool printed as the content of a tool message. This API has no way
// to flag a tool result as a failure and no message content may be empty, so a failure is said in
// words and an empty result says so.
func toolResultText(p Part) string {
	text := p.Text
	if text == "" {
		text = "(the tool printed nothing)"
	}
	if p.IsError {
		return "the tool failed: " + text
	}
	return text
}

// openAITools turns Marshal's tool list into the SDK's. A tool with no schema is sent with an empty
// object schema, which is how this API spells a tool that takes no arguments.
func openAITools(in []Tool) ([]openai.ChatCompletionToolParam, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]openai.ChatCompletionToolParam, 0, len(in))
	for _, t := range in {
		schema := map[string]any{"type": "object", "properties": map[string]any{}}
		if len(t.InputSchema) > 0 {
			if err := json.Unmarshal(t.InputSchema, &schema); err != nil {
				return nil, fmt.Errorf("the schema of tool %q is not JSON: %w", t.Name, err)
			}
		}
		if _, ok := schema["type"]; !ok {
			schema["type"] = "object"
		}
		fn := shared.FunctionDefinitionParam{Name: t.Name, Parameters: shared.FunctionParameters(schema)}
		if t.Description != "" {
			fn.Description = param.NewOpt(t.Description)
		}
		out = append(out, openai.ChatCompletionToolParam{Function: fn})
	}
	return out, nil
}

// openAIReply turns a whole completion into a Reply. An answer with no choices at all is not
// something Marshal can read, so it is an error rather than an empty turn.
func openAIReply(completion *openai.ChatCompletion) (Reply, error) {
	if len(completion.Choices) == 0 {
		return Reply{}, fmt.Errorf("%w: the provider answered with no choices", ErrUnavailable)
	}
	choice := completion.Choices[0]
	parts := make([]Part, 0, len(choice.Message.ToolCalls)+1)
	if choice.Message.Content != "" {
		parts = append(parts, Part{Kind: PartText, Text: choice.Message.Content})
	}
	for _, call := range choice.Message.ToolCalls {
		parts = append(parts, Part{
			Kind:      PartToolUse,
			ToolUseID: call.ID,
			ToolName:  call.Function.Name,
			ToolInput: toolInputOf(call.Function.Arguments),
		})
	}
	return Reply{
		Parts:      parts,
		StopReason: openAIStopReason(choice.FinishReason),
		Usage: Usage{
			InputTokens:  completion.Usage.PromptTokens,
			OutputTokens: completion.Usage.CompletionTokens,
		},
	}, nil
}

// toolInputOf keeps a tool call's arguments as the JSON text the model sent, and reads an empty
// answer as a call with no arguments.
func toolInputOf(arguments string) json.RawMessage {
	if arguments == "" {
		return json.RawMessage("{}")
	}
	return json.RawMessage(arguments)
}

// openAIStopReason turns this API's finish reason into Marshal's. Anything Marshal does not know -
// and an empty reason - reads as the end of a turn, which is what it means: the answer is complete.
func openAIStopReason(reason string) string {
	switch reason {
	case "tool_calls", "function_call":
		return StopToolUse
	case "length":
		return StopMaxTokens
	case "content_filter":
		return StopRefusal
	default:
		return StopEndTurn
	}
}

// mapOpenAIError turns an SDK error into the provider sentinels callers act on. An error that is
// not an API answer at all (a broken connection, or an error the stream reports inside a chunk) is
// unavailable.
func mapOpenAIError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return errorForStatus(apiErr.StatusCode, apiErr.Error())
	}
	return fmt.Errorf("%w: %w", ErrUnavailable, err)
}

// openAIStream is one streamed answer. It turns the SDK's chunks into the small set of Marshal
// Events, collecting a tool call's arguments as they arrive so the call is only announced once it
// is whole, and it ends with one EventDone.
type openAIStream struct {
	sdk   *ssestream.Stream[openai.ChatCompletionChunk]
	queue []Event

	// calls holds the tool calls being collected, one per index the provider used.
	calls      []openAIPendingCall
	stopReason string
	usage      Usage
	done       bool
}

// openAIPendingCall is one tool call being collected from a stream: this API sends a call's id and
// name once and then its arguments in pieces.
type openAIPendingCall struct {
	id   string
	name string
	args strings.Builder
}

// Recv returns the next event, or io.EOF once the answer is over.
func (s *openAIStream) Recv() (Event, error) {
	for {
		if len(s.queue) > 0 {
			ev := s.queue[0]
			s.queue = s.queue[1:]
			return ev, nil
		}
		if s.done {
			return Event{}, io.EOF
		}
		if !s.sdk.Next() {
			if err := s.sdk.Err(); err != nil {
				return Event{}, mapOpenAIError(err)
			}
			// The answer is over: the collected tool calls are whole now, so they go out
			// before the last event.
			s.done = true
			s.finish()
			continue
		}
		s.handle(s.sdk.Current())
	}
}

// Close abandons the stream. It is safe to call more than once.
func (s *openAIStream) Close() error {
	s.done = true
	return s.sdk.Close()
}

// handle turns one chunk into zero or more Marshal events.
func (s *openAIStream) handle(chunk openai.ChatCompletionChunk) {
	if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
		s.usage = Usage{InputTokens: chunk.Usage.PromptTokens, OutputTokens: chunk.Usage.CompletionTokens}
	}
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" {
			s.queue = append(s.queue, Event{Kind: EventText, Text: choice.Delta.Content})
		}
		for _, call := range choice.Delta.ToolCalls {
			s.collect(int(call.Index), call)
		}
		if choice.FinishReason != "" {
			s.stopReason = openAIStopReason(choice.FinishReason)
		}
	}
}

// collect adds one piece of a tool call to the call it belongs to.
func (s *openAIStream) collect(index int, call openai.ChatCompletionChunkChoiceDeltaToolCall) {
	for len(s.calls) <= index {
		s.calls = append(s.calls, openAIPendingCall{})
	}
	pending := &s.calls[index]
	if call.ID != "" {
		pending.id = call.ID
	}
	if call.Function.Name != "" {
		pending.name = call.Function.Name
	}
	pending.args.WriteString(call.Function.Arguments)
}

// finish sends the collected tool calls and then the last event.
func (s *openAIStream) finish() {
	for _, pending := range s.calls {
		if pending.id == "" {
			continue
		}
		s.queue = append(s.queue, Event{
			Kind:      EventToolUse,
			ToolUseID: pending.id,
			ToolName:  pending.name,
			ToolInput: toolInputOf(pending.args.String()),
		})
	}
	if s.stopReason == "" {
		s.stopReason = StopEndTurn
	}
	s.queue = append(s.queue, Event{Kind: EventDone, StopReason: s.stopReason, Usage: s.usage})
}
