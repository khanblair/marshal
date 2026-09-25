package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/agents/catalog"
	"github.com/khanblair/marshal/daemon/internal/proc"
)

const (
	// eventBuffer is how many events wait in a session's channel before the agent slows down.
	eventBuffer = 256
	// abandonGrace is how long a process that failed to start gets to exit before it is killed.
	abandonGrace = 500 * time.Millisecond
	// errorTailBytes is how much of Claude Code's last output goes into a start error.
	errorTailBytes = 1024
	// initialLineBufferBytes is the starting size of the buffer that reads Claude Code's output
	// one line at a time. readLine grows past it for a longer line, so this only bounds the
	// number of reallocations for an ordinary one.
	initialLineBufferBytes = 64 << 10
)

// session is one Claude Code process and the session id it carries. Only one process runs for a
// session at a time; Interrupt's fallback and an external Resume both replace it, never run two
// side by side. All the methods that change its state take session.mu; sendMu and stdinMu guard
// the event channel and standard input separately, so a slow reader or a slow write never blocks
// the other.
type session struct {
	adapter *Adapter
	log     *slog.Logger
	cfg     Config
	label   string
	cwd     string
	env     []string

	life       context.Context
	lifeCancel context.CancelFunc

	mu           sync.Mutex
	id           string
	commonArgs   []string
	proc         *proc.Process
	turnActive   bool
	interrupted  bool
	resumeNeeded bool
	stopping     bool
	ended        bool
	endedInTurn  bool
	interruptTmr *time.Timer
	controlSeq   uint64

	stdinMu sync.Mutex

	// sink delivers events to the caller. Every send goes through emit or emitFinal below, and
	// only closeEvents closes it.
	sink *agents.EventSink

	finishOnce sync.Once
	done       chan struct{}
}

// newSession makes the session for one Start or Resume. Its process comes later, from spawn.
func newSession(a *Adapter, spec agents.StartSpec, life context.Context, cancel context.CancelFunc) *session {
	return &session{
		adapter: a, log: a.log.With("label", spec.Label), cfg: a.cfg,
		label: spec.Label, cwd: spec.Cwd,
		env:        slices.Concat(catalog.ProgramEnv(a.cfg.Path), a.cfg.Env),
		life:       life,
		lifeCancel: cancel,
		sink:       agents.NewEventSink(eventBuffer),
		done:       make(chan struct{}),
	}
}

// open starts a new Claude Code process: a new session when resumeID is empty, or the one it
// names otherwise.
func (a *Adapter) open(ctx context.Context, spec agents.StartSpec, resumeID string) (agents.SessionHandle, error) {
	if err := absoluteCwd(spec); err != nil {
		return agents.SessionHandle{}, err
	}
	common, err := a.cfg.commonArgs(spec)
	if err != nil {
		return agents.SessionHandle{}, err
	}
	id, args, err := idAndArgs(resumeID, common, spec.Instructions)
	if err != nil {
		return agents.SessionHandle{}, err
	}
	life, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s := newSession(a, spec, life, cancel)
	s.id, s.commonArgs = id, common
	s.log = s.log.With("session_id", id)
	s.env = append(s.env, spec.Env...)

	p, stdoutDone, ready, err := s.spawn(args)
	if err != nil {
		cancel()
		return agents.SessionHandle{}, err
	}
	startCtx, cancelStart := context.WithTimeout(ctx, a.cfg.StartTimeout)
	defer cancelStart()
	if err := s.awaitReady(startCtx, p, ready); err != nil {
		return agents.SessionHandle{}, s.abandon(ctx, p, stdoutDone, err)
	}
	s.mu.Lock()
	s.proc = p
	s.mu.Unlock()
	if err := a.register(id, s); err != nil {
		return agents.SessionHandle{}, s.abandon(ctx, p, stdoutDone, err)
	}
	go s.watchThisProcess(p, stdoutDone)
	s.log.Info("claude code session started", "resumed", resumeID != "")
	return agents.SessionHandle{
		ID: id, Label: spec.Label, Model: spec.Model, Thinking: spec.Thinking, PermissionMode: spec.PermissionMode,
		Applied: appliedOf(spec), Capabilities: a.Capabilities(),
	}, nil
}

// idAndArgs picks the session id and the argument list for a Start (resumeID empty) or a Resume.
func idAndArgs(resumeID string, common []string, instructions string) (string, []string, error) {
	if resumeID != "" {
		if !looksLikeUUID(resumeID) {
			return "", nil, fmt.Errorf("%w: %q is not a claude code session id", agents.ErrCannotResume, resumeID)
		}
		return resumeID, resumeArgs(common, resumeID), nil
	}
	id, err := newSessionID()
	if err != nil {
		return "", nil, err
	}
	return id, startArgs(common, id, instructions), nil
}

// spawn starts the claude program with the session's working folder and environment, and reads
// its standard output in a new goroutine. ready is closed once Claude Code's init line arrives.
func (s *session) spawn(args []string) (p *proc.Process, stdoutDone, ready chan struct{}, err error) {
	p, err = proc.Start(s.life, proc.Spec{Path: s.cfg.Path, Args: args, Dir: s.cwd, Env: s.env, Stdin: true})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("start claude code: %w", err)
	}
	stdoutDone = make(chan struct{})
	ready = make(chan struct{})
	go s.readLoop(p, stdoutDone, ready)
	return p, stdoutDone, ready, nil
}

// awaitReady waits for Claude Code's init line, which is the fast path. It is not certain that
// every version sends one before it needs input (see the report), so a process that is still
// alive after Config.ReadyWindow, with no init line yet, is treated as ready anyway: a later init
// line, if one still comes, is still checked for a matching session id (see handleSystem). Only
// the process exiting first, or ctx ending first (still bounded by Config.StartTimeout), fails
// the start.
func (s *session) awaitReady(ctx context.Context, p *proc.Process, ready <-chan struct{}) error {
	window := time.NewTimer(s.cfg.ReadyWindow)
	defer window.Stop()
	select {
	case <-ready:
		return nil
	case <-p.Done():
		exit := p.Wait()
		return fmt.Errorf("claude code exited before it was ready (exit code %d)%s", exit.Code, tailSuffix(p.StderrTail()))
	case <-window.C:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("claude code did not become ready in time: %w", ctx.Err())
	}
}

// abandon cleans up after a start or resume that failed before the session was registered: the
// process is stopped, its output is drained, and what it printed last is added to the error.
func (s *session) abandon(ctx context.Context, p *proc.Process, stdoutDone <-chan struct{}, cause error) error {
	if err := p.Stop(context.WithoutCancel(ctx), abandonGrace); err != nil {
		s.log.Warn("could not stop a claude code process that failed to start", "err", err)
	}
	p.Wait()
	<-stdoutDone
	s.lifeCancel()
	if tail := strings.TrimSpace(endOf(p.StderrTail(), errorTailBytes)); tail != "" {
		return fmt.Errorf("%w (claude code printed: %s)", cause, tail)
	}
	return cause
}

// watchThisProcess is spawned once per process generation: the first launch, and any relaunch
// that Interrupt's fallback or an internal resume starts. It notices when Claude Code exits, and
// decides whether that is the whole session ending or just the old half of a relaunch that is
// already under way (see turn.go's relaunchForResume and forceCancelFallback).
func (s *session) watchThisProcess(p *proc.Process, stdoutDone <-chan struct{}) {
	<-p.Done()
	<-stdoutDone
	s.mu.Lock()
	if s.proc != p {
		// A relaunch already replaced this process; its own watcher owns reporting now.
		s.mu.Unlock()
		return
	}
	s.ended = true
	s.endedInTurn = s.endedInTurn || s.turnActive
	stopping, resuming := s.stopping, s.resumeNeeded
	s.mu.Unlock()
	if !stopping && resuming {
		// The next Send starts a fresh process; this generation's end is not the session's end.
		return
	}
	s.finish(p.Wait(), p.StderrTail())
}

// finish ends the session once: it reports how the process ended, releases anything waiting to
// send an event, closes the event channel, forgets the session, and lets its lifetime end.
func (s *session) finish(exit proc.Exit, stderrTail string) {
	s.finishOnce.Do(func() {
		s.mu.Lock()
		s.stopInterruptTimer()
		s.mu.Unlock()
		s.report(exit, stderrTail)
		s.releaseSenders()
		s.closeEvents()
		s.adapter.forget(s.id, s)
		s.lifeCancel()
		close(s.done)
	})
}

// report sends the last events: a Failed event when the process ended without being asked to,
// and then Exited.
func (s *session) report(exit proc.Exit, stderrTail string) {
	s.mu.Lock()
	expected := s.stopping
	inTurn := s.endedInTurn
	s.mu.Unlock()
	if expected {
		s.log.Info("claude code session ended", "exit_code", exit.Code)
	} else {
		s.log.Warn("claude code exited without being stopped", "exit_code", exit.Code)
		s.emitFinal(agents.Failed{Message: crashMessage(inTurn), Detail: crashDetail(exit, stderrTail)})
	}
	s.emitFinal(agents.Exited{Code: exit.Code, Err: exit.Err})
}

// emit puts an event on the channel. It waits for room, and gives up when the sender is released
// or the channel has been closed, so a late event is dropped rather than sent on a closed channel.
func (s *session) emit(ev agents.AgentEvent) { s.sink.Emit(ev) }

// emitFinal sends an event that must not be lost when there is room for it, even after senders
// have been released. It only waits for a reader when the channel is full.
func (s *session) emitFinal(ev agents.AgentEvent) { s.sink.EmitFinal(ev) }

// releaseSenders lets every blocked emit give up.
func (s *session) releaseSenders() { s.sink.Release() }

// closeEvents closes the event channel once, after every sender is done or released.
func (s *session) closeEvents() { s.sink.Close() }

// writeStdin serializes writes to Claude Code's standard input: Send and Interrupt can both write
// while a turn runs, and lines must not interleave.
func (s *session) writeStdin(line []byte) error {
	s.stdinMu.Lock()
	defer s.stdinMu.Unlock()
	s.mu.Lock()
	p := s.proc
	s.mu.Unlock()
	if p == nil || p.Stdin == nil {
		return agents.ErrStopped
	}
	if _, err := p.Stdin.Write(line); err != nil {
		return fmt.Errorf("write to claude code: %w", err)
	}
	return nil
}

// nextControlRequestID returns an id for one control_request, unique within the session.
func (s *session) nextControlRequestID() string {
	s.mu.Lock()
	s.controlSeq++
	n := s.controlSeq
	s.mu.Unlock()
	return "marshal-" + strconv.FormatUint(n, 10)
}

// crashMessage is the plain sentence for a process that ended without being asked to.
func crashMessage(inTurn bool) string {
	if inTurn {
		return "The agent stopped in the middle of its work."
	}
	return "The agent stopped unexpectedly."
}

// crashDetail describes how the process ended, and what it printed last.
func crashDetail(exit proc.Exit, stderrTail string) string {
	detail := "exit code " + strconv.Itoa(exit.Code)
	if exit.Err != nil {
		detail = exit.Err.Error()
	}
	if stderrTail != "" {
		detail += "\n" + stderrTail
	}
	return detail
}

// tailSuffix formats what a process printed last for a one-line error, or "" when it printed
// nothing.
func tailSuffix(stderrTail string) string {
	tail := strings.TrimSpace(endOf(stderrTail, errorTailBytes))
	if tail == "" {
		return ""
	}
	return fmt.Sprintf(" (claude code printed: %s)", tail)
}

// endOf returns the last n bytes of text, without a broken character at the start.
func endOf(text string, n int) string {
	if len(text) <= n {
		return text
	}
	cut := len(text) - n
	for cut < len(text) && !utf8.RuneStart(text[cut]) {
		cut++
	}
	return text[cut:]
}

// readLine reads one line from r, growing past bufio's usual limits so a large tool result does
// not break the stream. The trailing newline is not included.
func readLine(r *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		chunk, isPrefix, err := r.ReadLine()
		line = append(line, chunk...)
		if !isPrefix || err != nil {
			return line, err
		}
	}
}

// readLoop reads newline-delimited JSON from the process's standard output until it ends. It is
// the only reader of stdout, and the only thing that closes ready and stdoutDone.
func (s *session) readLoop(p *proc.Process, stdoutDone, ready chan struct{}) {
	defer close(stdoutDone)
	defer func() {
		if err := p.Stdout.Close(); err != nil {
			s.log.Debug("closing claude code's output", "err", err)
		}
	}()
	closeReady := sync.OnceFunc(func() { close(ready) })
	r := bufio.NewReaderSize(p.Stdout, initialLineBufferBytes)
	for {
		line, err := readLine(r)
		if len(line) > 0 {
			s.handleLine(line, closeReady)
		}
		if err != nil {
			return
		}
	}
}

// handleLine reads a line's type and dispatches it. A line whose type is not recognized is
// logged and skipped, never fatal.
func (s *session) handleLine(line []byte, closeReady func()) {
	var head lineHead
	if err := json.Unmarshal(line, &head); err != nil {
		s.log.Warn("could not read a line from claude code", "err", err)
		return
	}
	switch head.Type {
	case "system":
		s.handleSystem(head.Subtype, line, closeReady)
	case "stream_event":
		s.handleStreamEvent(line)
	case "assistant":
		s.handleAssistant(line)
	case "user":
		s.handleUser(line)
	case "result":
		s.handleResult(line)
	case "control_response":
		s.handleControlResponse(line)
	default:
		s.log.Debug("skipped a line from claude code", "type", head.Type)
	}
}
