package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// fakeProbe answers for each kind from a table, counts how often it was asked, and can hold its
// answers back until a test lets them go.
type fakeProbe struct {
	mu      sync.Mutex
	answers map[protocol.AgentKind]fakeAnswer
	calls   map[protocol.AgentKind]int
	gate    chan struct{}
}

// fakeAnswer is what a fake probe says about one kind.
type fakeAnswer struct {
	path    string
	version string
	err     error
}

func newFakeProbe(answers map[protocol.AgentKind]fakeAnswer) *fakeProbe {
	return &fakeProbe{answers: answers, calls: map[protocol.AgentKind]int{}}
}

func (f *fakeProbe) Probe(ctx context.Context, kind protocol.AgentKind) (Found, error) {
	f.mu.Lock()
	f.calls[kind]++
	answer, gate := f.answers[kind], f.gate
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return Found{}, fmt.Errorf("%w: %w", ErrUnreadable, ctx.Err())
		}
	}
	if answer.err != nil {
		return Found{Path: answer.path}, answer.err
	}
	if answer.path == "" {
		return Found{}, ErrNotFound
	}
	version, ok := ParseSemver(answer.version)
	if !ok {
		return Found{Path: answer.path}, ErrUnreadable
	}
	return Found{Path: answer.path, Version: version}, nil
}

// total is how many times the probe was asked, for all kinds together.
func (f *fakeProbe) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		n += c
	}
	return n
}

// fakeClock is a clock that a test moves by hand.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC)}
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// installedProbe is a probe for a machine where Claude Code and Gemini CLI are at the tested
// versions and Codex is not installed.
func installedProbe() *fakeProbe {
	return newFakeProbe(map[protocol.AgentKind]fakeAnswer{
		protocol.AgentKindClaude: {path: "/home/pat/.local/bin/claude", version: "2.1.282 (Claude Code)"},
		protocol.AgentKindGemini: {path: "/opt/homebrew/bin/gemini", version: "0.35.1"},
	})
}

// agentOfKind finds one agent of a catalog.
func agentOfKind(t *testing.T, list protocol.AgentCatalog, kind protocol.AgentKind) protocol.Agent {
	t.Helper()
	for _, agent := range list.Agents {
		if agent.Kind == kind {
			return agent
		}
	}
	t.Fatalf("the catalog has no %s agent: %+v", kind, list.Agents)
	return protocol.Agent{}
}

func TestListReportsEachAgent(t *testing.T) {
	tests := []struct {
		name        string
		answers     map[protocol.AgentKind]fakeAnswer
		kind        protocol.AgentKind
		status      protocol.AgentStatus
		version     string
		warning     string // a piece that the warning must hold; empty means no warning at all
		hint        bool
		startable   bool
		displayName string
	}{
		{
			name:        "a tested Claude Code is supported",
			answers:     map[protocol.AgentKind]fakeAnswer{protocol.AgentKindClaude: {path: "/bin/claude", version: "2.1.282 (Claude Code)"}},
			kind:        protocol.AgentKindClaude,
			status:      protocol.AgentStatusSupported,
			version:     "2.1.282",
			startable:   true,
			displayName: "Claude Code",
		},
		{
			name:        "a tested Gemini CLI is supported",
			answers:     map[protocol.AgentKind]fakeAnswer{protocol.AgentKindGemini: {path: "/bin/gemini", version: "0.35.1"}},
			kind:        protocol.AgentKindGemini,
			status:      protocol.AgentStatusSupported,
			version:     "0.35.1",
			startable:   true,
			displayName: "Gemini CLI",
		},
		{
			name:        "another Claude Code version is untested, and the warning names both versions",
			answers:     map[protocol.AgentKind]fakeAnswer{protocol.AgentKindClaude: {path: "/bin/claude", version: "2.1.300"}},
			kind:        protocol.AgentKindClaude,
			status:      protocol.AgentStatusUntested,
			version:     "2.1.300",
			warning:     "2.1.300",
			startable:   true,
			displayName: "Claude Code",
		},
		{
			name:        "a pre-release of a tested version is untested",
			answers:     map[protocol.AgentKind]fakeAnswer{protocol.AgentKindGemini: {path: "/bin/gemini", version: "0.35.1-preview.2"}},
			kind:        protocol.AgentKindGemini,
			status:      protocol.AgentStatusUntested,
			version:     "0.35.1-preview.2",
			warning:     "try version 0.35.1",
			startable:   true,
			displayName: "Gemini CLI",
		},
		{
			name:        "an installed Codex is untested by design and cannot be started",
			answers:     map[protocol.AgentKind]fakeAnswer{protocol.AgentKindCodex: {path: "/bin/codex", version: "codex-cli 0.42.0"}},
			kind:        protocol.AgentKindCodex,
			status:      protocol.AgentStatusUntested,
			version:     "0.42.0",
			warning:     "cannot start Codex",
			displayName: "Codex",
		},
		{
			name:        "a program that is not there is missing, with an install hint",
			answers:     map[protocol.AgentKind]fakeAnswer{},
			kind:        protocol.AgentKindCodex,
			status:      protocol.AgentStatusMissing,
			hint:        true,
			displayName: "Codex",
		},
		{
			name:        "a program that gives no version is missing, with a warning",
			answers:     map[protocol.AgentKind]fakeAnswer{protocol.AgentKindClaude: {path: "/bin/claude", version: "garbage"}},
			kind:        protocol.AgentKindClaude,
			status:      protocol.AgentStatusMissing,
			warning:     `"claude --version"`,
			hint:        true,
			displayName: "Claude Code",
		},
		{
			name: "a program that fails is missing, with a warning",
			answers: map[protocol.AgentKind]fakeAnswer{
				protocol.AgentKindGemini: {path: "/bin/gemini", err: fmt.Errorf("%w: exit 3", ErrUnreadable)},
			},
			kind:        protocol.AgentKindGemini,
			status:      protocol.AgentStatusMissing,
			warning:     `"gemini --version"`,
			hint:        true,
			displayName: "Gemini CLI",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New(Options{Probe: newFakeProbe(tt.answers), Logger: quiet()})
			list, err := c.List(t.Context())
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			agent := agentOfKind(t, list, tt.kind)
			if agent.Name != tt.displayName || agent.Status != tt.status || agent.Version != tt.version {
				t.Errorf("agent = %q %q %q, want %q %q %q",
					agent.Name, agent.Status, agent.Version, tt.displayName, tt.status, tt.version)
			}
			if tt.warning == "" && agent.Warning != "" {
				t.Errorf("warning = %q, want none", agent.Warning)
			}
			if !strings.Contains(agent.Warning, tt.warning) {
				t.Errorf("warning = %q, want it to hold %q", agent.Warning, tt.warning)
			}
			if (agent.InstallHint != "") != tt.hint {
				t.Errorf("install hint = %q, want one: %v", agent.InstallHint, tt.hint)
			}
			if len(agent.Models) == 0 {
				t.Error("an agent has no models, so its picker would be empty")
			}
			detected, err := c.Detect(t.Context())
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			for _, d := range detected {
				if d.Kind == tt.kind && d.Startable != tt.startable {
					t.Errorf("startable = %v, want %v", d.Startable, tt.startable)
				}
			}
		})
	}
}

func TestListHasTheThreeAgentsInOrder(t *testing.T) {
	clock := newClock()
	c := New(Options{Probe: installedProbe(), Now: clock.Now, Logger: quiet()})
	list, err := c.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var kinds []protocol.AgentKind
	for _, agent := range list.Agents {
		kinds = append(kinds, agent.Kind)
	}
	want := []protocol.AgentKind{protocol.AgentKindClaude, protocol.AgentKindGemini, protocol.AgentKindCodex}
	if fmt.Sprint(kinds) != fmt.Sprint(want) {
		t.Errorf("kinds = %v, want %v (the built-in agent is not listed)", kinds, want)
	}
	if !list.ServerTime.Time().Equal(clock.Now()) {
		t.Errorf("server time = %v, want %v", list.ServerTime.Time(), clock.Now())
	}
	statuses := map[protocol.AgentKind]protocol.AgentStatus{}
	for _, agent := range list.Agents {
		statuses[agent.Kind] = agent.Status
	}
	if statuses[protocol.AgentKindClaude] != protocol.AgentStatusSupported ||
		statuses[protocol.AgentKindGemini] != protocol.AgentStatusSupported ||
		statuses[protocol.AgentKindCodex] != protocol.AgentStatusMissing {
		t.Errorf("statuses = %v, want claude and gemini supported and codex missing", statuses)
	}
}

func TestTheCatalogTellsTheTruthAboutWhatAgentsCanDo(t *testing.T) {
	c := New(Options{Probe: installedProbe(), Logger: quiet()})
	list, err := c.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	claude := agentOfKind(t, list, protocol.AgentKindClaude).Capabilities
	if !claude.Resume || !claude.StructuredEvents || !claude.ModelSwitching || !claude.Thinking || claude.Approvals {
		t.Errorf("claude capabilities = %+v; it resumes, streams, takes a model and effort, and cannot ask yet", claude)
	}
	gemini := agentOfKind(t, list, protocol.AgentKindGemini).Capabilities
	if !gemini.Resume || !gemini.Approvals || gemini.Thinking {
		t.Errorf("gemini capabilities = %+v; it resumes and asks, and has no thinking setting", gemini)
	}
	if codex := agentOfKind(t, list, protocol.AgentKindCodex).Capabilities; codex != (protocol.AgentCapabilities{}) {
		t.Errorf("codex capabilities = %+v, want none, since it cannot be started", codex)
	}
}

func TestModelsThatTheScreensRelyOnAreThere(t *testing.T) {
	c := New(Options{Probe: installedProbe(), Logger: quiet()})
	list, err := c.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ids := func(kind protocol.AgentKind) []string {
		var out []string
		for _, m := range agentOfKind(t, list, kind).Models {
			out = append(out, m.ID)
		}
		return out
	}
	tests := []struct {
		kind protocol.AgentKind
		want []string
	}{
		{protocol.AgentKindClaude, []string{"sonnet", "opus", "haiku", "fable"}},
		{protocol.AgentKindGemini, []string{"gemini-2.5-pro", "gemini-2.5-flash"}},
		{protocol.AgentKindCodex, []string{"gpt-5-codex", "gpt-5", "gpt-5-mini"}},
	}
	for _, tt := range tests {
		got := ids(tt.kind)
		for _, want := range tt.want {
			found := false
			for _, id := range got {
				found = found || id == want
			}
			if !found {
				t.Errorf("%s models = %v, missing %q", tt.kind, got, want)
			}
		}
		if got[0] != tt.want[0] {
			t.Errorf("%s default model = %q, want %q first", tt.kind, got[0], tt.want[0])
		}
	}
	// Haiku has no thinking setting, and the others do.
	for _, m := range agentOfKind(t, list, protocol.AgentKindClaude).Models {
		want := !strings.Contains(m.ID, "haiku")
		if m.Thinking != want {
			t.Errorf("model %s thinking = %v, want %v", m.ID, m.Thinking, want)
		}
	}
}

func TestTheWireCatalogDoesNotCarryPaths(t *testing.T) {
	probe := installedProbe()
	c := New(Options{Probe: probe, Logger: quiet()})
	list, err := c.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"/home/pat", ".local/bin", "/opt/homebrew"} {
		if strings.Contains(string(body), secret) {
			t.Errorf("the catalog holds %q, which is a folder of the machine: %s", secret, body)
		}
	}
	detected, err := c.Detect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	paths := map[protocol.AgentKind]string{}
	for _, d := range detected {
		paths[d.Kind] = d.Path
	}
	if paths[protocol.AgentKindClaude] != "/home/pat/.local/bin/claude" || paths[protocol.AgentKindCodex] != "" {
		t.Errorf("Detect paths = %v, want the daemon to keep them", paths)
	}
}

func TestDetectReturnsACopy(t *testing.T) {
	c := New(Options{Probe: installedProbe(), Logger: quiet()})
	first, err := c.Detect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	first[0].Path = "changed"
	second, err := c.Detect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if second[0].Path == "changed" {
		t.Error("a caller changed the kept answer")
	}
}

func TestTheKeptAnswerIsUsedForFiveMinutes(t *testing.T) {
	clock := newClock()
	probe := installedProbe()
	c := New(Options{Probe: probe, Now: clock.Now, Logger: quiet()})
	ctx := t.Context()

	if _, err := c.List(ctx); err != nil {
		t.Fatal(err)
	}
	if got := probe.total(); got != 3 {
		t.Fatalf("the first List asked the probe %d times, want 3 (one for each kind)", got)
	}
	clock.advance(4*time.Minute + 59*time.Second)
	if _, err := c.List(ctx); err != nil {
		t.Fatal(err)
	}
	if got := probe.total(); got != 3 {
		t.Errorf("a List at 4m59s asked again: %d calls, want 3", got)
	}
	clock.advance(time.Second)
	if _, err := c.List(ctx); err != nil {
		t.Fatal(err)
	}
	if got := probe.total(); got != 6 {
		t.Errorf("a List at 5m asked %d times in all, want 6", got)
	}
}

func TestRefreshLooksAgain(t *testing.T) {
	probe := installedProbe()
	c := New(Options{Probe: probe, Logger: quiet()})
	ctx := t.Context()
	before, err := c.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := agentOfKind(t, before, protocol.AgentKindCodex).Status; got != protocol.AgentStatusMissing {
		t.Fatalf("codex status before = %q, want missing", got)
	}

	probe.mu.Lock()
	probe.answers[protocol.AgentKindCodex] = fakeAnswer{path: "/bin/codex", version: "0.42.0"}
	probe.mu.Unlock()
	kept, err := c.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := agentOfKind(t, kept, protocol.AgentKindCodex).Status; got != protocol.AgentStatusMissing {
		t.Errorf("List after the install used %q, want the kept answer (missing)", got)
	}
	fresh, err := c.Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := agentOfKind(t, fresh, protocol.AgentKindCodex).Status; got != protocol.AgentStatusUntested {
		t.Errorf("Refresh status = %q, want untested", got)
	}
	again, err := c.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := agentOfKind(t, again, protocol.AgentKindCodex).Status; got != protocol.AgentStatusUntested {
		t.Errorf("List after Refresh = %q, want the new answer", got)
	}
}

func TestCallersThatArriveTogetherShareOneRun(t *testing.T) {
	probe := installedProbe()
	probe.gate = make(chan struct{})
	c := New(Options{Probe: probe, Logger: quiet()})

	const callers = 12
	var wg sync.WaitGroup
	results := make([]protocol.AgentCatalog, callers)
	errs := make([]error, callers)
	for i := range callers {
		wg.Go(func() { results[i], errs[i] = c.List(t.Context()) })
	}
	// Give every caller time to arrive at the run that is waiting on the gate.
	time.Sleep(100 * time.Millisecond)
	close(probe.gate)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("caller %d: %v", i, err)
		}
	}
	if got := probe.total(); got != 3 {
		t.Errorf("the probe was asked %d times, want 3: the callers should share one run", got)
	}
}

func TestACallerThatGivesUpDoesNotStopTheRun(t *testing.T) {
	probe := installedProbe()
	probe.gate = make(chan struct{})
	c := New(Options{Probe: probe, Logger: quiet()})

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.List(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("List err = %v, want the deadline", err)
	}
	close(probe.gate)
	list, err := c.List(t.Context())
	if err != nil {
		t.Fatalf("second List: %v", err)
	}
	if got := agentOfKind(t, list, protocol.AgentKindClaude).Status; got != protocol.AgentStatusSupported {
		t.Errorf("claude status = %q, want supported", got)
	}
	if got := probe.total(); got != 3 {
		t.Errorf("the probe was asked %d times, want 3: the second List should use the run that went on", got)
	}
}

func TestARunThatTakesTooLongMarksTheAgentsMissing(t *testing.T) {
	probe := installedProbe()
	probe.gate = make(chan struct{})
	defer close(probe.gate)
	c := New(Options{Probe: probe, Timeout: 100 * time.Millisecond, Logger: quiet()})
	start := time.Now()
	list, err := c.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("List took %v with a 100ms limit", took)
	}
	for _, agent := range list.Agents {
		if agent.Status != protocol.AgentStatusMissing {
			t.Errorf("%s status = %q, want missing when the check ran out of time", agent.Kind, agent.Status)
		}
	}
}

func TestCatalogWithProgramsOnDisk(t *testing.T) {
	dir := t.TempDir()
	installFake(t, dir, "claude", "2.1.282 (Claude Code)")
	installFake(t, dir, "gemini", "0.35.1")
	c := New(Options{
		Probe:  NewProbe(ProbeOptions{Dirs: []string{dir}, LookPath: notOnPath, VersionTimeout: 10 * time.Second}),
		Logger: quiet(),
	})
	list, err := c.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := map[protocol.AgentKind]protocol.AgentStatus{
		protocol.AgentKindClaude: protocol.AgentStatusSupported,
		protocol.AgentKindGemini: protocol.AgentStatusSupported,
		protocol.AgentKindCodex:  protocol.AgentStatusMissing,
	}
	for kind, status := range want {
		if got := agentOfKind(t, list, kind).Status; got != status {
			t.Errorf("%s status = %q, want %q", kind, got, status)
		}
	}
	detected, err := c.Detect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(detected[0].Path) != dir {
		t.Errorf("claude path = %q, want it inside %q", detected[0].Path, dir)
	}
}

func TestNewFillsInTheDefaults(t *testing.T) {
	c := New(Options{})
	if c.ttl != 5*time.Minute || c.timeout != 8*time.Second || c.probe == nil || c.now == nil || c.log == nil {
		t.Errorf("defaults = ttl %v, timeout %v, probe %v", c.ttl, c.timeout, c.probe)
	}
}
