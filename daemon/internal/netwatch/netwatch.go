// Package netwatch tells the rest of the daemon whether this computer can reach the internet, and lets
// a caller wait until it can. It costs nothing while things work: it does not probe on a timer. A
// caller that hit a network error reports it, and only then does the monitor check, with a short
// connection to a well-known address. While the check keeps failing the monitor is offline, and it
// keeps checking with growing waits until one succeeds.
//
// What it decides is only "can I reach the internet", not "is this one service up". A failure that
// the check cannot reproduce (the service was down, the key was refused) leaves the monitor online.
package netwatch

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Timings of the check. They are fields of Options so a test can run without waiting.
const (
	// firstWait is the wait before the second check while offline. It doubles from there.
	firstWait = 2 * time.Second
	// maxWait caps the wait between checks however long the outage lasts.
	maxWait = 30 * time.Second
	// recheckGap is how soon after a successful check a new report is ignored, so a service that is
	// down while the internet is fine does not turn every failed call into a new check.
	recheckGap = 10 * time.Second
	// dialTimeout bounds one connection attempt of the default check.
	dialTimeout = 3 * time.Second
)

// DefaultTargets are the addresses the default check connects to. A connection to any one of them
// means the internet is reachable. Nothing is sent: the connection is closed as soon as it opens.
// MARSHAL_NETWORK_PROBE replaces them, or turns the check off with "off".
var DefaultTargets = []string{"1.1.1.1:443", "8.8.8.8:443"}

// Status is whether the computer can reach the internet, and since when that has been true.
type Status struct {
	Online bool
	Since  time.Time
}

// Options sets up a Monitor. Every field is optional.
type Options struct {
	// Probe says whether the internet is reachable now. The default connects to DefaultTargets.
	Probe func(ctx context.Context) bool
	// Sleep waits between checks while offline, or ends early when the context is done.
	Sleep func(ctx context.Context, d time.Duration) error
	// Now is the clock. The default is time.Now.
	Now func() time.Time
	// Logger receives one line for each change. The default is slog.Default().
	Logger *slog.Logger
}

// Monitor follows whether the internet is reachable. It is safe for concurrent use.
type Monitor struct {
	ctx   context.Context
	probe func(ctx context.Context) bool
	sleep func(ctx context.Context, d time.Duration) error
	now   func() time.Time
	log   *slog.Logger

	mu        sync.Mutex
	online    bool
	since     time.Time
	probing   bool
	lastOK    time.Time
	changed   chan struct{}
	listeners []func(Status)
}

// New makes a monitor that assumes the internet is reachable until a failure is reported. ctx ends
// its checking; a monitor whose context is done answers what it last knew.
func New(ctx context.Context, opts Options) *Monitor {
	m := &Monitor{
		ctx: ctx, probe: opts.Probe, sleep: opts.Sleep, now: opts.Now, log: opts.Logger,
		online: true, changed: make(chan struct{}),
	}
	if m.probe == nil {
		m.probe = TargetProbe(DefaultTargets)
	}
	if m.sleep == nil {
		m.sleep = sleepFor
	}
	if m.now == nil {
		m.now = time.Now
	}
	if m.log == nil {
		m.log = slog.Default()
	}
	m.since = m.now()
	return m
}

// A nil *Monitor is a monitor that is always online and checks nothing, so a caller that was built
// without one needs no guard around each call.

// Status answers whether the internet is reachable, as last known.
func (m *Monitor) Status() Status {
	if m == nil {
		return Status{Online: true}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return Status{Online: m.online, Since: m.since}
}

// Online reports whether the internet is reachable, as last known.
func (m *Monitor) Online() bool { return m.Status().Online }

// Subscribe calls fn each time the monitor goes offline or comes back. fn runs on the goroutine that
// found the change, so it must not block: start a goroutine for anything slow.
func (m *Monitor) Subscribe(fn func(Status)) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, fn)
}

// ReportFailure tells the monitor a call failed with a network error. It returns at once. If the
// monitor was not already checking, and did not just confirm the internet is fine, it checks in the
// background and flips to offline when the check fails.
func (m *Monitor) ReportFailure() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.probing || (m.online && m.now().Sub(m.lastOK) < recheckGap && !m.lastOK.IsZero()) {
		m.mu.Unlock()
		return
	}
	m.probing = true
	m.mu.Unlock()
	go m.watch(false)
}

// Check says whether the internet is reachable now, and tells the monitor a call just failed with a
// network error. It is the question a caller asks before it decides a failure was the network's: a
// failure aimed at one server (a local model that is not running, a service that is down) leaves the
// internet reachable, and Check says so. It answers at once when the monitor is already offline, or
// confirmed the internet a moment ago; otherwise it makes one connection attempt, which takes a few
// seconds at most. When it finds the internet gone it goes offline and keeps checking in the
// background until the internet is back. A cancelled context answers true: nothing was learned.
func (m *Monitor) Check(ctx context.Context) bool {
	if m == nil {
		return true
	}
	m.mu.Lock()
	switch {
	case !m.online:
		m.mu.Unlock()
		return false
	case !m.lastOK.IsZero() && m.now().Sub(m.lastOK) < recheckGap:
		m.mu.Unlock()
		return true
	}
	m.mu.Unlock()
	if m.probe(ctx) {
		m.mu.Lock()
		m.lastOK = m.now()
		m.mu.Unlock()
		return true
	}
	if ctx.Err() != nil || m.ctx.Err() != nil {
		return true
	}
	m.set(false)
	m.mu.Lock()
	start := !m.probing
	m.probing = true
	m.mu.Unlock()
	if start {
		go m.watch(true)
	}
	return false
}

// MarkOnline tells the monitor a call to the internet just worked, which is proof it is reachable. A
// monitor that was offline because its own check cannot get out (a network that only allows a proxy,
// say) comes back this way instead of waiting for a check that will never pass.
func (m *Monitor) MarkOnline() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.lastOK = m.now()
	m.mu.Unlock()
	m.set(true)
}

// watch checks, and while the internet is unreachable keeps checking with growing waits until it is.
// With sleepFirst it waits before the first check, because the caller has just made one. It is the
// only place the state changes back to online.
func (m *Monitor) watch(sleepFirst bool) {
	defer func() {
		m.mu.Lock()
		m.probing = false
		m.mu.Unlock()
	}()
	wait := firstWait
	for {
		if m.ctx.Err() != nil {
			return
		}
		if sleepFirst {
			sleepFirst = false
			if err := m.sleep(m.ctx, wait); err != nil {
				return
			}
			wait = min(wait*2, maxWait)
			continue
		}
		if m.probe(m.ctx) {
			m.mu.Lock()
			m.lastOK = m.now()
			m.mu.Unlock()
			m.set(true)
			return
		}
		if m.ctx.Err() != nil {
			// The daemon is shutting down: a check that was cut short says nothing about the internet.
			return
		}
		m.set(false)
		if err := m.sleep(m.ctx, wait); err != nil {
			return
		}
		wait = min(wait*2, maxWait)
	}
}

// set records the state and, when it changed, wakes the waiters and tells the listeners.
func (m *Monitor) set(online bool) {
	m.mu.Lock()
	if m.online == online {
		m.mu.Unlock()
		return
	}
	m.online, m.since = online, m.now()
	status := Status{Online: m.online, Since: m.since}
	close(m.changed)
	m.changed = make(chan struct{})
	listeners := append([]func(Status){}, m.listeners...)
	m.mu.Unlock()
	if online {
		m.log.Info("the internet is reachable again")
	} else {
		m.log.Warn("the internet is not reachable")
	}
	for _, fn := range listeners {
		fn(status)
	}
}

// WaitOnline blocks until the internet is reachable, and answers true. It answers false when max
// passes first or ctx is done. It answers true at once when the monitor is already online.
func (m *Monitor) WaitOnline(ctx context.Context, max time.Duration) bool {
	if m == nil {
		return ctx.Err() == nil
	}
	var expire <-chan time.Time
	if max > 0 {
		timer := time.NewTimer(max)
		defer timer.Stop()
		expire = timer.C
	}
	for {
		m.mu.Lock()
		online, changed := m.online, m.changed
		m.mu.Unlock()
		if online {
			return true
		}
		select {
		case <-changed:
		case <-expire:
			return false
		case <-ctx.Done():
			return false
		}
	}
}

// sleepFor is the real wait: a timer the context can cut short.
func sleepFor(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TargetProbe makes the default check: connect to each address at the same time and answer true as
// soon as one opens. Nothing is written to the connection. An empty list, or "off", answers true
// always, which is how a person turns the monitor off.
func TargetProbe(targets []string) func(ctx context.Context) bool {
	if len(targets) == 0 {
		return func(context.Context) bool { return true }
	}
	return func(ctx context.Context) bool {
		ctx, cancel := context.WithTimeout(ctx, dialTimeout)
		defer cancel()
		ok := make(chan bool, len(targets))
		for _, target := range targets {
			go func() {
				var d net.Dialer
				conn, err := d.DialContext(ctx, "tcp", target)
				if err == nil {
					_ = conn.Close()
				}
				ok <- err == nil
			}()
		}
		for range targets {
			if <-ok {
				return true
			}
		}
		return false
	}
}

// ParseTargets reads MARSHAL_NETWORK_PROBE: a comma-separated list of host:port addresses, or "off".
// An empty value is the default targets.
func ParseTargets(value string) []string {
	value = strings.TrimSpace(value)
	switch strings.ToLower(value) {
	case "":
		return DefaultTargets
	case "off":
		return nil
	}
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// connectionWords are pieces of an error's text that mean a connection could not be made or was
// lost, for errors that a library flattened to plain text so errors.As cannot see what they were.
var connectionWords = []string{
	"no such host", "connection refused", "network is unreachable", "no route to host",
	"i/o timeout", "tls handshake timeout", "connection reset by peer",
}

// agentWords are more of the ways an agent describes the network failing, in the words it prints. They
// are only read from an agent's own message, never from an error a library returned.
var agentWords = []string{"unable to connect", "fetch failed", "econnreset", "enotfound", "econnrefused", "etimedout"}

// IsNetworkError reports whether err looks like the network failing: no route, no answer, a dropped
// connection. It is a first filter, not a verdict: an error aimed at one server, such as a local model
// that is not running, looks the same, so a caller asks Monitor.Check before it believes the internet
// is down. It is not a certificate or address error, which no wait will fix, and not a cancelled call
// or a deadline the caller set itself.
func IsNetworkError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if isPermanent(err) {
		return false
	}
	// An error from a connection is a *net.OpError, a failed lookup is a *net.DNSError, and a request
	// that ran out of time inside the HTTP client is a *url.Error that says it timed out. A bare
	// system error (a missing file, a broken pipe to a program) also looks like a net.Error to the
	// compiler, so the interface is not used.
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return !isLocal(opErr.Addr)
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return true
	}
	for _, errno := range []error{syscall.ECONNRESET, syscall.ECONNREFUSED, syscall.ENETUNREACH, syscall.EHOSTUNREACH} {
		if errors.Is(err, errno) {
			return true
		}
	}
	return hasWords(err.Error(), connectionWords)
}

// HasNetworkWords reports whether text says the network failed. It is for a message that came back as
// words, such as the last line an agent printed as it failed.
func HasNetworkWords(text string) bool {
	return hasWords(text, connectionWords) || hasWords(text, agentWords)
}

func hasWords(text string, words []string) bool {
	text = strings.ToLower(text)
	for _, word := range words {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

// isPermanent says err is a failure no wait will fix: a certificate that cannot be trusted, an address
// that cannot be read, a scheme nothing speaks.
func isPermanent(err error) bool {
	var unknown x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	var host x509.HostnameError
	var verify *tls.CertificateVerificationError
	if errors.As(err, &unknown) || errors.As(err, &invalid) || errors.As(err, &host) || errors.As(err, &verify) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Op == "parse" || strings.Contains(urlErr.Err.Error(), "unsupported protocol scheme")
	}
	return false
}

// isLocal says an address is this computer: a connection error to it is a program that is not
// running, not the internet failing.
func isLocal(addr net.Addr) bool {
	if addr == nil {
		return false
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		host = addr.String()
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
