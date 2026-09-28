package preview_test

import (
	"context"
	"image"
	"image/png"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/preview"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The whole module is driven over fakes: a fake projects service, a fake dev runner, a fake browser
// lookup, a fake port picker, a fake prober, and a fake shooter. Nothing here starts a dev server and
// nothing here launches a browser, so this file proves B6.6 without a machine that has Chrome on it.
//
// The one thing that is real is the disk: the shots are written as real PNG files and read back, so
// "the pair survives a daemon restart" is proven against the same code the daemon runs.

var testNow = time.Date(2026, time.September, 27, 11, 0, 0, 0, time.UTC)

const (
	testBranch = "marshal/41-fix-token-refresh"
	// cardA is the card every test previews unless it says otherwise, and cardB is the second card
	// the tests that prove two previews at once use.
	cardA = "01M3C107JB041061050R3GG28A"
	cardB = "01M3C107JB041061050R3GG28B"
)

// fakeProjects answers each card's own card, worktree, and project's dev command, from a map. A card
// the test never added has no worktree, which is what a card that has not started looks like. It
// touches no repository and no store.
type fakeProjects struct {
	mu        sync.Mutex
	worktrees map[string]string // card id -> the card's worktree
	projectID string
	command   string
	readErr   error
}

func (f *fakeProjects) Card(_ context.Context, id string) (protocol.Card, error) {
	if f.readErr != nil {
		return protocol.Card{}, f.readErr
	}
	return protocol.Card{ID: id, ProjectID: f.projectID}, nil
}

func (f *fakeProjects) Worktree(_ context.Context, id string) (string, string, error) {
	if f.readErr != nil {
		return "", "", f.readErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	dir := f.worktrees[id]
	if dir == "" {
		return "", testBranch, nil // a card that has not started has a branch and no folder
	}
	return dir, testBranch, nil
}

func (f *fakeProjects) Get(_ context.Context, id string) (protocol.Project, error) {
	return protocol.Project{ID: id, DevCommand: f.command}, nil
}

// set records a card's worktree, so a test can add a second card to the same service.
func (f *fakeProjects) set(id, dir string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.worktrees[id] = dir
}

// worktree is where a card's dev server would run.
func (f *fakeProjects) worktree(id string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.worktrees[id]
}

// fakeProcess is one dev server. The test decides when it looks like it has exited, so a dev server
// that dies before it answers can be driven without starting anything.
type fakeProcess struct {
	mu      sync.Mutex
	done    chan struct{}
	stopped bool
}

func newFakeProcess() *fakeProcess { return &fakeProcess{done: make(chan struct{})} }

func (p *fakeProcess) Done() <-chan struct{} { return p.done }

func (p *fakeProcess) Stop(context.Context, time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopped = true
	select {
	case <-p.done:
	default:
		close(p.done) // a dev server that is asked to stop ends, so its Done closes
	}
	return nil
}

// exit makes the dev server look like it ended by itself.
func (p *fakeProcess) exit() { _ = p.Stop(context.Background(), 0) }

func (p *fakeProcess) wasStopped() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stopped
}

// fakeRunner records every dev server it was asked to start and answers a process the test holds.
type fakeRunner struct {
	mu       sync.Mutex
	requests []preview.DevRequest
	procs    []*fakeProcess
	err      error
}

func (f *fakeRunner) Start(_ context.Context, req preview.DevRequest) (preview.Process, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	proc := newFakeProcess()
	f.procs = append(f.procs, proc)
	return proc, nil
}

func (f *fakeRunner) started() []preview.DevRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]preview.DevRequest(nil), f.requests...)
}

func (f *fakeRunner) processes() []*fakeProcess {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*fakeProcess(nil), f.procs...)
}

// fakePorts hands out a port of its own to each card, so two cards never race for the same number.
type fakePorts struct {
	mu   sync.Mutex
	next int
}

func (f *fakePorts) Pick() (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	return 5100 + f.next, nil
}

// fakeBrowser stands in for the lookup over the places Chrome and Edge install themselves.
type fakeBrowser struct {
	path  string
	found bool
}

func (f fakeBrowser) Find() (string, bool) { return f.path, f.found }

// fakeProbe answers whether a dev server answers yet, as the test tells it to.
type fakeProbe struct {
	mu      sync.Mutex
	answers bool
	urls    []string
}

func (f *fakeProbe) Answers(_ context.Context, url string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.urls = append(f.urls, url)
	return f.answers
}

func (f *fakeProbe) set(answers bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answers = answers
}

// fakeShooter writes a real PNG of the size it was asked for and records what it was asked. It never
// launches anything.
type fakeShooter struct {
	mu       sync.Mutex
	requests []preview.ShotRequest
	err      error
}

func (f *fakeShooter) Shoot(_ context.Context, req preview.ShotRequest) (preview.Shot, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	err := f.err
	f.mu.Unlock()
	if err != nil {
		return preview.Shot{}, err
	}
	file, err := os.Create(req.Path)
	if err != nil {
		return preview.Shot{}, err
	}
	defer func() { _ = file.Close() }()
	if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, req.Width, req.Height))); err != nil {
		return preview.Shot{}, err
	}
	return preview.Shot{Width: req.Width, Height: req.Height}, nil
}

func (f *fakeShooter) shots() []preview.ShotRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]preview.ShotRequest(nil), f.requests...)
}

// event is one published event, as the bus would have delivered it.
type event struct {
	topic     string
	eventType string
	data      any
	critical  bool
}

// fakeBus records what was published.
type fakeBus struct {
	mu     sync.Mutex
	events []event
}

func (b *fakeBus) Publish(topic, eventType string, data any, critical bool) uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, event{topic: topic, eventType: eventType, data: data, critical: critical})
	return uint64(len(b.events))
}

func (b *fakeBus) all() []event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]event(nil), b.events...)
}

// harness is a service over fakes, with everything a test needs to look at kept beside it.
type harness struct {
	t        *testing.T
	svc      *preview.Service
	dir      string
	bus      *fakeBus
	runner   *fakeRunner
	shooter  *fakeShooter
	probe    *fakeProbe
	browser  fakeBrowser
	projects *fakeProjects
}

// newHarness builds the service over fakes. The tweak runs last, so a test can change one seam or one
// option without repeating the rest.
func newHarness(t *testing.T, tweak ...func(*preview.Deps, *preview.Options)) *harness {
	t.Helper()
	h := &harness{
		t: t, dir: t.TempDir(), bus: &fakeBus{}, runner: &fakeRunner{}, shooter: &fakeShooter{},
		probe: &fakeProbe{}, browser: fakeBrowser{path: "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", found: true},
		projects: &fakeProjects{
			worktrees: map[string]string{cardA: t.TempDir()},
			projectID: "web-dashboard", command: "pnpm dev",
		},
	}
	opts := preview.Options{
		Runner: h.runner, Shooter: h.shooter, Browser: h.browser, Port: &fakePorts{}, Probe: h.probe,
		ProbeEvery: time.Millisecond, ReadyTimeout: time.Second,
	}
	deps := preview.Deps{
		Projects: h.projects, Bus: h.bus, DataDir: h.dir, Now: func() time.Time { return testNow },
		Options: opts,
	}
	for _, apply := range tweak {
		apply(&deps, &deps.Options)
	}
	svc, err := preview.New(deps)
	if err != nil {
		t.Fatalf("build the preview service: %v", err)
	}
	h.svc = svc
	t.Cleanup(svc.Close)
	return h
}

// waitForState waits until a card's preview is in the state the test is waiting for, so the tests
// that let the probe goroutine run do not have to sleep for a fixed time.
func (h *harness) waitForState(cardID string, want protocol.PreviewState) protocol.Preview {
	h.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last protocol.Preview
	for time.Now().Before(deadline) {
		snapshot, err := h.svc.Snapshot(context.Background(), cardID)
		if err != nil {
			h.t.Fatalf("read the preview: %v", err)
		}
		last = snapshot.Preview
		if last.State == want {
			return last
		}
		time.Sleep(time.Millisecond)
	}
	h.t.Fatalf("the preview never reached %q, it is %q with %q", want, last.State, last.Error)
	return last
}

// startRunning starts a card's preview and waits for it to be running, which is the state every
// screenshot test needs.
func (h *harness) startRunning(cardID string) protocol.Preview {
	h.t.Helper()
	h.probe.set(true)
	started, err := h.svc.Start(context.Background(), cardID)
	if err != nil {
		h.t.Fatalf("start the preview: %v", err)
	}
	if started.Preview.State != protocol.PreviewStateStarting {
		h.t.Fatalf("a preview that was asked to start is %q, want starting", started.Preview.State)
	}
	return h.waitForState(cardID, protocol.PreviewStateRunning)
}

// addCard gives the service a second card of its own, with a worktree and a preview of its own.
func (h *harness) addCard(cardID string) {
	h.t.Helper()
	h.projects.set(cardID, h.t.TempDir())
}
