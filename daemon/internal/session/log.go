package session

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// sessionLog is one session's on-disk log: JSON-lines segments under sessionLogDir, rotated once
// a segment passes maxBytes. Old segments are kept: retention and cleanup are a later concern, not
// this package's (docs/backend-checklist.md B1.11 asks only that memory, not disk, stays flat).
// Writes are buffered and flushed at least once a second, or at once for a state-changing event
// (write's own flush parameter), so a crash loses at most a second of chat text, not the session.
type sessionLog struct {
	dir      string
	maxBytes int64

	mu      sync.Mutex
	file    *os.File
	writer  *bufio.Writer
	segment int
	written int64
	closed  bool

	stop chan struct{}
	done chan struct{}
}

// newSessionLog opens a session's log folder, continuing its highest-numbered segment if one
// already exists (a resumed session's folder is not new), or starting at 0001.jsonl. maxBytes is
// Config.LogSegmentBytes.
func newSessionLog(dataDir, sessionID string, maxBytes int64) (*sessionLog, error) {
	dir := sessionLogDir(dataDir, sessionID)
	if err := os.MkdirAll(dir, sessionLogDirMode); err != nil {
		return nil, fmt.Errorf("make the session log folder %s: %w", dir, err)
	}
	segment, err := latestSegment(dir)
	if err != nil {
		return nil, err
	}
	l := &sessionLog{dir: dir, maxBytes: maxBytes, stop: make(chan struct{}), done: make(chan struct{})}
	if err := l.openSegment(segment); err != nil {
		return nil, err
	}
	go l.flushLoop()
	return l, nil
}

// segmentName is the file name of a segment, for example "0001.jsonl".
func segmentName(n int) string { return fmt.Sprintf("%04d.jsonl", n) }

// latestSegment returns the highest segment number already in dir, or 1 if the folder holds none
// yet.
func latestSegment(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("read the session log folder %s: %w", dir, err)
	}
	highest := 0
	for _, entry := range entries {
		n, ok := segmentNumber(entry.Name())
		if ok && n > highest {
			highest = n
		}
	}
	if highest == 0 {
		return 1, nil
	}
	return highest, nil
}

// segmentNumber reads the number out of a segment file name, if name is one.
func segmentNumber(name string) (int, bool) {
	base, ok := strings.CutSuffix(name, ".jsonl")
	if !ok || len(base) != 4 {
		return 0, false
	}
	n, err := strconv.Atoi(base)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// openSegment opens (or creates) segment n for appending and measures what is already in it.
func (l *sessionLog) openSegment(n int) error {
	path := filepath.Join(l.dir, segmentName(n))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, sessionLogFileMode)
	if err != nil {
		return fmt.Errorf("open the session log segment %s: %w", path, err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("read the session log segment %s: %w", path, err)
	}
	l.file, l.writer, l.segment, l.written = file, bufio.NewWriter(file), n, info.Size()
	return nil
}

// write appends one already-newline-terminated line, rotating to a new segment first if line
// would push the current one past maxBytes. flush forces the buffer to disk at once, for a
// state-changing event (TurnEnded, Exited); other events wait for the flush timer.
func (l *sessionLog) write(line []byte, flush bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	if l.written > 0 && l.written+int64(len(line)) > l.maxBytes {
		if err := l.rotateLocked(); err != nil {
			return err
		}
	}
	n, err := l.writer.Write(line)
	l.written += int64(n)
	if err != nil {
		return fmt.Errorf("write to the session log %s: %w", l.dir, err)
	}
	if flush {
		return l.flushLocked()
	}
	return nil
}

func (l *sessionLog) rotateLocked() error {
	if err := l.flushLocked(); err != nil {
		return err
	}
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("close the session log segment %s: %w", segmentName(l.segment), err)
	}
	return l.openSegment(l.segment + 1)
}

func (l *sessionLog) flushLocked() error {
	if err := l.writer.Flush(); err != nil {
		return fmt.Errorf("flush the session log %s: %w", l.dir, err)
	}
	return nil
}

// flushLoop flushes the buffer on a timer until stop is closed.
func (l *sessionLog) flushLoop() {
	defer close(l.done)
	ticker := time.NewTicker(logFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			l.mu.Lock()
			if !l.closed {
				_ = l.flushLocked()
			}
			l.mu.Unlock()
		case <-l.stop:
			return
		}
	}
}

// close flushes and closes the current segment, and stops the flush timer. It is safe to call
// more than once.
func (l *sessionLog) close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	flushErr := l.flushLocked()
	closeErr := l.file.Close()
	l.mu.Unlock()
	close(l.stop)
	<-l.done
	return errors.Join(flushErr, closeErr)
}
