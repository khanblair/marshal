package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	systemdUnitBaseName = "marshald"
	systemdUnitDirMode  = 0o755
	systemdUnitFileMode = 0o644
	systemdCommand      = "systemctl"
	systemdUserFlag     = "--user"
	systemdActiveResult = "active"
)

// ErrLingerNotEnabled means Install itself succeeded and the service is running now, but
// "loginctl enable-linger" failed, so the service will not start again at boot without an active
// login session until lingering is enabled by hand. Some setups (for example a container without
// polkit) do not grant this account the privilege for it.
var ErrLingerNotEnabled = errors.New("could not enable lingering for this account")

// systemdInstaller is the Linux ServiceInstaller: a per-user systemd unit. Its command runner and
// environment reader are both injected, so tests can check exactly what it would run and write
// without ever calling the real systemctl or touching a real home folder.
type systemdInstaller struct {
	run commandRunner
	env func() (Env, error)
}

func newSystemdInstaller(run commandRunner, env func() (Env, error)) *systemdInstaller {
	return &systemdInstaller{run: run, env: env}
}

// systemdUnitName is the unit's file name, distinct for the dev daemon so the two services never
// collide.
func systemdUnitName(mode Mode) string {
	name := systemdUnitBaseName
	if mode == ModeDev {
		name += devSuffix
	}
	return name + ".service"
}

// systemdUnitPath is where the unit file lives, per systemd's per-user unit search path.
func systemdUnitPath(home string, mode Mode) string {
	return filepath.Join(home, ".config", "systemd", "user", systemdUnitName(mode))
}

// systemdExecStart builds the ExecStart line's value: the absolute executable path and its
// arguments, space joined, with an argument quoted only when it holds a character systemd's own
// unit file line splitting would otherwise treat as a separator (systemd.unit(5), "Command
// lines"). Args already come from internal/proc-style lists, never a shell line, so this only
// ever needs to protect a value that itself contains a space or a quote.
func systemdExecStart(spec ServiceSpec) string {
	parts := make([]string, 0, len(spec.Args)+1)
	parts = append(parts, systemdQuoteArg(spec.ExecutablePath))
	for _, arg := range spec.Args {
		parts = append(parts, systemdQuoteArg(arg))
	}
	return strings.Join(parts, " ")
}

// systemdQuoteArg returns s unchanged when it needs no quoting, and otherwise wraps it in double
// quotes with its own quotes and backslashes escaped, systemd's documented C-style quoting.
func systemdQuoteArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\"'\\") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		if r == '"' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

// renderSystemdUnit builds the unit file content for spec.
func renderSystemdUnit(spec ServiceSpec) []byte {
	var b strings.Builder
	b.WriteString("[Unit]\n")
	b.WriteString("Description=Marshal daemon\n")
	b.WriteString("\n[Service]\n")
	fmt.Fprintf(&b, "ExecStart=%s\n", systemdExecStart(spec))
	b.WriteString("Restart=on-failure\n")
	b.WriteString("\n[Install]\n")
	b.WriteString("WantedBy=default.target\n")
	return []byte(b.String())
}

// Install writes the unit file, reloads systemd's user manager, and enables and starts the unit.
// It also tries to enable lingering, so the service can start at boot without an active login
// session. That one step's own failure does not fail the whole install (the unit is already
// enabled and running): it comes back wrapped in ErrLingerNotEnabled, for the caller to show as a
// warning rather than a failure, since some setups lack the privilege for it.
func (i *systemdInstaller) Install(ctx context.Context, spec ServiceSpec) error {
	env, err := i.env()
	if err != nil {
		return err
	}
	unitPath := systemdUnitPath(env.Home, spec.Mode)
	if err := os.MkdirAll(filepath.Dir(unitPath), systemdUnitDirMode); err != nil {
		return fmt.Errorf("make the systemd user unit folder: %w", err)
	}
	if err := os.WriteFile(unitPath, renderSystemdUnit(spec), systemdUnitFileMode); err != nil {
		return fmt.Errorf("write the systemd unit file: %w", err)
	}
	// A missing systemctl binary and a missing user session (headless, or no active login) both
	// surface here, as this call's own error: there is nothing further to reach for either case,
	// so both are reported the same plain way rather than as a stack of raw command output.
	if out, err := i.run(ctx, systemdCommand, systemdUserFlag, "daemon-reload"); err != nil {
		return fmt.Errorf("this machine has no systemd user session to install the service into: %s", commandFailure(out, err))
	}
	if out, err := i.run(ctx, systemdCommand, systemdUserFlag, "enable", "--now", systemdUnitName(spec.Mode)); err != nil {
		return fmt.Errorf("could not start the systemd user service: %s", commandFailure(out, err))
	}
	if out, err := i.run(ctx, "loginctl", "enable-linger"); err != nil {
		return fmt.Errorf("%w: %s", ErrLingerNotEnabled, commandFailure(out, err))
	}
	return nil
}

// Uninstall disables and stops the unit, removes its file, and reloads systemd's user manager. It
// does not fail if the unit was already gone.
func (i *systemdInstaller) Uninstall(ctx context.Context, mode Mode) error {
	env, err := i.env()
	if err != nil {
		return err
	}
	unit := systemdUnitName(mode)
	out, err := i.run(ctx, systemdCommand, systemdUserFlag, "disable", "--now", unit)
	if err != nil && !systemdDisableMissingOK(out) {
		return fmt.Errorf("stop the systemd user service: %s", commandFailure(out, err))
	}
	unitPath := systemdUnitPath(env.Home, mode)
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove the systemd unit file: %w", err)
	}
	if out, err := i.run(ctx, systemdCommand, systemdUserFlag, "daemon-reload"); err != nil {
		return fmt.Errorf("reload the systemd user manager: %s", commandFailure(out, err))
	}
	return nil
}

// systemdDisableMissingOK reports whether a failed disable only means the unit file was not
// there, systemctl's documented message for that case (systemctl(1)).
func systemdDisableMissingOK(out []byte) bool {
	return strings.Contains(strings.ToLower(string(out)), "does not exist")
}

// Status checks the unit file first, the same way Status on macOS checks the plist first: a
// query for a unit that plainly is not there should never depend on parsing is-enabled's answer
// (whose "not-found" word only appears on its standard output, and can be missed if systemctl
// writes anything to standard error first) or on a user session bus that may not be reachable.
// When the file exists, it asks systemctl whether the unit is enabled and active.
func (i *systemdInstaller) Status(ctx context.Context, mode Mode) (ServiceStatus, error) {
	env, err := i.env()
	if err != nil {
		return ServiceStatus{}, err
	}
	if _, statErr := os.Stat(systemdUnitPath(env.Home, mode)); statErr != nil {
		return ServiceStatus{}, nil
	}
	unit := systemdUnitName(mode)
	enabledOut, _ := i.run(ctx, systemdCommand, systemdUserFlag, "is-enabled", unit)
	activeOut, _ := i.run(ctx, systemdCommand, systemdUserFlag, "is-active", unit)
	return parseSystemdStatus(enabledOut, activeOut), nil
}

// parseSystemdStatus reads the first line of is-enabled and is-active's output, the documented
// form of both (systemctl(1)): a single word, "enabled"/"disabled"/... and
// "active"/"inactive"/"failed".
func parseSystemdStatus(enabledOut, activeOut []byte) ServiceStatus {
	enabled, active := firstLine(enabledOut), firstLine(activeOut)
	return ServiceStatus{
		Installed: true,
		Running:   active == systemdActiveResult,
		Detail:    fmt.Sprintf("enabled: %s, active: %s", enabled, active),
	}
}

// firstLineParts is how many pieces strings.SplitN needs to isolate the first line: the line
// itself, and everything after it.
const firstLineParts = 2

// firstLine returns the first line of out, trimmed, for the single-word answers systemctl's
// query subcommands print.
func firstLine(out []byte) string {
	return strings.TrimSpace(strings.SplitN(string(out), "\n", firstLineParts)[0])
}
