package platform

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestLaunchdLabel(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{ModeNormal, "com.marshal.daemon"},
		{ModeDev, "com.marshal.daemon-dev"},
	}
	for _, tc := range tests {
		if got := launchdLabel(tc.mode); got != tc.want {
			t.Errorf("launchdLabel(%v) = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

func TestLaunchdPlistPath(t *testing.T) {
	home := filepath.Join("home", "sam")
	tests := []struct {
		mode Mode
		want string
	}{
		{ModeNormal, filepath.Join(home, "Library", "LaunchAgents", "com.marshal.daemon.plist")},
		{ModeDev, filepath.Join(home, "Library", "LaunchAgents", "com.marshal.daemon-dev.plist")},
	}
	for _, tc := range tests {
		if got := launchdPlistPath(home, tc.mode); got != tc.want {
			t.Errorf("launchdPlistPath(%v) = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

func TestRenderLaunchdPlistNormal(t *testing.T) {
	spec := ServiceSpec{ExecutablePath: "/usr/local/bin/marshald", Mode: ModeNormal}
	got := string(renderLaunchdPlist(spec, "/data/logs/daemon/service.log"))
	want := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.marshal.daemon</string>
	<key>ProgramArguments</key>
	<array>
		<string>/usr/local/bin/marshald</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>StandardOutPath</key>
	<string>/data/logs/daemon/service.log</string>
	<key>StandardErrorPath</key>
	<string>/data/logs/daemon/service.log</string>
</dict>
</plist>
`
	if got != want {
		t.Errorf("renderLaunchdPlist(normal) =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderLaunchdPlistDevWithArgsAndEscaping(t *testing.T) {
	spec := ServiceSpec{ExecutablePath: "/usr/local/bin/marshald", Args: []string{"--dev", "--data-dir", "a & b"}, Mode: ModeDev}
	got := string(renderLaunchdPlist(spec, "/data-dev/logs/daemon/service.log"))
	want := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.marshal.daemon-dev</string>
	<key>ProgramArguments</key>
	<array>
		<string>/usr/local/bin/marshald</string>
		<string>--dev</string>
		<string>--data-dir</string>
		<string>a &amp; b</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>StandardOutPath</key>
	<string>/data-dev/logs/daemon/service.log</string>
	<key>StandardErrorPath</key>
	<string>/data-dev/logs/daemon/service.log</string>
</dict>
</plist>
`
	if got != want {
		t.Errorf("renderLaunchdPlist(dev) =\n%s\nwant\n%s", got, want)
	}
}

// fakeRunner records every call it receives and answers from a canned script, keyed by the
// joined command line, so a test can assert the exact commands the installer runs without ever
// starting a real program.
type fakeRunner struct {
	calls   [][]string
	answers map[string]fakeAnswer
}

type fakeAnswer struct {
	out []byte
	err error
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	f.calls = append(f.calls, call)
	a := f.answers[strings.Join(call, " ")]
	return a.out, a.err
}

func TestLaunchdInstallerInstallWritesAndBootstraps(t *testing.T) {
	home := t.TempDir()
	runner := &fakeRunner{}
	inst := newLaunchdInstaller(runner.run, func() (Env, error) {
		return Env{GOOS: "darwin", Home: home}, nil
	})
	spec := ServiceSpec{ExecutablePath: "/usr/local/bin/marshald", Args: []string{"--dev"}, Mode: ModeDev}

	if err := inst.Install(context.Background(), spec); err != nil {
		t.Fatalf("Install: %v", err)
	}

	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.marshal.daemon-dev.plist")
	if _, err := readFile(t, plistPath); err != nil {
		t.Fatalf("the plist was not written: %v", err)
	}
	logPath := filepath.Join(home, "Library", "Application Support", "Marshal-dev", "logs", "daemon", "service.log")
	if _, err := readFile(t, logPath); err != nil {
		t.Fatalf("the log placeholder was not created: %v", err)
	}
	wantCalls := [][]string{
		{"launchctl", "bootout", "gui/" + uidString() + "/com.marshal.daemon-dev"},
		{"launchctl", "bootstrap", "gui/" + uidString(), plistPath},
	}
	assertCalls(t, runner.calls, wantCalls)
}

func TestLaunchdInstallerInstallFallsBackWhenBootstrapIsUnavailable(t *testing.T) {
	home := t.TempDir()
	label := "com.marshal.daemon"
	plistPath := launchdPlistPath(home, ModeNormal)
	runner := &fakeRunner{answers: map[string]fakeAnswer{
		"launchctl bootstrap gui/" + uidString() + " " + plistPath: {
			out: []byte("launchctl: unrecognized subcommand bootstrap"),
			err: errors.New("exit status 64"),
		},
	}}
	inst := newLaunchdInstaller(runner.run, func() (Env, error) { return Env{GOOS: "darwin", Home: home}, nil })

	if err := inst.Install(context.Background(), ServiceSpec{ExecutablePath: "/usr/local/bin/marshald", Mode: ModeNormal}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	wantCalls := [][]string{
		{"launchctl", "bootout", "gui/" + uidString() + "/" + label},
		{"launchctl", "bootstrap", "gui/" + uidString(), plistPath},
		{"launchctl", "load", "-w", plistPath},
	}
	assertCalls(t, runner.calls, wantCalls)
}

func TestLaunchdInstallerInstallReturnsAPlainErrorWhenBootstrapFails(t *testing.T) {
	home := t.TempDir()
	runner := &fakeRunner{answers: map[string]fakeAnswer{
		"launchctl bootstrap gui/" + uidString() + " " + launchdPlistPath(home, ModeNormal): {
			out: []byte("Bootstrap failed: 5: Input/output error"),
			err: errors.New("exit status 5"),
		},
	}}
	inst := newLaunchdInstaller(runner.run, func() (Env, error) { return Env{GOOS: "darwin", Home: home}, nil })

	err := inst.Install(context.Background(), ServiceSpec{ExecutablePath: "/usr/local/bin/marshald", Mode: ModeNormal})
	if err == nil || !strings.Contains(err.Error(), "start the launch agent") {
		t.Fatalf("Install error = %v, want it to mention starting the launch agent", err)
	}
}

func TestLaunchdInstallerUninstallWhenNothingWasInstalled(t *testing.T) {
	home := t.TempDir()
	runner := &fakeRunner{}
	inst := newLaunchdInstaller(runner.run, func() (Env, error) { return Env{GOOS: "darwin", Home: home}, nil })

	if err := inst.Uninstall(context.Background(), ModeNormal); err != nil {
		t.Fatalf("Uninstall with no plist on disk = %v, want nil", err)
	}
	if len(runner.calls) != 0 {
		t.Errorf("launchctl was called although there was nothing to uninstall: %v", runner.calls)
	}
}

func TestLaunchdInstallerUninstallToleratesAnAlreadyUnloadedAgent(t *testing.T) {
	tests := []struct {
		name string
		out  string
	}{
		{"older wording", `Could not find service "com.marshal.daemon" in domain for port`},
		{"newer wording", "Boot-out failed: 3: No such process"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			plistPath := launchdPlistPath(home, ModeNormal)
			writeFile(t, plistPath, "placeholder")
			runner := &fakeRunner{answers: map[string]fakeAnswer{
				"launchctl bootout gui/" + uidString() + "/com.marshal.daemon": {
					out: []byte(tc.out),
					err: errors.New("exit status 1"),
				},
			}}
			inst := newLaunchdInstaller(runner.run, func() (Env, error) { return Env{GOOS: "darwin", Home: home}, nil })

			if err := inst.Uninstall(context.Background(), ModeNormal); err != nil {
				t.Fatalf("Uninstall of an already-unloaded agent = %v, want nil", err)
			}
			if exists(plistPath) {
				t.Error("the plist file still exists after Uninstall")
			}
		})
	}
}

func TestLaunchdInstallerUninstallRemovesThePlist(t *testing.T) {
	home := t.TempDir()
	plistPath := launchdPlistPath(home, ModeNormal)
	writeFile(t, plistPath, "placeholder")
	runner := &fakeRunner{}
	inst := newLaunchdInstaller(runner.run, func() (Env, error) { return Env{GOOS: "darwin", Home: home}, nil })

	if err := inst.Uninstall(context.Background(), ModeNormal); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if exists(plistPath) {
		t.Error("the plist file still exists after Uninstall")
	}
}

func TestLaunchdInstallerStatusNotInstalled(t *testing.T) {
	home := t.TempDir()
	inst := newLaunchdInstaller(func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("launchctl was called for a plist that does not exist")
		return nil, nil
	}, func() (Env, error) { return Env{GOOS: "darwin", Home: home}, nil })

	got, err := inst.Status(context.Background(), ModeNormal)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got.Installed {
		t.Errorf("Status = %+v, want Installed false", got)
	}
}

func TestLaunchdInstallerStatusRunning(t *testing.T) {
	home := t.TempDir()
	writeFile(t, launchdPlistPath(home, ModeNormal), "placeholder")
	// A shape close to real launchctl print output: a header line, then indented "key = value"
	// pairs, one of them the documented "state" field.
	printOutput := "gui/501/com.marshal.daemon = {\n\tactive count = 1\n\tstate = running\n\tpid = 4242\n}\n"
	runner := &fakeRunner{answers: map[string]fakeAnswer{
		"launchctl print gui/" + uidString() + "/com.marshal.daemon": {out: []byte(printOutput)},
	}}
	inst := newLaunchdInstaller(runner.run, func() (Env, error) { return Env{GOOS: "darwin", Home: home}, nil })

	got, err := inst.Status(context.Background(), ModeNormal)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !got.Installed || !got.Running {
		t.Errorf("Status = %+v, want Installed and Running true", got)
	}
}

func TestLaunchdInstallerStatusNotRunning(t *testing.T) {
	home := t.TempDir()
	writeFile(t, launchdPlistPath(home, ModeNormal), "placeholder")
	printOutput := "gui/501/com.marshal.daemon = {\n\tstate = not running\n}\n"
	runner := &fakeRunner{answers: map[string]fakeAnswer{
		"launchctl print gui/" + uidString() + "/com.marshal.daemon": {out: []byte(printOutput)},
	}}
	inst := newLaunchdInstaller(runner.run, func() (Env, error) { return Env{GOOS: "darwin", Home: home}, nil })

	got, err := inst.Status(context.Background(), ModeNormal)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !got.Installed || got.Running {
		t.Errorf("Status = %+v, want Installed true and Running false", got)
	}
}

func TestLaunchdInstallerStatusUnparsableAnswerIsNotAnError(t *testing.T) {
	home := t.TempDir()
	writeFile(t, launchdPlistPath(home, ModeNormal), "placeholder")
	runner := &fakeRunner{answers: map[string]fakeAnswer{
		"launchctl print gui/" + uidString() + "/com.marshal.daemon": {
			out: []byte("some unexpected format"),
			err: errors.New("exit status 1"),
		},
	}}
	inst := newLaunchdInstaller(runner.run, func() (Env, error) { return Env{GOOS: "darwin", Home: home}, nil })

	got, err := inst.Status(context.Background(), ModeNormal)
	if err != nil {
		t.Fatalf("Status returned an error instead of an unknown-running status: %v", err)
	}
	if !got.Installed || got.Running || got.Detail == "" {
		t.Errorf("Status = %+v, want Installed true, Running false, and a Detail", got)
	}
}

func TestParseLaunchdPrintNoStateLine(t *testing.T) {
	got := parseLaunchdPrint([]byte("gui/501/com.marshal.daemon = {\n\tactive count = 1\n}\n"))
	if !got.Installed || got.Running || got.Detail == "" {
		t.Errorf("parseLaunchdPrint = %+v, want Installed true, Running false, and a Detail", got)
	}
}
