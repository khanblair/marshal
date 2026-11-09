package tailnet

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
)

type exposureRunner struct {
	serve  string
	funnel string
	calls  [][]string
	mutate bool
}

func (r *exposureRunner) run(_ context.Context, _ string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	switch strings.Join(args, " ") {
	case "status --json":
		return []byte(statusRunning), nil
	case "serve status --json":
		if r.mutate {
			return []byte(`{"TCP":{"29888":{"HTTP":true}},"Web":{"laptop.tail1234.ts.net:29888":{"Handlers":{"/":{"Proxy":"http://127.0.0.1:9999"}}}}}`), nil
		}
		return []byte(r.serve), nil
	case "funnel status --json":
		return []byte(r.funnel), nil
	case "serve --bg --https=29888 http://127.0.0.1:47801", "serve --bg --http=29888 http://127.0.0.1:47801":
		r.serve = `{"TCP":{"47800":{"HTTP":true},"29888":{"HTTPS":true}},"Web":{"laptop.tail1234.ts.net:47800":{"Handlers":{"/":{"Proxy":"http://127.0.0.1:9000"}}},"laptop.tail1234.ts.net:29888":{"Handlers":{"/":{"Proxy":"http://127.0.0.1:47801"}}}}}`
		return nil, nil
	case "serve --https=29888 off", "serve --http=29888 off":
		r.serve = serveOtherPort
		return nil, nil
	default:
		return nil, errors.New("unexpected command")
	}
}

func testPort() (int, error)                            { return 29888, nil }
func freePort(_ string, _ string) (net.Listener, error) { return &noopListener{}, nil }

type noopListener struct{}

func (*noopListener) Accept() (net.Conn, error) { return nil, errors.New("unused") }
func (*noopListener) Close() error              { return nil }
func (*noopListener) Addr() net.Addr            { return &net.TCPAddr{} }

func TestPreviewExposureAddsAndRemovesOnlyItsOwnSecureRoute(t *testing.T) {
	runner := &exposureRunner{
		serve:  serveOtherPort,
		funnel: `{"TCP":{"47800":{"HTTP":true}},"Web":{"laptop.tail1234.ts.net:47800":{"Handlers":{"/":{"Proxy":"http://127.0.0.1:9000"}}}},"AllowFunnel":{"laptop.tail1234.ts.net:47800":false}}`,
	}
	callback := newPreviewExposure(nil, func() string { return "tailscale" }, runner.run, testPort, freePort)
	base, cleanup, err := callback(context.Background(), 47801, true)
	if err != nil {
		t.Fatal(err)
	}
	if base != "https://laptop.tail1234.ts.net:29888" {
		t.Fatalf("base = %q", base)
	}
	cleanup()
	cleanup()
	if len(runner.calls) != 6 || strings.Join(runner.calls[3], " ") != "serve --bg --https=29888 http://127.0.0.1:47801" || strings.Join(runner.calls[5], " ") != "serve --https=29888 off" {
		t.Fatalf("commands = %v", runner.calls)
	}
	if runner.serve != serveOtherPort {
		t.Fatalf("remaining Serve config = %s, want the pre-existing route preserved", runner.serve)
	}
}

func TestPreviewExposureRefusesAnActiveFunnel(t *testing.T) {
	runner := &exposureRunner{serve: `{}`, funnel: `{"TCP":{"443":{"HTTPS":true}},"Web":{"laptop.tail1234.ts.net:443":{"Handlers":{"/":{"Proxy":"http://127.0.0.1:9000"}}}},"AllowFunnel":{"laptop.tail1234.ts.net:443":true}}`}
	callback := newPreviewExposure(nil, func() string { return "tailscale" }, runner.run, testPort, freePort)
	if _, _, err := callback(context.Background(), 47801, false); err == nil || !strings.Contains(err.Error(), "Funnel") {
		t.Fatalf("error = %v, want Funnel refusal", err)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("commands = %v; route should not be added", runner.calls)
	}
}

func TestPreviewExposureCleanupLeavesAChangedRouteAlone(t *testing.T) {
	runner := &exposureRunner{serve: `{}`, funnel: `{}`}
	callback := newPreviewExposure(nil, func() string { return "tailscale" }, runner.run, testPort, freePort)
	_, cleanup, err := callback(context.Background(), 47801, false)
	if err != nil {
		t.Fatal(err)
	}
	runner.mutate = true
	cleanup()
	if len(runner.calls) != 5 || strings.Join(runner.calls[4], " ") != "serve status --json" {
		t.Fatalf("commands = %v; cleanup should only inspect a route now owned by someone else", runner.calls)
	}
}

func TestPreviewExposureFailsClosedWhenFunnelCannotBeInspected(t *testing.T) {
	runner := &exposureRunner{serve: `{}`, funnel: `invalid`}
	callback := newPreviewExposure(nil, func() string { return "tailscale" }, runner.run, testPort, freePort)
	if _, _, err := callback(context.Background(), 47801, false); err == nil {
		t.Fatal("invalid Funnel status should be refused")
	}
}

func TestFunnelStatusChecksForegroundAllowFunnelEntries(t *testing.T) {
	privateForeground := []byte(`{"Foreground":{"session":{"TCP":{"29888":{"HTTP":true}},"Web":{"laptop.tail1234.ts.net:29888":{"Handlers":{"/":{"Proxy":"http://127.0.0.1:47801"}}}},"AllowFunnel":{"laptop.tail1234.ts.net:29888":false}}}}`)
	if !json.Valid(privateForeground) {
		t.Fatal("private foreground status fixture is invalid JSON")
	}
	if funnelConfigured(privateForeground) {
		t.Fatal("a private foreground Serve route was treated as Funnel")
	}
	publicForeground := []byte(`{"Foreground":{"session":{"TCP":{"29888":{"HTTP":true}},"AllowFunnel":{"laptop.tail1234.ts.net:29888":true}}}}`)
	if !json.Valid(publicForeground) {
		t.Fatal("public foreground status fixture is invalid JSON")
	}
	if !funnelConfigured(publicForeground) {
		t.Fatal("an active foreground Funnel route was not detected")
	}
}

func TestPreviewPortSelectionReservesForegroundServePorts(t *testing.T) {
	cfg := serveConfig{Foreground: map[string]*serveConfig{
		"session": {TCP: map[string]struct {
			HTTP       bool   `json:"HTTP"`
			HTTPS      bool   `json:"HTTPS"`
			TCPForward string `json:"TCPForward"`
		}{"29888": {HTTP: true}}},
	}}
	ports := []int{29888, 29889}
	index := 0
	choose := func() (int, error) {
		port := ports[index]
		index++
		return port, nil
	}
	got, err := chooseUnusedPort(cfg, choose, freePort)
	if err != nil {
		t.Fatal(err)
	}
	if got != 29889 || index != 2 {
		t.Fatalf("port = %d after %d choices, want foreground port skipped", got, index)
	}
}
