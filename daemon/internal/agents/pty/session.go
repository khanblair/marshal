package pty

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	gopty "github.com/aymanbagabas/go-pty"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/proc"
)

const (
	// killWait is how long Stop waits for the process to disappear after the hard kill.
	killWait = 5 * time.Second
	// finishWait is how long Stop waits for the session to finish after the process is gone.
	finishWait = 10 * time.Second
)

// session is one program running in a terminal. Three goroutines belong to it, and each has a
// way to stop: the pump ends when the terminal does, the batcher ends when the pump does, and the
// supervisor ends after both and after the process. The supervisor owns the end of life and is
// the only one that closes the event channel.
type session struct {
	adapter *Adapter
	log     *slog.Logger
	id      string
	label   string

	pty gopty.Pty
	cmd *gopty.Cmd
	pid int

	ring      *ring
	bytesRead atomic.Int64

	// sizeMu guards cols and rows, the size the terminal has now.
	sizeMu     sync.Mutex
	cols, rows int

	// events is what the caller reads. chunks carries reads from the pump to the batcher.
	events chan agents.AgentEvent
	chunks chan []byte
	// abort releases senders that wait for a reader who is not there.
	abort     chan struct{}
	abortOnce sync.Once

	// writeMu keeps the parts of one message together on the way in.
	writeMu   sync.Mutex
	closePTY  sync.Once
	politeRun sync.Once
	stopping  atomic.Bool

	// exited closes when the process has exited and been collected, and exit is valid after that.
	exited chan struct{}
	exit   agents.Exited
	// readerDone, batcherDone, and done close when the pump, the batcher, and the whole session
	// have finished. The event channel is closed by the time done is.
	readerDone  chan struct{}
	batcherDone chan struct{}
	done        chan struct{}
}

// launched is a program that has started in a terminal, and what it is called.
type launched struct {
	id, label string
	pty       gopty.Pty
	cmd       *gopty.Cmd
}

// newSession makes the session for a program that has started in the terminal.
func newSession(a *Adapter, l launched) *session {
	return &session{
		cols:        a.cfg.Cols,
		rows:        a.cfg.Rows,
		adapter:     a,
		log:         a.log.With("session_id", l.id, "label", l.label),
		id:          l.id,
		label:       l.label,
		pty:         l.pty,
		cmd:         l.cmd,
		pid:         l.cmd.Process.Pid,
		ring:        newRing(RingBytes),
		events:      make(chan agents.AgentEvent, eventBuffer),
		chunks:      make(chan []byte, chunkBuffer),
		abort:       make(chan struct{}),
		exited:      make(chan struct{}),
		readerDone:  make(chan struct{}),
		batcherDone: make(chan struct{}),
		done:        make(chan struct{}),
	}
}

// begin starts the session's goroutines.
func (s *session) begin() {
	go s.pump()
	go s.batch()
	go s.supervise()
}

// emit puts an event on the channel. It takes a free slot at once, and otherwise waits for the
// reader, until the senders are released. Only the batcher and then the supervisor send, one
// after the other, so events stay in order and the channel is never closed under a sender.
func (s *session) emit(ev agents.AgentEvent) {
	select {
	case s.events <- ev:
		return
	default:
	}
	select {
	case s.events <- ev:
	case <-s.abort:
	}
}

// emitLast puts the last event on the channel, and makes sure that it gets there. When the
// senders have been released and nobody reads, it drops the oldest output to make room: the
// reader learns that the session ended, and only stale output is lost.
func (s *session) emitLast(ev agents.AgentEvent) {
	for {
		select {
		case s.events <- ev:
			return
		default:
		}
		select {
		case s.events <- ev:
			return
		case <-s.abort:
			select {
			case <-s.events:
			default:
			}
		}
	}
}

// releaseSenders lets every blocked send give up.
func (s *session) releaseSenders() {
	s.abortOnce.Do(func() { close(s.abort) })
}

// supervise owns the end of the session: it waits for the process, lets the output be read to the
// end, reports the exit, and closes the event channel.
func (s *session) supervise() {
	defer close(s.done)
	err := s.cmd.Wait()
	s.exit = agents.Exited{Code: exitCode(s.cmd, err), Err: err}
	close(s.exited)
	// Whatever the program left running in its group could hold the terminal open.
	proc.Sweep(s.pid)
	s.drain()
	s.closeTerminal()
	<-s.readerDone
	<-s.batcherDone
	if s.stopping.Load() {
		s.log.Info("terminal session ended", "exit_code", s.exit.Code)
	} else {
		s.log.Info("terminal program exited by itself", "exit_code", s.exit.Code)
	}
	s.emitLast(s.exit)
	s.releaseSenders()
	// Forget first, so that a reader who sees the channel close finds the session unknown.
	s.adapter.forget(s.id, s)
	close(s.events)
}

// exitCode reads the code from the finished command, or -1 when it has none or a signal ended it.
func exitCode(cmd *gopty.Cmd, err error) int {
	if cmd.ProcessState == nil {
		return -1
	}
	if err == nil {
		return 0
	}
	return cmd.ProcessState.ExitCode()
}

// drain waits for the last output after the process has exited. On Unix the reader sees the end
// at once. A pseudo console on Windows only ends when it is closed, so there the wait ends when
// the output has been quiet for a moment.
func (s *session) drain() {
	ticker := time.NewTicker(drainPoll)
	defer ticker.Stop()
	last := s.bytesRead.Load()
	idle, total := time.Duration(0), time.Duration(0)
	for {
		select {
		case <-s.readerDone:
			return
		case <-ticker.C:
		}
		total += drainPoll
		if now := s.bytesRead.Load(); now != last {
			last, idle = now, 0
		} else {
			idle += drainPoll
		}
		if idle >= drainQuiet || total >= drainMax {
			return
		}
	}
}

// closeTerminal closes the terminal once. That is what makes a blocked read or write return.
func (s *session) closeTerminal() {
	s.closePTY.Do(func() {
		err := s.pty.Close()
		if err != nil && !errors.Is(err, os.ErrClosed) {
			s.log.Warn("closing the terminal failed", "err", err)
		}
	})
}

// write sends bytes to the program's input. A message goes in as one write, so two callers cannot
// mix their bytes.
func (s *session) write(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("write to the terminal: %w", err)
	}
	if s.isExited() {
		return fmt.Errorf("write to the terminal: %w", agents.ErrStopped)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := s.pty.Write(data); err != nil {
		if s.isExited() {
			return fmt.Errorf("write to the terminal: %w", agents.ErrStopped)
		}
		return fmt.Errorf("write to the terminal: %w", err)
	}
	return nil
}

// interrupt types Ctrl-C. It does not wait for other writers: it is a single byte, and it has to
// get through when a long message is stuck because the program does not read.
func (s *session) interrupt(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("interrupt: %w", err)
	}
	if s.isExited() {
		return fmt.Errorf("interrupt: %w", agents.ErrStopped)
	}
	if _, err := s.pty.Write([]byte{interruptByte}); err != nil {
		if s.isExited() {
			return fmt.Errorf("interrupt: %w", agents.ErrStopped)
		}
		return fmt.Errorf("interrupt: %w", err)
	}
	return nil
}

// resize tells the program that its terminal changed size.
func (s *session) resize(cols, rows int) error {
	if s.isExited() {
		return fmt.Errorf("resize the terminal: %w", agents.ErrStopped)
	}
	if err := s.pty.Resize(cols, rows); err != nil {
		return fmt.Errorf("resize the terminal: %w", err)
	}
	s.sizeMu.Lock()
	s.cols, s.rows = cols, rows
	s.sizeMu.Unlock()
	return nil
}

// size is the size the terminal has now.
func (s *session) size() (cols, rows int) {
	s.sizeMu.Lock()
	defer s.sizeMu.Unlock()
	return s.cols, s.rows
}

// isExited says whether the process has exited.
func (s *session) isExited() bool {
	select {
	case <-s.exited:
		return true
	default:
		return false
	}
}

// stop ends the process and everything it started, and waits until the session is over. It asks
// politely first, waits up to the grace period, and then kills the whole process tree. It is safe
// to call more than once and from several goroutines.
func (s *session) stop(ctx context.Context) error {
	// A process that has been collected gets no signal: its process group is gone, and the id
	// could already belong to someone else.
	if !s.isExited() {
		if err := s.terminate(ctx); err != nil {
			return err
		}
	}
	// The process is gone. Whoever reads the events has had its chance, so a full channel no
	// longer holds the session back.
	s.releaseSenders()
	select {
	case <-s.done:
		return nil
	case <-time.After(finishWait):
		return fmt.Errorf("stop process %d: the terminal did not close", s.pid)
	}
}

// terminate ends a process that is still running: a polite request, a wait of the grace period,
// and then the kill of the whole process tree.
func (s *session) terminate(ctx context.Context) error {
	s.stopping.Store(true)
	s.politeRun.Do(s.askPolitely)
	if s.waitExited(ctx, s.adapter.cfg.StopGrace) {
		return nil
	}
	if err := proc.KillTree(s.pid); err != nil {
		s.log.Warn("killing the process tree failed", "err", err)
	}
	if !s.waitExited(context.Background(), killWait) {
		return fmt.Errorf("stop process %d: it did not exit after being killed", s.pid)
	}
	return nil
}

// askPolitely is the first, gentle step of stop.
func (s *session) askPolitely() {
	if err := proc.Polite(s.pid); err != nil {
		// A failed polite request only means that the hard kill comes sooner.
		s.log.Debug("asking the process to end failed", "err", err)
	}
	if closeOnPolite {
		s.closeTerminal()
	}
}

// waitExited reports whether the process exited before d passed or ctx ended.
func (s *session) waitExited(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-s.exited:
		return true
	case <-timer.C:
		return false
	case <-ctx.Done():
		return s.isExited()
	}
}
