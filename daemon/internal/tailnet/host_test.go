package tailnet

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// The JSON below has the shape of Tailscale's own answers, with made-up names, addresses and accounts.
const statusRunning = `{
  "BackendState": "Running",
  "Self": {"DNSName": "laptop.tail1234.ts.net.", "TailscaleIPs": ["100.64.0.1", "fd7a::1"], "UserID": 42},
  "User": {"42": {"LoginName": "owner@example.com"}},
  "CurrentTailnet": {"Name": "owner@example.com"},
  "Peer": {
    "a": {"HostName": "Pixel", "OS": "android", "Online": false, "LastSeen": "2026-09-24T10:00:00Z"},
    "b": {"HostName": "iPhone", "OS": "iOS", "Online": true, "LastSeen": "2026-10-03T10:00:00Z"},
    "c": {"HostName": "desk", "OS": "macOS", "Online": true, "LastSeen": "2026-10-03T10:00:00Z"},
    "d": {"HostName": "Tablet", "OS": "android", "Online": false, "LastSeen": "0001-01-01T00:00:00Z"}
  }
}`

const serveNone = `{}`

const servePlain = `{
  "TCP": {"3200": {"HTTPS": true}, "47800": {"HTTP": true}},
  "Web": {
    "laptop.tail1234.ts.net:3200": {"Handlers": {"/": {"Proxy": "http://localhost:3200"}}},
    "laptop.tail1234.ts.net:47800": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:47801"}}}
  }
}`

const serveSecureOnly = `{
  "TCP": {"443": {"HTTPS": true}},
  "Web": {"laptop.tail1234.ts.net:443": {"Handlers": {"/": {"Proxy": "http://localhost:47801"}}}}
}`

const serveForward = `{"TCP": {"47801": {"TCPForward": "127.0.0.1:47801"}}}`

const serveOtherPort = `{
  "TCP": {"47800": {"HTTP": true}},
  "Web": {"laptop.tail1234.ts.net:47800": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:9000"}}}}
}`

// fakeTailscale answers the two commands with canned JSON and counts the calls.
type fakeTailscale struct {
	mu     sync.Mutex
	status string
	serve  string
	calls  int
	probed []string
	up     bool
}

func (f *fakeTailscale) run(_ context.Context, _ string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	switch strings.Join(args, " ") {
	case "status --json":
		if f.status == "" {
			return nil, errors.New("Tailscale is not running")
		}
		return []byte(f.status), nil
	case "serve status --json":
		if f.serve == "" {
			return nil, errors.New("unknown command")
		}
		return []byte(f.serve), nil
	}
	return nil, errors.New("unexpected arguments")
}

func (f *fakeTailscale) probe(_ context.Context, address string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.probed = append(f.probed, address)
	return f.up
}

func hostOf(f *fakeTailscale, found bool) *Host {
	find := func() string { return "" }
	if found {
		find = func() string { return "/usr/local/bin/tailscale" }
	}
	return NewHostWith(f.run, find, f.probe, time.Now)
}

func TestWithNoTailscaleInstalledNothingIsFoundAndTheListsAreEmptyNotNull(t *testing.T) {
	got := hostOf(&fakeTailscale{}, false).Status(context.Background(), 47801)
	if got.Found || got.State != "" || got.IPs == nil || got.TakenPorts == nil || got.Phones == nil {
		t.Fatalf("host = %+v, want nothing found and empty lists", got)
	}
}

func TestATailscaleThatCannotBeReachedIsNotRunning(t *testing.T) {
	got := hostOf(&fakeTailscale{}, true).Status(context.Background(), 47801)
	if !got.Found || got.State != hostNotRunning {
		t.Fatalf("host = %+v, want found and not running", got)
	}
}

func TestARunningTailscaleSaysWhoAndWhatAndWhichPhonesAreOnline(t *testing.T) {
	f := &fakeTailscale{status: statusRunning, serve: serveNone}
	got := hostOf(f, true).Status(context.Background(), 47801)
	if got.State != hostRunning || got.DNSName != "laptop.tail1234.ts.net" || got.Account != "owner@example.com" || got.Tailnet != "owner@example.com" {
		t.Fatalf("host = %+v", got)
	}
	if len(got.IPs) != 2 || got.IPs[0] != "100.64.0.1" {
		t.Errorf("ips = %v", got.IPs)
	}
	names := []string{}
	for _, phone := range got.Phones {
		names = append(names, phone.Name)
	}
	if strings.Join(names, ",") != "iPhone,Pixel,Tablet" {
		t.Errorf("phones = %v, want the online phone first, then by name, and no computer", names)
	}
	if got.Phones[0].LastSeen == nil || got.Phones[2].LastSeen != nil {
		t.Errorf("last seen = %v and %v, want a time for the iPhone and none for a phone that never says", got.Phones[0].LastSeen, got.Phones[2].LastSeen)
	}
	if got.ServePort != 0 || got.Reachable || len(f.probed) != 0 {
		t.Errorf("no rule hands a port to the daemon, yet serve = %d reachable = %v probed = %v", got.ServePort, got.Reachable, f.probed)
	}
}

func TestAPlainServeRuleToTheDaemonIsFoundProbedAndTheOtherPortsAreTaken(t *testing.T) {
	f := &fakeTailscale{status: statusRunning, serve: servePlain, up: true}
	got := hostOf(f, true).Status(context.Background(), 47801)
	if got.ServePort != 47800 || got.SecureServePort != 0 || !got.Reachable {
		t.Fatalf("host = %+v, want tailnet port 47800 reachable", got)
	}
	if len(got.TakenPorts) != 1 || got.TakenPorts[0] != 3200 {
		t.Errorf("taken = %v, want only the other rule's port", got.TakenPorts)
	}
	if len(f.probed) != 1 || f.probed[0] != "http://laptop.tail1234.ts.net:47800/v1/health" {
		t.Errorf("probed = %v, want the daemon's health through the tailnet name", f.probed)
	}
}

func TestARuleThatDoesNotAnswerIsFoundButNotReachable(t *testing.T) {
	f := &fakeTailscale{status: statusRunning, serve: servePlain, up: false}
	got := hostOf(f, true).Status(context.Background(), 47801)
	if got.ServePort != 47800 || got.Reachable {
		t.Fatalf("host = %+v, want the rule found and not reachable", got)
	}
}

func TestAnHTTPSOnlyRuleIsNotUsableByThePhoneApp(t *testing.T) {
	f := &fakeTailscale{status: statusRunning, serve: serveSecureOnly, up: true}
	got := hostOf(f, true).Status(context.Background(), 47801)
	if got.ServePort != 0 || got.SecureServePort != 443 || got.Reachable || len(f.probed) != 0 {
		t.Fatalf("host = %+v probed = %v, want a secure-only rule noted and nothing probed", got, f.probed)
	}
}

func TestARawTCPForwardToTheDaemonCounts(t *testing.T) {
	f := &fakeTailscale{status: statusRunning, serve: serveForward, up: true}
	if got := hostOf(f, true).Status(context.Background(), 47801); got.ServePort != 47801 || !got.Reachable {
		t.Fatalf("host = %+v, want the forward on 47801", got)
	}
}

func TestARuleToSomeOtherLocalPortIsJustATakenPort(t *testing.T) {
	f := &fakeTailscale{status: statusRunning, serve: serveOtherPort, up: true}
	got := hostOf(f, true).Status(context.Background(), 47801)
	if got.ServePort != 0 || len(got.TakenPorts) != 1 || got.TakenPorts[0] != 47800 {
		t.Fatalf("host = %+v, want port 47800 taken by a rule that does not lead to the daemon", got)
	}
}

func TestATailscaleThatIsNotRunningYetIsReportedByItsState(t *testing.T) {
	for backend, want := range map[string]string{
		"NeedsLogin": hostNeedsLogin, "Stopped": hostStopped, "Starting": hostStarting,
		"NeedsMachineAuth": hostNeedsApprove, "Surprise": hostUnknown,
	} {
		f := &fakeTailscale{status: strings.Replace(statusRunning, `"Running"`, `"`+backend+`"`, 1), serve: servePlain, up: true}
		got := hostOf(f, true).Status(context.Background(), 47801)
		if got.State != want || got.ServePort != 0 || len(f.probed) != 0 {
			t.Errorf("%s: host = %+v, want %q with no serve read", backend, got, want)
		}
	}
}

func TestATailscaleWithNoServeListingStillAnswers(t *testing.T) {
	f := &fakeTailscale{status: statusRunning}
	got := hostOf(f, true).Status(context.Background(), 47801)
	if got.State != hostRunning || got.ServePort != 0 {
		t.Fatalf("host = %+v, want running with no rules", got)
	}
}

func TestOneAnswerIsKeptForAFewSecondsAndAskedAgainAfterwards(t *testing.T) {
	f := &fakeTailscale{status: statusRunning, serve: serveNone}
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	host := NewHostWith(f.run, func() string { return "tailscale" }, f.probe, func() time.Time { return now })
	host.Status(context.Background(), 47801)
	host.Status(context.Background(), 47801)
	if f.calls != 2 {
		t.Fatalf("calls = %d, want the two commands run once for two asks", f.calls)
	}
	host.Status(context.Background(), 47802)
	if f.calls != 4 {
		t.Errorf("calls = %d, want another port to ask again", f.calls)
	}
	now = now.Add(hostCacheTTL + time.Second)
	host.Status(context.Background(), 47802)
	if f.calls != 6 {
		t.Errorf("calls = %d, want an old answer to be asked for again", f.calls)
	}
}
