package agents

import (
	"fmt"
	"slices"
	"sync"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Factory makes an Agent. It captures whatever the agent needs, such as the path of a program, so
// the code that asks for an agent by kind knows nothing about how it is run.
type Factory func() (Agent, error)

// Registry finds an agent by kind. Configuration decides which factory sits behind a kind, so
// dev mode and tests can register the stub agent where a real one would go. It is safe for
// concurrent use.
type Registry struct {
	mu        sync.RWMutex
	factories map[protocol.AgentKind]Factory
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{factories: make(map[protocol.AgentKind]Factory)}
}

// Register adds a factory for a kind. A kind can only be registered once. Dev mode and tests
// register the stub agent under the kind it stands in for.
func (r *Registry) Register(kind protocol.AgentKind, f Factory) error {
	if !kind.Valid() || f == nil {
		return fmt.Errorf("register an agent: %q is not an agent kind, or the factory is missing", kind)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, taken := r.factories[kind]; taken {
		return fmt.Errorf("register an agent: kind %q is already registered", kind)
	}
	r.factories[kind] = f
	return nil
}

// New makes an agent of a kind. Every call makes a new one.
func (r *Registry) New(kind protocol.AgentKind) (Agent, error) {
	r.mu.RLock()
	f, ok := r.factories[kind]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownKind, kind)
	}
	agent, err := f()
	if err != nil {
		return nil, fmt.Errorf("make the %s agent: %w", kind, err)
	}
	return agent, nil
}

// Kinds lists the registered kinds in alphabetical order.
func (r *Registry) Kinds() []protocol.AgentKind {
	r.mu.RLock()
	defer r.mu.RUnlock()
	kinds := make([]protocol.AgentKind, 0, len(r.factories))
	for kind := range r.factories {
		kinds = append(kinds, kind)
	}
	slices.Sort(kinds)
	return kinds
}
