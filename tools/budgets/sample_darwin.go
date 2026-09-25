//go:build darwin

package main

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
)

func takeSample(ctx context.Context, pid int) (sample, error) {
	out, err := exec.CommandContext(ctx, "ps", "-o", "rss=", "-o", "cputime=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return sample{}, fmt.Errorf("ask ps about process %d: %w", pid, err)
	}
	return parsePS(string(out))
}
