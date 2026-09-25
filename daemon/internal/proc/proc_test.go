package proc

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	quickGrace = 150 * time.Millisecond
	longGrace  = 30 * time.Second
)

func TestStartAndRead(t *testing.T) {
	t.Parallel()
	p := startHelper(t, "echo", true)
	if p.Pid() <= 0 {
		t.Fatalf("Pid = %d, want a real process id", p.Pid())
	}
	if _, err := io.WriteString(p.Stdin, "hello\n"); err != nil {
		t.Fatalf("write to the child: %v", err)
	}
	if err := p.Stdin.Close(); err != nil {
		t.Fatalf("close the input: %v", err)
	}
	if got := string(readAllWithin(t, p.Stdout)); got != "hello\n" {
		t.Errorf("output = %q, want %q", got, "hello\n")
	}
	if exit := waitExit(t, p); exit.Code != 0 || exit.Err != nil {
		t.Errorf("exit = %+v, want a clean exit", exit)
	}
}

func TestNoInputPipe(t *testing.T) {
	t.Parallel()
	p := startHelper(t, "echo", false)
	if p.Stdin != nil {
		t.Error("Stdin is set, but no input pipe was asked for")
	}
	// The child reads nothing, so its copy loop ends at once.
	if got := readAllWithin(t, p.Stdout); len(got) != 0 {
		t.Errorf("output = %q, want none", got)
	}
	if exit := waitExit(t, p); exit.Err != nil {
		t.Errorf("exit = %+v, want a clean exit", exit)
	}
}

func TestExitCode(t *testing.T) {
	t.Parallel()
	p := startHelper(t, "exit3", false)
	exit := waitExit(t, p)
	if exit.Code != 3 || exit.Err == nil {
		t.Errorf("exit = %+v, want code 3 and an error", exit)
	}
	if got := p.Wait(); got != exit {
		t.Errorf("a second Wait gave %+v, want the same result %+v", got, exit)
	}
}

func TestStopPolitely(t *testing.T) {
	t.Parallel()
	skipWindows(t)
	p := startHelper(t, "graceful", true)
	readReady(t, p)
	begin := time.Now()
	if err := p.Stop(t.Context(), longGrace); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if took := time.Since(begin); took > longGrace/2 {
		t.Errorf("Stop took %v, so the polite request was not enough", took)
	}
	if exit := p.Wait(); exit.Code != 0 || exit.Err != nil {
		t.Errorf("exit = %+v, want the child's own clean exit", exit)
	}
}

func TestStopHardWhenTermIsIgnored(t *testing.T) {
	t.Parallel()
	skipWindows(t)
	p := startHelper(t, "ignore-term", true)
	readReady(t, p)
	begin := time.Now()
	if err := p.Stop(t.Context(), quickGrace); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if took := time.Since(begin); took < quickGrace {
		t.Errorf("Stop took %v, which is less than the grace of %v, so it did not wait", took, quickGrace)
	}
	if exit := p.Wait(); exit.Code != -1 || exit.Err == nil {
		t.Errorf("exit = %+v, want a kill (code -1 and an error)", exit)
	}
}

func TestStopKillsTheWholeTree(t *testing.T) {
	t.Parallel()
	skipWindows(t)
	p := startHelper(t, "grandchild", true)
	out := readReady(t, p)
	if err := p.Stop(t.Context(), quickGrace); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	// The grandchild holds the same output pipe. End of file means it is gone as well.
	readAllWithin(t, out)
}

func TestLeaderExitSweepsTheGroup(t *testing.T) {
	t.Parallel()
	skipWindows(t)
	p := startHelper(t, "orphan", false)
	readAllWithin(t, p.Stdout)
	if exit := waitExit(t, p); exit.Err != nil {
		t.Errorf("exit = %+v, want the leader's clean exit", exit)
	}
}

func TestContextCancelKills(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p, err := Start(ctx, Spec{
		Path: os.Args[0],
		Env:  []string{helperEnv + "=sleep"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = p.Stdout.Close() })
	readReady(t, p)
	cancel()
	if exit := waitExit(t, p); exit.Err == nil {
		t.Errorf("exit = %+v, want the process to have been killed", exit)
	}
}

func TestStopIsSafeToRepeatAndToShare(t *testing.T) {
	t.Parallel()
	p := startHelper(t, "sleep", true)
	readReady(t, p)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if err := p.Stop(t.Context(), quickGrace); err != nil {
				t.Errorf("Stop: %v", err)
			}
		})
	}
	wg.Wait()
	if err := p.Stop(t.Context(), quickGrace); err != nil {
		t.Errorf("Stop after the exit: %v", err)
	}
}

func TestStopWithACancelledContextKillsAtOnce(t *testing.T) {
	t.Parallel()
	skipWindows(t)
	p := startHelper(t, "ignore-term", true)
	readReady(t, p)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	begin := time.Now()
	if err := p.Stop(ctx, longGrace); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if took := time.Since(begin); took > longGrace/2 {
		t.Errorf("Stop took %v, so it waited out the grace period", took)
	}
}

func TestStderrTailIsBounded(t *testing.T) {
	t.Parallel()
	p := startHelper(t, "noisy", false)
	waitExit(t, p)
	tail := p.StderrTail()
	if len(tail) > StderrTailBytes || len(tail) < StderrTailBytes/2 {
		t.Errorf("tail has %d bytes, want close to but not over %d", len(tail), StderrTailBytes)
	}
	// A test binary built with coverage adds a warning after the last line, so look for it inside.
	if !strings.Contains(tail, "line 19999\n") {
		t.Errorf("tail lacks the last line: %q", tail[max(0, len(tail)-40):])
	}
}

func TestEnvironmentIsFiltered(t *testing.T) {
	t.Setenv("MARSHAL_SECRET", "should-not-pass")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "should-not-pass")
	t.Setenv("HOME", "/home/someone")
	p, err := Start(t.Context(), Spec{
		Path: os.Args[0],
		Env:  []string{helperEnv + "=env", "MARSHAL_SESSION=abc", "HOME=/home/override"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = p.Stdout.Close() })
	out := string(readAllWithin(t, p.Stdout))
	for _, want := range []string{"MARSHAL_SESSION=abc", "HOME=/home/override", helperEnv + "=env"} {
		if !strings.Contains(out, want) {
			t.Errorf("environment lacks %q:\n%s", want, out)
		}
	}
	for _, banned := range []string{"MARSHAL_SECRET", "AWS_SECRET_ACCESS_KEY", "/home/someone"} {
		if strings.Contains(out, banned) {
			t.Errorf("environment holds %q, which must not reach the child:\n%s", banned, out)
		}
	}
	if !strings.Contains(out, "PATH=") {
		t.Errorf("environment lacks PATH:\n%s", out)
	}
}

func TestWorkingFolder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p, err := Start(t.Context(), Spec{
		Path: os.Args[0], Dir: dir,
		Env: []string{helperEnv + "=cwd"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = p.Stdout.Close() })
	got := string(readAllWithin(t, p.Stdout))
	want, _ := filepath.EvalSymlinks(dir)
	gotReal, _ := filepath.EvalSymlinks(got)
	if gotReal != want {
		t.Errorf("working folder = %q, want %q", got, dir)
	}
}

func TestStartErrors(t *testing.T) {
	tests := []struct {
		name string
		spec Spec
		want string
	}{
		{"no program", Spec{}, "no program"},
		{"missing program", Spec{Path: filepath.Join(t.TempDir(), "absent")}, "absent"},
		{"missing folder", Spec{Path: os.Args[0], Dir: filepath.Join(t.TempDir(), "absent")}, "absent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Start(t.Context(), tt.spec)
			if err == nil {
				_ = p.Stop(context.Background(), 0)
				t.Fatal("Start succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to mention %q", err, tt.want)
			}
		})
	}
}
