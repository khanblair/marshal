// Command budgets measures the daemon against the performance budgets in docs/architecture.md
// section 14 and exits with an error when one is over. It starts the daemon in dev mode, lets it
// sit idle, and reads its memory and processor use.
//
//	budgets -daemon path/to/marshald [-idle 10s]
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"
)

const (
	defaultIdle          = 10 * time.Second
	defaultMaxRSSMB      = 50
	defaultMaxCPUPercent = 1.0
	exitOK               = 0
	exitOver             = 1
	exitBadInput         = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func say(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format+"\n", args...)
}

func run(args []string, stdout, stderr io.Writer) int {
	var opts options
	fs := flag.NewFlagSet("budgets", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opts.daemon, "daemon", "", "path to the marshald program (required)")
	fs.DurationVar(&opts.idle, "idle", defaultIdle, "how long to let the daemon sit idle (use 60s in CI)")
	fs.Float64Var(&opts.maxRSSMB, "max-rss-mb", defaultMaxRSSMB, "idle memory budget in MB")
	fs.Float64Var(&opts.maxCPUPercent, "max-cpu-percent", defaultMaxCPUPercent, "idle processor budget in percent of one core")
	if err := fs.Parse(args); err != nil {
		return exitBadInput
	}
	if opts.daemon == "" || opts.idle <= 0 {
		say(stderr, "Usage: budgets -daemon path/to/marshald [-idle 10s]")
		return exitBadInput
	}
	opts.daemon = withExecutableSuffix(opts.daemon)
	got, err := measure(context.Background(), opts)
	if err != nil {
		say(stderr, "%v", err)
		return exitOver
	}
	return report(stdout, got.verdicts(opts))
}

// report prints one line per budget and returns the exit code.
func report(w io.Writer, verdicts []verdict) int {
	code := exitOK
	for _, v := range verdicts {
		status := "ok  "
		if !v.within {
			status, code = "FAIL", exitOver
		}
		say(w, "%s  %-16s %.2f %s of %.2f %s", status, v.name, v.got, v.unit, v.limit, v.unit)
	}
	return code
}

// withExecutableSuffix adds ".exe" on Windows, where Go writes programs with it.
func withExecutableSuffix(path string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(path, ".exe") {
		return path + ".exe"
	}
	return path
}
