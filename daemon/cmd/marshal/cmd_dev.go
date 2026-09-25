package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/platform"
)

const daemonCheckTimeout = time.Second

func runDev(env platform.Env, args []string, term terminal) int {
	if len(args) == 0 || args[0] != "reset" {
		say(term.stderr, "Usage: marshal dev reset [--yes]")
		return exitBadInput
	}
	return runDevReset(env, args[1:], term)
}

// runDevReset deletes the dev daemon's data folder. It never touches a normal install: the
// folder is chosen the same way the dev daemon chooses it, and platform.ResetDevData refuses
// any folder that is not a dev data folder.
func runDevReset(env platform.Env, args []string, term terminal) int {
	fs := flag.NewFlagSet("dev reset", flag.ContinueOnError)
	fs.SetOutput(term.stderr)
	yes := fs.Bool("yes", false, "delete without asking")
	if err := fs.Parse(args); err != nil {
		return exitBadInput
	}
	settings, err := config.Load([]string{"--dev"}, env, term.stderr)
	if err != nil {
		say(term.stderr, "%v", err)
		return exitFailed
	}
	if err := requireStopped(settings.Port); err != nil {
		say(term.stderr, "%v", err)
		return exitFailed
	}
	if err := platform.CheckResettable(settings.DataDir, env.Home); err != nil {
		say(term.stderr, "%v", err)
		return exitFailed
	}
	if !*yes && !confirm(term, settings.DataDir) {
		say(term.stdout, "Nothing was deleted.")
		return exitOK
	}
	if err := platform.ResetDevData(settings.DataDir, env.Home); err != nil {
		say(term.stderr, "%v", err)
		return exitFailed
	}
	say(term.stdout, "Deleted %s.", settings.DataDir)
	return exitOK
}

// requireStopped refuses to reset while a dev daemon is running, since it would still be
// writing to the folder.
func requireStopped(port int) error {
	ctx, cancel := context.WithTimeout(context.Background(), daemonCheckTimeout)
	defer cancel()
	if _, err := fetchHealth(ctx, port); err == nil {
		return errors.New("the dev daemon is running. Stop it first, then reset")
	}
	return nil
}

func confirm(term terminal, dir string) bool {
	_, _ = fmt.Fprintf(term.stdout, "This deletes the dev data folder %s, including dev worktrees. Type yes to continue: ", dir)
	answer, err := bufio.NewReader(term.stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintln(os.Stderr, err)
		return false
	}
	return strings.EqualFold(strings.TrimSpace(answer), "yes")
}
