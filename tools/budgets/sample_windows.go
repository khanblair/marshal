//go:build windows

package main

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
)

const powerShellScript = `$p = Get-Process -Id %d; "$($p.WorkingSet64) $($p.TotalProcessorTime.TotalSeconds)"`

func takeSample(ctx context.Context, pid int) (sample, error) {
	script := fmt.Sprintf(powerShellScript, pid)
	out, err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return sample{}, fmt.Errorf("ask PowerShell about process %s: %w", strconv.Itoa(pid), err)
	}
	return parsePowerShell(string(out))
}
