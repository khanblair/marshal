package protocol_test

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// An opaque id carries the time it was made, so a stored row that keeps only its id (an approval,
// architecture.md section 10) still reports when it happened.
func TestIDTimeReadsBackTheTimeAnIDWasMade(t *testing.T) {
	now := time.Date(2026, 9, 26, 21, 30, 15, 123_000_000, time.UTC)
	id, err := protocol.NewID(now, rand.Reader)
	if err != nil {
		t.Fatalf("make an id: %v", err)
	}
	got, ok := protocol.IDTime(id)
	if !ok {
		t.Fatalf("IDTime(%q) said it is not an id", id)
	}
	if !got.Equal(now) {
		t.Errorf("IDTime = %s, want %s", got.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	}

	// An id made a millisecond later sorts after it, which is why a row needs no time of its own.
	later, err := protocol.NewID(now.Add(time.Millisecond), rand.Reader)
	if err != nil {
		t.Fatalf("make a later id: %v", err)
	}
	if id >= later {
		t.Errorf("ids did not sort by time: %q then %q", id, later)
	}
}

func TestIDTimeRejectsWhatIsNotAnID(t *testing.T) {
	if _, ok := protocol.IDTime("not-an-id"); ok {
		t.Error("IDTime accepted something that is not an id")
	}
}
