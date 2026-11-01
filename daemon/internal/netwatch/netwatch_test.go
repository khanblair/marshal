package netwatch

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// script answers the probe from a list, then repeats its last answer. It counts the calls.
type script struct {
	mu      sync.Mutex
	answers []bool
	calls   int
}

func (s *script) probe(context.Context) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := min(s.calls, len(s.answers)-1)
	s.calls++
	return s.answers[i]
}

func (s *script) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// newTest makes a monitor with no waiting: its sleep returns at once and records what it was asked for.
func newTest(t *testing.T, answers ...bool) (*Monitor, *script, *[]time.Duration) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := &script{answers: answers}
	var mu sync.Mutex
	var slept []time.Duration
	m := New(ctx, Options{
		Probe: s.probe,
		Sleep: func(ctx context.Context, d time.Duration) error {
			mu.Lock()
			slept = append(slept, d)
			mu.Unlock()
			return ctx.Err()
		},
	})
	return m, s, &slept
}

// changes collects the monitor's changes on a channel.
func changes(m *Monitor) <-chan Status {
	out := make(chan Status, 8)
	m.Subscribe(func(s Status) { out <- s })
	return out
}

func next(t *testing.T, ch <-chan Status) Status {
	t.Helper()
	select {
	case s := <-ch:
		return s
	case <-time.After(2 * time.Second):
		t.Fatal("no change arrived")
		return Status{}
	}
}

func TestAMonitorStartsOnlineAndDoesNotProbeOnItsOwn(t *testing.T) {
	m, s, _ := newTest(t, false)
	if !m.Online() {
		t.Error("a new monitor is offline, want online")
	}
	time.Sleep(30 * time.Millisecond)
	if s.count() != 0 {
		t.Errorf("the probe ran %d times with nothing reported, want none", s.count())
	}
}

func TestAReportedFailureThatTheProbeCannotRepeatLeavesTheMonitorOnline(t *testing.T) {
	m, s, _ := newTest(t, true)
	got := changes(m)
	m.ReportFailure()
	deadline := time.Now().Add(2 * time.Second)
	for s.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	select {
	case c := <-got:
		t.Fatalf("a change %+v was announced for a false alarm", c)
	case <-time.After(50 * time.Millisecond):
	}
	if !m.Online() {
		t.Error("the monitor went offline for a failure the probe could not repeat")
	}
}

func TestTheMonitorGoesOfflineWhenTheProbeFailsAndComesBackWithGrowingWaits(t *testing.T) {
	m, _, slept := newTest(t, false, false, false, true)
	got := changes(m)
	m.ReportFailure()

	if c := next(t, got); c.Online {
		t.Fatalf("first change = %+v, want offline", c)
	}
	if c := next(t, got); !c.Online {
		t.Fatalf("second change = %+v, want online", c)
	}
	if !m.Online() {
		t.Error("the monitor is offline after the probe succeeded")
	}
	want := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}
	if fmt.Sprint(*slept) != fmt.Sprint(want) {
		t.Errorf("waits = %v, want %v", *slept, want)
	}
}

func TestTheWaitBetweenChecksIsCapped(t *testing.T) {
	answers := make([]bool, 10)
	answers = append(answers, true)
	m, _, slept := newTest(t, answers...)
	got := changes(m)
	m.ReportFailure()
	next(t, got)
	next(t, got)
	for _, d := range *slept {
		if d > maxWait {
			t.Errorf("a wait of %v is past the cap of %v", d, maxWait)
		}
	}
	if last := (*slept)[len(*slept)-1]; last != maxWait {
		t.Errorf("last wait = %v, want the cap %v", last, maxWait)
	}
}

func TestOnlyOneCheckRunsAtATime(t *testing.T) {
	m, s, _ := newTest(t, false, false, true)
	got := changes(m)
	for range 20 {
		m.ReportFailure()
	}
	next(t, got)
	next(t, got)
	if s.count() > 3 {
		t.Errorf("the probe ran %d times for 20 reports, want the one loop's 3", s.count())
	}
}

func TestWaitOnlineReturnsAtOnceWhenOnlineAndWaitsWhenNot(t *testing.T) {
	m, _, _ := newTest(t, false, true)
	if !m.WaitOnline(context.Background(), time.Second) {
		t.Fatal("WaitOnline answered false for a monitor that is online")
	}
	m.set(false)
	var done atomic.Bool
	go func() {
		m.WaitOnline(context.Background(), 5*time.Second)
		done.Store(true)
	}()
	time.Sleep(30 * time.Millisecond)
	if done.Load() {
		t.Fatal("WaitOnline returned while offline")
	}
	m.set(true)
	deadline := time.Now().Add(2 * time.Second)
	for !done.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !done.Load() {
		t.Error("WaitOnline did not return after the monitor came back")
	}
}

func TestWaitOnlineGivesUpAtItsLimitOrWhenCancelled(t *testing.T) {
	m, _, _ := newTest(t, true)
	m.set(false)
	if m.WaitOnline(context.Background(), 30*time.Millisecond) {
		t.Error("WaitOnline answered true after its limit with the monitor still offline")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if m.WaitOnline(ctx, time.Minute) {
		t.Error("WaitOnline answered true for a cancelled context")
	}
}

func TestIsNetworkErrorTellsTheNetworkFromAServiceThatAnswered(t *testing.T) {
	timeout := &net.OpError{Op: "dial", Err: errTimeout{}}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"no error", nil, false},
		{"a cancelled call", context.Canceled, false},
		{"a dial error", timeout, true},
		{"a DNS failure", &net.DNSError{Err: "no such host", Name: "api.example.com"}, true},
		{"a wrapped refusal", fmt.Errorf("call it: %w", &net.OpError{Op: "dial", Err: errors.New("connection refused")}), true},
		{"flattened text", errors.New("Post https://x: dial tcp: lookup x: no such host"), true},
		{"a refused key", errors.New("401 invalid x-api-key"), false},
		{"a plain failure", errors.New("the card has no worktree"), false},
		{"a local server that is not running", &net.OpError{Op: "dial", Addr: tcpAddr("127.0.0.1:11434"), Err: syscall.ECONNREFUSED}, false},
		{"a connection refused by a remote server", &net.OpError{Op: "dial", Addr: tcpAddr("203.0.113.9:443"), Err: syscall.ECONNREFUSED}, true},
		{"a deadline the caller set", context.DeadlineExceeded, false},
		{"a timeout inside the HTTP client", &url.Error{Op: "Post", URL: "https://x", Err: errTimeout{}}, true},
		{"an address that cannot be read", &url.Error{Op: "parse", URL: "::bad", Err: errors.New("missing protocol scheme")}, false},
		{"a scheme nothing speaks", &url.Error{Op: "Post", URL: "ftp://x", Err: errors.New(`unsupported protocol scheme "ftp"`)}, false},
		{"a certificate that cannot be trusted", &url.Error{Op: "Post", URL: "https://x", Err: x509.UnknownAuthorityError{}}, false},
		{"a broken pipe", fmt.Errorf("write: %w", syscall.EPIPE), false},
		{"words only an agent prints", errors.New("API Error: Unable to connect to API"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsNetworkError(tc.err); got != tc.want {
				t.Errorf("IsNetworkError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func tcpAddr(hostport string) net.Addr {
	a, err := net.ResolveTCPAddr("tcp", hostport)
	if err != nil {
		panic(err)
	}
	return a
}

type errTimeout struct{}

func (errTimeout) Error() string   { return "i/o timeout" }
func (errTimeout) Timeout() bool   { return true }
func (errTimeout) Temporary() bool { return true }

func TestANilMonitorIsAlwaysOnline(t *testing.T) {
	var m *Monitor
	m.ReportFailure()
	m.Subscribe(func(Status) { t.Error("a nil monitor announced a change") })
	if !m.Online() || !m.Status().Online || !m.WaitOnline(context.Background(), time.Second) {
		t.Error("a nil monitor must read as online and never make a caller wait")
	}
}

func TestHasNetworkWordsReadsWhatAnAgentPrints(t *testing.T) {
	for text, want := range map[string]bool{
		"API Error: Unable to connect to API (ConnectionRefused)": true,
		"Connection reset by peer":                                true,
		"fetch failed":                                            true,
		"error_during_execution":                                  false,
		"the build failed":                                        false,
	} {
		if got := HasNetworkWords(text); got != want {
			t.Errorf("HasNetworkWords(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestCheckConfirmsTheInternetWithOneProbeAndIsNotRepeatedForAMoment(t *testing.T) {
	m, s, _ := newTest(t, true)
	if !m.Check(context.Background()) || s.count() != 1 {
		t.Fatalf("first Check = false or probed %d times, want true after one probe", s.count())
	}
	if !m.Check(context.Background()) || s.count() != 1 {
		t.Errorf("second Check probed again (%d probes), want the answer from a moment ago", s.count())
	}
	if !m.Online() {
		t.Error("the monitor went offline although the probe passed")
	}
}

func TestCheckFindsTheInternetGoneGoesOfflineAndKeepsWatchingUntilItIsBack(t *testing.T) {
	m, s, _ := newTest(t, false, false, true)
	got := changes(m)
	if m.Check(context.Background()) {
		t.Fatal("Check answered true although the probe failed")
	}
	if c := next(t, got); c.Online {
		t.Fatalf("first change = %+v, want offline", c)
	}
	if c := next(t, got); !c.Online {
		t.Fatalf("second change = %+v, want online once the background watch succeeds", c)
	}
	if s.count() != 3 {
		t.Errorf("probes = %d, want the Check's own and two from the watch", s.count())
	}
	// Offline answers at once, without another probe.
	m.set(false)
	before := s.count()
	if m.Check(context.Background()) || s.count() != before {
		t.Errorf("Check while offline = true or probed (%d -> %d), want false with no probe", before, s.count())
	}
}

func TestCheckOnACancelledContextLearnsNothing(t *testing.T) {
	m, _, _ := newTest(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !m.Check(ctx) || !m.Online() {
		t.Error("a cancelled Check changed the monitor or answered false")
	}
}

func TestAProbeCutShortByShutdownDoesNotAnnounceOffline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	probe := func(context.Context) bool {
		cancel()
		return false
	}
	m := New(ctx, Options{Probe: probe, Sleep: func(context.Context, time.Duration) error { return context.Canceled }})
	got := changes(m)
	m.ReportFailure()
	select {
	case c := <-got:
		t.Fatalf("a change %+v was announced as the daemon shut down", c)
	case <-time.After(100 * time.Millisecond):
	}
	if !m.Online() {
		t.Error("the monitor read offline because its check was cut short")
	}
}

func TestMarkOnlineBringsAnOfflineMonitorBackWhateverItsOwnCheckSays(t *testing.T) {
	m, _, _ := newTest(t, false)
	got := changes(m)
	m.set(false)
	next(t, got)
	m.MarkOnline()
	if c := next(t, got); !c.Online {
		t.Errorf("change = %+v, want online", c)
	}
	var nilMonitor *Monitor
	nilMonitor.MarkOnline()
}

func TestParseTargetsReadsTheOverrideAndOff(t *testing.T) {
	if got := ParseTargets(""); len(got) != len(DefaultTargets) {
		t.Errorf("empty = %v, want the defaults", got)
	}
	if got := ParseTargets(" OFF "); got != nil {
		t.Errorf("off = %v, want no targets", got)
	}
	if got := ParseTargets("a:1, b:2 ,,"); fmt.Sprint(got) != "[a:1 b:2]" {
		t.Errorf("list = %v", got)
	}
	if !TargetProbe(nil)(context.Background()) {
		t.Error("a monitor with no targets must read as always online")
	}
}
