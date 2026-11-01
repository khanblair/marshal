package main

import (
	"context"
	"errors"
	"net"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/netwatch"
	"github.com/khanblair/marshal/daemon/internal/outbox"
	"github.com/khanblair/marshal/daemon/internal/store"
)

func openBox(t *testing.T) *outbox.Service {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "marshal.db"), store.WithLogger(nil))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return outbox.New(st, outbox.Options{})
}

var errDown = &net.OpError{Op: "dial", Err: errors.New("network is unreachable")}

func pending(t *testing.T, box *outbox.Service) int {
	t.Helper()
	n, err := box.Pending(context.Background())
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	return n
}

func TestANetworkFailureIsSavedAndAnythingElseIsLeftToTheCaller(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		err       error
		withBox   bool
		wantSaved bool
		wantInBox int
	}{
		{"the network failed", errDown, true, true, 1},
		{"no error", nil, true, false, 0},
		{"a service refused it", errors.New("the bot token is wrong"), true, false, 0},
		{"the network failed but there is no outbox", errDown, false, false, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			box := openBox(t)
			var use *outbox.Service
			if tc.withBox {
				use = box
			}
			saved, err := saveIfOffline(ctx, use, nil, outbox.KindNotice, "ntfy", "hello", tc.err)
			if saved != tc.wantSaved || err != nil {
				t.Errorf("saveIfOffline = %v, %v; want saved=%v and no error", saved, err, tc.wantSaved)
			}
			if got := pending(t, box); got != tc.wantInBox {
				t.Errorf("the outbox holds %d, want %d", got, tc.wantInBox)
			}
		})
	}
}

// A message saved while offline is sent by the handler registered for its kind once the internet is
// back, and a late one says so.
func TestASavedNoticeGoesOutLateWithANoteWhenTheInternetReturns(t *testing.T) {
	ctx := context.Background()
	box := openBox(t)
	if saved, err := saveIfOffline(ctx, box, nil, outbox.KindNotice, "ntfy", map[string]string{"Title": "Card needs you", "Body": "Pick one."}, errDown); !saved || err != nil {
		t.Fatalf("saveIfOffline = %v, %v", saved, err)
	}
	var sentBody string
	box.Register(outbox.KindNotice, func(_ context.Context, item outbox.Item) error {
		sentBody = string(item.Payload)
		return nil
	})
	result, err := box.Flush(ctx)
	if err != nil || result.Sent != 1 || !strings.Contains(sentBody, "Card needs you") {
		t.Errorf("flush = %+v, %v with %q, want the saved notice sent", result, err, sentBody)
	}
}

// A late message says so in its title, which a channel never cuts short, and leaves its body alone.
func TestALateMessageSaysSoInItsTitle(t *testing.T) {
	if got := lateTitle("Card api#41 needs you"); got != "Card api#41 needs you (sent late)" {
		t.Errorf("lateTitle = %q", got)
	}
}

// A network-looking error is saved only when the monitor says the internet is down. When it is up, the
// error is one server's or one address's problem, and saving it would only make the outbox retry it.
func TestANetworkLookingErrorIsNotSavedWhileTheInternetIsReachable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	var up atomic.Bool
	var skew atomic.Int64
	monitor := netwatch.New(ctx, netwatch.Options{
		Probe: func(context.Context) bool { return up.Load() },
		Sleep: func(ctx context.Context, _ time.Duration) error { return ctx.Err() },
		// A clock the test can move, because a check that just passed is trusted for a few seconds.
		Now: func() time.Time { return time.Unix(1_800_000_000+skew.Load(), 0) },
	})
	box := openBox(t)

	up.Store(true)
	saved, err := saveIfOffline(ctx, box, monitor, outbox.KindNotice, "ntfy", "x", errDown)
	if saved || err != nil || pending(t, box) != 0 {
		t.Errorf("with the internet up: saved=%v err=%v pending=%d, want nothing saved", saved, err, pending(t, box))
	}

	up.Store(false)
	skew.Add(60)
	saved, err = saveIfOffline(ctx, box, monitor, outbox.KindNotice, "ntfy", "x", errDown)
	if !saved || err != nil || pending(t, box) != 1 {
		t.Errorf("with the internet down: saved=%v err=%v pending=%d, want it saved", saved, err, pending(t, box))
	}
}

// An error that is not the network is never saved, whatever the monitor says.
func TestAPermanentErrorIsNeverSaved(t *testing.T) {
	box := openBox(t)
	for _, err := range []error{
		errors.New("the bot token is wrong"),
		&url.Error{Op: "Post", URL: "ftp://x", Err: errors.New(`unsupported protocol scheme "ftp"`)},
		context.DeadlineExceeded,
	} {
		if saved, _ := saveIfOffline(context.Background(), box, nil, outbox.KindNotice, "ntfy", "x", err); saved {
			t.Errorf("%v was saved, want it left to the caller", err)
		}
	}
	if pending(t, box) != 0 {
		t.Errorf("the outbox holds %d, want none", pending(t, box))
	}
}
