package builtin

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/providers"
)

// eventBuffer is how many events wait in a session's channel before the agent slows down. It
// matches the CLI adapters' own buffer.
const eventBuffer = 256

// session is one built-in agent conversation. It has no process: its state is the message history,
// which lives in memory for the session's life (Resume seeds it from what was persisted). One turn
// runs at a time; Send starts it and returns at once.
type session struct {
	adapter *Adapter
	cfg     Config
	log     *slog.Logger

	id    string
	label string
	cwd   string
	// instructions is the card's role instruction. It goes to the model with a new session's
	// first message, and is not sent again when a session is resumed (StartSpec.Instructions).
	instructions string
	// resumed says this session continues an earlier one, so instructions are not repeated.
	resumed bool

	life       context.Context
	lifeCancel context.CancelFunc

	mu          sync.Mutex
	resolved    Resolved
	model       string
	thinking    string
	permMode    protocol.PermissionMode
	history     []providers.Message
	tools       []tool
	willAllow   map[string]bool
	willDeny    map[string]bool
	pending     map[string]*pendingPermission
	permSeq     int
	turnActive  bool
	interrupted bool
	stopping    bool
	ended       bool
	turnCancel  context.CancelFunc

	// sink delivers events to the caller. Every send goes through emit or emitFinal below, and
	// only closeEvents closes it.
	sink *agents.EventSink

	finishOnce sync.Once
	done       chan struct{}
}

// pendingPermission is a permission request that waits for an answer. answer has room for one, so
// the responder never blocks.
type pendingPermission struct {
	id      string
	options []agents.PermissionOption
	answer  chan agents.ApprovalResponse
}

// newSession builds a session, with its provider and model already resolved.
func newSession(
	a *Adapter, spec agents.StartSpec, resolved Resolved, history []providers.Message,
	life context.Context, cancel context.CancelFunc,
) *session {
	return &session{
		adapter: a, cfg: a.cfg, log: a.log.With("label", spec.Label),
		id:    resolved.ProviderID + ":" + newSessionID(),
		label: spec.Label, cwd: spec.Cwd,
		instructions: spec.Instructions,
		life:         life, lifeCancel: cancel,
		resolved: resolved, model: spec.Model, thinking: spec.Thinking,
		permMode: protocol.PermissionMode(spec.PermissionMode),
		history:  history, tools: toolsFor(),
		willAllow: map[string]bool{}, willDeny: map[string]bool{},
		pending: map[string]*pendingPermission{},
		sink:    agents.NewEventSink(eventBuffer),
		done:    make(chan struct{}),
	}
}

// emit puts an event on the channel, giving up once senders are released or the channel has closed.
func (s *session) emit(ev agents.AgentEvent) { s.sink.Emit(ev) }

// emitFinal sends an event that must not be lost when there is room for it.
func (s *session) emitFinal(ev agents.AgentEvent) { s.sink.EmitFinal(ev) }

// finish ends the session once: it stops any running turn, releases blocked senders, sends Exited,
// closes the event channel, forgets the session, and lets its lifetime end.
func (s *session) finish(code int, err error) {
	s.finishOnce.Do(func() {
		s.mu.Lock()
		s.stopping, s.ended = true, true
		cancel := s.turnCancel
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		s.releaseSenders()
		s.emitFinal(agents.Exited{Code: code, Err: err})
		s.closeEvents()
		s.adapter.forget(s.id, s)
		s.lifeCancel()
		close(s.done)
	})
}

// releaseSenders lets every blocked emit give up.
func (s *session) releaseSenders() { s.sink.Release() }

// closeEvents closes the event channel once.
func (s *session) closeEvents() { s.sink.Close() }

// registerPermission adds a waiting request and returns it. It returns nil when the turn is being
// interrupted or the session stopped, so the caller answers "no" at once.
func (s *session) registerPermission(p *pendingPermission) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.interrupted || s.stopping || s.ended {
		return false
	}
	s.permSeq++
	p.id = fmt.Sprintf("perm-%d", s.permSeq)
	s.pending[p.id] = p
	return true
}

// dropPending forgets a request that is no longer waited for.
func (s *session) dropPending(id string) {
	s.mu.Lock()
	delete(s.pending, id)
	s.mu.Unlock()
}

// respond delivers the caller's answer to the request that waits for it.
func (s *session) respond(r agents.ApprovalResponse) error {
	s.mu.Lock()
	p, ok := s.pending[r.RequestID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("%w: %q", agents.ErrUnknownRequest, r.RequestID)
	}
	if !r.Cancelled {
		found := false
		for _, o := range p.options {
			if o.ID == r.OptionID {
				found = true
				break
			}
		}
		if !found {
			s.mu.Unlock()
			return fmt.Errorf("%w: %q", agents.ErrUnknownOption, r.OptionID)
		}
	}
	delete(s.pending, r.RequestID)
	s.mu.Unlock()
	p.answer <- r
	return nil
}

// cancelPending answers every waiting request with "cancelled". The caller holds s.mu.
func (s *session) cancelPending() {
	for id, p := range s.pending {
		p.answer <- agents.ApprovalResponse{RequestID: id, Cancelled: true}
		delete(s.pending, id)
	}
}

// takeTurn starts a turn: it returns a context for the loop and reports whether a turn may run. It
// returns agents.ErrBusy when one is already running, and agents.ErrStopped when the session is
// over.
func (s *session) takeTurn() (context.Context, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case s.stopping || s.ended:
		return nil, agents.ErrStopped
	case s.turnActive:
		return nil, agents.ErrBusy
	}
	s.turnActive = true
	s.interrupted = false
	ctx, cancel := context.WithCancel(s.life)
	s.turnCancel = cancel
	return ctx, nil
}

// endTurn clears the turn state and reports how the turn ended. The state is cleared before the
// event is sent, so a reader that reacts to TurnEnded by sending again is not refused with ErrBusy.
func (s *session) endTurn(reason string) {
	s.mu.Lock()
	s.turnActive = false
	if s.turnCancel != nil {
		s.turnCancel()
		s.turnCancel = nil
	}
	s.cancelPending()
	s.mu.Unlock()
	s.emit(agents.TurnEnded{Reason: reason})
}

// interrupt stops the running turn, keeping the session. It does nothing when no turn runs.
func (s *session) interrupt() error {
	s.mu.Lock()
	if !s.turnActive || s.stopping || s.ended || s.interrupted {
		s.mu.Unlock()
		return nil
	}
	s.interrupted = true
	cancel := s.turnCancel
	s.cancelPending()
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// stop ends the session and waits for it to be fully over.
func (s *session) stop(ctx context.Context) error {
	s.mu.Lock()
	already := s.stopping
	s.mu.Unlock()
	if already {
		select {
		case <-s.done:
		case <-ctx.Done():
			return fmt.Errorf("stop the agent: %w", ctx.Err())
		}
		return nil
	}
	s.finish(0, nil)
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("stop the agent: %w", ctx.Err())
	}
}

// failedTurn reports a turn that could not finish and ends it with the "error" reason.
func (s *session) failedTurn(message string, err error) {
	detail := ""
	if err != nil {
		detail = err.Error()
		s.log.Warn("a built-in agent turn failed", "err", err)
	}
	s.emit(agents.Failed{Message: message, Detail: detail})
	s.endTurn(agents.TurnError)
}

// systemPrompt is the instruction the session starts with: the package's base, then any extra the
// config carries, then the card's own instructions.
func (s *session) systemPrompt(instructions string) string {
	parts := []string{s.cfg.System}
	if instructions != "" {
		parts = append(parts, instructions)
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += "\n\n" + p
	}
	return out
}

// applySettings gives the session the settings asked for. A model that cannot be resolved, or a
// permission mode the daemon does not know, is reported as not applied rather than failing: the
// turn that follows runs with what the session already had.
func (s *session) applySettings(settings agents.SessionSettings) agents.Applied {
	var applied agents.Applied
	s.mu.Lock()
	defer s.mu.Unlock()
	if settings.Model != "" {
		if resolved, err := s.cfg.Resolver.Resolve(settings.Model); err == nil {
			s.model, s.resolved, applied.Model = settings.Model, resolved, true
		} else {
			s.log.Warn("could not switch a running session's model", "model", settings.Model, "err", err)
		}
	}
	if settings.Thinking != "" {
		s.thinking, applied.Thinking = settings.Thinking, true
	}
	if settings.PermissionMode != "" && validPermissionMode(settings.PermissionMode) {
		s.permMode, applied.PermissionMode = protocol.PermissionMode(settings.PermissionMode), true
	}
	return applied
}
