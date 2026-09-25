package acp

import (
	"context"
	"fmt"

	sdk "github.com/coder/acp-go-sdk"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// pendingPermission is a permission request that waits for an answer.
type pendingPermission struct {
	options []sdk.PermissionOption
	// answer takes exactly one outcome. It has room for it, so nobody blocks on sending.
	answer chan sdk.RequestPermissionOutcome
}

// cancelledOutcome is the answer for a request that nobody will answer.
func cancelledOutcome() sdk.RequestPermissionOutcome {
	return sdk.NewRequestPermissionOutcomeCancelled()
}

// hasOption says whether the request offered an option with this id.
func (p *pendingPermission) hasOption(id string) bool {
	for _, o := range p.options {
		if string(o.OptionId) == id {
			return true
		}
	}
	return false
}

// takePending removes every waiting request and returns them. The caller holds s.mu.
func (s *session) takePending() []*pendingPermission {
	waiting := make([]*pendingPermission, 0, len(s.pending))
	for id, p := range s.pending {
		waiting = append(waiting, p)
		delete(s.pending, id)
	}
	return waiting
}

// cancelAll answers each request with "cancelled".
func cancelAll(waiting []*pendingPermission) {
	for _, p := range waiting {
		p.answer <- cancelledOutcome()
	}
}

// register adds a request and returns its id. It returns false when the turn is being
// interrupted or the session stopped, and the request must be answered "cancelled" at once.
func (s *session) registerPermission(p *pendingPermission) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.interrupted || s.stopping || s.ended {
		return "", false
	}
	s.requests++
	id := fmt.Sprintf("perm-%d", s.requests)
	s.pending[id] = p
	return id, true
}

// dropPending forgets a request that is no longer waited for.
func (s *session) dropPending(id string) {
	s.mu.Lock()
	delete(s.pending, id)
	s.mu.Unlock()
}

// askPermission shows a request to the caller and waits for the answer. The answer comes from
// Respond, or it is "cancelled" when the turn is interrupted, the session stops, or the agent
// withdraws the request.
func (s *session) askPermission(
	ctx context.Context, req sdk.RequestPermissionRequest,
) sdk.RequestPermissionResponse {
	p := &pendingPermission{options: req.Options, answer: make(chan sdk.RequestPermissionOutcome, 1)}
	id, ok := s.registerPermission(p)
	if !ok {
		return sdk.RequestPermissionResponse{Outcome: cancelledOutcome()}
	}
	// The request is registered before it is announced, so an answer cannot arrive too early.
	s.emit(permissionEvent(id, req))
	select {
	case outcome := <-p.answer:
		return sdk.RequestPermissionResponse{Outcome: outcome}
	case <-ctx.Done():
	case <-s.processEnded:
	}
	s.dropPending(id)
	return sdk.RequestPermissionResponse{Outcome: cancelledOutcome()}
}

// permissionEvent describes a request to the caller.
func permissionEvent(id string, req sdk.RequestPermissionRequest) agents.PermissionRequested {
	call := req.ToolCall
	ev := agents.PermissionRequested{
		RequestID:  id,
		ToolCallID: string(call.ToolCallId),
		Path:       toolPath(call.Locations, call.RawInput),
		Command:    rawField(call.RawInput, "command"),
	}
	if call.Title != nil {
		ev.Title = *call.Title
	}
	if call.Kind != nil {
		ev.Kind = string(*call.Kind)
	}
	for _, o := range req.Options {
		ev.Options = append(ev.Options, agents.PermissionOption{
			ID: string(o.OptionId), Name: o.Name, Kind: string(o.Kind),
		})
	}
	return ev
}

// respond delivers the caller's answer to the request that waits for it.
func (s *session) respond(r agents.ApprovalResponse) error {
	s.mu.Lock()
	p, ok := s.pending[r.RequestID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("%w: %q", agents.ErrUnknownRequest, r.RequestID)
	}
	if !r.Cancelled && !p.hasOption(r.OptionID) {
		s.mu.Unlock()
		return fmt.Errorf("%w: %q", agents.ErrUnknownOption, r.OptionID)
	}
	delete(s.pending, r.RequestID)
	s.mu.Unlock()

	if r.Cancelled {
		p.answer <- cancelledOutcome()
		return nil
	}
	p.answer <- sdk.NewRequestPermissionOutcomeSelected(sdk.PermissionOptionId(r.OptionID))
	return nil
}
