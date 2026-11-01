package providers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"
)

// networkDown is what a provider adapter answers when the connection itself failed: unavailable,
// wrapping a dial error to a remote address.
func networkDown() error {
	addr, _ := net.ResolveTCPAddr("tcp", "203.0.113.9:443")
	return fmt.Errorf("%w: %w", ErrUnavailable, &net.OpError{Op: "dial", Addr: addr, Err: errors.New("network is unreachable")})
}

// fakeNet is the internet as a test wants it. reachable is what Check answers; a wait for the internet
// sets it, unless timesOut says the wait runs out first.
type fakeNet struct {
	reachable bool
	checks    int
	waits     int
	marked    int
	timesOut  bool
}

func (f *fakeNet) Check(context.Context) bool {
	f.checks++
	return f.reachable
}

func (f *fakeNet) WaitOnline(context.Context, time.Duration) bool {
	f.waits++
	if f.timesOut {
		return false
	}
	f.reachable = true
	return true
}

func (f *fakeNet) MarkOnline() { f.marked++ }

// failTimes answers the given error that many times, then a value, and counts the calls.
func failTimes(n int, err error) (func() (string, error), *int) {
	calls := 0
	return func() (string, error) {
		calls++
		if calls <= n {
			return "", err
		}
		return "ok", nil
	}, &calls
}

func noSleep(context.Context, time.Duration) error { return nil }

func TestWithNothingWatchingANetworkFailureIsTriedManyMoreTimesThanAProviderError(t *testing.T) {
	pol := retryPolicy{attempts: defaultAttempts, backoff: time.Millisecond}

	attempt, calls := failTimes(8, networkDown())
	got, err := withRetry(context.Background(), pol, noSleep, attempt)
	if err != nil || got != "ok" || *calls != 9 {
		t.Errorf("a network failure: got %q, %v after %d calls; want ok on the 9th", got, err, *calls)
	}

	attempt, calls = failTimes(100, ErrUnavailable)
	_, err = withRetry(context.Background(), pol, noSleep, attempt)
	if !errors.Is(err, ErrUnavailable) || *calls != defaultOtherAttempts {
		t.Errorf("a provider error: %v after %d calls; want it to stop after %d", err, *calls, defaultOtherAttempts)
	}
}

func TestANetworkFailureStopsAtTheAttemptCapToo(t *testing.T) {
	attempt, calls := failTimes(100, networkDown())
	_, err := withRetry(context.Background(), retryPolicy{attempts: 6, backoff: time.Millisecond}, noSleep, attempt)
	if !errors.Is(err, ErrUnavailable) || *calls != 6 {
		t.Errorf("got %v after %d calls, want the error after 6", err, *calls)
	}
}

func TestNetworkWaitsGrowFromOneSecondAndStopAtTheCap(t *testing.T) {
	var waits []time.Duration
	sleep := func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		return nil
	}
	attempt, _ := failTimes(100, networkDown())
	_, _ = withRetry(context.Background(), retryPolicy{attempts: 9, backoff: time.Millisecond}, sleep, attempt)
	want := []time.Duration{1, 2, 4, 8, 16, 30, 30, 30}
	for i, w := range want {
		if i >= len(waits) || waits[i] != w*time.Second {
			t.Fatalf("waits = %v, want %v seconds", waits, want)
		}
	}
}

// A request aimed at a server that is down while the internet is fine, such as a local model that is
// not running, is a short failure: the monitor says the internet is there, so there is nothing to wait for.
func TestANetworkLookingFailureWithTheInternetFineIsAShortFailure(t *testing.T) {
	n := &fakeNet{reachable: true}
	attempt, calls := failTimes(100, networkDown())
	_, err := withRetry(context.Background(), retryPolicy{attempts: 12, backoff: time.Millisecond, net: n}, noSleep, attempt)
	if !errors.Is(err, ErrUnavailable) || *calls != defaultOtherAttempts {
		t.Errorf("got %v after %d calls, want the error after %d short tries", err, *calls, defaultOtherAttempts)
	}
	if n.waits != 0 {
		t.Errorf("waited for the internet %d times while it was reachable, want none", n.waits)
	}
}

func TestARequestWaitsForTheInternetInsteadOfUsingItsTriesUp(t *testing.T) {
	n := &fakeNet{}
	attempt, calls := failTimes(1, networkDown())
	got, err := withRetry(context.Background(), retryPolicy{attempts: 2, backoff: time.Millisecond, net: n}, noSleep, attempt)
	if err != nil || got != "ok" || *calls != 2 {
		t.Fatalf("got %q, %v after %d calls; want ok on the second", got, err, *calls)
	}
	if n.waits != 1 || n.marked != 1 {
		t.Errorf("waited for the internet %d times and marked it online %d times, want 1 and 1", n.waits, n.marked)
	}
}

// A wait for the internet that runs out does not stop the request: it tries anyway, and a success
// proves the internet is there, which the monitor is told.
func TestAWaitThatRunsOutTriesAnywayAndASuccessMarksTheInternetOnline(t *testing.T) {
	n := &fakeNet{timesOut: true}
	attempt, calls := failTimes(2, networkDown())
	got, err := withRetry(context.Background(), retryPolicy{attempts: 12, backoff: time.Millisecond, net: n}, noSleep, attempt)
	if err != nil || got != "ok" || *calls != 3 {
		t.Fatalf("got %q, %v after %d calls; want ok on the third", got, err, *calls)
	}
	if n.waits != 2 || n.marked != 1 {
		t.Errorf("waits = %d and marks = %d, want 2 and 1", n.waits, n.marked)
	}
}

func TestTheBudgetEndsTheRetryOfANetworkFailure(t *testing.T) {
	attempt, calls := failTimes(100, networkDown())
	_, err := withRetry(context.Background(), retryPolicy{attempts: 12, backoff: time.Millisecond, budget: time.Nanosecond}, noSleep, attempt)
	if !errors.Is(err, ErrUnavailable) || *calls != 1 {
		t.Errorf("got %v after %d calls, want the error after one: the budget was spent", err, *calls)
	}
}

func TestTheBudgetAlsoEndsAWaitForTheInternet(t *testing.T) {
	n := &fakeNet{timesOut: true}
	attempt, calls := failTimes(100, networkDown())
	_, err := withRetry(context.Background(), retryPolicy{attempts: 12, backoff: time.Millisecond, budget: time.Nanosecond, net: n}, noSleep, attempt)
	if !errors.Is(err, ErrUnavailable) || *calls != 1 || n.waits != 0 {
		t.Errorf("got %v after %d calls and %d waits, want the error after one call and no wait", err, *calls, n.waits)
	}
}

func TestARefusedKeyIsNotRetriedEvenWhenTheNetworkIsDown(t *testing.T) {
	attempt, calls := failTimes(100, ErrAuth)
	_, err := withRetry(context.Background(), retryPolicy{attempts: 12, backoff: time.Millisecond, net: &fakeNet{}}, noSleep, attempt)
	if !errors.Is(err, ErrAuth) || *calls != 1 {
		t.Errorf("got %v after %d calls, want the refusal after one", err, *calls)
	}
}

func TestAnObserverSeesEachWaitAndThenTheEnd(t *testing.T) {
	var seen []RetryInfo
	ctx := WithRetryObserver(context.Background(), func(info RetryInfo) { seen = append(seen, info) })
	attempt, _ := failTimes(2, networkDown())
	if _, err := withRetry(ctx, retryPolicy{attempts: 12, backoff: time.Millisecond}, noSleep, attempt); err != nil {
		t.Fatal(err)
	}
	want := []RetryInfo{
		{Attempt: 2, Max: 12, Wait: time.Second, Network: true},
		{Attempt: 3, Max: 12, Wait: 2 * time.Second, Network: true},
		{},
	}
	if fmt.Sprint(seen) != fmt.Sprint(want) {
		t.Errorf("observed %+v, want %+v", seen, want)
	}
}

// A rate limit stops after a few tries, so its count says so instead of the network's longer one.
func TestAnObserverCountsAProviderErrorAgainstItsOwnShortLimit(t *testing.T) {
	var seen []RetryInfo
	ctx := WithRetryObserver(context.Background(), func(info RetryInfo) { seen = append(seen, info) })
	attempt, _ := failTimes(1, ErrRateLimited)
	if _, err := withRetry(ctx, retryPolicy{attempts: 12, backoff: time.Millisecond}, noSleep, attempt); err != nil {
		t.Fatal(err)
	}
	if len(seen) == 0 || seen[0].Max != defaultOtherAttempts || seen[0].Network {
		t.Errorf("observed %+v, want a count out of %d and not the network", seen, defaultOtherAttempts)
	}
}

func TestAnObserverIsToldWhenTheRequestWaitsForTheInternet(t *testing.T) {
	var seen []RetryInfo
	ctx := WithRetryObserver(context.Background(), func(info RetryInfo) { seen = append(seen, info) })
	attempt, _ := failTimes(1, networkDown())
	if _, err := withRetry(ctx, retryPolicy{attempts: 3, backoff: time.Millisecond, net: &fakeNet{}}, noSleep, attempt); err != nil {
		t.Fatal(err)
	}
	if len(seen) == 0 || !seen[0].Offline || !seen[0].Network {
		t.Errorf("observed %+v, want the first to say the internet is offline", seen)
	}
	if last := seen[len(seen)-1]; last != (RetryInfo{}) {
		t.Errorf("the last observation is %+v, want the zero value that says waiting is over", last)
	}
}
