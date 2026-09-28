package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/providers"
)

// scriptedClient is a providers.Client that answers with a fixed script, so no test ever reaches a
// real provider. Each Stream call takes the next scripted answer; the requests it was given are
// kept so a test can check what was sent.
type scriptedClient struct {
	id string

	mu      sync.Mutex
	replies [][]providers.Event
	idx     int
	sent    []providers.Request
	block   bool
}

func newScriptedClient(replies ...[]providers.Event) *scriptedClient {
	return &scriptedClient{id: providers.AnthropicID, replies: replies}
}

func (c *scriptedClient) ID() string { return c.id }

func (c *scriptedClient) Complete(_ context.Context, req providers.Request) (providers.Reply, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, req)
	if c.idx >= len(c.replies) {
		return providers.Reply{}, fmt.Errorf("no scripted reply left")
	}
	events := c.replies[c.idx]
	c.idx++
	return replyOfEvents(events), nil
}

func (c *scriptedClient) Stream(ctx context.Context, req providers.Request) (providers.Stream, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, req)
	if c.block {
		return &scriptedStream{ctx: ctx, block: true}, nil
	}
	if c.idx >= len(c.replies) {
		return nil, fmt.Errorf("no scripted reply left")
	}
	events := c.replies[c.idx]
	c.idx++
	return &scriptedStream{ctx: ctx, events: events}, nil
}

// requests returns every request the client was given, oldest first.
func (c *scriptedClient) requests() []providers.Request {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]providers.Request, len(c.sent))
	copy(out, c.sent)
	return out
}

// scriptedStream walks a fixed list of events, then reports io.EOF. A blocking stream waits until
// the context ends, so a test can interrupt a turn mid-answer.
type scriptedStream struct {
	ctx    context.Context
	events []providers.Event
	at     int
	block  bool
	closed bool
	mu     sync.Mutex
}

func (s *scriptedStream) Recv() (providers.Event, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return providers.Event{}, io.EOF
	}
	block, at := s.block, s.at
	s.mu.Unlock()
	if block {
		<-s.ctx.Done()
		return providers.Event{}, s.ctx.Err()
	}
	if at >= len(s.events) {
		return providers.Event{}, io.EOF
	}
	s.mu.Lock()
	s.at++
	s.mu.Unlock()
	return s.events[at], nil
}

func (s *scriptedStream) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return nil
}

// replyOfEvents turns a script's events into a whole reply, for Complete.
func replyOfEvents(events []providers.Event) providers.Reply {
	var out providers.Reply
	for _, ev := range events {
		switch ev.Kind {
		case providers.EventText:
			out.Parts = append(out.Parts, providers.Part{Kind: providers.PartText, Text: ev.Text})
		case providers.EventThinking:
			out.Parts = append(out.Parts, providers.Part{Kind: providers.PartThinking, Text: ev.Text, Signature: ev.Signature})
		case providers.EventToolUse:
			out.Parts = append(out.Parts, providers.Part{
				Kind: providers.PartToolUse, ToolUseID: ev.ToolUseID, ToolName: ev.ToolName, ToolInput: ev.ToolInput,
			})
		case providers.EventDone:
			out.StopReason = ev.StopReason
			out.Usage = ev.Usage
		}
	}
	if out.StopReason == "" {
		out.StopReason = providers.StopEndTurn
	}
	return out
}

// text is a scripted piece of the model's answer.
func text(s string) providers.Event { return providers.Event{Kind: providers.EventText, Text: s} }

// think is a scripted piece of the model's reasoning.
func think(s string) providers.Event {
	return providers.Event{Kind: providers.EventThinking, Text: s, Signature: "sig-" + s}
}

// toolUse is a scripted tool call.
func callTool(id, name, input string) providers.Event {
	return providers.Event{Kind: providers.EventToolUse, ToolUseID: id, ToolName: name, ToolInput: json.RawMessage(input)}
}

// done ends a scripted answer.
func done(stop string) providers.Event {
	return providers.Event{Kind: providers.EventDone, StopReason: stop, Usage: providers.Usage{InputTokens: 10, OutputTokens: 5}}
}

// fakeResolver hands out one client for every model.
type fakeResolver struct {
	client providers.Client
	models map[string]string
	err    error
}

func (r *fakeResolver) Resolve(model string) (Resolved, error) {
	if r.err != nil {
		return Resolved{}, r.err
	}
	name := model
	if r.models != nil {
		if mapped, ok := r.models[model]; ok {
			name = mapped
		} else if model != "" {
			return Resolved{}, fmt.Errorf("no such model %q", model)
		}
	}
	if name == "" {
		name = "claude-sonnet-4-5"
	}
	return Resolved{ProviderID: r.client.ID(), Model: name, Client: r.client}, nil
}

// newTestAgent builds an adapter over a scripted client, with settings tests can rely on.
func newTestAgent(t *testing.T, client providers.Client, mutate ...func(*Config)) agents.Agent {
	t.Helper()
	cfg := Config{
		Resolver:  &fakeResolver{client: client},
		Profile:   testProfile(),
		Logger:    quiet(),
		MaxTurns:  6,
		MaxTokens: 1024,
	}
	for _, m := range mutate {
		m(&cfg)
	}
	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return agent
}

// quiet is a logger that throws its output away.
func quiet() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// kinds lists the type names of a run of events, for a test's failure message.
func kinds(events []agents.AgentEvent) []string {
	out := make([]string, len(events))
	for i, ev := range events {
		out[i] = fmt.Sprintf("%T", ev)
	}
	return out
}

// waitFor reads events until one of the wanted type arrives, and returns it plus everything read so
// far. It fails if the channel closes first.
func waitFor[T agents.AgentEvent](t *testing.T, ch <-chan agents.AgentEvent) (T, []agents.AgentEvent) {
	t.Helper()
	var zero T
	var seen []agents.AgentEvent
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("the channel closed before a %T arrived; saw %v", zero, kinds(seen))
			}
			seen = append(seen, ev)
			if want, ok := ev.(T); ok {
				return want, seen
			}
		case <-timeout:
			t.Fatalf("no %T arrived; saw %v", zero, kinds(seen))
		}
	}
}
