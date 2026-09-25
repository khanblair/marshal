//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

func takeSample(_ context.Context, pid int) (sample, error) {
	dir := filepath.Join("/proc", strconv.Itoa(pid))
	stat, err := os.ReadFile(filepath.Join(dir, "stat"))
	if err != nil {
		return sample{}, fmt.Errorf("read the process times: %w", err)
	}
	status, err := os.ReadFile(filepath.Join(dir, "status"))
	if err != nil {
		return sample{}, fmt.Errorf("read the process memory: %w", err)
	}
	cpu, err := parseProcStat(string(stat))
	if err != nil {
		return sample{}, err
	}
	rss, err := parseVMRSS(string(status))
	if err != nil {
		return sample{}, err
	}
	return sample{rssBytes: rss, cpu: cpu}, nil
}
