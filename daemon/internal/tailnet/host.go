package tailnet

// This file asks the Tailscale app on this computer what it knows (docs/mobile.md section 2.1).
// A computer that already runs Tailscale is on a tailnet whether or not Marshal joined one, so the
// daemon reads Tailscale's own answers instead of assuming: who is signed in, what the computer is
// called, which phones the tailnet has, and whether Tailscale Serve already hands a port to the
// daemon. It only reads, with fixed arguments, and never changes Tailscale.

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const (
	hostCallTimeout  = 3 * time.Second
	hostProbeTimeout = 2 * time.Second
	hostCacheTTL     = 8 * time.Second
)

// The words Status gives a Tailscale that is not simply running.
const (
	hostRunning      = "running"
	hostStarting     = "starting"
	hostNeedsLogin   = "needs-login"
	hostNeedsApprove = "needs-approval"
	hostStopped      = "stopped"
	hostNotRunning   = "not-running"
	hostUnknown      = "unknown"
)

// Runner runs a command and answers what it printed. A test hands it canned answers.
type Runner func(ctx context.Context, bin string, args ...string) ([]byte, error)

// Probe says whether a URL answers with a success. A test hands it a fixed answer.
type Probe func(ctx context.Context, address string) bool

// Host asks the Tailscale app on this computer. It keeps the last answer for a few seconds, because
// several screens ask at once and each ask starts two commands.
type Host struct {
	run   Runner
	find  func() string
	probe Probe
	now   func() time.Time

	mu     sync.Mutex
	at     time.Time
	port   int
	answer protocol.TailnetHost
}

// NewHost makes a Host that runs Tailscale's real command.
func NewHost() *Host {
	return &Host{run: runCommand, find: findCLI, probe: probeHealth, now: time.Now}
}

// NewHostWith makes a Host from its parts, for a test.
func NewHostWith(run Runner, find func() string, probe Probe, now func() time.Time) *Host {
	return &Host{run: run, find: find, probe: probe, now: now}
}

func runCommand(ctx context.Context, bin string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, bin, args...).Output()
}

// cliCandidates are where Tailscale's command lives. The installed app's service does not have
// /usr/local/bin on its path, so the usual places are tried by name.
func cliCandidates() []string {
	return []string{
		"/usr/local/bin/tailscale",
		"/opt/homebrew/bin/tailscale",
		"/Applications/Tailscale.app/Contents/MacOS/Tailscale",
		"/usr/bin/tailscale",
		`C:\Program Files\Tailscale\tailscale.exe`,
	}
}

func findCLI() string {
	if path, err := exec.LookPath("tailscale"); err == nil {
		return path
	}
	for _, path := range cliCandidates() {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func probeHealth(ctx context.Context, address string) bool {
	ctx, cancel := context.WithTimeout(ctx, hostProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Status is what Tailscale on this computer says, for the daemon that serves on port.
func (h *Host) Status(ctx context.Context, port int) protocol.TailnetHost {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.at.IsZero() && h.port == port && h.now().Sub(h.at) < hostCacheTTL {
		return h.answer
	}
	h.answer, h.port, h.at = h.ask(ctx, port), port, h.now()
	return h.answer
}

func (h *Host) ask(ctx context.Context, port int) protocol.TailnetHost {
	out := protocol.EmptyTailnetHost()
	bin := h.find()
	if bin == "" {
		return out
	}
	out.Found = true
	statusJSON, serveJSON := h.readBoth(ctx, bin)
	if statusJSON == nil {
		out.State = hostNotRunning
		return out
	}
	status := parseStatus(statusJSON)
	status.fill(&out)
	if out.State != hostRunning {
		return out
	}
	applyServe(&out, serveJSON, port)
	if out.ServePort > 0 && out.DNSName != "" {
		out.Reachable = h.probe(ctx, "http://"+net.JoinHostPort(out.DNSName, strconv.Itoa(out.ServePort))+"/v1/health")
	}
	return out
}

// readBoth runs Tailscale's two read-only commands at once. A command that fails answers nil: a
// Tailscale that cannot be reached is a state to report, and an old one with no serve listing is no
// serve rules.
func (h *Host) readBoth(ctx context.Context, bin string) (status, serve []byte) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		status = h.output(ctx, bin, "status", "--json")
	}()
	go func() {
		defer wg.Done()
		serve = h.output(ctx, bin, "serve", "status", "--json")
	}()
	wg.Wait()
	return status, serve
}

func (h *Host) output(ctx context.Context, bin string, args ...string) []byte {
	ctx, cancel := context.WithTimeout(ctx, hostCallTimeout)
	defer cancel()
	out, err := h.run(ctx, bin, args...)
	if err != nil {
		return nil
	}
	return out
}

// cliStatus is the part of `tailscale status --json` that is read.
type cliStatus struct {
	BackendState string `json:"BackendState"`
	Self         struct {
		DNSName      string   `json:"DNSName"`
		TailscaleIPs []string `json:"TailscaleIPs"`
		UserID       int64    `json:"UserID"`
	} `json:"Self"`
	User           map[string]struct{ LoginName string } `json:"User"`
	CurrentTailnet struct{ Name string }                 `json:"CurrentTailnet"`
	Peer           map[string]struct {
		HostName string    `json:"HostName"`
		OS       string    `json:"OS"`
		Online   bool      `json:"Online"`
		LastSeen time.Time `json:"LastSeen"`
	} `json:"Peer"`
}

func parseStatus(raw []byte) cliStatus {
	var status cliStatus
	_ = json.Unmarshal(raw, &status)
	return status
}

func stateWord(backend string) string {
	switch backend {
	case "Running":
		return hostRunning
	case "Starting", "NoState":
		return hostStarting
	case "NeedsLogin":
		return hostNeedsLogin
	case "NeedsMachineAuth":
		return hostNeedsApprove
	case "Stopped":
		return hostStopped
	}
	return hostUnknown
}

// fill copies the facts of one status answer into the wire form.
func (s cliStatus) fill(out *protocol.TailnetHost) {
	out.State = stateWord(s.BackendState)
	out.DNSName = strings.TrimSuffix(s.Self.DNSName, ".")
	out.IPs = append(out.IPs, s.Self.TailscaleIPs...)
	out.Account = s.User[strconv.FormatInt(s.Self.UserID, 10)].LoginName
	out.Tailnet = s.CurrentTailnet.Name
	out.Phones = s.phones()
}

// phones lists the Android and iOS devices, online ones first, then by name.
func (s cliStatus) phones() []protocol.TailnetPhone {
	phones := []protocol.TailnetPhone{}
	for _, peer := range s.Peer {
		if peer.OS != "android" && peer.OS != "iOS" {
			continue
		}
		phone := protocol.TailnetPhone{Name: peer.HostName, OS: peer.OS, Online: peer.Online}
		if peer.LastSeen.Year() > 1 {
			seen := protocol.NewTimestamp(peer.LastSeen)
			phone.LastSeen = &seen
		}
		phones = append(phones, phone)
	}
	sort.Slice(phones, func(i, j int) bool {
		if phones[i].Online != phones[j].Online {
			return phones[i].Online
		}
		return phones[i].Name < phones[j].Name
	})
	return phones
}

// serveConfig is the part of `tailscale serve status --json` that is read.
type serveConfig struct {
	TCP map[string]struct {
		HTTP       bool   `json:"HTTP"`
		HTTPS      bool   `json:"HTTPS"`
		TCPForward string `json:"TCPForward"`
	} `json:"TCP"`
	Web map[string]struct {
		Handlers map[string]struct {
			Proxy string `json:"Proxy"`
		} `json:"Handlers"`
	} `json:"Web"`
}

// isLocalTo says an address is this computer's own loopback on the daemon's port, which is where a
// Serve rule has to point to reach the daemon.
func isLocalTo(address string, port int) bool {
	if !strings.Contains(address, "://") {
		address = "tcp://" + address
	}
	u, err := url.Parse(address)
	if err != nil {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
	default:
		return false
	}
	return u.Port() == strconv.Itoa(port)
}

// portsToDaemon is the tailnet ports whose Serve rule leads to the daemon: a web rule proxying to its
// loopback port, or a raw TCP forward to it.
func portsToDaemon(cfg serveConfig, port int) map[int]bool {
	set := map[int]bool{}
	for key, web := range cfg.Web {
		_, p, err := net.SplitHostPort(key)
		tailnetPort, convErr := strconv.Atoi(p)
		if err != nil || convErr != nil {
			continue
		}
		for _, handler := range web.Handlers {
			if isLocalTo(handler.Proxy, port) {
				set[tailnetPort] = true
			}
		}
	}
	for key, rule := range cfg.TCP {
		tailnetPort, err := strconv.Atoi(key)
		if err == nil && rule.TCPForward != "" && isLocalTo(rule.TCPForward, port) {
			set[tailnetPort] = true
		}
	}
	return set
}

// applyServe reads which tailnet ports Serve uses, which hands plain http to the daemon, and which
// hands it https only.
func applyServe(out *protocol.TailnetHost, raw []byte, port int) {
	var cfg serveConfig
	if raw == nil || json.Unmarshal(raw, &cfg) != nil {
		return
	}
	toDaemon := portsToDaemon(cfg, port)
	for key, rule := range cfg.TCP {
		tailnetPort, err := strconv.Atoi(key)
		if err != nil {
			continue
		}
		switch {
		case toDaemon[tailnetPort] && (rule.HTTP || rule.TCPForward != ""):
			if out.ServePort == 0 || tailnetPort < out.ServePort {
				out.ServePort = tailnetPort
			}
		case toDaemon[tailnetPort] && rule.HTTPS:
			out.SecureServePort = tailnetPort
		default:
			out.TakenPorts = append(out.TakenPorts, tailnetPort)
		}
	}
	sort.Ints(out.TakenPorts)
	if out.ServePort > 0 {
		out.SecureServePort = 0
	}
}
