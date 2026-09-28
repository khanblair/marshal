// Package preview implements a card's live preview (docs/architecture.md section 11.2 and N10,
// docs/backend-checklist.md B6.6 and B6.7, build-plan 6.6 and 6.7, docs/marshal-product-scope.md
// 15.1 and 15.2).
//
// The service runs one dev server per card, on its own port, in the card's own worktree, and keeps
// what state that preview is in. It is the daemon's half of the Preview tab: the tab draws the three
// state names this module answers with, and it does not change them. Screenshots are taken through a
// browser on the person's own machine, each card in its own profile, and are served back as files.
//
// Nothing here starts a browser or a dev server unless it is asked to: looking at the tab reads the
// state and starts nothing, and a test gives a fake for both, so no test on any machine launches a
// real browser or a real dev server.
package preview

import (
	"context"
	"errors"
	"image"
	_ "image/png" // the shot reader needs the PNG decoder, which registers itself here
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const (
	// DefaultReadyTimeout is how long a dev server is given to answer before Marshal gives up and
	// says so. A dev server that takes longer than this is nearly always misconfigured.
	DefaultReadyTimeout = 60 * time.Second
	// DefaultProbeEvery is how often Marshal asks whether the dev server answers yet. A dev server
	// usually answers in a few hundred milliseconds, so this is fast enough to feel immediate and
	// slow enough to be nearly free.
	DefaultProbeEvery = 400 * time.Millisecond
	// DefaultViewportWidth is the width a screenshot is taken at, so two shots of the same page
	// line up.
	DefaultViewportWidth = 1440
	// DefaultViewportHeight is the height a screenshot is taken at, so two shots of the same page
	// line up.
	DefaultViewportHeight = 900
	// stopGrace is how long a dev server is given to stop politely before it is killed.
	stopGrace = 3 * time.Second
	// previewsFolder is the folder under the daemon's data directory that holds the previews:
	// `<data>/previews/<card id>/before.png`, `after.png`, and the browser profile.
	previewsFolder = "previews"
	// profileFolder is the per-card browser profile inside a card's preview folder. It is what keeps
	// two cards' previews from sharing cookies or state.
	profileFolder = "profile"
	// shotExt is the image kind a screenshot is written as.
	shotExt = ".png"
)

// The answers for a preview that cannot be run, in the words of docs/ui-rules.md.
const (
	// messageNoWorktree is answered for a card that has not started, which has nothing to run in.
	messageNoWorktree = "This card has no worktree yet, so there is nothing to preview. Start the card first."
	// messageNoCommand is answered for a project whose dev command nobody set, so Marshal does not
	// know how to start the app.
	messageNoCommand = "This project has no dev command, so Marshal does not know how to start it. Add one in project settings."
	// messageNotRunning is answered when a screenshot is asked for a preview that is not running.
	messageNotRunning = "This card's preview is not running, so there is nothing to capture. Start the preview first."
	// messageNoBrowser is the notice a skipped screenshot check carries. The check is skipped, never
	// silently passed (docs/library-docs.md).
	messageNoBrowser = "Marshal found no Chrome or Edge on this machine, so the screenshot check was skipped. Install Chrome or Edge to take screenshots."
	// messageShotFailed is the sentence behind an answer of `unavailable` when a browser is present
	// but the shot could not be written.
	messageShotFailed = "Marshal could not take the screenshot. Check that the preview is still running and try again."
	// messageReadyTimeout is the sentence a preview carries when its dev server never answered.
	messageReadyTimeout = "The dev server did not answer in time. Check the project's dev command and try again."
	// messageStoppedEarly is the sentence a preview carries when its dev server exited first.
	messageStoppedEarly = "The dev server stopped before it answered. Check the project's dev command and try again."
	// messageStartFailed is answered when the dev server could not be started at all.
	messageStartFailed = "Marshal could not start the dev server. Check the project's dev command and try again."
)

// Projects is the part of the projects module this module needs: where a card's agent worked, what
// project the card is on, and that project's dev command.
type Projects interface {
	// Worktree returns the folder and branch recorded for a card's worktree. Both are empty until
	// the card starts.
	Worktree(ctx context.Context, cardID string) (path, branch string, err error)
	// Card reads one card, for the project it is on.
	Card(ctx context.Context, id string) (protocol.Card, error)
	// Get reads one project, for its dev command.
	Get(ctx context.Context, id string) (protocol.Project, error)
}

// Publisher is the part of the event bus this module needs. A preview's state is published on the
// card's own topic, so one screen's change reaches every screen watching that card.
type Publisher interface {
	Publish(topic, eventType string, data any, critical bool) uint64
}

// Process is one running dev server.
type Process interface {
	// Done is closed when the process has exited.
	Done() <-chan struct{}
	// Stop asks the process to stop, then kills it after the grace period.
	Stop(ctx context.Context, grace time.Duration) error
}

// DevRunner starts a dev server. Nil in Options uses CommandRunner, which is what the daemon uses; a
// test gives a fake so no dev server is ever started.
type DevRunner interface {
	Start(ctx context.Context, req DevRequest) (Process, error)
}

// DevRequest is what a dev server is started with: the card's worktree, the project's dev command,
// and the port Marshal picked for this card.
type DevRequest struct {
	Dir     string
	Command string
	Port    int
}

// Shooter takes one screenshot. Nil in Options uses the chromedp-backed shooter, which drives the
// person's own Chrome or Edge; a test gives a fake so no browser is ever launched.
type Shooter interface {
	Shoot(ctx context.Context, req ShotRequest) (Shot, error)
}

// ShotRequest is one screenshot to take: the preview's address, where to write the image, the
// profile to use, and the size to render at.
type ShotRequest struct {
	URL        string
	Path       string
	ProfileDir string
	Browser    string
	Width      int
	Height     int
}

// Shot is what came of a screenshot: the size of the image that was written.
type Shot struct {
	Width  int
	Height int
}

// BrowserFinder looks for a browser Marshal can drive. Nil in Options uses the real lookup over the
// places Chrome and Edge install themselves.
type BrowserFinder interface {
	// Find returns the path of a browser Marshal can drive, or false when neither Chrome nor Edge
	// is installed.
	Find() (path string, found bool)
}

// PortPicker picks a free port for one card's dev server. Nil in Options binds a real socket to find
// one; a test gives a fake so two cards never race for the same number.
type PortPicker interface {
	Pick() (int, error)
}

// Prober asks whether a dev server answers yet. Nil in Options makes a real HTTP request; a test
// gives a fake so a preview reaches `running` without a server.
type Prober interface {
	Answers(ctx context.Context, url string) bool
}

// Options are the seams a test replaces. Every one of them has a real default, and nothing but a
// test sets them.
type Options struct {
	// Runner starts the dev server. Nil uses CommandRunner.
	Runner DevRunner
	// Shooter takes a screenshot. Nil uses the chromedp-backed shooter.
	Shooter Shooter
	// Browser looks for Chrome or Edge. Nil uses the real lookup.
	Browser BrowserFinder
	// Port picks a free port. Nil binds a socket to find one.
	Port PortPicker
	// Probe asks whether the dev server answers. Nil makes a real HTTP request.
	Probe Prober
	// ReadyTimeout bounds how long a dev server is given to answer. Zero takes
	// DefaultReadyTimeout.
	ReadyTimeout time.Duration
	// ProbeEvery is how often a dev server is asked whether it answers. Zero takes
	// DefaultProbeEvery.
	ProbeEvery time.Duration
	// Width and Height are the size a screenshot is taken at. Zero takes the defaults.
	Width  int
	Height int
}

// Deps are the parts the service is built from. Projects and DataDir are required.
type Deps struct {
	// Projects reads the card's worktree, the card, and the project's dev command.
	Projects Projects
	// Bus publishes a preview's state on the card's topic. Nil publishes nothing, which is what the
	// tests that only read state use.
	Bus Publisher
	// DataDir is the daemon's data directory, under which previews live. Required.
	DataDir string
	// Log is where a screenshot that could not be taken is written. Nil discards.
	Log *slog.Logger
	// Now is the clock. Nil uses the real one.
	Now func() time.Time
	// Options are the test seams. Nothing but a test sets them.
	Options Options
}

// Service runs one dev server per card and takes the screenshots of it. It is safe for use by many
// goroutines: every card's state is guarded by its own lock, and the map of cards by the service's.
type Service struct {
	projects Projects
	bus      Publisher
	log      *slog.Logger
	now      func() time.Time
	root     string
	runner   DevRunner
	shooter  Shooter
	browser  BrowserFinder
	ports    PortPicker
	probe    Prober
	ready    time.Duration
	every    time.Duration
	width    int
	height   int

	mu    sync.Mutex
	cards map[string]*live
	base  context.Context
	stop  context.CancelFunc
}

// New builds the service. Without the projects module there is no worktree to run in; without a data
// directory there is nowhere to keep a screenshot or a browser profile.
func New(deps Deps) (*Service, error) {
	if deps.Projects == nil {
		return nil, errors.New("preview: the projects module is required")
	}
	if strings.TrimSpace(deps.DataDir) == "" {
		return nil, errors.New("preview: a data directory is required")
	}
	log := deps.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	opts := deps.Options
	runner := opts.Runner
	if runner == nil {
		runner = CommandRunner{}
	}
	browser := opts.Browser
	if browser == nil {
		browser = browserLookup{}
	}
	shooter := opts.Shooter
	if shooter == nil {
		shooter = chromeShooter{}
	}
	ports := opts.Port
	if ports == nil {
		ports = socketPicker{}
	}
	probe := opts.Probe
	if probe == nil {
		probe = newHTTPProber()
	}
	ready := opts.ReadyTimeout
	if ready <= 0 {
		ready = DefaultReadyTimeout
	}
	every := opts.ProbeEvery
	if every <= 0 {
		every = DefaultProbeEvery
	}
	width, height := opts.Width, opts.Height
	if width <= 0 {
		width = DefaultViewportWidth
	}
	if height <= 0 {
		height = DefaultViewportHeight
	}
	base, stop := context.WithCancel(context.Background())
	return &Service{
		projects: deps.Projects, bus: deps.Bus, log: log, now: now,
		root:   filepath.Join(filepath.Clean(deps.DataDir), previewsFolder),
		runner: runner, shooter: shooter, browser: browser, ports: ports, probe: probe,
		ready: ready, every: every, width: width, height: height,
		cards: make(map[string]*live), base: base, stop: stop,
	}, nil
}

// Snapshot answers a card's preview as it is now. It starts nothing: a person looking at the tab
// must not start a dev server on the machine by looking. A card that was never previewed answers
// `stopped` with the project's dev command already in it, so the tab can say what pressing Start
// would run.
func (s *Service) Snapshot(ctx context.Context, cardID string) (protocol.PreviewSnapshot, error) {
	target, err := s.target(ctx, cardID)
	if err != nil {
		return protocol.PreviewSnapshot{}, err
	}
	s.mu.Lock()
	l := s.liveFor(target)
	s.mu.Unlock()
	return protocol.NewPreviewSnapshot(l.snapshot(), s.now()), nil
}

// Start runs the project's dev command for one card in the card's own worktree, on a port of its own,
// and answers the preview as it is now: `starting`, because a dev server rarely answers at once.
// Starting a preview that is already starting or running changes nothing and answers it.
//
// A card with no worktree and a project with no dev command are both refused in a plain sentence: the
// person asked for something that cannot be done, and must be told why rather than shown a spinner
// that never ends.
func (s *Service) Start(ctx context.Context, cardID string) (protocol.PreviewSnapshot, error) {
	target, err := s.target(ctx, cardID)
	if err != nil {
		return protocol.PreviewSnapshot{}, err
	}
	if strings.TrimSpace(target.dir) == "" {
		return protocol.PreviewSnapshot{}, protocol.Refused(messageNoWorktree)
	}
	if target.command == "" {
		return protocol.PreviewSnapshot{}, protocol.Refused(messageNoCommand)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.liveFor(target)
	if state := l.currentState(); state == protocol.PreviewStateRunning || state == protocol.PreviewStateStarting {
		return protocol.NewPreviewSnapshot(l.snapshot(), s.now()), nil
	}
	port, err := s.ports.Pick()
	if err != nil {
		return protocol.PreviewSnapshot{}, err
	}
	// The dev server is started before the state changes, so a failure is one answer with a sentence
	// rather than a spin that turns into a stop.
	proc, startErr := s.runner.Start(s.base, DevRequest{Dir: target.dir, Command: target.command, Port: port})
	l.setTarget(target, port)
	if startErr != nil {
		s.log.WarnContext(ctx, "a dev server could not be started", "card", cardID, "error", startErr)
		l.fail(messageStartFailed)
		answer := l.snapshot()
		s.publish(cardID, answer)
		return protocol.NewPreviewSnapshot(answer, s.now()), nil
	}
	l.setProcess(proc, s.base)
	l.markStarting()
	answer := l.snapshot()
	s.publish(cardID, answer)
	go s.await(l)
	return protocol.NewPreviewSnapshot(answer, s.now()), nil
}

// Stop stops a card's dev server and answers the preview as it is now: `stopped`. Stopping a preview
// that is not running is not an error: the person asked for the state they wanted and have it.
func (s *Service) Stop(ctx context.Context, cardID string) (protocol.PreviewSnapshot, error) {
	target, err := s.target(ctx, cardID)
	if err != nil {
		return protocol.PreviewSnapshot{}, err
	}
	s.mu.Lock()
	l := s.liveFor(target)
	s.mu.Unlock()
	proc := l.markStopped()
	if proc != nil {
		if err := proc.Stop(context.Background(), stopGrace); err != nil {
			s.log.WarnContext(ctx, "a dev server would not stop", "card", cardID, "error", err)
		}
	}
	answer := l.snapshot()
	s.publish(cardID, answer)
	return protocol.NewPreviewSnapshot(answer, s.now()), nil
}

// Screenshot takes one half of the before/after pair of a running preview and answers the preview
// with the new shot in it. A browser Marshal cannot find is not an error: the check is skipped, and
// the answer says so in a sentence, because a check that never ran must never read as passed.
func (s *Service) Screenshot(ctx context.Context, cardID string, kind protocol.PreviewShotKind) (protocol.PreviewShotResult, error) {
	if !kind.Valid() {
		return protocol.PreviewShotResult{}, protocol.InvalidArgument(
			"Marshal does not know that screenshot kind. Use before or after.").With("kind", string(kind))
	}
	target, err := s.target(ctx, cardID)
	if err != nil {
		return protocol.PreviewShotResult{}, err
	}
	s.mu.Lock()
	l := s.liveFor(target)
	s.mu.Unlock()
	if l.currentState() != protocol.PreviewStateRunning {
		return protocol.PreviewShotResult{}, protocol.Refused(messageNotRunning)
	}
	browser, found := s.browser.Find()
	if !found {
		// A skipped check is an answer, not a failure: the person asked for a screenshot and needs to
		// know it did not happen, not to see an error they cannot act on.
		return protocol.NewPreviewShotResult(l.snapshot(), protocol.PreviewShotOutcomeSkipped, messageNoBrowser, s.now()), nil
	}
	path := s.shotPath(cardID, kind)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return protocol.PreviewShotResult{}, err
	}
	shot, err := s.shooter.Shoot(s.base, ShotRequest{
		URL: l.currentURL(), Path: path, ProfileDir: s.profileDir(cardID),
		Browser: browser, Width: s.width, Height: s.height,
	})
	if err != nil {
		s.log.WarnContext(ctx, "a screenshot could not be taken", "card", cardID, "kind", string(kind), "error", err)
		return protocol.PreviewShotResult{}, protocol.Unavailable(messageShotFailed).WithCause(err)
	}
	// The shot's time and the version its address carries come from the file that was written, not
	// from the clock, exactly as they do when the pair is read back after a restart. The address a
	// client is handed now is then the same one it is handed later, and the route can let the image
	// be kept for as long as its address still names this version.
	taken := s.now()
	if info, statErr := os.Stat(path); statErr == nil {
		taken = info.ModTime()
	}
	l.putShot(protocol.PreviewShot{
		Kind: kind, URL: s.shotURL(cardID, kind, taken.UnixMilli()),
		Width: shot.Width, Height: shot.Height, TakenAt: protocol.NewTimestamp(taken),
	})
	answer := l.snapshot()
	s.publish(cardID, answer)
	return protocol.NewPreviewShotResult(answer, protocol.PreviewShotOutcomeTaken, "The screenshot was taken.", s.now()), nil
}

// ShotFile is where one of a card's screenshots is on disk and when it was written, so the route can
// serve the bytes. It reads the disk rather than the state in memory, so a shot taken before a
// restart is still served after it. A card with no such shot is not found.
func (s *Service) ShotFile(cardID string, kind protocol.PreviewShotKind) (path string, modified time.Time, err error) {
	if !kind.Valid() {
		return "", time.Time{}, protocol.NotFound("screenshot").With("kind", string(kind))
	}
	path = s.shotPath(cardID, kind)
	info, statErr := os.Stat(path)
	if statErr != nil {
		return "", time.Time{}, protocol.NotFound("screenshot").
			With("cardId", cardID).With("kind", string(kind))
	}
	return path, info.ModTime(), nil
}

// Close stops every dev server this service started. It is called when the daemon shuts down, so a
// person's machine is not left with a dev server running after Marshal is gone.
func (s *Service) Close() {
	s.mu.Lock()
	procs := make([]Process, 0, len(s.cards))
	for _, l := range s.cards {
		if p := l.markStopped(); p != nil {
			procs = append(procs, p)
		}
	}
	s.mu.Unlock()
	s.stop()
	for _, p := range procs {
		_ = p.Stop(context.Background(), stopGrace)
	}
}

// target is what a card's preview needs from the projects module: the worktree the dev server runs
// in, the branch the card is on, and the project's dev command.
type target struct {
	cardID    string
	projectID string
	dir       string
	branch    string
	command   string
}

func (s *Service) target(ctx context.Context, cardID string) (target, error) {
	card, err := s.projects.Card(ctx, cardID)
	if err != nil {
		return target{}, err
	}
	dir, branch, err := s.projects.Worktree(ctx, cardID)
	if err != nil {
		return target{}, err
	}
	project, err := s.projects.Get(ctx, card.ProjectID)
	if err != nil {
		return target{}, err
	}
	return target{
		cardID: card.ID, projectID: card.ProjectID, dir: dir, branch: branch,
		command: strings.TrimSpace(project.DevCommand),
	}, nil
}

// await asks the dev server whether it answers yet, until it does, until it exits, or until the
// ready timeout. It runs in its own goroutine, so starting a preview answers at once.
func (s *Service) await(l *live) {
	deadline := s.now().Add(s.ready)
	ticker := time.NewTicker(s.every)
	defer ticker.Stop()
	for {
		if l.currentState() != protocol.PreviewStateStarting {
			return // stopped underneath us: the person pressed Stop, or the daemon is closing
		}
		if s.probe.Answers(s.base, l.currentURL()) {
			l.markRunning(s.now())
			s.publish(l.cardID, l.snapshot())
			return
		}
		select {
		case <-s.base.Done():
			return
		case <-l.exited():
			// The dev server exited before it answered: a command that was wrong, a port already
			// taken, a crash. Saying so at once beats a spinner that runs out the clock.
			if l.currentState() == protocol.PreviewStateStarting {
				l.fail(messageStoppedEarly)
				s.publish(l.cardID, l.snapshot())
			}
			return
		case <-ticker.C:
		}
		if s.now().After(deadline) {
			if l.currentState() == protocol.PreviewStateStarting {
				l.fail(messageReadyTimeout)
				s.publish(l.cardID, l.snapshot())
			}
			return
		}
	}
}

// publish sends a preview's state to every screen watching the card. It is the same payload for every
// state, so a screen applies an event exactly as it applies the snapshot.
func (s *Service) publish(cardID string, p protocol.Preview) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(string(protocol.CardTopic(cardID)), string(protocol.EventTypePreviewStateChanged),
		protocol.PreviewEventData{Preview: p}, false)
}

// liveFor returns the state for one card, creating it the first time the card is looked at. It
// records the screenshots already on disk, so a pair survives a daemon restart. It is called with
// the service's lock held.
func (s *Service) liveFor(t target) *live {
	if l, ok := s.cards[t.cardID]; ok {
		return l
	}
	l := &live{
		cardID: t.cardID, projectID: t.projectID, dir: t.dir, branch: t.branch,
		command: t.command, state: protocol.PreviewStateStopped, shots: s.loadShots(t.cardID),
	}
	s.cards[t.cardID] = l
	return l
}

// shotPath, profileDir, and shotURL are where a screenshot lives, where its card's browser profile
// lives, and the address the app fetches it from.
func (s *Service) shotPath(cardID string, kind protocol.PreviewShotKind) string {
	return filepath.Join(s.root, filepath.Base(cardID), string(kind)+shotExt)
}

func (s *Service) profileDir(cardID string) string {
	return filepath.Join(s.root, filepath.Base(cardID), profileFolder)
}

func (s *Service) shotURL(cardID string, kind protocol.PreviewShotKind, version int64) string {
	return "/v1/cards/" + cardID + "/preview/shots/" + string(kind) + shotExt + "?v=" + strconv.FormatInt(version, 10)
}

// loadShots reads the screenshots already on disk for a card, so a pair taken before a restart is
// still shown after it. A file that cannot be read is left out rather than failing the answer.
func (s *Service) loadShots(cardID string) []protocol.PreviewShot {
	var shots []protocol.PreviewShot
	for _, kind := range protocol.PreviewShotKindValues() {
		path := s.shotPath(cardID, kind)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		width, height := 0, 0
		if file, err := os.Open(path); err == nil {
			if config, _, err := image.DecodeConfig(file); err == nil {
				width, height = config.Width, config.Height
			}
			_ = file.Close()
		}
		shots = append(shots, protocol.PreviewShot{
			Kind: kind, URL: s.shotURL(cardID, kind, info.ModTime().UnixMilli()),
			Width: width, Height: height, TakenAt: protocol.NewTimestamp(info.ModTime()),
		})
	}
	return shots
}

// live is one card's preview: where it is, the process behind it, and the shots of it.
type live struct {
	cardID    string
	projectID string
	dir       string
	branch    string
	command   string

	mu        sync.Mutex
	state     protocol.PreviewState
	port      int
	url       string
	startedAt *time.Time
	sentence  string
	proc      Process
	done      chan struct{}
	shots     []protocol.PreviewShot
}

// currentState, currentURL, and exited are the reads the probe loop and the routes need.
func (l *live) currentState() protocol.PreviewState {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.state
}

func (l *live) currentURL() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.url
}

func (l *live) exited() <-chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.done == nil {
		return make(chan struct{}) // no process: a channel that never closes
	}
	return l.done
}

// snapshot builds the wire shape for this preview, with a copy of the shots so a caller cannot change
// the state by holding on to the answer.
func (l *live) snapshot() protocol.Preview {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.snapshotLocked()
}

func (l *live) snapshotLocked() protocol.Preview {
	shots := make([]protocol.PreviewShot, len(l.shots))
	copy(shots, l.shots)
	var startedAt *protocol.Timestamp
	if l.startedAt != nil {
		value := protocol.NewTimestamp(*l.startedAt)
		startedAt = &value
	}
	url := l.url
	if l.state != protocol.PreviewStateRunning {
		url = "" // an address that does not answer yet is worse than none
	}
	return protocol.Preview{
		CardID: l.cardID, State: l.state, URL: url, Port: l.port, Command: l.command,
		StartedAt: startedAt, Error: l.sentence, Shots: shots,
	}
}

// setTarget records where this preview runs and on which port, before its process exists.
func (l *live) setTarget(t target, port int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.dir, l.branch, l.command = t.dir, t.branch, t.command
	l.port = port
	l.url = "http://127.0.0.1:" + strconv.Itoa(port)
}

// setProcess records the dev server and watches for its exit, so the probe loop learns about it.
func (l *live) setProcess(proc Process, base context.Context) {
	done := make(chan struct{})
	l.mu.Lock()
	l.proc = proc
	l.done = done
	l.mu.Unlock()
	go func() {
		select {
		case <-proc.Done():
		case <-base.Done():
		}
		close(done)
	}()
}

// markStarting moves the preview to `starting` with nothing wrong with it yet.
func (l *live) markStarting() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.state = protocol.PreviewStateStarting
	l.startedAt = nil
	l.sentence = ""
}

// markRunning records that the dev server answered.
func (l *live) markRunning(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.state != protocol.PreviewStateStarting {
		return
	}
	l.state = protocol.PreviewStateRunning
	l.startedAt = &now
	l.sentence = ""
}

// fail moves the preview to `stopped` with the sentence that says what went wrong. There is no
// failure state on the wire: a preview that failed is a preview that is not running, plus why.
func (l *live) fail(sentence string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.state = protocol.PreviewStateStopped
	l.startedAt = nil
	l.url = ""
	l.sentence = sentence
}

// markStopped moves the preview to `stopped` and hands back the process to stop, if there was one.
func (l *live) markStopped() Process {
	l.mu.Lock()
	defer l.mu.Unlock()
	proc := l.proc
	l.proc = nil
	l.state = protocol.PreviewStateStopped
	l.startedAt = nil
	l.url = ""
	l.sentence = ""
	return proc
}

// putShot records a screenshot, replacing an earlier one of the same kind, and keeps the pair in the
// order it is read in: before, then after.
func (l *live) putShot(shot protocol.PreviewShot) {
	l.mu.Lock()
	defer l.mu.Unlock()
	kept := l.shots[:0:0]
	for _, existing := range l.shots {
		if existing.Kind != shot.Kind {
			kept = append(kept, existing)
		}
	}
	kept = append(kept, shot)
	sort.Slice(kept, func(i, j int) bool {
		return kept[i].Kind == protocol.PreviewShotKindBefore && kept[j].Kind != protocol.PreviewShotKindBefore
	})
	l.shots = kept
}
