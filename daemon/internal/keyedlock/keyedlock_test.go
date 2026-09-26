package keyedlock_test

import (
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/keyedlock"
)

// Two callers of one key are never inside at once, and a key that is free is forgotten.
func TestOneKeyIsHeldByOneCallerAtATime(t *testing.T) {
	var locks keyedlock.Locks
	var inside, most int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := locks.Lock("chat-1")
			defer unlock()
			mu.Lock()
			inside++
			most = max(most, inside)
			mu.Unlock()
			time.Sleep(time.Millisecond)
			mu.Lock()
			inside--
			mu.Unlock()
		}()
	}
	wg.Wait()
	if most != 1 {
		t.Errorf("%d callers were inside the lock of one key at once, want 1", most)
	}
	if n := locks.Held(); n != 0 {
		t.Errorf("%d keys are still remembered after every caller left, want 0", n)
	}
}

// Another key is not held up by a key that is held.
func TestKeysDoNotWaitForEachOther(t *testing.T) {
	var locks keyedlock.Locks
	unlockOne := locks.Lock("one")
	done := make(chan struct{})
	go func() {
		unlockTwo := locks.Lock("two")
		unlockTwo()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a caller of another key waited for a held key")
	}
	if n := locks.Held(); n != 1 {
		t.Errorf("%d keys are remembered while one is held, want 1", n)
	}
	unlockOne()
	if n := locks.Held(); n != 0 {
		t.Errorf("%d keys are remembered after the last unlock, want 0", n)
	}
}
