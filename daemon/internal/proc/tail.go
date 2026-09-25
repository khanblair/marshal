package proc

import (
	"strings"
	"sync"
)

// tailBuffer keeps the last limit bytes that were written to it. It is safe to read while the
// child is still writing.
type tailBuffer struct {
	mu    sync.Mutex
	buf   []byte
	limit int
}

// Write keeps the end of what it is given. It never fails, so the child never blocks on it.
func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	written := len(p)
	if len(p) >= t.limit {
		t.buf = append(t.buf[:0], p[len(p)-t.limit:]...)
		return written, nil
	}
	t.buf = append(t.buf, p...)
	if over := len(t.buf) - t.limit; over > 0 {
		t.buf = append(t.buf[:0], t.buf[over:]...)
	}
	return written, nil
}

// String returns the kept text. Cutting at a byte count can split a character at the start, and
// that broken piece is dropped.
func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.ToValidUTF8(string(t.buf), "")
}
