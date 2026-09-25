package acp

import (
	"context"
	"errors"
	"fmt"
	"time"

	sdk "github.com/coder/acp-go-sdk"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

const (
	// closeSessionTimeout bounds the polite "close the session" request during Stop.
	closeSessionTimeout = 2 * time.Second
	// requestCancelledCode is the protocol's code for a request that was cancelled.
	requestCancelledCode = -32800
)

// send starts a turn in its own goroutine, and returns at once.
func (s *session) send(msg agents.UserMessage) error {
	s.mu.Lock()
	switch {
	case s.stopping || s.ended:
		s.mu.Unlock()
		return agents.ErrStopped
	case s.turnActive:
		s.mu.Unlock()
		return agents.ErrBusy
	}
	// The turn runs on the session's own lifetime, not on the context of this call, which is
	// over as soon as Send returns.
	turnCtx, cancel := context.WithCancel(s.life)
	s.turnActive = true
	s.interrupted = false
	s.turnCancel = cancel
	prompt := s.promptFor(msg)
	sessionID := s.id
	s.turns.Add(1)
	s.mu.Unlock()

	go s.runTurn(turnCtx, cancel, sdk.PromptRequest{SessionId: sessionID, Prompt: prompt})
	return nil
}

// promptFor builds the prompt. The role instructions ride along with the first message of a new
// session, as their own block. The caller holds s.mu.
func (s *session) promptFor(msg agents.UserMessage) []sdk.ContentBlock {
	var blocks []sdk.ContentBlock
	if s.instructions != "" {
		blocks = append(blocks, sdk.TextBlock(s.instructions))
		s.instructions = ""
	}
	return append(blocks, sdk.TextBlock(msg.Text))
}

// runTurn runs one turn to its end and reports how it ended.
func (s *session) runTurn(ctx context.Context, cancel context.CancelFunc, req sdk.PromptRequest) {
	defer s.turns.Done()
	defer cancel()
	resp, err := s.conn.Prompt(ctx, req)
	s.endTurn(resp, err)
}

// endTurn clears the turn state and sends the event that ends the turn. The state is cleared
// first, so a reader that reacts to TurnEnded by sending again does not get ErrBusy.
func (s *session) endTurn(resp sdk.PromptResponse, err error) {
	s.mu.Lock()
	s.turnActive = false
	interrupted := s.interrupted
	s.interrupted = false
	if s.interruptTmr != nil {
		s.interruptTmr.Stop()
		s.interruptTmr = nil
	}
	gone := s.stopping || s.ended
	s.mu.Unlock()

	switch {
	case err == nil:
		s.emit(agents.TurnEnded{Reason: turnReason(resp.StopReason)})
	case gone || s.connectionClosed():
		// The process is ending. The supervisor tells the caller, with Failed when it was not
		// asked to, and it needs to know that this turn was cut short.
		s.noteLostTurn()
	case interrupted && isCancellation(err):
		s.emit(agents.TurnEnded{Reason: agents.TurnCancelled})
	default:
		s.log.Warn("the agent ended a turn with an error", "session_id", s.sessionID(), "err", err)
		s.emit(agents.Failed{Message: "The agent could not finish its turn.", Detail: err.Error()})
		s.emit(agents.TurnEnded{Reason: agents.TurnError})
	}
}

// noteLostTurn records that a turn was cut short by the end of the process.
func (s *session) noteLostTurn() {
	s.mu.Lock()
	s.endedInTurn = true
	s.mu.Unlock()
}

// connectionClosed says whether the protocol connection has ended.
func (s *session) connectionClosed() bool {
	select {
	case <-s.conn.Done():
		return true
	default:
		return false
	}
}

// isCancellation says whether an error means that the request was cancelled.
func isCancellation(err error) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	var reqErr *sdk.RequestError
	return errors.As(err, &reqErr) && reqErr.Code == requestCancelledCode
}

// turnReason maps the protocol's stop reason to ours. A reason from a newer protocol is treated
// as a normal end, since the turn did end without an error.
func turnReason(reason sdk.StopReason) string {
	switch reason {
	case sdk.StopReasonCancelled:
		return agents.TurnCancelled
	case sdk.StopReasonMaxTokens:
		return agents.TurnMaxTokens
	case sdk.StopReasonMaxTurnRequests:
		return agents.TurnMaxRequests
	case sdk.StopReasonRefusal:
		return agents.TurnRefusal
	default:
		return agents.TurnEndTurn
	}
}

// interrupt asks the agent to stop the running turn. The turn then ends with its own answer, and
// if the agent does not give one within the grace period, the adapter stops waiting for it.
func (s *session) interrupt(ctx context.Context) error {
	s.mu.Lock()
	if !s.turnActive || s.stopping || s.ended {
		s.mu.Unlock()
		return nil
	}
	s.interrupted = true
	waiting := s.takePending()
	if s.interruptTmr == nil {
		s.interruptTmr = time.AfterFunc(s.adapter.cfg.InterruptGrace, s.turnCancel)
	}
	sessionID := s.id
	s.mu.Unlock()

	// The protocol asks the client to answer every open permission request itself.
	cancelAll(waiting)
	if err := s.conn.Cancel(ctx, sdk.CancelNotification{SessionId: sessionID}); err != nil {
		return fmt.Errorf("interrupt the agent: %w", err)
	}
	return nil
}

// stop ends the session: it asks the agent to close it, closes the agent's input, and stops the
// process and everything it started.
func (s *session) stop(ctx context.Context) error {
	s.mu.Lock()
	first := !s.stopping
	s.stopping = true
	active := s.turnActive
	waiting := s.takePending()
	sessionID := s.id
	s.mu.Unlock()

	cancelAll(waiting)
	if first {
		s.closeGracefully(ctx, sessionID, active)
	}
	stopErr := s.proc.Stop(ctx, s.adapter.cfg.StopGrace)
	// The process is gone (or Stop gave up), so nobody can be waiting for a reader any more.
	s.releaseSenders()
	select {
	case <-s.done:
		return stopErr
	case <-ctx.Done():
		return fmt.Errorf("stop the agent: %w", ctx.Err())
	}
}

// closeGracefully cancels the running turn and asks the agent to close the session. Both are a
// courtesy: the process is stopped whatever happens to them.
func (s *session) closeGracefully(ctx context.Context, id sdk.SessionId, turnActive bool) {
	if turnActive {
		if err := s.conn.Cancel(ctx, sdk.CancelNotification{SessionId: id}); err != nil {
			s.log.Debug("could not cancel the turn before closing", "err", err)
		}
	}
	if !s.closeSupported {
		return
	}
	closeCtx, cancel := context.WithTimeout(ctx, closeSessionTimeout)
	defer cancel()
	if _, err := s.conn.CloseSession(closeCtx, sdk.CloseSessionRequest{SessionId: id}); err != nil {
		s.log.Debug("the agent did not close the session politely", "err", err)
	}
}
