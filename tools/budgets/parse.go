package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// fieldCount is how many fields both the ps line and the PowerShell line have.
	fieldCount      = 2
	bytesPerKB      = 1024
	secondsPerMin   = 60
	secondsPerHour  = 3600
	clockTicksPerS  = 100 // Linux reports process times in ticks of 1/100 second
	statCPUTimeIdx  = 13  // utime is the 14th field of /proc/<pid>/stat, counted from zero
	statFieldsAfter = 2   // fields after the "(name)" that hold the state and the parent id
)

// parseClockTime reads a CPU time as `ps` prints it on macOS: seconds with hundredths after
// an optional minutes and hours part, such as "0:01.25" or "1:02:03.50".
func parseClockTime(text string) (time.Duration, error) {
	parts := strings.Split(strings.TrimSpace(text), ":")
	if len(parts) == 0 || len(parts) > 3 {
		return 0, fmt.Errorf("cannot read the CPU time %q", text)
	}
	multipliers := []float64{1, secondsPerMin, secondsPerHour}
	var seconds float64
	for i := range parts {
		value, err := strconv.ParseFloat(parts[len(parts)-1-i], 64)
		if err != nil {
			return 0, fmt.Errorf("cannot read the CPU time %q: %w", text, err)
		}
		seconds += value * multipliers[i]
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

// parsePS reads the output of `ps -o rss= -o cputime=` for one process: the resident size in
// kilobytes, then the CPU time.
func parsePS(output string) (sample, error) {
	fields := strings.Fields(output)
	if len(fields) != fieldCount {
		return sample{}, fmt.Errorf("unexpected ps output %q", output)
	}
	kb, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return sample{}, fmt.Errorf("cannot read the memory size %q: %w", fields[0], err)
	}
	cpu, err := parseClockTime(fields[1])
	if err != nil {
		return sample{}, err
	}
	return sample{rssBytes: kb * bytesPerKB, cpu: cpu}, nil
}

// parseProcStat reads the user and system CPU ticks from the text of /proc/<pid>/stat. The
// process name is in brackets and may contain spaces, so the fields are counted after it.
func parseProcStat(stat string) (time.Duration, error) {
	end := strings.LastIndex(stat, ")")
	if end < 0 {
		return 0, fmt.Errorf("unexpected stat text %q", stat)
	}
	fields := strings.Fields(stat[end+1:])
	// After the name, the first field is the state, which is field 3 of the file. utime and
	// stime are fields 14 and 15, so they sit at these positions here.
	utimeIdx := statCPUTimeIdx - statFieldsAfter
	if len(fields) <= utimeIdx+1 {
		return 0, fmt.Errorf("stat text is too short: %q", stat)
	}
	user, err := strconv.ParseUint(fields[utimeIdx], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot read the user time: %w", err)
	}
	system, err := strconv.ParseUint(fields[utimeIdx+1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot read the system time: %w", err)
	}
	return time.Duration(user+system) * time.Second / clockTicksPerS, nil
}

// parseVMRSS reads the resident size from the text of /proc/<pid>/status.
func parseVMRSS(status string) (uint64, error) {
	for _, line := range strings.Split(status, "\n") {
		rest, found := strings.CutPrefix(line, "VmRSS:")
		if !found {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			break
		}
		kb, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("cannot read the memory size %q: %w", fields[0], err)
		}
		return kb * bytesPerKB, nil
	}
	return 0, fmt.Errorf("no VmRSS line in the status text")
}

// parsePowerShell reads "<working set bytes> <cpu seconds>" as printed by the Windows sampler.
func parsePowerShell(output string) (sample, error) {
	fields := strings.Fields(output)
	if len(fields) != fieldCount {
		return sample{}, fmt.Errorf("unexpected PowerShell output %q", output)
	}
	bytes, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return sample{}, fmt.Errorf("cannot read the memory size %q: %w", fields[0], err)
	}
	seconds, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return sample{}, fmt.Errorf("cannot read the CPU time %q: %w", fields[1], err)
	}
	return sample{rssBytes: bytes, cpu: time.Duration(seconds * float64(time.Second))}, nil
}
