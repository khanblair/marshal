package preview_test

import (
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
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

// A card nobody has previewed answers `stopped` with the command pressing Start would run, and
// looking at it starts nothing: a person must not start a dev server on their machine by looking.
func TestALookStartsNothingAndSaysWhatStartWouldRun(t *testing.T) {
	h := newHarness(t)
	snapshot, err := h.svc.Snapshot(context.Background(), cardA)
	if err != nil {
		t.Fatalf("read the preview: %v", err)
	}
	if snapshot.Preview.State != protocol.PreviewStateStopped {
		t.Errorf("a preview nobody started is %q, want stopped", snapshot.Preview.State)
	}
	if snapshot.Preview.Command != "pnpm dev" {
		t.Errorf("the preview says the command is %q, want the project's dev command", snapshot.Preview.Command)
	}
	if snapshot.Preview.URL != "" {
		t.Errorf("a stopped preview carries the address %q, want none", snapshot.Preview.URL)
	}
	if snapshot.Preview.Shots == nil {
		t.Error("the shot list is nil, want an empty list so the JSON is [] and never null")
	}
	if got := h.runner.started(); len(got) != 0 {
		t.Errorf("looking at the preview started %d dev servers, want none", len(got))
	}
	if !snapshot.ServerTime.Time().Equal(testNow) {
		t.Errorf("the answer is stamped %v, want the daemon's time", snapshot.ServerTime)
	}
}

// The three state names are the ones the Preview tab already reads: starting at once, running when
// the dev server answers, and stopped when it is stopped. None of them is renamed here.
func TestPreviewMovesThroughTheThreeStatesTheTabReads(t *testing.T) {
	h := newHarness(t)
	running := h.startRunning(cardA)
	if running.URL != "http://127.0.0.1:5101" {
		t.Errorf("a running preview answers on %q, want the port it was given", running.URL)
	}
	if running.StartedAt == nil {
		t.Error("a running preview has no start time, want the moment it answered")
	}
	if port := h.runner.started()[0].Port; running.Port != port {
		t.Errorf("the preview says port %d and the dev server was given port %d", running.Port, port)
	}
	stopped, err := h.svc.Stop(context.Background(), cardA)
	if err != nil {
		t.Fatalf("stop the preview: %v", err)
	}
	if stopped.Preview.State != protocol.PreviewStateStopped {
		t.Errorf("a stopped preview is %q, want stopped", stopped.Preview.State)
	}
	if stopped.Preview.URL != "" || stopped.Preview.StartedAt != nil {
		t.Errorf("a stopped preview still shows %q at %v, want neither", stopped.Preview.URL, stopped.Preview.StartedAt)
	}
	// Stopping what is already stopped is the state the person asked for, not an error.
	if _, err := h.svc.Stop(context.Background(), cardA); err != nil {
		t.Errorf("stopping a stopped preview: %v, want no error", err)
	}
}

// The port is handed to the dev command in the environment, so a project's own dev command needs no
// knowledge of Marshal to answer on the port it was given.
func TestTheDevServerIsGivenItsOwnPortAndTheCardsWorktree(t *testing.T) {
	h := newHarness(t)
	h.startRunning(cardA)
	requests := h.runner.started()
	if len(requests) != 1 {
		t.Fatalf("the dev server was started %d times, want once", len(requests))
	}
	got := requests[0]
	if got.Dir != h.projects.worktree(cardA) {
		t.Errorf("the dev server runs in %q, want the card's worktree %q", got.Dir, h.projects.worktree(cardA))
	}
	if got.Command != "pnpm dev" {
		t.Errorf("the dev server runs %q, want the project's dev command", got.Command)
	}
	if got.Port != 5101 {
		t.Errorf("the dev server was given port %d, want the port the preview answers on", got.Port)
	}
}

// Two cards preview at once without sharing state: their own worktrees, their own ports, their own
// browser profiles, and their own screenshots.
func TestTwoCardsPreviewAtOnceWithoutSharingState(t *testing.T) {
	h := newHarness(t)
	h.addCard(cardB)
	h.probe.set(true)
	if _, err := h.svc.Start(context.Background(), cardA); err != nil {
		t.Fatalf("start the first card's preview: %v", err)
	}
	if _, err := h.svc.Start(context.Background(), cardB); err != nil {
		t.Fatalf("start the second card's preview: %v", err)
	}
	h.waitForState(cardA, protocol.PreviewStateRunning)
	h.waitForState(cardB, protocol.PreviewStateRunning)
	if _, err := h.svc.Screenshot(context.Background(), cardA, protocol.PreviewShotKindBefore); err != nil {
		t.Fatalf("take the first card's screenshot: %v", err)
	}

	requests := h.runner.started()
	if len(requests) != 2 {
		t.Fatalf("two cards started %d dev servers, want two", len(requests))
	}
	if requests[0].Port == requests[1].Port {
		t.Errorf("both cards were given port %d, want one port each", requests[0].Port)
	}
	if requests[0].Dir == requests[1].Dir {
		t.Errorf("both cards run in %q, want one worktree each", requests[0].Dir)
	}
	shots := h.shooter.shots()
	if len(shots) != 1 {
		t.Fatalf("one screenshot was taken, got %d", len(shots))
	}
	// The first card's shot must not be servable as the second card's, and the second card has none.
	if _, _, err := h.svc.ShotFile(cardB, protocol.PreviewShotKindBefore); err == nil {
		t.Error("the second card answers with the first card's screenshot")
	}
	if _, _, err := h.svc.ShotFile(cardA, protocol.PreviewShotKindBefore); err != nil {
		t.Errorf("the first card's own screenshot: %v", err)
	}
	// The second card is still its own preview, untouched by the first card's screenshot.
	snapshot, err := h.svc.Snapshot(context.Background(), cardB)
	if err != nil {
		t.Fatalf("read the second card's preview: %v", err)
	}
	if len(snapshot.Preview.Shots) != 0 {
		t.Errorf("the second card carries %d shots, want none of the first card's", len(snapshot.Preview.Shots))
	}
	if snapshot.Preview.URL == "http://127.0.0.1:5101" {
		t.Error("the second card answers on the first card's port")
	}
}

// One browser profile per card is what keeps two previews from sharing cookies and state.
func TestEachCardGetsItsOwnBrowserProfile(t *testing.T) {
	h := newHarness(t)
	h.addCard(cardB)
	h.startRunning(cardA)
	if _, err := h.svc.Screenshot(context.Background(), cardA, protocol.PreviewShotKindBefore); err != nil {
		t.Fatalf("take the first card's screenshot: %v", err)
	}
	h.startRunning(cardB)
	if _, err := h.svc.Screenshot(context.Background(), cardB, protocol.PreviewShotKindAfter); err != nil {
		t.Fatalf("take the second card's screenshot: %v", err)
	}
	shots := h.shooter.shots()
	if len(shots) != 2 {
		t.Fatalf("two screenshots were taken, got %d", len(shots))
	}
	if shots[0].ProfileDir == shots[1].ProfileDir {
		t.Errorf("both cards used the browser profile %q, want one each", shots[0].ProfileDir)
	}
	for _, shot := range shots {
		if shot.Browser != h.browser.path {
			t.Errorf("the shooter was given browser %q, want the one the lookup found", shot.Browser)
		}
		if shot.URL != "http://127.0.0.1:5101" && shot.URL != "http://127.0.0.1:5102" {
			t.Errorf("the shooter was sent to %q, want the card's own address", shot.URL)
		}
	}
}

// The before and after shots attach to the card, at most one of each, and taking one again replaces
// its own half rather than adding a third.
func TestThePairIsAtMostOneBeforeAndOneAfter(t *testing.T) {
	h := newHarness(t)
	h.startRunning(cardA)
	before, err := h.svc.Screenshot(context.Background(), cardA, protocol.PreviewShotKindBefore)
	if err != nil {
		t.Fatalf("take the before shot: %v", err)
	}
	if before.Outcome != protocol.PreviewShotOutcomeTaken {
		t.Fatalf("the shot outcome is %q, want taken", before.Outcome)
	}
	if before.Notice == "" {
		t.Error("a shot that was taken carries no sentence saying what was captured")
	}
	if len(before.Preview.Shots) != 1 || before.Preview.Shots[0].Kind != protocol.PreviewShotKindBefore {
		t.Fatalf("after the before shot the pair is %+v, want just the before half", before.Preview.Shots)
	}
	after, err := h.svc.Screenshot(context.Background(), cardA, protocol.PreviewShotKindAfter)
	if err != nil {
		t.Fatalf("take the after shot: %v", err)
	}
	if len(after.Preview.Shots) != 2 {
		t.Fatalf("the pair is %+v, want the before and the after", after.Preview.Shots)
	}
	if after.Preview.Shots[0].Kind != protocol.PreviewShotKindBefore ||
		after.Preview.Shots[1].Kind != protocol.PreviewShotKindAfter {
		t.Errorf("the pair reads %+v, want before then after", after.Preview.Shots)
	}
	again, err := h.svc.Screenshot(context.Background(), cardA, protocol.PreviewShotKindBefore)
	if err != nil {
		t.Fatalf("take the before shot again: %v", err)
	}
	if len(again.Preview.Shots) != 2 {
		t.Fatalf("taking a shot again made the pair %+v, want still two", again.Preview.Shots)
	}
	// A shot's size is the image that was written, so a screen can lay it out before it arrives.
	if again.Preview.Shots[0].Width != 1440 || again.Preview.Shots[0].Height != 900 {
		t.Errorf("the before shot is %dx%d, want 1440x900",
			again.Preview.Shots[0].Width, again.Preview.Shots[0].Height)
	}
}

// A pair taken before a restart is still shown after it, and the image is still served.
func TestTheScreenshotsSurviveARestart(t *testing.T) {
	h := newHarness(t)
	h.startRunning(cardA)
	if _, err := h.svc.Screenshot(context.Background(), cardA, protocol.PreviewShotKindBefore); err != nil {
		t.Fatalf("take the before shot: %v", err)
	}
	if _, err := h.svc.Screenshot(context.Background(), cardA, protocol.PreviewShotKindAfter); err != nil {
		t.Fatalf("take the after shot: %v", err)
	}
	h.svc.Close()

	// A daemon starting again builds a new service over the same data folder and the same card.
	restarted, err := preview.New(preview.Deps{
		Projects: h.projects, DataDir: h.dir, Now: func() time.Time { return testNow },
		Options: preview.Options{Runner: h.runner, Shooter: h.shooter, Browser: h.browser, Port: &fakePorts{}, Probe: h.probe},
	})
	if err != nil {
		t.Fatalf("build the service again: %v", err)
	}
	defer restarted.Close()
	snapshot, err := restarted.Snapshot(context.Background(), cardA)
	if err != nil {
		t.Fatalf("read the preview after the restart: %v", err)
	}
	if snapshot.Preview.State != protocol.PreviewStateStopped {
		t.Errorf("a preview after a restart is %q, want stopped: no dev server came back with it", snapshot.Preview.State)
	}
	if len(snapshot.Preview.Shots) != 2 {
		t.Fatalf("the pair after a restart is %+v, want both halves", snapshot.Preview.Shots)
	}
	if snapshot.Preview.Shots[0].Width != 1440 || snapshot.Preview.Shots[0].Height != 900 {
		t.Errorf("the shot after a restart reads %dx%d, want the size the image really is",
			snapshot.Preview.Shots[0].Width, snapshot.Preview.Shots[0].Height)
	}
	path, modified, err := restarted.ShotFile(cardA, protocol.PreviewShotKindBefore)
	if err != nil {
		t.Fatalf("find the before shot after the restart: %v", err)
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Errorf("the shot file at %s cannot be served: %v", path, err)
	}
	// The file's own time comes from the filesystem, so it is only checked for being a real time:
	// the version the address carries is what a client caches on.
	if modified.IsZero() {
		t.Error("the shot file has no written time, so its address cannot carry a version")
	}
	if _, _, err := restarted.ShotFile(cardA, protocol.PreviewShotKind("during")); err == nil {
		t.Error("the service answers with a file for a kind that is neither half of the pair")
	}
}

// A machine with neither Chrome nor Edge skips the check with the sentence that says so, and never
// reports it as passed (docs/library-docs.md).
func TestNoBrowserSkipsTheCheckInASentence(t *testing.T) {
	h := newHarness(t, func(_ *preview.Deps, opts *preview.Options) {
		opts.Browser = fakeBrowser{found: false}
	})
	h.startRunning(cardA)
	answer, err := h.svc.Screenshot(context.Background(), cardA, protocol.PreviewShotKindBefore)
	if err != nil {
		t.Fatalf("take a screenshot with no browser: %v", err)
	}
	if answer.Outcome != protocol.PreviewShotOutcomeSkipped {
		t.Errorf("the outcome is %q, want skipped", answer.Outcome)
	}
	if answer.Notice == "" {
		t.Fatal("a skipped check carries no sentence, so nothing says the check did not run")
	}
	if len(answer.Preview.Shots) != 0 {
		t.Errorf("a skipped check attached %+v, want no shot", answer.Preview.Shots)
	}
	if len(h.shooter.shots()) != 0 {
		t.Error("a browser was driven even though none was found")
	}
	if _, _, err := h.svc.ShotFile(cardA, protocol.PreviewShotKindBefore); err == nil {
		t.Error("a skipped check left a file behind, which would read as a check that ran")
	}
	// The preview itself is untouched: a skipped check is not a preview that stopped.
	snapshot, err := h.svc.Snapshot(context.Background(), cardA)
	if err != nil {
		t.Fatalf("read the preview: %v", err)
	}
	if snapshot.Preview.State != protocol.PreviewStateRunning {
		t.Errorf("the preview is %q after a skipped check, want still running", snapshot.Preview.State)
	}
}

// A screenshot of a preview that is not running is refused in a plain sentence: there is nothing to
// capture, and the person is told to start it.
func TestAShotOfAStoppedPreviewIsRefused(t *testing.T) {
	h := newHarness(t)
	_, err := h.svc.Screenshot(context.Background(), cardA, protocol.PreviewShotKindBefore)
	var answer *protocol.Error
	if !errors.As(err, &answer) || answer.Code != protocol.ErrorCodeRefused {
		t.Fatalf("a screenshot of a stopped preview: %v, want a refusal", err)
	}
	if answer.Message == "" {
		t.Error("the refusal has no sentence saying what to do")
	}
	if len(h.shooter.shots()) != 0 {
		t.Error("a browser was driven for a preview that is not running")
	}
}

// A kind that is neither half of the pair is a bad request, not a guess.
func TestAnUnknownShotKindIsRefused(t *testing.T) {
	h := newHarness(t)
	h.startRunning(cardA)
	_, err := h.svc.Screenshot(context.Background(), cardA, protocol.PreviewShotKind("during"))
	var answer *protocol.Error
	if !errors.As(err, &answer) || answer.Code != protocol.ErrorCodeInvalidArgument {
		t.Fatalf("an unknown shot kind: %v, want an invalid argument", err)
	}
}

// A screenshot that cannot be written is reported as one to try again, with the sentence, and the
// preview keeps running: the browser was there, the shot just did not happen.
func TestAShotThatCannotBeTakenIsReported(t *testing.T) {
	h := newHarness(t, func(_ *preview.Deps, opts *preview.Options) {
		opts.Shooter = &fakeShooter{err: errors.New("the browser closed")}
	})
	h.startRunning(cardA)
	_, err := h.svc.Screenshot(context.Background(), cardA, protocol.PreviewShotKindAfter)
	var answer *protocol.Error
	if !errors.As(err, &answer) || answer.Code != protocol.ErrorCodeUnavailable {
		t.Fatalf("a screenshot that could not be taken: %v, want unavailable", err)
	}
	snapshot, err := h.svc.Snapshot(context.Background(), cardA)
	if err != nil {
		t.Fatalf("read the preview: %v", err)
	}
	if snapshot.Preview.State != protocol.PreviewStateRunning {
		t.Errorf("the preview is %q after a shot failed, want still running", snapshot.Preview.State)
	}
}

// A card with no worktree and a project with no dev command are both refused in a plain sentence: the
// person asked for something that cannot be done, and must be told why rather than shown a spinner.
func TestStartIsRefusedInSentencesThatSayWhatToDo(t *testing.T) {
	t.Run("no worktree", func(t *testing.T) {
		h := newHarness(t, func(deps *preview.Deps, _ *preview.Options) {
			deps.Projects = &fakeProjects{worktrees: map[string]string{cardA: ""},
				projectID: "web-dashboard", command: "pnpm dev"}
		})
		_, err := h.svc.Start(context.Background(), cardA)
		var answer *protocol.Error
		if !errors.As(err, &answer) || answer.Code != protocol.ErrorCodeRefused {
			t.Fatalf("starting a card with no worktree: %v, want a refusal", err)
		}
		if answer.Message == "" {
			t.Error("the refusal has no sentence saying what to do")
		}
		if len(h.runner.started()) != 0 {
			t.Error("a dev server was started for a card with no worktree")
		}
	})
	t.Run("no dev command", func(t *testing.T) {
		h := newHarness(t, func(deps *preview.Deps, _ *preview.Options) {
			deps.Projects = &fakeProjects{worktrees: map[string]string{cardA: t.TempDir()},
				projectID: "web-dashboard", command: ""}
		})
		_, err := h.svc.Start(context.Background(), cardA)
		var answer *protocol.Error
		if !errors.As(err, &answer) || answer.Code != protocol.ErrorCodeRefused {
			t.Fatalf("starting a project with no dev command: %v, want a refusal", err)
		}
		if answer.Message == "" {
			t.Error("the refusal has no sentence saying what to do")
		}
	})
}

// A dev server that will not start at all is one answer with a sentence, not a spin that turns into a
// stop: the state is stopped, and the sentence says what to check.
func TestADevServerThatWillNotStartSaysSoAtOnce(t *testing.T) {
	h := newHarness(t, func(_ *preview.Deps, opts *preview.Options) {
		opts.Runner = &fakeRunner{err: errors.New("no such command")}
	})
	answer, err := h.svc.Start(context.Background(), cardA)
	if err != nil {
		t.Fatalf("start a dev server that will not start: %v", err)
	}
	if answer.Preview.State != protocol.PreviewStateStopped {
		t.Errorf("the preview is %q, want stopped", answer.Preview.State)
	}
	if answer.Preview.Error == "" {
		t.Error("a preview that could not start carries no sentence saying so")
	}
	if answer.Preview.URL != "" {
		t.Errorf("a preview that could not start still answers on %q", answer.Preview.URL)
	}
}

// A dev server that ends before it answers says so at once, rather than spinning out the clock.
func TestADevServerThatStopsBeforeAnsweringSaysSo(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.Start(context.Background(), cardA); err != nil {
		t.Fatalf("start the preview: %v", err)
	}
	procs := h.runner.processes()
	if len(procs) != 1 {
		t.Fatalf("the dev server was started %d times, want once", len(procs))
	}
	procs[0].exit()
	stopped := h.waitForState(cardA, protocol.PreviewStateStopped)
	if stopped.Error == "" {
		t.Error("a dev server that stopped before answering carries no sentence saying so")
	}
}

// A dev server that never answers is given the ready timeout and then says so, rather than leaving
// the tab spinning for ever.
func TestADevServerThatNeverAnswersSaysSo(t *testing.T) {
	h := newHarness(t, func(deps *preview.Deps, opts *preview.Options) {
		opts.ReadyTimeout = 20 * time.Millisecond
		opts.ProbeEvery = time.Millisecond
		// The ready timeout is measured against the clock, so this one test needs a clock that moves.
		deps.Now = time.Now
	})
	h.probe.set(false)
	if _, err := h.svc.Start(context.Background(), cardA); err != nil {
		t.Fatalf("start the preview: %v", err)
	}
	stopped := h.waitForState(cardA, protocol.PreviewStateStopped)
	if stopped.Error == "" {
		t.Error("a dev server that never answered carries no sentence saying so")
	}
	if len(h.probe.urls) == 0 {
		t.Error("the dev server was never asked whether it answers")
	}
}

// Every state change reaches every screen watching the card, on the card's own topic, carrying the
// whole preview: an event applies exactly as the snapshot does.
func TestEveryStateChangeIsPublishedOnTheCardsTopic(t *testing.T) {
	h := newHarness(t)
	h.startRunning(cardA)
	wantTopic := string(protocol.CardTopic(cardA))
	events := h.bus.all()
	if len(events) < 2 {
		t.Fatalf("starting a preview published %d events, want at least starting and running", len(events))
	}
	for _, got := range events {
		if got.topic != wantTopic {
			t.Errorf("an event went to topic %q, want the card's own %q", got.topic, wantTopic)
		}
		if got.eventType != string(protocol.EventTypePreviewStateChanged) {
			t.Errorf("an event is of type %q, want %q", got.eventType, protocol.EventTypePreviewStateChanged)
		}
		if got.critical {
			t.Error("a preview's state was published as a critical event, which can drop others")
		}
	}
	if events[0].data.(protocol.PreviewEventData).Preview.State != protocol.PreviewStateStarting {
		t.Errorf("the first event carries %+v, want the preview starting",
			events[0].data.(protocol.PreviewEventData).Preview.State)
	}
	last := events[len(events)-1].data.(protocol.PreviewEventData).Preview
	if last.State != protocol.PreviewStateRunning {
		t.Errorf("the last event carries %q, want running", last.State)
	}
	if last.URL == "" {
		t.Error("the event that says the preview is running carries no address")
	}
}

// Shutting the daemon down stops every dev server it started, so nothing is left running on the
// person's machine after Marshal is gone.
func TestCloseStopsEveryDevServer(t *testing.T) {
	h := newHarness(t)
	h.addCard(cardB)
	h.startRunning(cardA)
	h.startRunning(cardB)
	procs := h.runner.processes()
	if len(procs) != 2 {
		t.Fatalf("two cards started %d dev servers, want two", len(procs))
	}
	h.svc.Close()
	for i, proc := range procs {
		if !proc.wasStopped() {
			t.Errorf("dev server %d was left running after the daemon closed", i)
		}
	}
}

// A card Marshal cannot read at all is answered as not found, not as a preview that failed.
func TestACardThatCannotBeReadIsNotAPreviewProblem(t *testing.T) {
	h := newHarness(t, func(deps *preview.Deps, _ *preview.Options) {
		deps.Projects = &fakeProjects{readErr: protocol.NotFound("card").With("id", cardA)}
	})
	_, err := h.svc.Snapshot(context.Background(), cardA)
	var answer *protocol.Error
	if !errors.As(err, &answer) || answer.Code != protocol.ErrorCodeNotFound {
		t.Fatalf("reading a card that cannot be found: %v, want not found", err)
	}
}

// A card's shot is kept under the daemon's own data folder, never beside the repository, so nothing
// Marshal writes while taking a screenshot lands in the person's worktree.
func TestScreenshotsAreKeptUnderTheDaemonsDataFolder(t *testing.T) {
	h := newHarness(t)
	h.startRunning(cardA)
	if _, err := h.svc.Screenshot(context.Background(), cardA, protocol.PreviewShotKindAfter); err != nil {
		t.Fatalf("take the after shot: %v", err)
	}
	path, _, err := h.svc.ShotFile(cardA, protocol.PreviewShotKindAfter)
	if err != nil {
		t.Fatalf("find the after shot: %v", err)
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("the shot is kept at %q, want an absolute path", path)
	}
	if rel, err := filepath.Rel(h.dir, path); err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		t.Errorf("the shot at %q is not under the daemon's data folder %q", path, h.dir)
	}
	if rel, err := filepath.Rel(h.projects.worktree(cardA), path); err == nil && !strings.HasPrefix(rel, "..") {
		t.Errorf("the shot at %q landed inside the card's worktree", path)
	}
}
