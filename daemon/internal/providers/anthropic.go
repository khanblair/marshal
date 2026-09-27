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

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
)

// Defaults every provider adapter shares.
const (
	// defaultMaxTokens is the answer limit an adapter uses when a request names none.
	defaultMaxTokens = 8192
	// minThinkingTokens is Anthropic's smallest thinking budget. A budget is raised so that
	// max_tokens always stays above it, as the API requires.
	minThinkingTokens = 1024
)

// Config says how to reach a provider. The zero value is not usable: APIKey is required for the
// providers that take one.
type Config struct {
	// APIKey is the provider key. It is never logged and never sent anywhere but the provider.
	APIKey string
	// BaseURL overrides the provider's own address. Empty uses the SDK's default. It is what the
	// OpenAI-compatible adapter uses to serve OpenRouter, DeepSeek, Ollama, and LM Studio, and
	// what a test uses to point at its own server.
	BaseURL string
	// HTTPClient overrides the default. Nil uses the SDK's own, which honors the context.
	HTTPClient *http.Client
	// Retries overrides how many times a failed call is tried again. Nil uses the SDK's own
	// default. A request queue of Marshal's own comes later (Phase 4, slice 4); this is only the
	// SDK's own last-resort retry, and a test points it at nil-or-zero to stay quick.
	Retries *int
}

// anthropicClient is one Anthropic Messages API client, behind the Client interface.
type anthropicClient struct {
	sdk *anthropic.Client
}

// NewAnthropic returns a Client for Anthropic's Messages API. It is the built-in agent's default
// provider.
func NewAnthropic(cfg Config) Client {
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	if cfg.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(cfg.HTTPClient))
	}
	if cfg.Retries != nil {
		opts = append(opts, option.WithMaxRetries(*cfg.Retries))
	}
	c := anthropic.NewClient(opts...)
	return &anthropicClient{sdk: &c}
}

// ID is Anthropic's own id.
func (c *anthropicClient) ID() string { return AnthropicID }

// Complete makes one non-streamed call and returns the whole answer. The response is asked for as
// well as the message, so the rate-limit headers Anthropic sends with it can be read (ratelimit.go)
// and handed to a connection test.
func (c *anthropicClient) Complete(ctx context.Context, req Request) (Reply, error) {
	params, err := anthropicParams(req)
	if err != nil {
		return Reply{}, err
	}
	var raw *http.Response
	msg, err := c.sdk.Messages.New(ctx, params, option.WithResponseInto(&raw))
	if err != nil {
		return Reply{}, mapAnthropicError(err)
	}
	reply := replyOf(msg)
	reply.RateLimit = anthropicRateLimit(raw, time.Now())
	return reply, nil
}

// Stream makes one call and returns its events as they arrive.
func (c *anthropicClient) Stream(ctx context.Context, req Request) (Stream, error) {
	params, err := anthropicParams(req)
	if err != nil {
		return nil, err
	}
	// NewStreaming does not send anything yet: the first error surfaces on the first Recv.
	return &anthropicStream{sdk: c.sdk.Messages.NewStreaming(ctx, params)}, nil
}

// anthropicParams turns a Request into the SDK's parameters. It fails only when something the
// caller passed cannot be expressed, such as a tool whose schema is not JSON.
func anthropicParams(req Request) (anthropic.MessageNewParams, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	msgs, err := anthropicMessages(req.Messages)
	if err != nil {
		return anthropic.MessageNewParams{}, err
	}
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		MaxTokens: maxTokens,
		Messages:  msgs,
	}
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.System}}
	}
	// A thinking budget must leave room to answer, so max_tokens is raised above it when the
	// caller asked for less. An empty or unknown mode leaves thinking to Anthropic's default.
	if budget := anthropicBudget(req.Thinking); budget > 0 {
		if params.MaxTokens <= budget {
			params.MaxTokens = budget + minThinkingTokens
		}
		params.Thinking = anthropic.ThinkingConfigParamUnion{
			OfEnabled: &anthropic.ThinkingConfigEnabledParam{BudgetTokens: budget},
		}
	}
	tools, err := anthropicTools(req.Tools)
	if err != nil {
		return anthropic.MessageNewParams{}, err
	}
	params.Tools = tools
	return params, nil
}

// anthropicMessages turns the conversation into the SDK's message list.
func anthropicMessages(in []Message) ([]anthropic.MessageParam, error) {
	out := make([]anthropic.MessageParam, 0, len(in))
	for _, m := range in {
		blocks := make([]anthropic.ContentBlockParamUnion, 0, len(m.Parts))
		for _, p := range m.Parts {
			switch p.Kind {
			case PartText:
				blocks = append(blocks, anthropic.NewTextBlock(p.Text))
			case PartThinking:
				blocks = append(blocks, anthropic.NewThinkingBlock(p.Signature, p.Text))
			case PartToolUse:
				blocks = append(blocks, anthropic.NewToolUseBlock(p.ToolUseID, json.RawMessage(p.ToolInput), p.ToolName))
			case PartToolResult:
				blocks = append(blocks, anthropic.NewToolResultBlock(p.ToolUseID, p.Text, p.IsError))
			default:
				return nil, fmt.Errorf("a message part of kind %q cannot be sent to Anthropic", p.Kind)
			}
		}
		if m.Role == RoleAssistant {
			out = append(out, anthropic.NewAssistantMessage(blocks...))
		} else {
			out = append(out, anthropic.NewUserMessage(blocks...))
		}
	}
	return out, nil
}

// anthropicTools turns Marshal's tool list into the SDK's. A tool with no schema is sent with an
// empty object schema, which is how Anthropic spells a tool that takes no arguments.
func anthropicTools(in []Tool) ([]anthropic.ToolUnionParam, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]anthropic.ToolUnionParam, 0, len(in))
	for _, t := range in {
		var schema struct {
			Properties map[string]any `json:"properties"`
			Required   []string       `json:"required"`
		}
		if len(t.InputSchema) > 0 {
			if err := json.Unmarshal(t.InputSchema, &schema); err != nil {
				return nil, fmt.Errorf("the schema of tool %q is not JSON: %w", t.Name, err)
			}
		}
		if schema.Properties == nil {
			schema.Properties = map[string]any{}
		}
		tool := &anthropic.ToolParam{
			Name:        t.Name,
			InputSchema: anthropic.ToolInputSchemaParam{Properties: schema.Properties, Required: schema.Required},
		}
		if t.Description != "" {
			tool.Description = param.NewOpt(t.Description)
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: tool})
	}
	return out, nil
}

// replyOf turns a whole Anthropic message into a Reply. Content blocks Marshal does not use
// (citations, server tools, redacted thinking) are dropped rather than guessed at.
func replyOf(msg *anthropic.Message) Reply {
	parts := make([]Part, 0, len(msg.Content))
	for _, b := range msg.Content {
		switch b.Type {
		case "text":
			if b.Text == "" {
				continue
			}
			parts = append(parts, Part{Kind: PartText, Text: b.Text})
		case "thinking":
			parts = append(parts, Part{Kind: PartThinking, Text: b.Thinking, Signature: b.Signature})
		case "tool_use":
			parts = append(parts, Part{
				Kind: PartToolUse, ToolUseID: b.ID, ToolName: b.Name, ToolInput: b.Input,
			})
		}
	}
	return Reply{
		Parts:      parts,
		StopReason: stopReasonOf(string(msg.StopReason)),
		Usage:      Usage{InputTokens: msg.Usage.InputTokens, OutputTokens: msg.Usage.OutputTokens},
	}
}

// stopReasonOf turns Anthropic's stop reason into Marshal's. Anything Marshal does not know - and
// an empty reason - reads as the end of a turn, which is what it means: the answer is complete.
func stopReasonOf(reason string) string {
	switch reason {
	case "tool_use":
		return StopToolUse
	case "max_tokens":
		return StopMaxTokens
	case "refusal":
		return StopRefusal
	default:
		return StopEndTurn
	}
}

// mapAnthropicError turns an SDK error into the provider sentinels callers act on. An error that is
// not an API answer at all (a broken connection, for example) is unavailable.
func mapAnthropicError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		return errorForStatus(apiErr.StatusCode, apiErr.Error())
	}
	return fmt.Errorf("%w: %w", ErrUnavailable, err)
}

// anthropicStream is one streamed answer. It turns the SDK's raw events into the small set of
// Marshal Events, buffering the ones that come out of a single SDK event (a tool call is announced
// only once its whole argument object has arrived).
type anthropicStream struct {
	sdk   *ssestream.Stream[anthropic.MessageStreamEventUnion]
	queue []Event

	// The tool block being read, between content_block_start and content_block_stop.
	inTool   bool
	toolID   string
	toolName string
	toolJSON strings.Builder

	stopReason string
	usage      Usage
	done       bool
}

// Recv returns the next event, or io.EOF once the answer is over.
func (s *anthropicStream) Recv() (Event, error) {
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
				return Event{}, mapAnthropicError(err)
			}
			s.done = true
			return Event{}, io.EOF
		}
		s.handle(s.sdk.Current())
	}
}

// Close abandons the stream. It is safe to call more than once.
func (s *anthropicStream) Close() error {
	s.done = true
	return s.sdk.Close()
}

// handle turns one SDK event into zero or more Marshal events.
func (s *anthropicStream) handle(ev anthropic.MessageStreamEventUnion) {
	switch ev.Type {
	case "message_start":
		s.usage.InputTokens = ev.Message.Usage.InputTokens
	case "content_block_start":
		if ev.ContentBlock.Type == "tool_use" {
			s.inTool = true
			s.toolID, s.toolName = ev.ContentBlock.ID, ev.ContentBlock.Name
			s.toolJSON.Reset()
		}
	case "content_block_delta":
		switch ev.Delta.Type {
		case "text_delta":
			s.queue = append(s.queue, Event{Kind: EventText, Text: ev.Delta.Text})
		case "thinking_delta":
			s.queue = append(s.queue, Event{Kind: EventThinking, Text: ev.Delta.Thinking})
		case "signature_delta":
			// A signature belongs to the thinking text sent just before it.
			s.queue = append(s.queue, Event{Kind: EventThinking, Signature: ev.Delta.Signature})
		case "input_json_delta":
			s.toolJSON.WriteString(ev.Delta.PartialJSON)
		}
	case "content_block_stop":
		if s.inTool {
			input := s.toolJSON.String()
			if input == "" {
				input = "{}"
			}
			s.queue = append(s.queue, Event{
				Kind: EventToolUse, ToolUseID: s.toolID, ToolName: s.toolName,
				ToolInput: json.RawMessage(input),
			})
			s.inTool = false
		}
	case "message_delta":
		if reason := string(ev.Delta.StopReason); reason != "" {
			s.stopReason = stopReasonOf(reason)
		}
		if ev.Usage.OutputTokens > 0 {
			s.usage.OutputTokens = ev.Usage.OutputTokens
		}
	case "message_stop":
		s.queue = append(s.queue, Event{Kind: EventDone, StopReason: s.stopReason, Usage: s.usage})
	}
}
