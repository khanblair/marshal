// Package providers talks to the model provider APIs the owner pays for with their own keys, so
// Marshal can run an agent of its own instead of shelling out to a CLI. One interface covers a
// non-streamed call and a streamed one, and one place maps Marshal's thinking modes onto each
// provider's own vocabulary (docs/library-docs.md section 2.4: "Thinking modes are mapped per
// provider in one place in `providers`. Do not spread provider-specific settings through other
// modules").
//
// The built-in agent (agents/builtin) drives this package. No other package calls a provider SDK
// directly: every call goes through a Client, which is where the queue, retries, fallback, and
// usage tracking join later (docs/library-docs.md section 2.4).
package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Errors that callers act on. An adapter wraps them with context, so callers compare with
// errors.Is. They are the few things a person can do something about: fix the key, wait, or pick
// another model.
var (
	// ErrAuth means the provider refused the key.
	ErrAuth = errors.New("the provider rejected the key")
	// ErrRateLimited means the provider asked the caller to slow down. The caller waits and tries
	// the same request again.
	ErrRateLimited = errors.New("the provider is rate limiting")
	// ErrUnavailable means the provider could not be reached, or answered with a server error. It
	// is the one worth falling back on another model or provider for.
	ErrUnavailable = errors.New("the provider is unavailable")
	// ErrNoSuchModel means the provider does not know the model that was asked for.
	ErrNoSuchModel = errors.New("the provider does not offer that model")
	// ErrNoProvider means no provider is set up at all, so Marshal has no model to call. It is
	// what a caller that only wants to spend as little as possible is told when the person has not
	// saved a key yet - a chat title, for example, which then falls back to naming itself from the
	// message (chats.Service's own rule).
	ErrNoProvider = errors.New("no model provider is set up")
)

// errorForStatus turns the HTTP status a provider answered with into the sentinel a caller acts on.
// Every adapter maps a status this way, so one status means the same thing whichever provider it
// came from. detail is the provider's own words, kept for the person reading the log.
func errorForStatus(status int, detail string) error {
	switch {
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return fmt.Errorf("%w: %s", ErrAuth, detail)
	case status == http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrNoSuchModel, detail)
	case status == http.StatusTooManyRequests:
		return fmt.Errorf("%w: %s", ErrRateLimited, detail)
	case status >= http.StatusInternalServerError:
		return fmt.Errorf("%w: %s", ErrUnavailable, detail)
	default:
		return fmt.Errorf("the provider refused the request: %s", detail)
	}
}

// Part kinds. A message is a list of parts, in order, so one message can hold text and a tool call
// together the way the providers send them.
const (
	// PartText is a piece of the model's answer, or of what the person said.
	PartText = "text"
	// PartThinking is a piece of the model's reasoning, when it shares it.
	PartThinking = "thinking"
	// PartToolUse is the model asking for a tool to run: ToolUseID names the call, ToolName the
	// tool, and ToolInput its arguments as JSON.
	PartToolUse = "tool_use"
	// PartToolResult is the answer to a tool call, on a user message. ToolUseID matches the call
	// it answers, Text holds what the tool printed, and IsError says it failed.
	PartToolResult = "tool_result"
)

// Roles of a Message.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Stop reasons. They are the same words the daemon's agents use for a turn's reason, so an adapter
// never has to translate twice.
const (
	// StopEndTurn means the model finished its answer.
	StopEndTurn = "end_turn"
	// StopToolUse means the model stopped so a tool it named can run.
	StopToolUse = "tool_use"
	// StopMaxTokens means the answer hit the token limit.
	StopMaxTokens = "max_tokens"
	// StopRefusal means the model declined to go on.
	StopRefusal = "refusal"
)

// Part is one piece of a message. Only the fields its Kind uses are set.
type Part struct {
	Kind string
	// Text is the text of a PartText or PartThinking part, or what a tool printed for a
	// PartToolResult part.
	Text string
	// Signature is the provider's proof that a PartThinking part is genuine. Anthropic requires
	// it back when a conversation with a tool call continues, so it is kept with the part and
	// sent again unchanged.
	Signature string
	// ToolUseID names a PartToolUse call, and matches it on the PartToolResult that answers it.
	ToolUseID string
	// ToolName is the tool a PartToolUse part asks for. On a PartToolResult it optionally names
	// the tool the result answers, which is what Gemini's function responses need; an adapter
	// that does not need it ignores it, and one that does can also find the name on the
	// PartToolUse part that carries the matching ToolUseID.
	ToolName string
	// ToolInput is the arguments of a PartToolUse part, as the model sent them.
	ToolInput json.RawMessage
	// IsError says that a PartToolResult is a failure.
	IsError bool
}

// Message is one turn of the conversation: what a person said, what the model answered, or the
// results of the tools the model asked for.
type Message struct {
	Role  string
	Parts []Part
}

// Tool is one tool the model may ask to run.
type Tool struct {
	Name        string
	Description string
	// InputSchema is a JSON Schema object describing the tool's arguments.
	InputSchema json.RawMessage
}

// Request is one call to a provider.
type Request struct {
	// Model is the provider's own model id.
	Model string
	// System is the system prompt. Empty sends none.
	System string
	// Messages is the conversation so far, oldest first.
	Messages []Message
	// Tools are the tools the model may ask to run. Empty sends none.
	Tools []Tool
	// Thinking is a protocol.ThinkingMode value. Empty means the provider's own default. A mode
	// the provider does not offer is ignored rather than refused, so a card set to think still
	// runs on a model that cannot.
	Thinking string
	// MaxTokens caps how much the model may answer with. Zero means the adapter's own default.
	MaxTokens int64
}

// Usage is what one call cost, in the provider's own token counts.
type Usage struct {
	InputTokens  int64
	OutputTokens int64
}

// Reply is a whole non-streamed answer.
type Reply struct {
	// Parts is the model's answer, in order.
	Parts []Part
	// StopReason is one of the Stop constants.
	StopReason string
	Usage      Usage
	// RateLimit is what the provider said about the caller's allowance on this call, or nil when
	// it said nothing (see ratelimit.go). It is read by the connection test; nothing on the call
	// path acts on it, because the queue (queue.go) is what keeps Marshal inside a provider's
	// allowance rather than reacting to a number after the fact.
	RateLimit *RateLimit
}

// Event kinds of a streamed answer.
const (
	// EventText is a piece of the answer's text.
	EventText = "text"
	// EventThinking is a piece of the model's reasoning.
	EventThinking = "thinking"
	// EventToolUse is a whole tool call, sent once its arguments have all arrived. ToolUseID,
	// ToolName, and ToolInput are set.
	EventToolUse = "tool_use"
	// EventDone is the last event. StopReason and Usage are set.
	EventDone = "done"
)

// Event is one piece of a streamed answer. Only the fields its Kind uses are set.
type Event struct {
	Kind      string
	Text      string
	Signature string
	ToolUseID string
	ToolName  string
	ToolInput json.RawMessage
	// StopReason is one of the Stop constants, on an EventDone.
	StopReason string
	// Usage is the call's cost, on an EventDone.
	Usage Usage
}

// Stream is one streamed answer. Recv returns the events in order and io.EOF once the answer is
// over and EventDone has been read.
type Stream interface {
	// Recv returns the next event, or io.EOF when there are no more.
	Recv() (Event, error)
	// Close abandons the stream. It is safe to call more than once.
	Close() error
}

// Client is one provider, as Marshal uses it. A single Client is safe to use from several
// goroutines.
type Client interface {
	// ID is the provider's own id, such as "anthropic" or "openai".
	ID() string
	// Complete makes one non-streamed call and returns the whole answer. It is what a connection
	// test and a chat title use, where a partial answer is of no use.
	Complete(ctx context.Context, req Request) (Reply, error)
	// Stream makes one call and returns its events as they arrive. The caller reads until
	// Recv returns io.EOF, and calls Close if it stops early.
	Stream(ctx context.Context, req Request) (Stream, error)
}
