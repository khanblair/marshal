package api_test

// The preview routes (docs/architecture.md sections 11.2 and N10, docs/backend-checklist.md B6.6,
// build-plan 6.6 and 6.7, docs/marshal-product-scope.md 15.1 and 15.2). internal/preview's own
// tests drive the service over its fakes; this file tests the wire - the tab reads a preview
// without starting anything, pressing Start runs the project's dev command for that one card on a
// port of its own, two cards preview at once without sharing state, a screenshot is served back as
// the image it is, and a machine with no browser answers a skipped check with the sentence that
// says so rather than a pass. Every seam is a fake, so no test here starts a dev server or launches
// a browser.

import (
	"context"
	"errors"
	"image"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/preview"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// fakeDevProcess is one dev server a fake runner started. It never exits on its own, and it
// remembers whether it was stopped, so a test can prove the route stopped it.
type fakeDevProcess struct {
	done    chan struct{}
	once    sync.Once
	mu      sync.Mutex
	stopped bool
}

func newFakeDevProcess() *fakeDevProcess { return &fakeDevProcess{done: make(chan struct{})} }

func (p *fakeDevProcess) Done() <-chan struct{} { return p.done }

func (p *fakeDevProcess) Stop(_ context.Context, _ time.Duration) error {
	p.mu.Lock()
	p.stopped = true
	p.mu.Unlock()
	p.once.Do(func() { close(p.done) })
	return nil
}

func (p *fakeDevProcess) wasStopped() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stopped
}

// fakeDevRunner stands in for the command runner, so the wire is proven without starting a process.
// It records what it was asked, answers a process of its own, and can be made to fail.
type fakeDevRunner struct {
	mu       sync.Mutex
	requests []preview.DevRequest
	procs    []*fakeDevProcess
	err      error
}

func (f *fakeDevRunner) Start(_ context.Context, req preview.DevRequest) (preview.Process, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	proc := newFakeDevProcess()
	f.procs = append(f.procs, proc)
	return proc, nil
}

func (f *fakeDevRunner) asked() []preview.DevRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]preview.DevRequest(nil), f.requests...)
}

func (f *fakeDevRunner) last() *fakeDevProcess {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.procs) == 0 {
		return nil
	}
	return f.procs[len(f.procs)-1]
}

// fakeShooter writes a real PNG where it is asked, so the serving route is proven over real bytes,
// and records what it was asked for. It can be made to fail.
type fakeShooter struct {
	mu     sync.Mutex
	shots  []preview.ShotRequest
	err    error
	width  int
	height int
}

func (f *fakeShooter) Shoot(_ context.Context, req preview.ShotRequest) (preview.Shot, error) {
	f.mu.Lock()
	f.shots = append(f.shots, req)
	err, w, h := f.err, f.width, f.height
	f.mu.Unlock()
	if err != nil {
		return preview.Shot{}, err
	}
	if w == 0 {
		w = preview.DefaultViewportWidth
	}
	if h == 0 {
		h = preview.DefaultViewportHeight
	}
	if err := os.MkdirAll(filepath.Dir(req.Path), 0o700); err != nil {
		return preview.Shot{}, err
	}
	file, err := os.Create(req.Path)
	if err != nil {
		return preview.Shot{}, err
	}
	defer func() { _ = file.Close() }()
	if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		return preview.Shot{}, err
	}
	return preview.Shot{Width: w, Height: h}, nil
}

func (f *fakeShooter) taken() []preview.ShotRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]preview.ShotRequest(nil), f.shots...)
}

// fakeBrowser answers whether Marshal found a browser, so the skipped-check path is proven on a
// machine that has Chrome and on a machine that does not.
type fakeBrowser struct {
	path  string
	found bool
}

func (f *fakeBrowser) Find() (string, bool) { return f.path, f.found }

// fakePorts hands out a free port per card, in order, so two cards never race for the same number
// and a test can prove each card got its own.
type fakePorts struct {
	mu   sync.Mutex
	next int
	got  []int
}

func (f *fakePorts) Pick() (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.next == 0 {
		f.next = 5100
	}
	port := f.next
	f.next++
	f.got = append(f.got, port)
	return port, nil
}

// fakeProbe answers whether a dev server answers yet. It answers at once by default, so a preview
// reaches `running` without a server behind it.
type fakeProbe struct {
	mu     sync.Mutex
	answer bool
	seen   []string
}

func (f *fakeProbe) Answers(_ context.Context, url string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen = append(f.seen, url)
	return f.answer
}

// previewFakes are the seams a preview stack was built with, so a test can prove what reached them.
type previewFakes struct {
	runner  *fakeDevRunner
	shooter *fakeShooter
	browser *fakeBrowser
	ports   *fakePorts
	probe   *fakeProbe
}

// previewStack builds a stack whose preview module runs over fakes and hands them back. The prober
// answers at once and the browser is found by default, so a preview reaches `running` and takes a
// screenshot with nothing real behind either.
func previewStack(t *testing.T, tweak func(*preview.Options)) (*stack, *previewFakes) {
	t.Helper()
	fakes := &previewFakes{
		runner:  &fakeDevRunner{},
		shooter: &fakeShooter{},
		browser: &fakeBrowser{path: "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", found: true},
		ports:   &fakePorts{},
		probe:   &fakeProbe{answer: true},
	}
	opts := preview.Options{
		Runner: fakes.runner, Shooter: fakes.shooter, Browser: fakes.browser,
		Port: fakes.ports, Probe: fakes.probe,
		ReadyTimeout: 10 * time.Second, ProbeEvery: time.Millisecond,
	}
	if tweak != nil {
		tweak(&opts)
	}
	return newStack(t, withPreviewOptions(opts)), fakes
}

// previewCard adds a card whose project has the given dev command and whose worktree is on disk -
// the two things starting a preview needs - and returns the card and the worktree folder.
func previewCard(t *testing.T, st *stack, command string) (protocol.Card, string) {
	t.Helper()
	project, repo := st.addProject("small-repo")
	st.setDevCommand(project.ID, command)
	card := st.addCard(project.ID, "Preview the change")
	return card, st.startWorktree(t, project, repo, card)
}

// setDevCommand sets a project's dev command through the API, as the project settings screen does.
func (st *stack) setDevCommand(projectID, command string) {
	st.t.Helper()
	st.do(http.MethodPatch, "/v1/projects/"+projectID,
		protocol.UpdateProjectRequest{DevCommand: &command}).want(st.t, http.StatusOK)
}

// readPreview reads a card's preview.
func readPreview(t *testing.T, st *stack, cardID string) protocol.Preview {
	t.Helper()
	r := st.do(http.MethodGet, "/v1/cards/"+cardID+"/preview", nil).want(t, http.StatusOK)
	return decode[protocol.PreviewSnapshot](t, r).Preview
}

// waitForPreview reads a card's preview until it reaches the state the test is waiting for, so a
// test never guesses how long a preview takes to answer.
func waitForPreview(t *testing.T, st *stack, cardID string, want protocol.PreviewState) protocol.Preview {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := readPreview(t, st, cardID)
		if got.State == want {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("the preview is %q, want %q", got.State, want)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// startRunning starts a card's preview and waits for it to answer, which is what every screenshot
// test needs first.
func startRunning(t *testing.T, st *stack, cardID string) protocol.Preview {
	t.Helper()
	st.do(http.MethodPost, "/v1/cards/"+cardID+"/preview/start", nil).want(t, http.StatusOK)
	return waitForPreview(t, st, cardID, protocol.PreviewStateRunning)
}

// Reading a preview starts nothing. A person looking at the tab must not start a dev server on
// their own machine by looking, and the answer carries the dev command so the tab can say what
// pressing Start would run.
func TestReadingAPreviewStartsNothing(t *testing.T) {
	st, fakes := previewStack(t, nil)
	card, _ := previewCard(t, st, "pnpm dev")

	r := st.do(http.MethodGet, "/v1/cards/"+card.ID+"/preview", nil).want(t, http.StatusOK)
	sameShape(t, "preview-snapshot", r.Body)
	got := decode[protocol.PreviewSnapshot](t, r)

	if got.Preview.CardID != card.ID || got.Preview.State != protocol.PreviewStateStopped {
		t.Errorf("the preview = %+v, want the card stopped", got.Preview)
	}
	if got.Preview.Command != "pnpm dev" {
		t.Errorf("the preview's command = %q, want the project's own dev command", got.Preview.Command)
	}
	if got.Preview.Port != 0 || got.Preview.URL != "" {
		t.Errorf("a stopped preview names port %d and address %q, want neither", got.Preview.Port, got.Preview.URL)
	}
	if got.Preview.StartedAt != nil {
		t.Errorf("a stopped preview has a start time, %v, want none", got.Preview.StartedAt)
	}
	if got.Preview.Error != "" {
		t.Errorf("a preview that was never started carries the sentence %q, want none", got.Preview.Error)
	}
	if got.Preview.Shots == nil {
		t.Error("the shot list is nil, want an empty list so the JSON is [] and never null")
	}
	if got.ServerTime.Time().IsZero() {
		t.Error("the answer carries no server time")
	}
	if asked := fakes.runner.asked(); len(asked) != 0 {
		t.Errorf("looking at a preview started %d dev servers, want none", len(asked))
	}
}

// Pressing Start runs the project's dev command in the card's own worktree, on a port picked for
// that card, and answers `starting` at once rather than holding the request open.
func TestStartingAPreviewRunsTheProjectsDevCommandForThatCard(t *testing.T) {
	st, fakes := previewStack(t, nil)
	card, dir := previewCard(t, st, "pnpm dev")

	started := decode[protocol.PreviewSnapshot](t, st.do(http.MethodPost,
		"/v1/cards/"+card.ID+"/preview/start", nil).want(t, http.StatusOK))
	if started.Preview.State != protocol.PreviewStateStarting {
		t.Errorf("a preview just started is %q, want starting", started.Preview.State)
	}
	if started.Preview.URL != "" {
		t.Errorf("a starting preview names the address %q, want none until it answers", started.Preview.URL)
	}
	if started.Preview.StartedAt != nil {
		t.Error("a starting preview has a start time, want none until it is running")
	}

	asked := fakes.runner.asked()
	if len(asked) != 1 {
		t.Fatalf("the dev runner was asked %d times, want one", len(asked))
	}
	if asked[0].Command != "pnpm dev" || asked[0].Dir != dir {
		t.Errorf("the dev server was started with %+v, want the project's command in the card's worktree", asked[0])
	}
	if asked[0].Port == 0 {
		t.Error("the dev server was given no port, so two cards would share one")
	}

	// When the dev server answers, the preview is running and shows the address it answers on.
	running := waitForPreview(t, st, card.ID, protocol.PreviewStateRunning)
	if running.Port != asked[0].Port {
		t.Errorf("the running preview is on port %d, want the one the dev server was given, %d",
			running.Port, asked[0].Port)
	}
	if want := "http://127.0.0.1:" + strconv.Itoa(asked[0].Port); running.URL != want {
		t.Errorf("the running preview answers at %q, want %q", running.URL, want)
	}
	if running.StartedAt == nil || running.StartedAt.Time().IsZero() {
		t.Error("a running preview has no start time, so a screen cannot count how long it has run")
	}
	if running.Error != "" {
		t.Errorf("a preview that started carries the sentence %q, want none", running.Error)
	}
}

// Two cards preview at once without sharing state: each has its own dev server, its own port, and
// its own address, so one stop is not the other's.
func TestTwoCardsPreviewAtOnceWithoutSharedState(t *testing.T) {
	st, fakes := previewStack(t, nil)
	project, repo := st.addProject("small-repo")
	st.setDevCommand(project.ID, "pnpm dev")
	first := st.addCard(project.ID, "First card")
	second := st.addCard(project.ID, "Second card")
	firstDir := st.startWorktree(t, project, repo, first)
	secondDir := st.startWorktree(t, project, repo, second)

	firstRunning := startRunning(t, st, first.ID)
	secondRunning := startRunning(t, st, second.ID)

	if firstRunning.Port == 0 || secondRunning.Port == 0 || firstRunning.Port == secondRunning.Port {
		t.Errorf("the two previews are on ports %d and %d, want one of its own each",
			firstRunning.Port, secondRunning.Port)
	}
	if firstRunning.CardID != first.ID || secondRunning.CardID != second.ID {
		t.Errorf("the two previews name %q and %q, want their own cards",
			firstRunning.CardID, secondRunning.CardID)
	}
	if firstRunning.URL == secondRunning.URL {
		t.Errorf("both previews answer at %q, want one address each", firstRunning.URL)
	}
	asked := fakes.runner.asked()
	if len(asked) != 2 {
		t.Fatalf("the dev runner was asked %d times, want one per card", len(asked))
	}
	if asked[0].Dir != firstDir || asked[1].Dir != secondDir {
		t.Errorf("the dev servers ran in %q and %q, want each card's own worktree", asked[0].Dir, asked[1].Dir)
	}
	if asked[0].Port != firstRunning.Port || asked[1].Port != secondRunning.Port {
		t.Errorf("the dev servers were given ports %d and %d, want each preview's own",
			asked[0].Port, asked[1].Port)
	}
	// Stopping the first leaves the second running: the two share nothing.
	st.do(http.MethodPost, "/v1/cards/"+first.ID+"/preview/stop", nil).want(t, http.StatusOK)
	if still := readPreview(t, st, second.ID); still.State != protocol.PreviewStateRunning {
		t.Errorf("the second card's preview is %q after the first stopped, want running", still.State)
	}
}

// A screenshot is taken through the browser the person has, written under the daemon's data folder,
// and served back as the image it is. Its address carries the image's version, so the bytes can be
// kept for as long as the address names that version.
func TestAScreenshotIsTakenAndServedBackAsAnImage(t *testing.T) {
	st, fakes := previewStack(t, nil)
	card, _ := previewCard(t, st, "pnpm dev")
	running := startRunning(t, st, card.ID)

	r := st.do(http.MethodPost, "/v1/cards/"+card.ID+"/preview/shots",
		protocol.PreviewShotRequest{Kind: protocol.PreviewShotKindBefore}).want(t, http.StatusOK)
	sameShape(t, "preview-shot-result", r.Body)
	got := decode[protocol.PreviewShotResult](t, r)
	if got.Outcome != protocol.PreviewShotOutcomeTaken {
		t.Errorf("the outcome = %q, want taken", got.Outcome)
	}
	if got.Notice != "The screenshot was taken." {
		t.Errorf("the notice = %q, want the sentence saying the screenshot was taken", got.Notice)
	}
	if got.ServerTime.Time().IsZero() {
		t.Error("the answer carries no server time")
	}
	if got.Preview.State != protocol.PreviewStateRunning {
		t.Errorf("taking a screenshot left the preview %q, want it still running", got.Preview.State)
	}
	if len(got.Preview.Shots) != 1 {
		t.Fatalf("the preview carries %+v, want the one shot that was taken", got.Preview.Shots)
	}
	shot := got.Preview.Shots[0]
	if shot.Kind != protocol.PreviewShotKindBefore {
		t.Errorf("the shot is the %q half, want before", shot.Kind)
	}
	if shot.Width != preview.DefaultViewportWidth || shot.Height != preview.DefaultViewportHeight {
		t.Errorf("the shot reads %dx%d, want the size it was rendered at",
			shot.Width, shot.Height)
	}
	if shot.TakenAt.Time().IsZero() {
		t.Error("the shot has no time, so a screen cannot say how old it is")
	}
	if !strings.HasPrefix(string(shot.URL), "/v1/cards/"+card.ID+"/preview/shots/before.png?v=") {
		t.Errorf("the shot's address = %q, want the card's own before.png with a version", shot.URL)
	}

	// The shooter was given the running preview's address, the card's own browser profile, and the
	// browser that was found - so two cards never drive one profile.
	taken := fakes.shooter.taken()
	if len(taken) != 1 {
		t.Fatalf("the shooter was asked %d times, want one", len(taken))
	}
	if taken[0].URL != running.URL {
		t.Errorf("the screenshot was taken of %q, want the running preview's address %q", taken[0].URL, running.URL)
	}
	if taken[0].Browser != fakes.browser.path {
		t.Errorf("the screenshot drove %q, want the browser that was found", taken[0].Browser)
	}
	if !strings.Contains(taken[0].ProfileDir, card.ID) {
		t.Errorf("the browser profile is %q, want one under the card's own folder", taken[0].ProfileDir)
	}
	if taken[0].Width != preview.DefaultViewportWidth || taken[0].Height != preview.DefaultViewportHeight {
		t.Errorf("the shot was rendered at %dx%d, want the default viewport", taken[0].Width, taken[0].Height)
	}
	if filepath.Dir(filepath.Dir(taken[0].Path)) != filepath.Join(st.dataDir, "previews") {
		t.Errorf("the shot was written to %q, want it under the daemon's own data folder", taken[0].Path)
	}

	// The image comes back as the image it is, and its address's version is the one the file has, so
	// a request that names the current version may be kept.
	image := st.do(http.MethodGet, shot.URL, nil).want(t, http.StatusOK)
	if contentType := image.Header.Get("Content-Type"); contentType != "image/png" {
		t.Errorf("the image's Content-Type = %q, want image/png", contentType)
	}
	if image.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("the image's X-Content-Type-Options = %q, want nosniff", image.Header.Get("X-Content-Type-Options"))
	}
	if cache := image.Header.Get("Cache-Control"); cache != "private, max-age=31536000, immutable" {
		t.Errorf("the image's Cache-Control = %q, want it kept for as long as its address names this version", cache)
	}
	if len(image.Body) < 8 || string(image.Body[1:4]) != "PNG" {
		t.Errorf("the image is not a PNG: % x", image.Body[:min(8, len(image.Body))])
	}

	// Without the version, the same address is checked every time rather than kept.
	bare := st.do(http.MethodGet, "/v1/cards/"+card.ID+"/preview/shots/before.png", nil).want(t, http.StatusOK)
	if cache := bare.Header.Get("Cache-Control"); cache != "private, no-cache" {
		t.Errorf("an address with no version has Cache-Control %q, want it checked each time", cache)
	}
}

// A machine with neither Chrome nor Edge skips the check with the sentence that says so, and leaves
// no shot behind: a check that never ran must never read as passed (docs/library-docs.md).
func TestAMissingBrowserSkipsTheCheckWithItsSentence(t *testing.T) {
	st, fakes := previewStack(t, func(o *preview.Options) {
		o.Browser = &fakeBrowser{found: false}
	})
	card, _ := previewCard(t, st, "pnpm dev")
	startRunning(t, st, card.ID)

	got := decode[protocol.PreviewShotResult](t, st.do(http.MethodPost, "/v1/cards/"+card.ID+"/preview/shots",
		protocol.PreviewShotRequest{Kind: protocol.PreviewShotKindBefore}).want(t, http.StatusOK))
	if got.Outcome != protocol.PreviewShotOutcomeSkipped {
		t.Errorf("the outcome = %q, want skipped when no browser was found", got.Outcome)
	}
	if got.Notice == "" || !strings.Contains(got.Notice, "skipped") ||
		(!strings.Contains(got.Notice, "Chrome") && !strings.Contains(got.Notice, "Edge")) {
		t.Errorf("the notice = %q, want a sentence saying the check was skipped and naming the browser", got.Notice)
	}
	if len(got.Preview.Shots) != 0 {
		t.Errorf("a skipped check attached %+v, want no shot", got.Preview.Shots)
	}
	if len(fakes.shooter.taken()) != 0 {
		t.Error("a browser was driven even though none was found")
	}
	// Nothing was written, so the image route has nothing to serve.
	st.do(http.MethodGet, "/v1/cards/"+card.ID+"/preview/shots/before.png", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}

// A screenshot of a preview that is not running is refused: the person asked for something that
// cannot be captured and must be told to start the preview first.
func TestAShotOfAPreviewThatIsNotRunningIsRefused(t *testing.T) {
	st, fakes := previewStack(t, nil)
	card, _ := previewCard(t, st, "pnpm dev")

	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/preview/shots",
		protocol.PreviewShotRequest{Kind: protocol.PreviewShotKindBefore}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if len(fakes.shooter.taken()) != 0 {
		t.Error("a browser was driven for a preview that was not running")
	}
}

// A kind that is neither half of the pair is refused, and its image address is not found. A route
// that guessed would serve one half as the other.
func TestAnUnknownShotKindIsRefused(t *testing.T) {
	st, _ := previewStack(t, nil)
	card, _ := previewCard(t, st, "pnpm dev")
	startRunning(t, st, card.ID)

	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/preview/shots",
		protocol.PreviewShotRequest{Kind: protocol.PreviewShotKind("during")}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	st.do(http.MethodGet, "/v1/cards/"+card.ID+"/preview/shots/during.png", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	// A file that is not a PNG at all is not found either.
	st.do(http.MethodGet, "/v1/cards/"+card.ID+"/preview/shots/before", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}

// Stopping a preview stops the dev server, and stopping one that is not running is not an error:
// the person asked for the state they wanted and have it.
func TestStoppingAPreviewStopsTheDevServer(t *testing.T) {
	st, fakes := previewStack(t, nil)
	card, _ := previewCard(t, st, "pnpm dev")
	startRunning(t, st, card.ID)

	stopped := decode[protocol.PreviewSnapshot](t, st.do(http.MethodPost,
		"/v1/cards/"+card.ID+"/preview/stop", nil).want(t, http.StatusOK))
	if stopped.Preview.State != protocol.PreviewStateStopped {
		t.Errorf("the preview after Stop is %q, want stopped", stopped.Preview.State)
	}
	if stopped.Preview.URL != "" || stopped.Preview.StartedAt != nil {
		t.Errorf("a stopped preview = %+v, want no address and no start time", stopped.Preview)
	}
	proc := fakes.runner.last()
	if proc == nil || !proc.wasStopped() {
		t.Error("the dev server was not stopped, so it would be left running on the person's machine")
	}
	// Stopping again answers the same state without an error.
	again := decode[protocol.PreviewSnapshot](t, st.do(http.MethodPost,
		"/v1/cards/"+card.ID+"/preview/stop", nil).want(t, http.StatusOK))
	if again.Preview.State != protocol.PreviewStateStopped {
		t.Errorf("stopping a stopped preview answers %q, want stopped", again.Preview.State)
	}
	if len(fakes.runner.asked()) != 1 {
		t.Error("stopping a preview started another dev server")
	}
}

// A card with nothing to run in, and a project with no dev command, are both refused in a plain
// sentence rather than answered with a spinner that never ends.
func TestStartingAPreviewRefusesACardWithNothingToRun(t *testing.T) {
	st, fakes := previewStack(t, nil)
	project, repo := st.addProject("small-repo")
	st.setDevCommand(project.ID, "pnpm dev")

	// A card that has not started has no worktree.
	notStarted := st.addCard(project.ID, "Not started")
	refusal := st.do(http.MethodPost, "/v1/cards/"+notStarted.ID+"/preview/start", nil).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if !strings.Contains(refusal.Message, "worktree") {
		t.Errorf("the refusal = %q, want it to say the card has no worktree", refusal.Message)
	}

	// A card with a worktree whose project has no dev command cannot be started either.
	st.setDevCommand(project.ID, "")
	noCommand := st.addCard(project.ID, "No dev command")
	st.startWorktree(t, project, repo, noCommand)
	refusal = st.do(http.MethodPost, "/v1/cards/"+noCommand.ID+"/preview/start", nil).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if !strings.Contains(refusal.Message, "dev command") {
		t.Errorf("the refusal = %q, want it to say the project has no dev command", refusal.Message)
	}

	if len(fakes.runner.asked()) != 0 {
		t.Error("a dev server was started for a card that has nothing to run")
	}
}

// A dev server that will not start is answered as a stopped preview with the sentence that says so,
// not as an error the person cannot act on.
func TestADevServerThatWillNotStartIsAnsweredWithItsSentence(t *testing.T) {
	st, fakes := previewStack(t, nil)
	card, _ := previewCard(t, st, "pnpm dev")
	// The service holds the fake runner itself, so it can be made to fail after the stack is built.
	fakes.runner.mu.Lock()
	fakes.runner.err = errors.New("the command is not on this machine")
	fakes.runner.mu.Unlock()

	got := decode[protocol.PreviewSnapshot](t, st.do(http.MethodPost,
		"/v1/cards/"+card.ID+"/preview/start", nil).want(t, http.StatusOK))
	if got.Preview.State != protocol.PreviewStateStopped {
		t.Errorf("a preview whose dev server would not start is %q, want stopped", got.Preview.State)
	}
	if got.Preview.Error == "" {
		t.Error("a preview whose dev server would not start carries no sentence saying why")
	}
	if got.Preview.URL != "" {
		t.Errorf("the failed preview names the address %q, want none", got.Preview.URL)
	}
}

// A screenshot that cannot be written is reported as unavailable with a sentence, rather than as a
// taken shot with no image behind it.
func TestAShotThatCannotBeWrittenIsUnavailable(t *testing.T) {
	st, fakes := previewStack(t, nil)
	card, _ := previewCard(t, st, "pnpm dev")
	startRunning(t, st, card.ID)
	fakes.shooter.mu.Lock()
	fakes.shooter.err = errors.New("the browser would not start")
	fakes.shooter.mu.Unlock()

	got := st.do(http.MethodPost, "/v1/cards/"+card.ID+"/preview/shots",
		protocol.PreviewShotRequest{Kind: protocol.PreviewShotKindAfter}).
		apiError(t, http.StatusServiceUnavailable, protocol.ErrorCodeUnavailable)
	if got.Message == "" {
		t.Error("the answer carries no sentence saying the screenshot could not be taken")
	}
}

// Every state change is published on the card's own topic as `preview.state_changed`, carrying the
// whole preview so a screen applies the event exactly as it applies the snapshot.
func TestThePreviewStateIsPublishedOnTheCardsTopic(t *testing.T) {
	st, _ := previewStack(t, nil)
	card, _ := previewCard(t, st, "pnpm dev")
	sub := st.bus.Subscribe(events.AllTopics())
	defer sub.Close()

	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/preview/start", nil).want(t, http.StatusOK)

	ev := nextOfType(t, sub, protocol.EventTypePreviewStateChanged)
	if ev.Topic != string(protocol.CardTopic(card.ID)) {
		t.Errorf("preview.state_changed arrived on %q, want the card's own topic", ev.Topic)
	}
	if ev.Critical {
		t.Error("preview.state_changed is critical, so a slow screen could miss a state change")
	}
	data, ok := ev.Data.(protocol.PreviewEventData)
	if !ok {
		t.Fatalf("preview.state_changed carries %T, want protocol.PreviewEventData", ev.Data)
	}
	if data.Preview.CardID != card.ID {
		t.Errorf("the event's preview names %q, want the card", data.Preview.CardID)
	}
	// The first event is the move to `starting`, which is what the tab shows while it waits.
	if data.Preview.State != protocol.PreviewStateStarting {
		t.Errorf("the first state change is to %q, want starting", data.Preview.State)
	}
	// The move to `running` follows, carrying the address.
	run := nextOfType(t, sub, protocol.EventTypePreviewStateChanged)
	if runData, ok := run.Data.(protocol.PreviewEventData); !ok {
		t.Fatalf("the second event carries %T, want protocol.PreviewEventData", run.Data)
	} else if runData.Preview.State != protocol.PreviewStateRunning || runData.Preview.URL == "" {
		t.Errorf("the second state change = %+v, want running with its address", runData.Preview)
	}
}
