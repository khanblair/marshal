package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"
)

const (
	healthPollInterval = 50 * time.Millisecond
	healthWait         = 15 * time.Second
	bytesPerMB         = 1024 * 1024
	percent            = 100
	stopWait           = 5 * time.Second
)

// options say what to measure and what counts as too much.
type options struct {
	daemon        string
	idle          time.Duration
	maxRSSMB      float64
	maxCPUPercent float64
}

// result is what the measurement found.
type result struct {
	rssMB      float64
	cpuPercent float64
}

// freePort asks the system for a port that nothing is using.
func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("find a free port: %w", err)
	}
	defer func() { _ = listener.Close() }()
	tcp, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("the free port has an unexpected address")
	}
	return tcp.Port, nil
}

// waitHealthy polls the daemon's health call until it answers.
func waitHealthy(ctx context.Context, port int) error {
	url := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) + "/v1/health"
	deadline := time.Now().Add(healthWait)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("build the health request: %w", err)
		}
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for the daemon: %w", ctx.Err())
		case <-time.After(healthPollInterval):
		}
	}
	return fmt.Errorf("the daemon did not answer within %s", healthWait)
}

// startDaemon runs the daemon in dev mode on a free port with a temporary data folder. The
// returned function stops it and removes the folder.
func startDaemon(path string) (pid, port int, stop func(), err error) {
	dataDir, err := os.MkdirTemp("", "marshal-budget-*")
	if err != nil {
		return 0, 0, nil, fmt.Errorf("make a data folder: %w", err)
	}
	port, err = freePort()
	if err != nil {
		_ = os.RemoveAll(dataDir)
		return 0, 0, nil, err
	}
	cmd := exec.Command(path, "--dev", "--port", strconv.Itoa(port), "--data-dir", dataDir, "--log-level", "error", "--agent", "stub")
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(dataDir)
		return 0, 0, nil, fmt.Errorf("start %s: %w", path, err)
	}
	stop = func() {
		_ = cmd.Process.Kill()
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(stopWait):
		}
		_ = os.RemoveAll(dataDir)
	}
	return cmd.Process.Pid, port, stop, nil
}

// measure starts the daemon, lets it sit idle, and reports its memory and processor use over
// that time. The start-up work happens before the first look, so it is not counted.
func measure(ctx context.Context, opts options) (result, error) {
	pid, port, stop, err := startDaemon(opts.daemon)
	if err != nil {
		return result{}, err
	}
	defer stop()
	if err := waitHealthy(ctx, port); err != nil {
		return result{}, err
	}
	first, err := takeSample(ctx, pid)
	if err != nil {
		return result{}, err
	}
	select {
	case <-ctx.Done():
		return result{}, fmt.Errorf("wait while idle: %w", ctx.Err())
	case <-time.After(opts.idle):
	}
	last, err := takeSample(ctx, pid)
	if err != nil {
		return result{}, err
	}
	used := last.cpu - first.cpu
	return result{
		rssMB:      float64(last.rssBytes) / bytesPerMB,
		cpuPercent: float64(used) / float64(opts.idle) * percent,
	}, nil
}

// verdict is one budget and whether the measurement stayed inside it.
type verdict struct {
	name   string
	unit   string
	got    float64
	limit  float64
	within bool
}

func (r result) verdicts(opts options) []verdict {
	return []verdict{
		{"Daemon idle RAM", "MB", r.rssMB, opts.maxRSSMB, r.rssMB <= opts.maxRSSMB},
		{"Daemon idle CPU", "%", r.cpuPercent, opts.maxCPUPercent, r.cpuPercent <= opts.maxCPUPercent},
	}
}
