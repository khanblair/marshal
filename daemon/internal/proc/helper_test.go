package proc

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// helperEnv selects what the test binary does when it is started as a child of a test. The tests
// start the test binary itself, so they need no other program on the machine.
const helperEnv = "MARSHAL_PROC_HELPER"

const (
	helperReady = "ready\n"
	readTimeout = 10 * time.Second
	longSleep   = time.Minute
)

func TestMain(m *testing.M) {
	if mode := os.Getenv(helperEnv); mode != "" {
		runHelper(mode)
		os.Exit(0)
	}
	goleak.VerifyTestMain(m)
}

// runHelper plays one child behavior.
func runHelper(mode string) {
	switch mode {
	case "echo":
		_, _ = io.Copy(os.Stdout, os.Stdin)
	case "sleep":
		fmt.Print(helperReady)
		time.Sleep(longSleep)
	case "graceful":
		term := make(chan os.Signal, 1)
		signal.Notify(term, syscall.SIGTERM)
		fmt.Print(helperReady)
		<-term
	case "ignore-term":
		signal.Ignore(syscall.SIGTERM)
		fmt.Print(helperReady)
		time.Sleep(longSleep)
	case "hold":
		signal.Ignore(syscall.SIGTERM)
		time.Sleep(longSleep)
	case "grandchild":
		signal.Ignore(syscall.SIGTERM)
		startHeldGrandchild()
		fmt.Print(helperReady)
		time.Sleep(longSleep)
	case "orphan":
		startHeldGrandchild()
	case "noisy":
		for i := range 20000 {
			fmt.Fprintf(os.Stderr, "line %d\n", i)
		}
	case "exit3":
		os.Exit(3)
	case "env":
		env := os.Environ()
		sort.Strings(env)
		fmt.Print(strings.Join(env, "\n"))
	case "cwd":
		dir, _ := os.Getwd()
		fmt.Print(dir)
	}
}

// startHeldGrandchild starts a child of the helper that keeps the helper's output pipe open and
// ignores the polite request, so only a kill of the whole tree can end it.
func startHeldGrandchild() {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), helperEnv+"=hold")
	cmd.Stdout = os.Stdout
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "cannot start the grandchild:", err)
		os.Exit(1)
	}
}

// startHelper starts the test binary in a helper mode and stops it when the test ends.
func startHelper(t *testing.T, mode string, stdin bool) *Process {
	t.Helper()
	p, err := Start(t.Context(), Spec{
		Path:  os.Args[0],
		Env:   []string{helperEnv + "=" + mode},
		Stdin: stdin,
	})
	if err != nil {
		t.Fatalf("start the helper: %v", err)
	}
	t.Cleanup(func() {
		if err := p.Stop(context.Background(), 0); err != nil {
			t.Errorf("stop the helper: %v", err)
		}
		_ = p.Stdout.Close()
	})
	return p
}

// readReady waits for the helper to say that it is set up, and returns a reader for the rest of
// its output.
func readReady(t *testing.T, p *Process) *bufio.Reader {
	t.Helper()
	reader := bufio.NewReader(p.Stdout)
	got := make(chan string, 1)
	go func() {
		line, _ := reader.ReadString('\n')
		got <- line
	}()
	select {
	case line := <-got:
		if line != helperReady {
			t.Fatalf("the helper said %q, want %q", line, helperReady)
		}
	case <-time.After(readTimeout):
		t.Fatal("the helper did not become ready")
	}
	return reader
}

// readAllWithin reads until end of file, and fails the test if that takes too long. End of file
// only comes when every process that held the pipe is gone.
func readAllWithin(t *testing.T, r io.Reader) []byte {
	t.Helper()
	type result struct {
		data []byte
		err  error
	}
	got := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(r)
		got <- result{data, err}
	}()
	select {
	case res := <-got:
		if res.err != nil {
			t.Fatalf("read the output: %v", res.err)
		}
		return res.data
	case <-time.After(readTimeout):
		t.Fatal("the output never reached its end, so something still holds the pipe")
		return nil
	}
}

// waitExit waits for the process to end, without hanging the test.
func waitExit(t *testing.T, p *Process) Exit {
	t.Helper()
	select {
	case <-p.Done():
		return p.Wait()
	case <-time.After(readTimeout):
		t.Fatal("the process did not exit")
		return Exit{}
	}
}

func skipWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("this test needs Unix signals")
	}
}
