package agents

import (
	"fmt"
	"sync"
)

// SessionRegistry tracks an adapter's running sessions by their own id, and is safe for concurrent
// use. An adapter that runs many sessions at once behind one Agent, each under its own id, holds
// one of these to find, register, and forget them: the built-in and Claude Code adapters both do
// (see each one's find, register, and forget).
type SessionRegistry[S any] struct {
	mu       sync.Mutex
	sessions map[string]*S
}

// NewSessionRegistry returns an empty registry.
func NewSessionRegistry[S any]() *SessionRegistry[S] {
	return &SessionRegistry[S]{sessions: make(map[string]*S)}
}

// Find returns the running session that a handle names.
func (r *SessionRegistry[S]) Find(h SessionHandle) (*S, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[h.ID]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownSession, h.ID)
	}
	return s, nil
}

// Register records a session that is ready. Two running sessions cannot share an id.
func (r *SessionRegistry[S]) Register(id string, s *S) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, taken := r.sessions[id]; taken {
		return fmt.Errorf("session %q is already running", id)
	}
	r.sessions[id] = s
	return nil
}

// Forget removes a session that has ended.
func (r *SessionRegistry[S]) Forget(id string, s *S) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessions[id] == s {
		delete(r.sessions, id)
	}
}
