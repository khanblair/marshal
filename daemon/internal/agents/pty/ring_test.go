package pty

import (
	"bytes"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRing(t *testing.T) {
	tests := []struct {
		name   string
		size   int
		writes []string
		want   string
	}{
		{name: "empty", size: 8, want: ""},
		{name: "less than the size", size: 8, writes: []string{"abc"}, want: "abc"},
		{name: "exactly the size", size: 4, writes: []string{"abcd"}, want: "abcd"},
		{name: "wraps around", size: 4, writes: []string{"abc", "def"}, want: "cdef"},
		{name: "wraps around twice", size: 4, writes: []string{"abc", "def", "ghi", "j"}, want: "ghij"},
		{name: "one write larger than the size", size: 4, writes: []string{"ab", "0123456789"}, want: "6789"},
		{name: "a write of exactly the size after others", size: 4, writes: []string{"ab", "wxyz"}, want: "wxyz"},
		{name: "many one byte writes", size: 3, writes: []string{"a", "b", "c", "d", "e"}, want: "cde"},
		{name: "an empty write changes nothing", size: 4, writes: []string{"ab", "", "c"}, want: "abc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newRing(tc.size)
			for _, w := range tc.writes {
				r.write([]byte(w))
			}
			if got := string(r.snapshot()); got != tc.want {
				t.Errorf("snapshot = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRingSnapshotIsACopy(t *testing.T) {
	r := newRing(8)
	r.write([]byte("abcd"))
	snap := r.snapshot()
	r.write([]byte("efgh"))
	if string(snap) != "abcd" {
		t.Errorf("the snapshot changed to %q after a later write", snap)
	}
}

func TestRingMatchesASimpleModelUnderManyWrites(t *testing.T) {
	const size = 37
	r := newRing(size)
	var model []byte
	for i := range 500 {
		chunk := bytes.Repeat([]byte{byte('a' + i%26)}, 1+i%53)
		r.write(chunk)
		model = append(model, chunk...)
		if len(model) > size {
			model = model[len(model)-size:]
		}
		if got := r.snapshot(); !bytes.Equal(got, model) {
			t.Fatalf("after write %d: snapshot %q, model %q", i, got, model)
		}
	}
}

// TestBurstKeepsMemoryFlat pushes a very large output through a terminal and checks that the ring
// stays at its bound, every event stays small, and the heap does not grow with the output.
func TestBurstKeepsMemoryFlat(t *testing.T) {
	total := 50 << 20
	if runtime.GOOS == "windows" {
		// ConPTY redraws what it is given and is much slower than a Unix terminal.
		total = 8 << 20
	}
	const heapGrowthLimit = 16 << 20

	r := startRun(t, Config{}, "burst", strconv.Itoa(total), "hold")
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	var peak atomic.Uint64
	stop := make(chan struct{})
	var sampler sync.WaitGroup
	sampler.Add(1)
	go func() {
		defer sampler.Done()
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				var ms runtime.MemStats
				runtime.ReadMemStats(&ms)
				if ms.HeapAlloc > peak.Load() {
					peak.Store(ms.HeapAlloc)
				}
			}
		}
	}()
	r.waitForWithin("BURST-END", 5*time.Minute)
	close(stop)
	sampler.Wait()

	// ConPTY redraws the text, so the byte count is only exact on Unix.
	if r.total < total && runtime.GOOS != "windows" {
		t.Errorf("%d bytes arrived, want at least %d", r.total, total)
	}
	if r.total == 0 {
		t.Error("no output arrived")
	}
	if r.maxEvent > MaxEventBytes {
		t.Errorf("an event held %d bytes, the limit is %d", r.maxEvent, MaxEventBytes)
	}
	snap := r.a.Snapshot(r.h)
	if len(snap) > RingBytes {
		t.Errorf("the ring holds %d bytes, the bound is %d", len(snap), RingBytes)
	}
	if runtime.GOOS != "windows" {
		if len(snap) != RingBytes || !bytes.HasSuffix(snap, []byte("BURST-END\r\n")) {
			t.Errorf("the ring holds %d bytes ending with %q, want the last %d bytes of the output",
				len(snap), tailOf(snap, 20), RingBytes)
		}
	}
	growth := int64(peak.Load()) - int64(before.HeapAlloc)
	t.Logf("the heap grew by at most %d KiB while %d MiB went through the terminal in %d events",
		growth>>10, total>>20, r.count)
	if growth > heapGrowthLimit {
		t.Errorf("the heap grew by %d MiB while %d MiB went through the terminal",
			growth>>20, total>>20)
	}
}
