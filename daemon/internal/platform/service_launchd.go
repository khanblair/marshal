package platform

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	launchdLabelBase    = "com.marshal.daemon"
	launchAgentsDirMode = 0o755
	launchdPlistMode    = 0o644
	serviceLogDirMode   = 0o755
	serviceLogFileMode  = 0o644
)

// launchdInstaller is the macOS ServiceInstaller: a per-user launchd agent. Its command runner
// and environment reader are both injected, so tests can check exactly what it would run and
// where it would write without ever calling the real launchctl or touching a real home folder.
type launchdInstaller struct {
	run commandRunner
	env func() (Env, error)
}

func newLaunchdInstaller(run commandRunner, env func() (Env, error)) *launchdInstaller {
	return &launchdInstaller{run: run, env: env}
}

// launchdLabel is the launch agent's identifier, distinct for the dev daemon so the two services
// never collide.
func launchdLabel(mode Mode) string {
	if mode == ModeDev {
		return launchdLabelBase + devSuffix
	}
	return launchdLabelBase
}

// launchdPlistPath is where the agent's property list lives, per Apple's per-user LaunchAgents
// convention.
func launchdPlistPath(home string, mode Mode) string {
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel(mode)+".plist")
}

// launchdGUIDomain is this user's launchd GUI domain, the target every bootstrap, bootout, and
// print command below addresses the agent through.
func launchdGUIDomain() string {
	return "gui/" + strconv.Itoa(os.Getuid())
}

// renderLaunchdPlist builds the property list content for spec. Values are XML-escaped, since a
// path or argument is not guaranteed free of characters such as "&" that would otherwise break
// the document.
func renderLaunchdPlist(spec ServiceSpec, logPath string) []byte {
	var b bytes.Buffer
	b.WriteString(xml.Header)
	b.WriteString("<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n")
	b.WriteString("<plist version=\"1.0\">\n<dict>\n")
	writePlistString(&b, "Label", launchdLabel(spec.Mode))
	b.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n")
	writePlistArrayItem(&b, spec.ExecutablePath)
	for _, arg := range spec.Args {
		writePlistArrayItem(&b, arg)
	}
	b.WriteString("\t</array>\n")
	b.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n")
	// SuccessfulExit false: restart the agent when it crashes, but not after a clean exit, since
	// `marshal dev reset` and a deliberate stop must not be restarted into a loop.
	b.WriteString("\t<key>KeepAlive</key>\n\t<dict>\n\t\t<key>SuccessfulExit</key>\n\t\t<false/>\n\t</dict>\n")
	writePlistString(&b, "StandardOutPath", logPath)
	writePlistString(&b, "StandardErrorPath", logPath)
	b.WriteString("</dict>\n</plist>\n")
	return b.Bytes()
}

func writePlistString(b *bytes.Buffer, key, value string) {
	fmt.Fprintf(b, "\t<key>%s</key>\n\t<string>", key)
	_ = xml.EscapeText(b, []byte(value))
	b.WriteString("</string>\n")
}

func writePlistArrayItem(b *bytes.Buffer, value string) {
	b.WriteString("\t\t<string>")
	_ = xml.EscapeText(b, []byte(value))
	b.WriteString("</string>\n")
}

// serviceLogPath is where the service's standard output and error go until the daemon's own
// logger takes over. Rotation is that logger's job; this file only exists so a crash before
// logging starts is not silently lost.
func serviceLogPath(dataDir string) string {
	return filepath.Join(dataDir, "logs", "daemon", "service.log")
}

// ensureFileExists creates an empty file at path if none exists yet, appending if one already
// does, so an install never truncates a log a previous run already wrote to.
func ensureFileExists(path string, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND, mode)
	if err != nil {
		return fmt.Errorf("create the service log file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("create the service log file: %w", err)
	}
	return nil
}

// Install writes the launch agent plist and starts it.
func (i *launchdInstaller) Install(ctx context.Context, spec ServiceSpec) error {
	env, err := i.env()
	if err != nil {
		return err
	}
	dataDir, err := DataDir(env, spec.Mode)
	if err != nil {
		return err
	}
	logPath := serviceLogPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(logPath), serviceLogDirMode); err != nil {
		return fmt.Errorf("make the service log folder: %w", err)
	}
	if err := ensureFileExists(logPath, serviceLogFileMode); err != nil {
		return err
	}
	plistPath := launchdPlistPath(env.Home, spec.Mode)
	if err := os.MkdirAll(filepath.Dir(plistPath), launchAgentsDirMode); err != nil {
		return fmt.Errorf("make the LaunchAgents folder: %w", err)
	}
	if err := os.WriteFile(plistPath, renderLaunchdPlist(spec, logPath), launchdPlistMode); err != nil {
		return fmt.Errorf("write the launch agent file: %w", err)
	}
	return i.bootstrap(ctx, launchdLabel(spec.Mode), plistPath)
}

// launchctlUnavailableMarker is the text this package looks for in launchctl's own error output
// to decide that "bootstrap" is not a command this OS version has, so it should fall back to the
// older "load -w". Not confirmed against a real pre-bootstrap launchctl: see the package report.
const launchctlUnavailableMarker = "unrecognized subcommand"

// bootstrap starts the agent through the modern "launchctl bootstrap", falling back to the older
// "launchctl load -w" when bootstrap itself is not a recognized subcommand. It first asks launchd
// to drop any copy of the agent already loaded under this label, so installing again over a
// previous install does not fail with "already bootstrapped"; a missing agent for that step is not
// an error.
func (i *launchdInstaller) bootstrap(ctx context.Context, label, plistPath string) error {
	_, _ = i.run(ctx, "launchctl", "bootout", launchdGUIDomain()+"/"+label)
	out, err := i.run(ctx, "launchctl", "bootstrap", launchdGUIDomain(), plistPath)
	if err == nil {
		return nil
	}
	if !strings.Contains(strings.ToLower(string(out)), launchctlUnavailableMarker) {
		return fmt.Errorf("start the launch agent: %s", commandFailure(out, err))
	}
	if out, err := i.run(ctx, "launchctl", "load", "-w", plistPath); err != nil {
		return fmt.Errorf("start the launch agent: %s", commandFailure(out, err))
	}
	return nil
}

// Uninstall stops the agent and removes its plist. Neither step fails if the agent was already
// gone. When the plist is not there at all, bootout is skipped entirely: there is nothing to
// stop, and launchctl's own wording for "nothing to boot out" is not consistent enough across
// macOS versions to match reliably (older releases have been seen to answer "Could not find...",
// newer ones "Boot-out failed: 3: No such process"), so the file's own presence is what decides
// this, not a text match against whichever wording this version happens to use.
func (i *launchdInstaller) Uninstall(ctx context.Context, mode Mode) error {
	env, err := i.env()
	if err != nil {
		return err
	}
	plistPath := launchdPlistPath(env.Home, mode)
	if _, statErr := os.Stat(plistPath); statErr != nil {
		return nil
	}
	label := launchdLabel(mode)
	out, err := i.run(ctx, "launchctl", "bootout", launchdGUIDomain()+"/"+label)
	if err != nil && !launchdBootoutMissingOK(out) {
		return fmt.Errorf("stop the launch agent: %s", commandFailure(out, err))
	}
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove the launch agent file: %w", err)
	}
	return nil
}

// launchdBootoutMissingOK reports whether a failed bootout only means the agent record was
// already gone by the time it ran (for example, unloaded by other means since the plist was
// written), matching the wordings this package has seen from different macOS versions.
func launchdBootoutMissingOK(out []byte) bool {
	lower := strings.ToLower(string(out))
	return strings.Contains(lower, "could not find") || strings.Contains(lower, "no such process")
}

// Status reports whether the agent's plist exists and, if so, asks launchd whether it is loaded
// and running.
func (i *launchdInstaller) Status(ctx context.Context, mode Mode) (ServiceStatus, error) {
	env, err := i.env()
	if err != nil {
		return ServiceStatus{}, err
	}
	plistPath := launchdPlistPath(env.Home, mode)
	if _, statErr := os.Stat(plistPath); statErr != nil {
		return ServiceStatus{}, nil
	}
	out, err := i.run(ctx, "launchctl", "print", launchdGUIDomain()+"/"+launchdLabel(mode))
	if err != nil {
		return ServiceStatus{Installed: true, Detail: "the running status could not be checked"}, nil
	}
	return parseLaunchdPrint(out), nil
}

// parseLaunchdPrint reads the "state = ..." line from "launchctl print"'s output, the field
// documented to say whether the service is running, and treats anything it does not recognize as
// installed with the running state unknown, rather than failing.
func parseLaunchdPrint(out []byte) ServiceStatus {
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		state, ok := strings.CutPrefix(line, "state = ")
		if !ok {
			continue
		}
		return ServiceStatus{Installed: true, Running: state == "running", Detail: "state: " + state}
	}
	return ServiceStatus{Installed: true, Detail: "the running status could not be read"}
}
