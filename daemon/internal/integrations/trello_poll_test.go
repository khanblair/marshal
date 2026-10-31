package integrations_test

import (
	"context"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrations"
)

// The poller reads the linked board when it starts, on a timer, and when a webhook kicks it. A kicked
// read waits out a debounce of a few seconds, which no test waits for, so these cover the start, the
// stop, and what a kick does not do.

const (
	// pollWait is how long a test waits for a read it expects. A read takes milliseconds.
	pollWait = 3 * time.Second
	// pollQuiet is how long a test watches for a read it does not expect.
	pollQuiet = 200 * time.Millisecond
	// closeWithin is far less than the poller's own debounce, so a Close that waits it out fails.
	closeWithin = time.Second
)

// startedPoller builds a poller over the engine, starts it, and stops it before the store closes.
func startedPoller(t *testing.T, e *engine) *integrations.TrelloPoller {
	t.Helper()
	p, err := integrations.NewTrelloPoller(e.f.svc)
	if err != nil {
		t.Fatalf("NewTrelloPoller: %v", err)
	}
	p.Start(context.Background())
	t.Cleanup(func() { _ = p.Close() })
	return p
}

// waitRead waits for the next read of the board to finish, which is when the sync tells the event
// stream the project changed.
func (e *engine) waitRead(t *testing.T) {
	t.Helper()
	select {
	case <-e.notified:
	case <-time.After(pollWait):
		t.Fatal("the board was not read")
	}
}

func (e *engine) wantNoRead(t *testing.T) {
	t.Helper()
	select {
	case <-e.notified:
		t.Error("the board was read when nothing asked for it")
	case <-time.After(pollQuiet):
	}
}

func TestThePollerNeedsAServiceWithAStore(t *testing.T) {
	if p, err := integrations.NewTrelloPoller(nil); err == nil || p != nil {
		t.Errorf("NewTrelloPoller(nil) = %v, %v, want a refusal", p, err)
	}
}

func TestKickAndCloseBeforeStartAreHarmless(t *testing.T) {
	e := newEngine(t)
	p, err := integrations.NewTrelloPoller(e.f.svc)
	if err != nil {
		t.Fatalf("NewTrelloPoller: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 5 {
			p.Kick() // The wait for a read is a buffer of one, so a kick never blocks.
		}
		if err := p.Close(); err != nil {
			t.Errorf("Close before Start: %v", err)
		}
	}()

	select {
	case <-done:
	case <-time.After(pollWait):
		t.Fatal("a kick or a close before Start blocked")
	}
	if got := e.trello.count("GET", "/boards/board-1"); got != 0 {
		t.Errorf("the board was read %d times by a poller that never started", got)
	}
}

func TestStartReadsTheBoardOnceAndASecondStartIsANoOp(t *testing.T) {
	e := newEngine(t)
	seedFixLogin(e.trello)
	p := startedPoller(t, e)

	e.waitRead(t)
	p.Start(context.Background())
	e.wantNoRead(t)

	if got := e.trello.count("GET", "/boards/board-1"); got != 1 {
		t.Errorf("the board was read %d times, want once for the start", got)
	}
	if got := len(e.board.order); got != 1 {
		t.Errorf("the start imported %d cards, want the 1 on the board", got)
	}
}

func TestClosingDuringTheDebounceReturnsAtOnce(t *testing.T) {
	e := newEngine(t)
	p := startedPoller(t, e)
	e.waitRead(t)
	p.Kick()

	closed := make(chan error, 1)
	go func() { closed <- p.Close() }()

	select {
	case err := <-closed:
		if err != nil {
			t.Errorf("Close: %v", err)
		}
	case <-time.After(closeWithin):
		t.Fatal("Close waited for the debounce")
	}
	if got := e.trello.count("GET", "/boards/board-1"); got != 1 {
		t.Errorf("the board was read %d times, want only the read at the start", got)
	}
}

func TestAPollerWithNoBoardLinkedReadsNothing(t *testing.T) {
	tr := newFakeTrello(t)
	f := newFixture(t, func(o *integrations.Options) { o.TrelloBaseURL = tr.URL })
	p, err := integrations.NewTrelloPoller(f.svc)
	if err != nil {
		t.Fatalf("NewTrelloPoller: %v", err)
	}
	p.Start(context.Background())

	p.Kick()
	time.Sleep(pollQuiet)
	if err := p.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}

	if got := tr.count("GET", "/boards/board-1"); got != 0 {
		t.Errorf("the board was read %d times with no board linked", got)
	}
}
