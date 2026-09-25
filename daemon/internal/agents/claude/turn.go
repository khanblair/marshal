package claude

import (
	"context"
	"fmt"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/proc"
)

// send starts a turn: it relaunches the process first when Interrupt's fallback left one pending,
// then writes one stream-json input line and returns at once.
func (s *session) send(msg agents.UserMessage) error {
	s.mu.Lock()
	switch {
	case s.stopping || (s.ended && !s.resumeNeeded):
		s.mu.Unlock()
		return agents.ErrStopped
	case s.turnActive:
		s.mu.Unlock()
		return agents.ErrBusy
	}
	needsRelaunch := s.resumeNeeded
	s.turnActive = true
	s.interrupted = false
	s.mu.Unlock()

	if needsRelaunch {
		if err := s.relaunchForResume(); err != nil {
			s.clearTurnActive()
			return err
		}
	}
	line, err := encodeUserMessage(msg.Text)
	if err != nil {
		s.clearTurnActive()
		return fmt.Errorf("send a message: %w", err)
	}
	if err := s.writeStdin(line); err != nil {
		s.clearTurnActive()
		return fmt.Errorf("send a message: %w", err)
	}
	return nil
}

// clearTurnActive undoes send's turnActive flag when a send attempt fails before Claude Code
// could see it, so the caller may try again instead of being stuck with ErrBusy forever.
func (s *session) clearTurnActive() {
	s.mu.Lock()
	s.turnActive = false
	s.mu.Unlock()
}

// relaunchForResume starts a fresh claude code process with --resume, for the same session id, in
// place of one that Interrupt's grace period gave up on. It stops the old process first (safe
// even if the grace timer already did), and aborts if Stop was asked for while it was starting.
func (s *session) relaunchForResume() error {
	s.mu.Lock()
	old, args := s.proc, resumeArgs(s.commonArgs, s.id)
	s.mu.Unlock()
	if old != nil {
		if err := old.Stop(context.Background(), s.cfg.StopGrace); err != nil {
			s.log.Warn("stopping the old process before a resume failed", "err", err)
		}
	}
	p, stdoutDone, ready, err := s.spawn(args)
	if err != nil {
		return fmt.Errorf("resume the agent: %w", err)
	}
	startCtx, cancel := context.WithTimeout(s.life, s.cfg.StartTimeout)
	defer cancel()
	if err := s.awaitReady(startCtx, p, ready); err != nil {
		if stopErr := p.Stop(context.Background(), s.cfg.StopGrace); stopErr != nil {
			s.log.Warn("stopping a process that failed to resume failed", "err", stopErr)
		}
		<-stdoutDone
		return fmt.Errorf("resume the agent: %w", err)
	}
	return s.commitResumed(p, stdoutDone)
}

// commitResumed makes a freshly resumed process the session's current one, unless Stop raced it,
// in which case the new process is stopped instead and ErrStopped is returned.
func (s *session) commitResumed(p *proc.Process, stdoutDone <-chan struct{}) error {
	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		if err := p.Stop(context.Background(), s.cfg.StopGrace); err != nil {
			s.log.Warn("stopping a resumed process after Stop raced it failed", "err", err)
		}
		return agents.ErrStopped
	}
	s.proc, s.resumeNeeded, s.ended = p, false, false
	s.mu.Unlock()
	go s.watchThisProcess(p, stdoutDone)
	return nil
}

// endTurn is called with every "result" line. It clears the turn state and reports how the turn
// ended, unless Interrupt already claimed it: the state is cleared before the event is sent, so a
// reader that reacts to TurnEnded by sending again does not get ErrBusy.
func (s *session) endTurn(m resultLine) {
	s.mu.Lock()
	if !s.turnActive {
		s.mu.Unlock()
		return
	}
	s.turnActive = false
	interrupted := s.interrupted
	s.interrupted = false
	s.stopInterruptTimer()
	s.mu.Unlock()

	if interrupted {
		s.emit(agents.TurnEnded{Reason: agents.TurnCancelled})
		return
	}
	if m.IsError {
		s.emit(agents.Failed{Message: resultFailureMessage(m), Detail: resultFailureDetail(m)})
	}
	s.emit(agents.TurnEnded{Reason: turnReason(m)})
}

// stopInterruptTimer stops and clears the interrupt grace timer. The caller holds s.mu.
func (s *session) stopInterruptTimer() {
	if s.interruptTmr != nil {
		s.interruptTmr.Stop()
		s.interruptTmr = nil
	}
}

// interrupt asks Claude Code to end the turn in flight with a control_request (see wire.go and
// the report's Ruling on this: the shape is confirmed from the Claude Agent SDK's own source, not
// from Anthropic's published docs, since --help says nothing about it). It does nothing when no
// turn is running. If Claude Code does not answer within Config.InterruptGrace,
// forceCancelFallback tries SIGINT next, which Claude Code's own documentation names as the way
// to end a turn without leaving it "unfinished" the way SIGTERM does; only if that also gets no
// answer does the adapter fall back to stopping the process outright.
func (s *session) interrupt() error {
	s.mu.Lock()
	if !s.turnActive || s.stopping || s.ended || s.interrupted {
		s.mu.Unlock()
		return nil
	}
	s.interrupted = true
	s.mu.Unlock()

	line, err := encodeInterruptRequest(s.nextControlRequestID())
	if err != nil {
		return fmt.Errorf("interrupt the agent: %w", err)
	}
	if err := s.writeStdin(line); err != nil {
		return fmt.Errorf("interrupt the agent: %w", err)
	}
	s.armGraceTimer(s.forceCancelFallback)
	return nil
}

// armGraceTimer (re)starts the single timer that bounds how long an interrupt's current stage
// waits for Claude Code to answer on its own. endTurn stops it as soon as a "result" line arrives,
// whichever stage is running.
func (s *session) armGraceTimer(onExpiry func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.turnActive {
		return // the turn already ended while this stage was being armed
	}
	s.stopInterruptTimer()
	s.interruptTmr = time.AfterFunc(s.cfg.InterruptGrace, onExpiry)
}

// forceCancelFallback runs when Claude Code did not answer the control_request within the grace
// period. It sends SIGINT, Claude Code's documented way to end a turn, and gives it one more grace
// period through forceStopAndResume. When SIGINT cannot be sent at all (Windows; see
// interrupt_windows.go), it skips straight to forceStopAndResume.
func (s *session) forceCancelFallback() {
	s.mu.Lock()
	if !s.turnActive || !s.interrupted || s.stopping || s.ended {
		s.mu.Unlock()
		return
	}
	pid := s.proc.Pid()
	s.mu.Unlock()

	s.log.Warn("claude code did not answer an interrupt in time; sending an interrupt signal")
	if err := sigintPid(pid); err != nil {
		s.log.Debug("could not send an interrupt signal", "err", err)
		s.forceStopAndResume()
		return
	}
	s.armGraceTimer(s.forceStopAndResume)
}

// forceStopAndResume is the last resort: it runs when Claude Code answered neither the
// control_request nor SIGINT within their grace periods, or SIGINT could not be sent at all. It
// stops the process outright and marks the session so the next Send starts a fresh one with
// --resume before it writes the message, continuing the same session, as agents.Agent.Interrupt's
// doc comment requires. The turn is reported as cancelled here, since no "result" line is coming
// for endTurn to react to.
func (s *session) forceStopAndResume() {
	s.mu.Lock()
	if !s.turnActive || !s.interrupted || s.stopping || s.ended {
		s.mu.Unlock()
		return
	}
	s.turnActive, s.interrupted, s.resumeNeeded = false, false, true
	p := s.proc
	s.mu.Unlock()

	s.log.Warn("claude code did not end the interrupted turn; the session will restart on the next message")
	s.emit(agents.TurnEnded{Reason: agents.TurnCancelled})
	if p != nil {
		if err := p.Stop(context.Background(), s.cfg.StopGrace); err != nil {
			s.log.Warn("stopping an agent that did not answer an interrupt failed", "err", err)
		}
	}
}

// stop ends the session: it stops the process (or, when Interrupt's fallback already stopped the
// last one and nothing has relaunched it since, finishes the session directly, since no watcher
// is left to do it), and waits for the session to be fully over.
func (s *session) stop(ctx context.Context) error {
	s.mu.Lock()
	first, alreadyEnded, p := !s.stopping, s.ended, s.proc
	s.stopping = true
	s.mu.Unlock()
	if p == nil {
		return nil
	}
	var stopErr error
	if alreadyEnded {
		if first {
			s.finish(p.Wait(), p.StderrTail())
		}
	} else {
		stopErr = p.Stop(ctx, s.cfg.StopGrace)
	}
	select {
	case <-s.done:
		return stopErr
	case <-ctx.Done():
		return fmt.Errorf("stop the agent: %w", ctx.Err())
	}
}
