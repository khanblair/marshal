package tailnet

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const previewExposureTimeout = 5 * time.Second

// PreviewExposure is the callback shape used by preview relay services.
type PreviewExposure func(ctx context.Context, localPort int, secure bool) (publicBase string, cleanup func(), err error)

// NewPreviewExposure returns a callback that exposes previews only on the host's tailnet.
// It uses the installed Tailscale CLI when present, otherwise the embedded tsnet node.
func NewPreviewExposure(node *Node) PreviewExposure {
	return newPreviewExposure(node, findCLI, runCommand, randomPort, net.Listen)
}

func newPreviewExposure(node *Node, find func() string, run Runner, choosePort func() (int, error), listen func(string, string) (net.Listener, error)) PreviewExposure {
	return func(ctx context.Context, localPort int, secure bool) (string, func(), error) {
		if localPort < 1 || localPort > 65535 {
			return "", nil, errors.New("invalid preview port")
		}
		if bin := find(); bin != "" {
			return exposeWithCLI(ctx, bin, localPort, secure, previewCLI{run: run, choosePort: choosePort, listen: listen})
		}
		if node == nil {
			return "", nil, errors.New("Tailscale is unavailable")
		}
		return exposeWithNode(ctx, node, localPort, secure, choosePort)
	}
}

type previewCLI struct {
	run        Runner
	choosePort func() (int, error)
	listen     func(string, string) (net.Listener, error)
}

func exposeWithCLI(ctx context.Context, bin string, localPort int, secure bool, cli previewCLI) (string, func(), error) {
	ctx, cancel := context.WithTimeout(ctx, previewExposureTimeout)
	defer cancel()
	status, err := runBounded(ctx, cli.run, bin, "status", "--json")
	if err != nil {
		return "", nil, errors.New("could not check host Tailscale status")
	}
	parsed := parseStatus(status)
	if parsed.BackendState != "Running" || strings.TrimSuffix(parsed.Self.DNSName, ".") == "" {
		return "", nil, errors.New("host Tailscale is not online")
	}
	serveRaw, err := runBounded(ctx, cli.run, bin, "serve", "status", "--json")
	if err != nil {
		return "", nil, errors.New("could not inspect existing Tailscale Serve routes")
	}
	var cfg serveConfig
	if err := json.Unmarshal(serveRaw, &cfg); err != nil {
		return "", nil, errors.New("could not inspect existing Tailscale Serve routes")
	}
	funnelRaw, err := runBounded(ctx, cli.run, bin, "funnel", "status", "--json")
	if err != nil || funnelConfigured(funnelRaw) {
		return "", nil, errors.New("host Tailscale Funnel is active or could not be checked")
	}
	port, err := chooseUnusedPort(cfg, cli.choosePort, cli.listen)
	if err != nil {
		return "", nil, err
	}
	mode := "--http="
	scheme := "http"
	if secure {
		mode, scheme = "--https=", "https"
	}
	target := "http://127.0.0.1:" + strconv.Itoa(localPort)
	if _, err := runBounded(ctx, cli.run, bin, "serve", "--bg", mode+strconv.Itoa(port), target); err != nil {
		return "", nil, errors.New("could not start private Tailscale Serve route")
	}
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			cleanupCtx, stop := context.WithTimeout(context.Background(), previewExposureTimeout)
			defer stop()
			raw, checkErr := runBounded(cleanupCtx, cli.run, bin, "serve", "status", "--json")
			if checkErr != nil {
				return
			}
			var current serveConfig
			if json.Unmarshal(raw, &current) != nil || !ownsServeRoute(current, port, target, secure) {
				return
			}
			_, _ = runBounded(cleanupCtx, cli.run, bin, "serve", mode+strconv.Itoa(port), "off")
		})
	}
	return scheme + "://" + strings.TrimSuffix(parsed.Self.DNSName, ".") + ":" + strconv.Itoa(port), cleanup, nil
}

func runBounded(ctx context.Context, run Runner, bin string, args ...string) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, previewExposureTimeout)
	defer cancel()
	out, err := run(commandCtx, bin, args...)
	if len(out) > 1<<20 {
		return nil, errors.New("Tailscale response was too large")
	}
	return out, err
}

func chooseUnusedPort(cfg serveConfig, choosePort func() (int, error), listen func(string, string) (net.Listener, error)) (int, error) {
	occupied := map[int]bool{}
	collectServePorts(&cfg, occupied)
	for range 8 {
		port, err := choosePort()
		if err != nil {
			return 0, errors.New("could not choose a private preview port")
		}
		if port < 1024 || port > 65535 || occupied[port] {
			continue
		}
		probe, err := listen("tcp", ":"+strconv.Itoa(port))
		if err != nil {
			continue
		}
		_ = probe.Close()
		return port, nil
	}
	return 0, errors.New("could not find an unused private preview port")
}

func collectServePorts(cfg *serveConfig, occupied map[int]bool) {
	if cfg == nil {
		return
	}
	for key := range cfg.TCP {
		if port, err := strconv.Atoi(key); err == nil {
			occupied[port] = true
		}
	}
	for key := range cfg.Web {
		_, rawPort, err := net.SplitHostPort(key)
		if err == nil {
			if port, convErr := strconv.Atoi(rawPort); convErr == nil {
				occupied[port] = true
			}
		}
	}
	for key := range cfg.AllowFunnel {
		_, rawPort, err := net.SplitHostPort(key)
		if err == nil {
			if port, convErr := strconv.Atoi(rawPort); convErr == nil {
				occupied[port] = true
			}
		}
	}
	for _, foreground := range cfg.Foreground {
		collectServePorts(foreground, occupied)
	}
}

func funnelConfigured(raw []byte) bool {
	var root map[string]json.RawMessage
	if json.Unmarshal(raw, &root) != nil || root == nil {
		return true // Fail closed when the status format is unknown.
	}
	for key, value := range root {
		switch key {
		case "TCP", "Web", "Services":
			var entries map[string]json.RawMessage
			if json.Unmarshal(value, &entries) != nil && string(value) != "null" {
				return true
			}
		case "Foreground":
			var foreground map[string]json.RawMessage
			if json.Unmarshal(value, &foreground) != nil && string(value) != "null" {
				return true
			}
			for _, nested := range foreground {
				if funnelConfigured(nested) {
					return true
				}
			}
		case "AllowFunnel":
			var routes map[string]bool
			if json.Unmarshal(value, &routes) != nil && string(value) != "null" {
				return true
			}
			for _, enabled := range routes {
				if enabled {
					return true
				}
			}
		default:
			return true
		}
	}
	return false
}

func ownsServeRoute(cfg serveConfig, port int, target string, secure bool) bool {
	portKey := strconv.Itoa(port)
	protocol := "http"
	if secure {
		protocol = "https"
	}
	found := false
	matchingHandlerCount := 0
	matchingProxy := ""
	for key, web := range cfg.Web {
		_, rawPort, err := net.SplitHostPort(key)
		if err != nil || rawPort != portKey {
			continue
		}
		if found {
			return false
		}
		found = true
		matchingHandlerCount = len(web.Handlers)
		if handler, ok := web.Handlers["/"]; ok {
			matchingProxy = handler.Proxy
		}
	}
	if !found || matchingHandlerCount != 1 || matchingProxy != target {
		return false
	}
	rule := cfg.TCP[portKey]
	return protocol == "http" && rule.HTTP && !rule.HTTPS || protocol == "https" && rule.HTTPS && !rule.HTTP
}

func exposeWithNode(ctx context.Context, node *Node, localPort int, secure bool, choosePort func() (int, error)) (string, func(), error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	status := node.Status()
	if status.State != StateOnline || strings.TrimSpace(status.DNSName) == "" {
		return "", nil, errors.New("embedded Tailscale node is not online")
	}
	port, err := choosePort()
	if err != nil || port < 1 || port > 65535 {
		return "", nil, errors.New("could not choose a private preview port")
	}
	addr := ":" + strconv.Itoa(port)
	var listener net.Listener
	if secure {
		listener, err = node.server().ListenTLS("tcp", addr)
	} else {
		listener, err = node.Listen("tcp", addr)
	}
	if err != nil {
		return "", nil, errors.New("could not open a private embedded Tailscale listener")
	}
	target, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(localPort))
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorLog = log.New(io.Discard, "", 0)
	server := &http.Server{Handler: proxy, ReadHeaderTimeout: 5 * time.Second}
	serveCtx, cancel := context.WithCancel(context.Background())
	go func() { _ = server.Serve(listener) }()
	go func() {
		<-serveCtx.Done()
		shutdownCtx, stop := context.WithTimeout(context.Background(), previewExposureTimeout)
		defer stop()
		_ = server.Shutdown(shutdownCtx)
	}()
	var once sync.Once
	cleanup := func() { once.Do(func() { cancel(); _ = listener.Close() }) }
	scheme := "http"
	if secure {
		scheme = "https"
	}
	return scheme + "://" + strings.TrimSuffix(status.DNSName, ".") + ":" + strconv.Itoa(port), cleanup, nil
}

func randomPort() (int, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	return 20000 + int(uint32(b[0])<<8|uint32(b[1]))%40000, nil
}
