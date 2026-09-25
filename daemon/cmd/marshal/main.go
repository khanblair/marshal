// Command marshal is the small command line client for the Marshal daemon.
package main

import (
	"os"

	"github.com/khanblair/marshal/daemon/internal/buildinfo"
	"github.com/khanblair/marshal/daemon/internal/platform"
)

const (
	exitOK       = 0
	exitFailed   = 1
	exitBadInput = 2
)

const usage = `Usage: marshal <command>

Commands:
  status [--dev] [--port N]              Show whether the daemon is running
  token [--dev] [--show]                 Show where the token file is, or the token itself with --show
  service install|uninstall|status [--dev]   Install, remove, or check the daemon's login service
  dev reset [--yes]                      Delete the dev daemon's data (dev mode only)
  version                                Print the version`

func main() {
	os.Exit(run(os.Args[1:], terminal{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr}))
}

// run runs one command on the real machine and returns the process exit code.
func run(args []string, term terminal) int {
	// A missing home folder only matters to commands that need it, which say so themselves.
	env, _ := platform.CurrentEnv()
	return runWith(env, args, term)
}

// runWith runs one command against the given machine, so tests can supply their own.
func runWith(env platform.Env, args []string, term terminal) int {
	if len(args) == 0 {
		say(term.stderr, "%s", usage)
		return exitBadInput
	}
	switch args[0] {
	case "version":
		say(term.stdout, "%s", buildinfo.Version)
		return exitOK
	case "status":
		return runStatus(args[1:], term)
	case "token":
		return runToken(env, args[1:], term)
	case "service":
		return runService(args[1:], term, platform.NewServiceInstaller, os.Executable)
	case "dev":
		return runDev(env, args[1:], term)
	case "help", "-h", "--help":
		say(term.stdout, "%s", usage)
		return exitOK
	default:
		say(term.stderr, "Unknown command %q.\n\n%s", args[0], usage)
		return exitBadInput
	}
}
