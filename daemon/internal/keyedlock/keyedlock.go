// Package keyedlock hands out one lock per key. The session manager takes one per chat, so the
// messages sent to a chat that is not running yet start its session once, and the chats module takes
// one per chat, so a message and an archive or a delete of the same chat cannot interleave. A key
// is forgotten when nobody holds or waits for its lock, so the set of keys never grows.
package keyedlock

import "sync"

// Locks is a set of locks by key. The zero value is ready to use, and it is safe for use by many
// goroutines.
type Locks struct {
	mu    sync.Mutex
	locks map[string]*entry
}

// entry is one key's lock and how many callers hold or wait for it.
type entry struct {
	mu   sync.Mutex
	refs int
}

// Lock waits for the key's lock and returns the function that releases it. The function must be
// called exactly once.
func (l *Locks) Lock(key string) (unlock func()) {
	l.mu.Lock()
	if l.locks == nil {
		l.locks = make(map[string]*entry)
	}
	e := l.locks[key]
	if e == nil {
		e = &entry{}
		l.locks[key] = e
	}
	e.refs++
	l.mu.Unlock()

	e.mu.Lock()
	return func() {
		e.mu.Unlock()
		l.mu.Lock()
		e.refs--
		if e.refs == 0 {
			delete(l.locks, key)
		}
		l.mu.Unlock()
	}
}

// Held says how many keys have a lock that someone holds or waits for. It is for tests, to prove
// that a key is forgotten when its lock is free.
func (l *Locks) Held() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.locks)
}
