package platform

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// serviceCommandTimeout bounds every call to a service manager's own command line tool
// (launchctl, systemctl, schtasks). These are short, one-shot admin commands, not long-lived
// agent processes, so ten seconds is generous.
const serviceCommandTimeout = 10 * time.Second

// ServiceInstaller registers, removes, and reports on marshald as a per-user service: a launchd
// agent on macOS, a systemd user unit on Linux, and a scheduled task on Windows. NewServiceInstaller
// picks the right one for this machine.
type ServiceInstaller interface {
	// Install writes the service definition for spec and starts it. Installing again over an
	// existing service replaces it.
	Install(ctx context.Context, spec ServiceSpec) error
	// Uninstall stops and removes the service for mode. It does not fail if the service was
	// already gone.
	Uninstall(ctx context.Context, mode Mode) error
	// Status reports whether the service is installed and, if so, whether it looks like it is
	// running.
	Status(ctx context.Context, mode Mode) (ServiceStatus, error)
}

// ServiceSpec is what to install: the program to run, its arguments, and which daemon mode it
// starts, since the normal and dev daemons are installed as two separate services that can run
// side by side.
type ServiceSpec struct {
	ExecutablePath string
	Args           []string
	Mode           Mode
}

// ServiceStatus is what Status found. Detail is a short, human sentence with anything Installed
// and Running alone do not capture, such as why the running state is unknown.
type ServiceStatus struct {
	Installed bool
	Running   bool
	Detail    string
}

// commandRunner runs one short admin command to completion and returns its combined output.
// Production code uses runServiceCommand; tests use a fake that never touches the real service
// manager, so the argument lists this package builds can be checked without registering or
// removing anything on the machine that runs the test.
type commandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// runServiceCommand runs a service manager's command line tool and waits for it, bounded by
// serviceCommandTimeout. Every argument is a separate list entry, never a shell string.
//
// This goes through os/exec directly rather than internal/proc, the package's usual one place to
// start child processes: internal/proc is built for long-lived, streamed, process-tree-managed
// children such as agent CLIs, and its environment allow-list drops session variables a user
// service manager needs (for example XDG_RUNTIME_DIR and DBUS_SESSION_BUS_ADDRESS on Linux, which
// systemctl --user needs to reach the user's session bus). A short admin command that blocks for
// its combined output and inherits the daemon's own environment is exactly the case the package
// comment on internal/proc calls out as not what it is for.
func runServiceCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, serviceCommandTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return out, err
}

// commandFailure turns a failed admin command into a short, plain message: the tool's own first
// line of output when it printed one (its real reason, such as "Access is denied." or "Failed to
// connect to bus: No such file or directory"), since that is far more useful to a person than the
// Go process error alone ("exit status 1"), which is the fallback when the command printed
// nothing.
func commandFailure(out []byte, err error) string {
	if line := firstNonEmptyLine(out); line != "" {
		return line
	}
	return err.Error()
}

// firstNonEmptyLine returns the first line of out that has visible content, trimmed.
func firstNonEmptyLine(out []byte) string {
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}
