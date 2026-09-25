package pty

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/proc"
)

const (
	// eventTimeout is how long a test waits for output. It is generous, since CI machines are
	// slow under the race detector.
	eventTimeout = 30 * time.Second
	// tailLimit is how much output a run remembers at least, counted from the end, so that a test
	// that pushes tens of megabytes through a terminal does not measure its own buffer. It may
	// hold up to twice as much.
	tailLimit = 512 << 10
)

// helperBuild is the one build of the helper program that a test process makes.
var helperBuild struct {
	once sync.Once
	dir  string
	path string
	err  error
}

func TestMain(m *testing.M) {
	code := m.Run()
	if helperBuild.dir != "" {
		_ = os.RemoveAll(helperBuild.dir)
	}
	if code == 0 {
		if err := goleak.Find(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}
	os.Exit(code)
}

// helperPath builds testdata/helper into a temporary folder, once per test process, and returns
// the path of the program.
func helperPath(t testing.TB) string {
	t.Helper()
	helperBuild.once.Do(func() {
		helperBuild.dir, helperBuild.path, helperBuild.err = buildHelper(context.Background())
	})
	if helperBuild.err != nil {
		t.Fatalf("build the helper program: %v", helperBuild.err)
	}
	return helperBuild.path
}

// buildHelper runs go build in the package folder, which is where the tests run.
func buildHelper(ctx context.Context) (dir, path string, err error) {
	dir, err = os.MkdirTemp("", "marshal-pty-helper-")
	if err != nil {
		return "", "", fmt.Errorf("make a folder for the build: %w", err)
	}
	name := "helper"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path = filepath.Join(dir, name)
	build, err := proc.Start(ctx, proc.Spec{
		Path: "go", Args: []string{"build", "-o", path, "./testdata/helper"},
		Env: goEnvironment(os.Environ()),
	})
	if err != nil {
		return dir, "", err
	}
	_, _ = io.Copy(io.Discard, build.Stdout)
	exit := build.Wait()
	_ = build.Stdout.Close()
	if exit.Err != nil {
		return dir, "", fmt.Errorf("go build: %w\n%s", exit.Err, build.StderrTail())
	}
	return dir, path, nil
}

// goEnvironment picks what the Go tool needs from an environment, and asks for a build without
// cgo. The child process helper hands a program nothing else.
func goEnvironment(environ []string) []string {
	env := []string{"CGO_ENABLED=0"}
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "GO") || name == "XDG_CACHE_HOME" {
			env = append(env, entry)
		}
	}
	return env
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newAdapter makes an adapter for the helper program, with test defaults, and returns it.
func newAdapter(t *testing.T, cfg Config) *Adapter {
	t.Helper()
	cfg.Path = helperPath(t)
	if cfg.Logger == nil {
		cfg.Logger = quietLogger()
	}
	if cfg.StopGrace == 0 {
		cfg.StopGrace = 2 * time.Second
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

// run is one session that a test drives. It reads the events in the test's goroutine and keeps
// the tail of the output.
type run struct {
	t      *testing.T
	a      *Adapter
	h      agents.SessionHandle
	events <-chan agents.AgentEvent

	tail     []byte
	total    int
	count    int
	maxEvent int
	exit     *agents.Exited
	closed   bool
}

// startRun starts the helper with the arguments in a fresh folder, and stops it when the test
// ends.
func startRun(t *testing.T, cfg Config, args ...string) *run {
	t.Helper()
	cfg.Args = args
	return startWith(t, newAdapter(t, cfg), agents.StartSpec{Cwd: t.TempDir(), Label: "test"})
}

// startWith starts a session on an adapter.
func startWith(t *testing.T, a *Adapter, spec agents.StartSpec) *run {
	t.Helper()
	h, err := a.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return watch(t, a, h)
}

// watch takes over a session that was started, and arranges for it to be stopped.
func watch(t *testing.T, a *Adapter, h agents.SessionHandle) *run {
	t.Helper()
	r := &run{t: t, a: a, h: h, events: a.Events(h)}
	t.Cleanup(r.cleanup)
	return r
}

// cleanup stops the session and reads its events to the end.
func (r *run) cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), eventTimeout)
	defer cancel()
	if err := r.a.Stop(ctx, r.h); err != nil {
		r.t.Errorf("Stop in cleanup: %v", err)
	}
	for !r.closed {
		if !r.pull(time.After(eventTimeout)) {
			break
		}
	}
}

// pull reads one event. It returns false when the channel is closed.
func (r *run) pull(deadline <-chan time.Time) bool {
	r.t.Helper()
	select {
	case ev, ok := <-r.events:
		if !ok {
			r.closed = true
			return false
		}
		r.absorb(ev)
		return true
	case <-deadline:
		r.t.Fatalf("timed out; the output ended with %q", tailOf(r.tail, 200))
		return false
	}
}

// absorb records one event.
func (r *run) absorb(ev agents.AgentEvent) {
	r.t.Helper()
	switch ev := ev.(type) {
	case agents.TerminalOutput:
		if r.exit != nil {
			r.t.Errorf("output arrived after Exited")
		}
		r.count++
		r.total += len(ev.Data)
		r.maxEvent = max(r.maxEvent, len(ev.Data))
		r.tail = append(r.tail, ev.Data...)
		if len(r.tail) > 2*tailLimit {
			// Compact in place, so that the harness itself makes no garbage that would blur a
			// memory measurement.
			n := copy(r.tail, r.tail[len(r.tail)-tailLimit:])
			r.tail = r.tail[:n]
		}
	case agents.Exited:
		if r.exit != nil {
			r.t.Errorf("Exited arrived twice")
		}
		r.exit = &ev
	default:
		r.t.Errorf("unexpected event %T", ev)
	}
}

// waitFor reads events until the output holds the text.
func (r *run) waitFor(text string) {
	r.t.Helper()
	r.waitForWithin(text, eventTimeout)
}

// waitForWithin is waitFor with a time limit of its own.
func (r *run) waitForWithin(text string, limit time.Duration) {
	r.t.Helper()
	deadline := time.After(limit)
	for !bytes.Contains(r.tail, []byte(text)) {
		if !r.pull(deadline) {
			r.t.Fatalf("the session ended before %q appeared; the output ended with %q",
				text, tailOf(r.tail, 200))
		}
	}
}

// waitExit reads events to the end of the channel and returns the Exited event.
func (r *run) waitExit() agents.Exited {
	r.t.Helper()
	deadline := time.After(eventTimeout)
	for r.pull(deadline) {
	}
	if r.exit == nil {
		r.t.Fatalf("the channel closed without an Exited event")
	}
	return *r.exit
}

// output returns the remembered output as a string.
func (r *run) output() string { return string(r.tail) }

// tailOf returns the last n bytes of b.
func tailOf(b []byte, n int) []byte {
	if len(b) > n {
		return b[len(b)-n:]
	}
	return b
}

// callCtx returns a context for one call.
func callCtx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), eventTimeout)
	t.Cleanup(cancel)
	return c
}

// syncBuffer is a bytes.Buffer that several goroutines may write and read.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
