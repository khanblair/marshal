package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeEnv is an environment with the given variables and a fixed temp folder.
func fakeEnv(vars map[string]string) environment {
	return environment{getenv: func(k string) string { return vars[k] }, tempDir: "/tmp-root"}
}

func TestParseConfig(t *testing.T) {
	defaultState := filepath.Join("/tmp-root", defaultStateDirName)
	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		want    config
		wantErr string
	}{
		{name: "defaults", want: config{scenario: "default", stateDir: defaultState, speed: 1}},
		{
			name: "flags",
			args: []string{"--scenario", "approve", "--state-dir", "/s", "--speed", "0.5"},
			want: config{scenario: "approve", stateDir: "/s", speed: 0.5},
		},
		{
			name: "single dash and equals",
			args: []string{"-scenario=resume", "-speed=2"},
			want: config{scenario: "resume", stateDir: defaultState, speed: 2},
		},
		{
			name: "environment",
			env:  map[string]string{"STUB_SCENARIO": "smells", "STUB_STATE_DIR": "/e", "STUB_SPEED": "0.25"},
			want: config{scenario: "smells", stateDir: "/e", speed: 0.25},
		},
		{
			name: "flag beats environment",
			args: []string{"--scenario", "approve", "--speed", "3"},
			env:  map[string]string{"STUB_SCENARIO": "smells", "STUB_SPEED": "0.25"},
			want: config{scenario: "approve", stateDir: defaultState, speed: 3},
		},
		{name: "speed zero", args: []string{"--speed", "0"}, want: config{scenario: "default", stateDir: defaultState}},
		{name: "version", args: []string{"--version"}, want: config{scenario: "default", stateDir: defaultState, speed: 1, showVersion: true}},
		{name: "negative speed", args: []string{"--speed", "-1"}, wantErr: "speed -1"},
		{name: "speed that is not a number", args: []string{"--speed", "fast"}, wantErr: "invalid value"},
		{name: "speed NaN", args: []string{"--speed", "NaN"}, wantErr: "speed NaN"},
		{name: "speed infinite", args: []string{"--speed", "Inf"}, wantErr: "speed +Inf"},
		{name: "environment speed that is not a number", env: map[string]string{"STUB_SPEED": "x"}, wantErr: "STUB_SPEED is not a number"},
		{name: "unknown flag", args: []string{"--nope"}, wantErr: "not defined"},
		{name: "stray argument", args: []string{"extra"}, wantErr: `unexpected argument "extra"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseConfig(tt.args, fakeEnv(tt.env))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("parseConfig = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	term := terminal{stdin: strings.NewReader(""), stdout: &out, stderr: &errOut}
	code = run(t.Context(), args, term, fakeEnv(nil))
	return code, out.String(), errOut.String()
}

func TestRunVersionAndHelp(t *testing.T) {
	code, out, _ := runCLI(t, "--version")
	if code != exitOK || strings.TrimSpace(out) != "stub-agent "+version {
		t.Errorf("--version = %d %q", code, out)
	}
	code, out, _ = runCLI(t, "--help")
	if code != exitOK || !strings.Contains(out, "Usage: stub-agent") {
		t.Errorf("--help = %d %q", code, out)
	}
}

func TestRunRejectsBadInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown flag", args: []string{"--nope"}, want: "Usage: stub-agent"},
		{name: "bad speed", args: []string{"--speed", "-2"}, want: "speed -2"},
		{name: "unknown scenario", args: []string{"--scenario", "nope"}, want: `unknown scenario "nope" (known: approve, default,`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := runCLI(t, tt.args...)
			if code != exitBadInput || out != "" || !strings.Contains(errOut, tt.want) {
				t.Errorf("run = %d, stdout %q, stderr %q; want exit %d and %q on stderr", code, out, errOut, exitBadInput, tt.want)
			}
		})
	}
}

// pipes is a run of the program with its streams held by the test.
type pipes struct {
	stdin    *io.PipeWriter
	stdout   *bufio.Scanner
	done     chan int
	closeAll func()
}

// startRun starts the program on pipes, the way a client that starts it as a child process would
// see it.
func startRun(t *testing.T, ctx context.Context, args ...string) *pipes {
	t.Helper()
	// run replaces the default logger, so the test puts its own back afterwards.
	t.Cleanup(func() { slog.SetDefault(slog.New(slog.DiscardHandler)) })
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	p := &pipes{stdin: inW, stdout: bufio.NewScanner(outR), done: make(chan int, 1)}
	p.closeAll = func() {
		for _, c := range []io.Closer{inW, inR, outW, outR} {
			_ = c.Close()
		}
	}
	t.Cleanup(p.closeAll)
	go func() {
		p.done <- run(ctx, args, terminal{stdin: inR, stdout: outW, stderr: io.Discard}, fakeEnv(nil))
	}()
	return p
}

func TestRunSpeaksACPOnStandardOutput(t *testing.T) {
	p := startRun(t, t.Context())
	request := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}` + "\n"
	if _, err := io.WriteString(p.stdin, request); err != nil {
		t.Fatal(err)
	}
	if !p.stdout.Scan() {
		t.Fatalf("no response: %v", p.stdout.Err())
	}
	var resp struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  struct {
			AgentCapabilities struct {
				LoadSession bool `json:"loadSession"`
			} `json:"agentCapabilities"`
		} `json:"result"`
	}
	if err := json.Unmarshal(p.stdout.Bytes(), &resp); err != nil {
		t.Fatalf("standard output is not JSON: %q: %v", p.stdout.Text(), err)
	}
	if resp.JSONRPC != "2.0" || resp.ID != 1 || !resp.Result.AgentCapabilities.LoadSession {
		t.Errorf("response = %+v", resp)
	}

	// The client closing its end is how the daemon stops the agent.
	_ = p.stdin.Close()
	select {
	case code := <-p.done:
		if code != exitOK {
			t.Errorf("exit code = %d", code)
		}
	case <-time.After(testTimeout):
		t.Fatal("the program did not stop when its input closed")
	}
}

func TestRunStopsWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	p := startRun(t, ctx)
	cancel()
	select {
	case code := <-p.done:
		if code != exitOK {
			t.Errorf("exit code = %d", code)
		}
	case <-time.After(testTimeout):
		t.Fatal("the program did not stop when its context ended")
	}
	// Closing the streams lets the connection's reader goroutine finish.
	p.closeAll()
}
