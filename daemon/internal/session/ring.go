package session

import "sync"

// entryRing keeps the most recent LogEntry values of a live session, bounded by a rough estimate
// of their encoded size rather than their count: a size in bytes is what Config.RingBytes
// documents, and entries vary a lot in size (a one-word message chunk against a full tool diff).
// Unlike agents/pty's ring, which holds raw terminal bytes, this ring holds structured entries and
// drops the oldest whole entry when it is over budget, never truncating one in half.
type entryRing struct {
	mu       sync.Mutex
	maxBytes int
	curBytes int
	entries  []LogEntry
	sizes    []int
}

// newEntryRing returns a ring bounded by maxBytes, which is at least 1.
func newEntryRing(maxBytes int) *entryRing {
	return &entryRing{maxBytes: max(maxBytes, 1)}
}

// add appends an entry of the given estimated size, then drops the oldest entries until the ring
// is back within its budget. At least one entry is always kept, even one that alone is over
// budget, so a single huge entry cannot make the ring report nothing at all.
func (r *entryRing) add(entry LogEntry, size int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, entry)
	r.sizes = append(r.sizes, size)
	r.curBytes += size
	for r.curBytes > r.maxBytes && len(r.entries) > 1 {
		r.curBytes -= r.sizes[0]
		r.entries, r.sizes = r.entries[1:], r.sizes[1:]
	}
}

// snapshot returns a copy of what the ring holds, oldest first.
func (r *entryRing) snapshot() []LogEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]LogEntry, len(r.entries))
	copy(out, r.entries)
	return out
}
