package pty

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

func TestOutputArrivesInOrderAndTheExitCodeIsReported(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		want     []string
		wantCode int
		wantErr  bool
	}{
		{name: "clean exit", args: []string{"lines", "5"},
			want: []string{"line 1", "line 2", "line 3", "line 4", "line 5"}},
		{name: "a failing exit code", args: []string{"exit", "7"},
			want: []string{"exiting"}, wantCode: 7, wantErr: true},
		{name: "no output at all", args: []string{"lines", "0"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := startRun(t, Config{}, tc.args...)
			exit := r.waitExit()
			if exit.Code != tc.wantCode || (exit.Err != nil) != tc.wantErr {
				t.Errorf("Exited = code %d, err %v; want code %d, error %v", exit.Code, exit.Err, tc.wantCode, tc.wantErr)
			}
			at := -1
			for _, want := range tc.want {
				next := strings.Index(r.output(), want)
				if next <= at {
					t.Fatalf("%q is missing or out of order in %q", want, r.output())
				}
				at = next
			}
		})
	}
}

func TestSendTypesTheTextAndALineEnd(t *testing.T) {
	r := startRun(t, Config{}, "echo")
	r.waitFor("ready")

	if err := r.a.Send(callCtx(t), r.h, agents.UserMessage{Text: "hello there"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	r.waitFor("echo: hello there")
}

func TestCustomLineEnd(t *testing.T) {
	// A carriage return is what the Enter key sends, and a cooked terminal turns it into a newline.
	r := startRun(t, Config{LineEnd: "\r"}, "echo")
	r.waitFor("ready")

	if err := r.a.Send(callCtx(t), r.h, agents.UserMessage{Text: "with return"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	r.waitFor("echo: with return")
}

func TestWriteRawTypesBytesAsTheyAre(t *testing.T) {
	r := startRun(t, Config{}, "upper")
	r.waitFor("ready")

	if err := r.a.WriteRaw(callCtx(t), r.h, []byte("abc def"+defaultLineEnd)); err != nil {
		t.Fatalf("WriteRaw: %v", err)
	}
	r.waitFor("ABC DEF")
	if exit := r.waitExit(); exit.Code != 0 || exit.Err != nil {
		t.Errorf("Exited = %+v, want a clean exit", exit)
	}
}

func TestInterruptSendsControlC(t *testing.T) {
	r := startRun(t, Config{}, "interrupt")
	r.waitFor("ready")

	if err := r.a.Interrupt(callCtx(t), r.h); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	r.waitFor("got interrupt")
	if exit := r.waitExit(); exit.Code != 0 {
		t.Errorf("Exited = %+v, want code 0", exit)
	}
}

func TestStop(t *testing.T) {
	const hardGrace = 300 * time.Millisecond
	tests := []struct {
		name      string
		args      []string
		grace     time.Duration
		wantKill  bool
		wantWords string
	}{
		// The program answers the polite request and ends by itself, well inside the grace period.
		{name: "polite", args: []string{"graceful"}, grace: time.Minute, wantWords: "goodbye"},
		// The program ignores every polite request, so the whole grace period passes and it is killed.
		{name: "hard", args: []string{"stubborn"}, grace: hardGrace, wantKill: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := startRun(t, Config{StopGrace: tc.grace}, tc.args...)
			r.waitFor("ready")

			start := time.Now()
			if err := r.a.Stop(callCtx(t), r.h); err != nil {
				t.Fatalf("Stop: %v", err)
			}
			elapsed := time.Since(start)
			exit := r.waitExit()

			if tc.wantKill {
				if runtime.GOOS == "windows" {
					// Closing the console is the polite step there, and whether a program that ignores
					// Ctrl-C survives that is not known, so only the exit is checked.
					return
				}
				if elapsed < tc.grace {
					t.Errorf("Stop returned after %v, before the grace period of %v", elapsed, tc.grace)
				}
				if exit.Err == nil || exit.Code != -1 {
					t.Errorf("Exited = %+v, want the process to have been killed", exit)
				}
				return
			}
			if elapsed > 20*time.Second {
				t.Errorf("Stop took %v, so the polite request did not work", elapsed)
			}
			if runtime.GOOS != "windows" {
				// Closing the console on Windows may cut the last words off, so only Unix checks them.
				r.waitForWordsAfterExit(tc.wantWords)
			}
		})
	}
}

// waitForWordsAfterExit checks output that arrived before the channel closed.
func (r *run) waitForWordsAfterExit(words string) {
	r.t.Helper()
	if !strings.Contains(r.output(), words) {
		r.t.Errorf("the output %q does not hold %q", r.output(), words)
	}
}

func TestStopIsSafeToRepeatAndForUnknownSessions(t *testing.T) {
	r := startRun(t, Config{}, "echo")
	r.waitFor("ready")

	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.a.Stop(callCtx(t), r.h); err != nil {
				t.Errorf("Stop: %v", err)
			}
		}()
	}
	wg.Wait()
	r.waitExit()

	if err := r.a.Stop(callCtx(t), r.h); err != nil {
		t.Errorf("Stop of a session that is gone: %v", err)
	}
	if err := r.a.Stop(callCtx(t), agents.SessionHandle{ID: "no-such-session"}); err != nil {
		t.Errorf("Stop of an unknown session: %v", err)
	}
	if _, open := <-r.a.Events(r.h); open {
		t.Error("Events of a session that is gone returned an open channel")
	}
}

func TestCallsOnASessionThatIsGone(t *testing.T) {
	r := startRun(t, Config{}, "exit", "0")
	r.waitExit()
	gone := agents.SessionHandle{ID: "no-such-session"}
	for _, h := range []agents.SessionHandle{r.h, gone} {
		calls := map[string]error{
			"Send":      r.a.Send(callCtx(t), h, agents.UserMessage{Text: "x"}),
			"WriteRaw":  r.a.WriteRaw(callCtx(t), h, []byte("x")),
			"Interrupt": r.a.Interrupt(callCtx(t), h),
			"Resize":    r.a.Resize(callCtx(t), h, 80, 24),
		}
		for name, err := range calls {
			if !errors.Is(err, agents.ErrUnknownSession) {
				t.Errorf("%s = %v, want ErrUnknownSession", name, err)
			}
		}
		if snap := r.a.Snapshot(h); snap != nil {
			t.Errorf("Snapshot = %q, want nil", snap)
		}
	}
}

func TestStopAfterTheProgramExitedSignalsNothing(t *testing.T) {
	a := newAdapter(t, Config{Args: []string{"exit", "0"}})
	h, err := a.Start(callCtx(t), agents.StartSpec{Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	// The session is in the table from the moment Start returns, so it can be picked up here
	// before it ends.
	s, err := a.find(h)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	r := watch(t, a, h)
	<-s.exited

	if err := s.stop(callCtx(t)); err != nil {
		t.Errorf("stop: %v", err)
	}
	if s.stopping.Load() {
		t.Error("stop went through the polite request and the kill for a process that had exited")
	}
	r.waitExit()
}

func TestAProgramThatEndsAtOnceIsForgotten(t *testing.T) {
	a := newAdapter(t, Config{ResumeArgs: func(string) []string { return []string{"exit", "0"} }})
	for i := range 20 {
		h, err := a.Resume(callCtx(t), "fast", agents.StartSpec{Cwd: t.TempDir()})
		if err != nil {
			t.Fatalf("Resume %d: %v", i, err)
		}
		watch(t, a, h).waitExit()
		// The channel is closed, so the session must be unknown, and its id free to use again.
		if a.isRunning("fast") {
			t.Fatalf("round %d: a session that ended is still in the table", i)
		}
	}
}

func TestRespondAlwaysFails(t *testing.T) {
	r := startRun(t, Config{}, "echo")
	err := r.a.Respond(callCtx(t), r.h, agents.ApprovalResponse{RequestID: "perm-1", OptionID: "allow"})
	if !errors.Is(err, agents.ErrUnknownRequest) {
		t.Errorf("Respond = %v, want ErrUnknownRequest", err)
	}
}

func TestCapabilities(t *testing.T) {
	resume := func(id string) []string { return []string{"args", id} }
	tests := []struct {
		name string
		cfg  Config
		want agents.Capabilities
	}{
		{name: "cannot resume", want: agents.Capabilities{}},
		{name: "can resume", cfg: Config{ResumeArgs: resume}, want: agents.Capabilities{Resume: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newAdapter(t, tc.cfg)
			if got := a.Capabilities(); got != tc.want {
				t.Errorf("Capabilities = %+v, want %+v", got, tc.want)
			}
			if a.Capabilities().StructuredEvents {
				t.Error("a terminal has no structured events")
			}
		})
	}
}

func TestResume(t *testing.T) {
	resume := func(id string) []string { return []string{"args", "--resume", id} }
	tests := []struct {
		name    string
		cfg     Config
		id      string
		wantErr error
		want    string
	}{
		{name: "the config has no resume arguments", id: "abc", wantErr: agents.ErrCannotResume},
		{name: "no session id", cfg: Config{ResumeArgs: resume}, wantErr: agents.ErrCannotResume},
		{name: "resume arguments are used", cfg: Config{ResumeArgs: resume}, id: "abc-123",
			want: "args: --resume abc-123"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newAdapter(t, tc.cfg)
			h, err := a.Resume(callCtx(t), tc.id, agents.StartSpec{Cwd: t.TempDir()})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Resume = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resume: %v", err)
			}
			r := watch(t, a, h)
			if h.ID != tc.id {
				t.Errorf("handle id = %q, want %q", h.ID, tc.id)
			}
			r.waitFor(tc.want)
		})
	}
}

func TestResumeRefusesASessionThatIsRunning(t *testing.T) {
	a := newAdapter(t, Config{ResumeArgs: func(id string) []string { return []string{"args", id} }})
	spec := agents.StartSpec{Cwd: t.TempDir()}
	h, err := a.Resume(callCtx(t), "same-id", spec)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	watch(t, a, h).waitFor("ready")

	if _, err := a.Resume(callCtx(t), "same-id", spec); err == nil {
		t.Error("a second Resume of a running session succeeded")
	}
}

func TestStartArgsAndInstructions(t *testing.T) {
	var seen string
	cfg := Config{
		StartArgs: func(id string) []string { seen = id; return []string{"args", "--session-id", id} },
		InstructionArgs: func(text string) []string {
			return []string{"--role", text}
		},
	}
	a := newAdapter(t, cfg)
	h, err := a.Start(callCtx(t), agents.StartSpec{Cwd: t.TempDir(), Instructions: "review"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	r := watch(t, a, h)
	r.waitFor("args: --session-id " + seen + " --role review")
	if h.ID != seen || seen == "" {
		t.Errorf("handle id = %q, but StartArgs was given %q", h.ID, seen)
	}
}

func TestStartWithoutInstructionArgsDropsTheInstructions(t *testing.T) {
	r := startRun(t, Config{}, "args", "--plain")
	spec := agents.StartSpec{Cwd: t.TempDir(), Instructions: "review"}
	h, err := r.a.Start(callCtx(t), spec)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	other := watch(t, r.a, h)
	other.waitFor("args: --plain")
	if strings.Contains(other.output(), "review") {
		t.Errorf("the instructions reached the program: %q", other.output())
	}
}

func TestHandleCarriesTheRequestedSettingsWithoutApplyingThem(t *testing.T) {
	a := newAdapter(t, Config{Args: []string{"echo"}})
	spec := agents.StartSpec{Cwd: t.TempDir(), Label: "card-7", Model: "m", Thinking: "high", PermissionMode: "plan"}
	h, err := a.Start(callCtx(t), spec)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	watch(t, a, h)
	if h.Label != "card-7" || h.Model != "m" || h.Thinking != "high" || h.PermissionMode != "plan" {
		t.Errorf("handle = %+v, want the requested settings kept", h)
	}
	if h.Applied != (agents.Applied{}) || h.Capabilities.StructuredEvents {
		t.Errorf("handle = %+v, want nothing applied and no structured events", h)
	}
}

func TestTheWorkingFolderIsRespected(t *testing.T) {
	dir := t.TempDir()
	a := newAdapter(t, Config{Args: []string{"pwd"}})
	r := startWith(t, a, agents.StartSpec{Cwd: dir})
	r.waitExit()

	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for line := range strings.SplitSeq(r.output(), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "pwd: "); ok {
			got = rest
		}
	}
	if resolved, err := filepath.EvalSymlinks(got); err != nil || !strings.EqualFold(resolved, want) {
		t.Errorf("the program ran in %q, want %q", got, want)
	}
}

func TestEnvironment(t *testing.T) {
	const secret = "MARSHAL_TEST_SECRET_TOKEN"
	t.Setenv(secret, "the-daemon-secret")
	tests := []struct {
		name string
		cfg  Config
		spec agents.StartSpec
		want string
	}{
		{name: "the daemon's variable is dropped", want: "env " + secret + "=[]"},
		{name: "the config passes it on", cfg: Config{Env: []string{secret + "=from-config"}},
			want: "env " + secret + "=[from-config]"},
		{name: "the session passes it on", spec: agents.StartSpec{Env: []string{secret + "=from-session"}},
			want: "env " + secret + "=[from-session]"},
		{name: "the session wins over the config",
			cfg:  Config{Env: []string{secret + "=from-config"}},
			spec: agents.StartSpec{Env: []string{secret + "=from-session"}},
			want: "env " + secret + "=[from-session]"},
		{name: "TERM is set for the program", want: "env TERM=[xterm-256color]"},
		{name: "TERM can be replaced", cfg: Config{Env: []string{"TERM=dumb"}}, want: "env TERM=[dumb]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.cfg.Args = []string{"env", secret, "TERM"}
			tc.spec.Cwd = t.TempDir()
			r := startWith(t, newAdapter(t, tc.cfg), tc.spec)
			r.waitExit()
			if !strings.Contains(r.output(), tc.want) {
				t.Errorf("the output %q does not hold %q", r.output(), tc.want)
			}
			if strings.Contains(tc.want, "[]") && strings.Contains(r.output(), "the-daemon-secret") {
				t.Errorf("the daemon's secret reached the program: %q", r.output())
			}
		})
	}
}

func TestTheSizeOfTheTerminal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the helper reads its terminal size with ioctl, which the standard library has only on Unix; " +
			"the ConPTY size is not checked in CI")
	}
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "the default", want: "size: 120x32"},
		{name: "from the config", cfg: Config{Cols: 90, Rows: 20}, want: "size: 90x20"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := startRun(t, tc.cfg, "size")
			r.waitFor(tc.want)
		})
	}
}

func TestResizeTellsTheProgram(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the helper reads its terminal size with ioctl, which the standard library has only on Unix")
	}
	r := startRun(t, Config{}, "size")
	r.waitFor("size: 120x32")

	if err := r.a.Resize(callCtx(t), r.h, 100, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	r.waitFor("size: 100x40")
}

func TestResizeRejectsASizeThatIsOutOfRange(t *testing.T) {
	r := startRun(t, Config{}, "echo")
	for _, size := range [][2]int{{0, 10}, {10, 0}, {-1, 10}, {MaxSize + 1, 10}, {10, MaxSize + 1}} {
		if err := r.a.Resize(callCtx(t), r.h, size[0], size[1]); err == nil {
			t.Errorf("Resize(%d, %d) succeeded", size[0], size[1])
		}
	}
	if err := r.a.Resize(callCtx(t), r.h, 80, 24); err != nil {
		t.Errorf("Resize(80, 24): %v", err)
	}
}

func TestALateViewerCanPaintTheScreen(t *testing.T) {
	r := startRun(t, Config{}, "echo")
	// Nobody reads the events yet, as when no viewer is open. The snapshot keeps the screen.
	waitForSnapshot(t, r, "ready")
	if err := r.a.Send(callCtx(t), r.h, agents.UserMessage{Text: "painted"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	snap := string(waitForSnapshot(t, r, "echo: painted"))
	if ready, painted := strings.Index(snap, "ready"), strings.Index(snap, "echo: painted"); ready < 0 || ready > painted {
		t.Errorf("the snapshot %q does not hold the first output before the later one", snap)
	}
	if runtime.GOOS != "windows" && !strings.HasPrefix(snap, "ready") {
		// ConPTY starts its output with setup sequences, so only Unix checks the very start.
		t.Errorf("the snapshot %q does not start with the first output", snap)
	}
	// The same bytes come through the events for a viewer that was listening from the start.
	r.waitFor("echo: painted")
}

// waitForSnapshot polls the snapshot until it holds the text, and returns it.
func waitForSnapshot(t *testing.T, r *run, text string) []byte {
	t.Helper()
	deadline := time.Now().Add(eventTimeout)
	for {
		snap := r.a.Snapshot(r.h)
		if strings.Contains(string(snap), text) {
			return snap
		}
		if time.Now().After(deadline) {
			t.Fatalf("the snapshot never held %q: %q", text, snap)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestManySmallWritesBecomeFewEvents(t *testing.T) {
	const pieces = 200
	r := startRun(t, Config{}, "trickle", fmt.Sprint(pieces))
	r.waitExit()

	// ConPTY redraws the screen, so only Unix counts the pieces exactly.
	if got := strings.Count(r.output(), "piece;"); got != pieces && runtime.GOOS != "windows" {
		t.Fatalf("%d pieces arrived, want %d", got, pieces)
	}
	if r.count >= pieces/2 {
		t.Errorf("%d events for %d small writes: the output was not batched", r.count, pieces)
	}
	if r.maxEvent > MaxEventBytes {
		t.Errorf("an event held %d bytes, the limit is %d", r.maxEvent, MaxEventBytes)
	}
}

func TestSendsFromSeveralGoroutinesDoNotMix(t *testing.T) {
	// A wide screen and short lines, so that no line is wrapped on a pseudo console.
	r := startRun(t, Config{Cols: 200}, "echo")
	r.waitFor("ready")

	const senders = 8
	payload := strings.Repeat("x", 60)
	var wg sync.WaitGroup
	for i := range senders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			text := fmt.Sprintf("message-%d-%s", i, payload)
			if err := r.a.Send(callCtx(t), r.h, agents.UserMessage{Text: text}); err != nil {
				t.Errorf("Send: %v", err)
			}
		}()
	}
	wg.Wait()
	for i := range senders {
		r.waitFor(fmt.Sprintf("echo: message-%d-%s", i, payload))
	}
}

func TestTeeReceivesEveryByte(t *testing.T) {
	tee := &syncBuffer{}
	r := startRun(t, Config{Tee: tee}, "lines", "50")
	r.waitExit()

	// Unix keeps the line ends exactly, and ConPTY redraws the text, so only "line 1" is checked
	// there.
	wantFirst := "line 1\r\n"
	if runtime.GOOS == "windows" {
		wantFirst = "line 1"
	}
	if !strings.Contains(tee.String(), "line 50") || !strings.Contains(tee.String(), wantFirst) {
		t.Errorf("the tee holds %q", tee.String())
	}
	if len(tee.String()) != r.total {
		t.Errorf("the tee got %d bytes and the events %d", len(tee.String()), r.total)
	}
}

type failingWriter struct{ calls int }

func (w *failingWriter) Write([]byte) (int, error) {
	w.calls++
	return 0, errors.New("disk full")
}

func TestATeeThatFailsIsDroppedAndTheOutputStillFlows(t *testing.T) {
	tee := &failingWriter{}
	r := startRun(t, Config{Tee: tee}, "lines", "3")
	r.waitExit()

	if !strings.Contains(r.output(), "line 3") {
		t.Errorf("the output %q lost its content", r.output())
	}
	if tee.calls != 1 {
		t.Errorf("the failing tee was called %d times, want once", tee.calls)
	}
}

func TestStartFailures(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "missing")
	tests := []struct {
		name string
		cfg  Config
		spec agents.StartSpec
	}{
		{name: "the working folder does not exist", spec: agents.StartSpec{Cwd: missingDir}},
		{name: "the working folder is a file", spec: agents.StartSpec{Cwd: os.Args[0]}},
		{name: "no working folder", spec: agents.StartSpec{}},
		{name: "the program does not exist", cfg: Config{Path: filepath.Join(missingDir, "nothing")},
			spec: agents.StartSpec{Cwd: t.TempDir()}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newAdapter(t, Config{})
			if tc.cfg.Path != "" {
				a.cfg.Path = tc.cfg.Path
			}
			if _, err := a.Start(callCtx(t), tc.spec); err == nil {
				t.Error("Start succeeded")
			}
		})
	}
}

func TestStartWithACancelledContext(t *testing.T) {
	a := newAdapter(t, Config{})
	c, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Start(c, agents.StartSpec{Cwd: t.TempDir()}); err == nil {
		t.Error("Start succeeded with a cancelled context")
	}
}

func TestTheSessionOutlivesTheContextThatStartedIt(t *testing.T) {
	a := newAdapter(t, Config{Args: []string{"echo"}})
	c, cancel := context.WithCancel(context.Background())
	h, err := a.Start(c, agents.StartSpec{Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	r := watch(t, a, h)
	cancel()

	r.waitFor("ready")
	if err := a.Send(callCtx(t), h, agents.UserMessage{Text: "still here"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	r.waitFor("echo: still here")
}

func TestNewChecksTheConfig(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		wantErr  bool
		wantSize [2]int
	}{
		{name: "no program", cfg: Config{}, wantErr: true},
		{name: "a negative size", cfg: Config{Path: "x", Cols: -1}, wantErr: true},
		{name: "a size that is too large", cfg: Config{Path: "x", Rows: MaxSize + 1}, wantErr: true},
		{name: "defaults", cfg: Config{Path: "x"}, wantSize: [2]int{DefaultCols, DefaultRows}},
		{name: "given", cfg: Config{Path: "x", Cols: 80, Rows: 24}, wantSize: [2]int{80, 24}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, err := New(tc.cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatal("New succeeded")
				}
				return
			}
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if got := [2]int{a.cfg.Cols, a.cfg.Rows}; got != tc.wantSize {
				t.Errorf("size = %v, want %v", got, tc.wantSize)
			}
			if a.cfg.LineEnd != defaultLineEnd || a.cfg.StopGrace != defaultStopGrace || a.cfg.Logger == nil {
				t.Errorf("defaults were not filled in: %+v", a.cfg)
			}
		})
	}
}

func TestSessionIDs(t *testing.T) {
	seen := map[string]bool{}
	for range 50 {
		id, err := newSessionID()
		if err != nil {
			t.Fatal(err)
		}
		if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' || id[14] != '4' {
			t.Errorf("%q is not a version 4 UUID", id)
		}
		if seen[id] {
			t.Errorf("%q was made twice", id)
		}
		seen[id] = true
	}
}

func TestAProgramThatIgnoresItsInputStillStops(t *testing.T) {
	// Nobody reads the events and the program prints without end, so both buffers fill up and
	// the program blocks. Stop must still finish.
	r := startRun(t, Config{StopGrace: 200 * time.Millisecond}, "burst", "40000000", "hold")
	time.Sleep(300 * time.Millisecond)

	done := make(chan error, 1)
	go func() { done <- r.a.Stop(callCtx(t), r.h) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Stop: %v", err)
		}
	case <-time.After(eventTimeout):
		t.Fatal("Stop did not return while the events were not being read")
	}
	r.waitExit()
}

func TestEventsStayWithinTheSizeLimit(t *testing.T) {
	r := startRun(t, Config{}, "burst", "300000")
	r.waitExit()
	if r.total < 300000 || r.maxEvent > MaxEventBytes {
		t.Errorf("total %d bytes and the largest event %d bytes", r.total, r.maxEvent)
	}
}
