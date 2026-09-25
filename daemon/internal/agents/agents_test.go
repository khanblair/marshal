package agents

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"go.uber.org/goleak"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// nullAgent is an Agent that does nothing, for the registry tests.
type nullAgent struct{ name string }

func (nullAgent) Start(context.Context, StartSpec) (SessionHandle, error) {
	return SessionHandle{}, nil
}
func (nullAgent) Resume(context.Context, string, StartSpec) (SessionHandle, error) {
	return SessionHandle{}, nil
}
func (nullAgent) Send(context.Context, SessionHandle, UserMessage) error { return nil }
func (nullAgent) Interrupt(context.Context, SessionHandle) error         { return nil }
func (nullAgent) Events(SessionHandle) <-chan AgentEvent                 { return nil }
func (nullAgent) Respond(context.Context, SessionHandle, ApprovalResponse) error {
	return nil
}
func (nullAgent) Stop(context.Context, SessionHandle) error { return nil }
func (nullAgent) Capabilities() Capabilities                { return Capabilities{} }

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	factory := func(name string) Factory {
		return func() (Agent, error) { return nullAgent{name: name}, nil }
	}
	if err := r.Register(protocol.AgentKindGemini, factory("gemini")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Register(protocol.AgentKindClaude, factory("claude")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got := r.Kinds(); !slices.Equal(got, []protocol.AgentKind{protocol.AgentKindClaude, protocol.AgentKindGemini}) {
		t.Errorf("Kinds = %v, want claude and gemini in order", got)
	}
	got, err := r.New(protocol.AgentKindGemini)
	if err != nil || got.(nullAgent).name != "gemini" {
		t.Errorf("New(gemini) = %v, %v", got, err)
	}
}

func TestRegistryRefusals(t *testing.T) {
	r := NewRegistry()
	ok := func() (Agent, error) { return nullAgent{}, nil }
	if err := r.Register(protocol.AgentKindClaude, ok); err != nil {
		t.Fatalf("Register: %v", err)
	}
	tests := []struct {
		name string
		kind protocol.AgentKind
		f    Factory
		want string
	}{
		{"twice", protocol.AgentKindClaude, ok, "already registered"},
		{"not a kind", "gpt", ok, "not an agent kind"},
		{"no factory", protocol.AgentKindCodex, nil, "factory is missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := r.Register(tt.kind, tt.f)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Register error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
	if _, err := r.New(protocol.AgentKindCodex); !errors.Is(err, ErrUnknownKind) {
		t.Errorf("New of an unregistered kind = %v, want ErrUnknownKind", err)
	}
	boom := errors.New("no such program")
	_ = r.Register(protocol.AgentKindBuiltin, func() (Agent, error) { return nil, boom })
	if _, err := r.New(protocol.AgentKindBuiltin); !errors.Is(err, boom) {
		t.Errorf("New with a failing factory = %v, want the factory's error", err)
	}
}

func TestRegistryIsSafeForConcurrentUse(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(protocol.AgentKindClaude, func() (Agent, error) { return nullAgent{}, nil })
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := r.New(protocol.AgentKindClaude); err != nil {
				t.Errorf("New: %v", err)
			}
			_ = r.Kinds()
		})
	}
	wg.Wait()
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		limit         int
		want          string
		wantTruncated bool
	}{
		{"fits", "abc", 3, "abc", false},
		{"cut", "abcdef", 3, "abc", true},
		{"zero", "abc", 0, "", true},
		{"negative", "abc", -5, "", true},
		{"empty", "", 4, "", false},
		{"never splits a character", "aéb", 2, "a", true},
		{"keeps a whole character", "aéb", 3, "aé", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, cut := Truncate(tt.text, tt.limit)
			if got != tt.want || cut != tt.wantTruncated {
				t.Errorf("Truncate(%q, %d) = %q, %v, want %q, %v", tt.text, tt.limit, got, cut, tt.want, tt.wantTruncated)
			}
		})
	}
}

func TestAuthRequiredErrorReadsAsASentence(t *testing.T) {
	err := &AuthRequiredError{Methods: []string{"Browser", "API key"}}
	if got := err.Error(); got != "the agent needs you to sign in first (Browser, API key)" {
		t.Errorf("Error = %q", got)
	}
	cause := errors.New("no account")
	if !errors.Is(&AuthRequiredError{Cause: cause}, cause) {
		t.Error("errors.Is cannot see the cause")
	}
}

func TestEveryEventIsAnAgentEvent(t *testing.T) {
	events := []AgentEvent{
		MessageChunk{}, ThoughtChunk{}, ToolCall{}, ToolCallUpdate{}, PlanUpdate{},
		PermissionRequested{}, TurnEnded{}, Failed{}, Exited{},
	}
	for _, ev := range events {
		ev.agentEvent()
	}
}
