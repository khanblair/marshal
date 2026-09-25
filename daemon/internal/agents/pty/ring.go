package pty

import "sync"

// ring keeps the last bytes that were written to it, and nothing older. Its memory is fixed when
// it is made, so a program that prints without end cannot make a session grow.
type ring struct {
	mu    sync.Mutex
	buf   []byte
	start int // index of the oldest byte
	n     int // how many bytes are held
}

// newRing returns a ring that holds up to size bytes.
func newRing(size int) *ring {
	return &ring{buf: make([]byte, size)}
}

// write adds p to the end, dropping the oldest bytes when there is no room.
func (r *ring) write(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	size := len(r.buf)
	if len(p) >= size {
		copy(r.buf, p[len(p)-size:])
		r.start, r.n = 0, size
		return
	}
	end := (r.start + r.n) % size
	first := copy(r.buf[end:], p)
	copy(r.buf, p[first:])
	if total := r.n + len(p); total > size {
		r.start = (r.start + total - size) % size
		r.n = size
	} else {
		r.n = total
	}
}

// snapshot returns a copy of what the ring holds, oldest byte first.
func (r *ring) snapshot() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]byte, r.n)
	first := copy(out, r.buf[r.start:min(r.start+r.n, len(r.buf))])
	copy(out[first:], r.buf[:r.n-first])
	return out
}
