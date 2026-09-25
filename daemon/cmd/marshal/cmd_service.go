package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/platform"
)

// serviceTimeout bounds one service subcommand's calls into the operating system's service
// manager. Install can run more than one admin command in a row (for example macOS tries
// "bootout" then "bootstrap", and may fall back to "load"), so this is generous next to any one
// call's own shorter timeout inside internal/platform.
const serviceTimeout = 30 * time.Second

const serviceUsage = "Usage: marshal service <install|uninstall|status> [--dev]"

// runService dispatches the three service subcommands. newInstaller and selfExecutable are
// injected so tests can run every code path here without ever calling launchctl, systemctl, or
// schtasks, and without depending on where the test binary itself happens to live.
func runService(args []string, term terminal, newInstaller func() platform.ServiceInstaller, selfExecutable func() (string, error)) int {
	if len(args) == 0 {
		say(term.stderr, "%s", serviceUsage)
		return exitBadInput
	}
	switch args[0] {
	case "install":
		return runServiceInstall(args[1:], term, newInstaller, selfExecutable)
	case "uninstall":
		return runServiceUninstall(args[1:], term, newInstaller)
	case "status":
		return runServiceStatus(args[1:], term, newInstaller)
	default:
		say(term.stderr, "Unknown service command %q.\n\n%s", args[0], serviceUsage)
		return exitBadInput
	}
}

// trimTrailingPeriod avoids a doubled "." when an error's own text already ends with one, so it
// can be dropped into a plain sentence this package builds around it.
func trimTrailingPeriod(err error) string {
	return strings.TrimSuffix(err.Error(), ".")
}

// daemonExecutablePath finds marshald next to a fully resolved copy of selfPath, marshal's own
// program location. EvalSymlinks resolves it first, since a symlinked or relative launcher would
// break a login-time launch that has no shell to resolve one for it.
func daemonExecutablePath(selfPath string) (string, error) {
	resolved, err := filepath.EvalSymlinks(selfPath)
	if err != nil {
		return "", fmt.Errorf("find where marshal itself is: %w", err)
	}
	name := "marshald"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	exe := filepath.Join(filepath.Dir(resolved), name)
	if _, err := os.Stat(exe); err != nil {
		return "", fmt.Errorf("marshald was not found next to marshal, at %s", exe)
	}
	return exe, nil
}

// runServiceInstall resolves marshald's path next to marshal's own, and installs it (not
// marshal itself) as the per-user service.
func runServiceInstall(args []string, term terminal, newInstaller func() platform.ServiceInstaller, selfExecutable func() (string, error)) int {
	fs := flag.NewFlagSet("service install", flag.ContinueOnError)
	fs.SetOutput(term.stderr)
	dev := fs.Bool("dev", false, "install the dev daemon as its own separate service, so it can run beside a normal install")
	if err := fs.Parse(args); err != nil {
		return exitBadInput
	}
	self, err := selfExecutable()
	if err != nil {
		say(term.stderr, "Marshal could not find its own program location: %s.", trimTrailingPeriod(err))
		return exitFailed
	}
	exe, err := daemonExecutablePath(self)
	if err != nil {
		say(term.stderr, "The service could not be installed: %s.", trimTrailingPeriod(err))
		return exitFailed
	}
	spec := platform.ServiceSpec{ExecutablePath: exe, Mode: platform.ModeNormal}
	if *dev {
		spec.Mode = platform.ModeDev
		spec.Args = []string{"--dev"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), serviceTimeout)
	defer cancel()
	if err := newInstaller().Install(ctx, spec); err != nil {
		if errors.Is(err, platform.ErrLingerNotEnabled) {
			// Install itself succeeded and the service is running now; only starting at boot
			// without an active login session needs a person's attention.
			say(term.stderr, "Warning: the service is installed and running, but %s.", trimTrailingPeriod(err))
		} else {
			say(term.stderr, "The service could not be installed: %s.", trimTrailingPeriod(err))
			return exitFailed
		}
	}
	say(term.stdout, "Installed the Marshal daemon as a service. It starts automatically from now on.")
	return exitOK
}

// runServiceUninstall stops and removes the service.
func runServiceUninstall(args []string, term terminal, newInstaller func() platform.ServiceInstaller) int {
	fs := flag.NewFlagSet("service uninstall", flag.ContinueOnError)
	fs.SetOutput(term.stderr)
	dev := fs.Bool("dev", false, "uninstall the dev daemon's service")
	if err := fs.Parse(args); err != nil {
		return exitBadInput
	}
	mode := platform.ModeNormal
	if *dev {
		mode = platform.ModeDev
	}
	ctx, cancel := context.WithTimeout(context.Background(), serviceTimeout)
	defer cancel()
	if err := newInstaller().Uninstall(ctx, mode); err != nil {
		say(term.stderr, "The service could not be removed: %s.", trimTrailingPeriod(err))
		return exitFailed
	}
	say(term.stdout, "Removed the Marshal daemon service.")
	return exitOK
}

// runServiceStatus reports whether the service is installed and running.
func runServiceStatus(args []string, term terminal, newInstaller func() platform.ServiceInstaller) int {
	fs := flag.NewFlagSet("service status", flag.ContinueOnError)
	fs.SetOutput(term.stderr)
	dev := fs.Bool("dev", false, "check the dev daemon's service")
	if err := fs.Parse(args); err != nil {
		return exitBadInput
	}
	mode := platform.ModeNormal
	if *dev {
		mode = platform.ModeDev
	}
	ctx, cancel := context.WithTimeout(context.Background(), serviceTimeout)
	defer cancel()
	status, err := newInstaller().Status(ctx, mode)
	if err != nil {
		say(term.stderr, "The service status could not be checked: %s.", trimTrailingPeriod(err))
		return exitFailed
	}
	say(term.stdout, "%s", serviceStatusMessage(status))
	return exitOK
}

// serviceStatusMessage turns a ServiceStatus into one plain sentence.
func serviceStatusMessage(status platform.ServiceStatus) string {
	switch {
	case !status.Installed:
		return "The service is not installed."
	case status.Running:
		return "The service is installed and running."
	default:
		return "The service is installed but not running."
	}
}
