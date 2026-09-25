package pty

import (
	"errors"
	"io"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

const (
	// chunkBuffer is how many reads wait for the batcher. When it is full the reader stops
	// reading, the terminal fills up, and the program waits, as it would for a slow screen.
	chunkBuffer = 32
	// eventBuffer is how many events wait in a session's channel for the caller. Each holds at
	// most MaxEventBytes.
	eventBuffer = 64
	// drainPoll, drainQuiet, and drainMax control the wait for the last output after the process
	// has exited: it ends when the reader has finished, when nothing new was read for
	// drainQuiet, or after drainMax.
	drainPoll  = 20 * time.Millisecond
	drainQuiet = 100 * time.Millisecond
	drainMax   = 2 * time.Second
)

// teeWriter serializes writes from every session to one writer, and gives up on it after the
// first failure so that a broken log does not slow every read down.
type teeWriter struct {
	mu     sync.Mutex
	w      io.Writer
	failed bool
}

// write sends p to the writer. It returns the error of the first failed write, once. After that
// it does nothing.
func (t *teeWriter) write(p []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.failed {
		return nil
	}
	if _, err := t.w.Write(p); err != nil {
		t.failed = true
		return err
	}
	return nil
}

// pump reads the terminal until it ends. Each read goes to the ring and the tee at once, so the
// snapshot is never behind, and then to the batcher. The pump owns the chunks channel and closes
// it when the terminal ends.
func (s *session) pump() {
	defer close(s.readerDone)
	defer close(s.chunks)
	buf := make([]byte, MaxEventBytes)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			s.bytesRead.Add(int64(n))
			s.ring.write(buf[:n])
			s.tee(buf[:n])
			chunk := append([]byte(nil), buf[:n]...)
			select {
			case s.chunks <- chunk:
			case <-s.abort:
			}
		}
		if err != nil {
			if !endOfOutput(err) {
				s.log.Warn("the terminal output ended with an error", "err", err)
			}
			return
		}
	}
}

// tee hands output to the adapter's tee, if it has one.
func (s *session) tee(p []byte) {
	if s.adapter.tee == nil {
		return
	}
	if err := s.adapter.tee.write(p); err != nil {
		s.log.Warn("the terminal log stopped: writing to it failed", "err", err)
	}
}

// endOfOutput says whether an error from reading the terminal is only its normal end. A Linux
// terminal reports EIO when the last program has closed it, others report end of file, and a
// terminal that we closed ourselves reports a closed file.
func endOfOutput(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) || errors.Is(err, syscall.EIO)
}

// batch joins reads into events. Output goes out when a full event has piled up, or
// FlushInterval after the first byte of a new batch, whichever comes first. It owns the end of
// the event stream before Exited: it returns after the last flush, once the pump has closed the
// chunks channel.
func (s *session) batch() {
	defer close(s.batcherDone)
	var pending []byte
	timer := time.NewTimer(FlushInterval)
	timer.Stop()
	var due <-chan time.Time
	for {
		select {
		case chunk, ok := <-s.chunks:
			if !ok {
				timer.Stop()
				s.flush(pending)
				return
			}
			if len(pending) == 0 {
				timer.Reset(FlushInterval)
				due = timer.C
			}
			pending = s.flushFull(append(pending, chunk...))
			if len(pending) == 0 {
				timer.Stop()
				due = nil
			}
		case <-due:
			s.flush(pending)
			pending, due = nil, nil
		}
	}
}

// flushFull sends every complete event that pending holds and returns what is left over.
func (s *session) flushFull(pending []byte) []byte {
	for len(pending) >= MaxEventBytes {
		s.emit(agents.TerminalOutput{Data: pending[:MaxEventBytes:MaxEventBytes]})
		pending = pending[MaxEventBytes:]
	}
	if len(pending) == 0 {
		return nil
	}
	return pending
}

// flush sends everything in pending, as one event or several.
func (s *session) flush(pending []byte) {
	pending = s.flushFull(pending)
	if len(pending) > 0 {
		s.emit(agents.TerminalOutput{Data: pending})
	}
}
