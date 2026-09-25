package platform

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemdUnitName(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{ModeNormal, "marshald.service"},
		{ModeDev, "marshald-dev.service"},
	}
	for _, tc := range tests {
		if got := systemdUnitName(tc.mode); got != tc.want {
			t.Errorf("systemdUnitName(%v) = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

func TestSystemdUnitPath(t *testing.T) {
	home := filepath.Join("home", "sam")
	want := filepath.Join(home, ".config", "systemd", "user", "marshald-dev.service")
	if got := systemdUnitPath(home, ModeDev); got != want {
		t.Errorf("systemdUnitPath = %q, want %q", got, want)
	}
}

func TestSystemdQuoteArg(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "--dev", "--dev"},
		{"path with no spaces", "/usr/local/bin/marshald", "/usr/local/bin/marshald"},
		{"needs quoting for a space", "a b", `"a b"`},
		{"escapes an inner quote", `a"b`, `"a\"b"`},
		{"escapes a backslash", `a\b`, `"a\\b"`},
		{"empty string is quoted", "", `""`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := systemdQuoteArg(tc.in); got != tc.want {
				t.Errorf("systemdQuoteArg(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRenderSystemdUnitNormal(t *testing.T) {
	spec := ServiceSpec{ExecutablePath: "/usr/local/bin/marshald", Mode: ModeNormal}
	got := string(renderSystemdUnit(spec))
	want := "[Unit]\n" +
		"Description=Marshal daemon\n" +
		"\n[Service]\n" +
		"ExecStart=/usr/local/bin/marshald\n" +
		"Restart=on-failure\n" +
		"\n[Install]\n" +
		"WantedBy=default.target\n"
	if got != want {
		t.Errorf("renderSystemdUnit(normal) =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderSystemdUnitDevWithArgs(t *testing.T) {
	spec := ServiceSpec{ExecutablePath: "/usr/local/bin/marshald", Args: []string{"--dev", "--data-dir", "a b"}, Mode: ModeDev}
	got := string(renderSystemdUnit(spec))
	want := "[Unit]\n" +
		"Description=Marshal daemon\n" +
		"\n[Service]\n" +
		`ExecStart=/usr/local/bin/marshald --dev --data-dir "a b"` + "\n" +
		"Restart=on-failure\n" +
		"\n[Install]\n" +
		"WantedBy=default.target\n"
	if got != want {
		t.Errorf("renderSystemdUnit(dev) =\n%s\nwant\n%s", got, want)
	}
}

func TestSystemdInstallerInstallWritesAndEnables(t *testing.T) {
	home := t.TempDir()
	runner := &fakeRunner{}
	inst := newSystemdInstaller(runner.run, func() (Env, error) { return Env{GOOS: "linux", Home: home}, nil })
	spec := ServiceSpec{ExecutablePath: "/usr/local/bin/marshald", Args: []string{"--dev"}, Mode: ModeDev}

	if err := inst.Install(context.Background(), spec); err != nil {
		t.Fatalf("Install: %v", err)
	}

	unitPath := filepath.Join(home, ".config", "systemd", "user", "marshald-dev.service")
	if _, err := readFile(t, unitPath); err != nil {
		t.Fatalf("the unit file was not written: %v", err)
	}
	wantCalls := [][]string{
		{"systemctl", "--user", "daemon-reload"},
		{"systemctl", "--user", "enable", "--now", "marshald-dev.service"},
		{"loginctl", "enable-linger"},
	}
	assertCalls(t, runner.calls, wantCalls)
}

func TestSystemdInstallerInstallFailsPlainlyWithNoSystemdSession(t *testing.T) {
	home := t.TempDir()
	runner := &fakeRunner{answers: map[string]fakeAnswer{
		"systemctl --user daemon-reload": {
			out: []byte("Failed to connect to bus: No such file or directory"),
			err: errors.New("exit status 1"),
		},
	}}
	inst := newSystemdInstaller(runner.run, func() (Env, error) { return Env{GOOS: "linux", Home: home}, nil })

	err := inst.Install(context.Background(), ServiceSpec{ExecutablePath: "/usr/local/bin/marshald", Mode: ModeNormal})
	if err == nil {
		t.Fatal("Install on a machine with no systemd user session = nil, want an error")
	}
	if got := err.Error(); !strings.Contains(got, "no systemd user session") {
		t.Errorf("Install error = %q, want a plain sentence about the systemd user session", got)
	}
	// enable and loginctl must never run once daemon-reload itself has failed.
	if len(runner.calls) != 1 {
		t.Errorf("ran %d commands after the failure, want 1: %v", len(runner.calls), runner.calls)
	}
}

func TestSystemdInstallerInstallReportsButDoesNotFailOnAFailingLingerCall(t *testing.T) {
	home := t.TempDir()
	runner := &fakeRunner{answers: map[string]fakeAnswer{
		"loginctl enable-linger": {out: []byte("Failed to enable linger: Access denied"), err: errors.New("exit status 1")},
	}}
	inst := newSystemdInstaller(runner.run, func() (Env, error) { return Env{GOOS: "linux", Home: home}, nil })

	err := inst.Install(context.Background(), ServiceSpec{ExecutablePath: "/usr/local/bin/marshald", Mode: ModeNormal})
	if !errors.Is(err, ErrLingerNotEnabled) {
		t.Fatalf("Install with a failing linger call = %v, want an error wrapping ErrLingerNotEnabled", err)
	}
	// The unit was still enabled and started: daemon-reload and enable --now must have run before
	// the linger call, and the unit file must exist.
	wantCalls := [][]string{
		{"systemctl", "--user", "daemon-reload"},
		{"systemctl", "--user", "enable", "--now", "marshald.service"},
		{"loginctl", "enable-linger"},
	}
	assertCalls(t, runner.calls, wantCalls)
	if _, err := readFile(t, systemdUnitPath(home, ModeNormal)); err != nil {
		t.Errorf("the unit file was not written: %v", err)
	}
}

func TestSystemdInstallerUninstallToleratesAMissingUnit(t *testing.T) {
	home := t.TempDir()
	runner := &fakeRunner{answers: map[string]fakeAnswer{
		"systemctl --user disable --now marshald.service": {
			out: []byte("Failed to disable unit: Unit file marshald.service does not exist."),
			err: errors.New("exit status 1"),
		},
	}}
	inst := newSystemdInstaller(runner.run, func() (Env, error) { return Env{GOOS: "linux", Home: home}, nil })

	if err := inst.Uninstall(context.Background(), ModeNormal); err != nil {
		t.Fatalf("Uninstall of a missing unit = %v, want nil", err)
	}
}

func TestSystemdInstallerUninstallRemovesTheUnitFile(t *testing.T) {
	home := t.TempDir()
	unitPath := systemdUnitPath(home, ModeNormal)
	writeFile(t, unitPath, "placeholder")
	runner := &fakeRunner{}
	inst := newSystemdInstaller(runner.run, func() (Env, error) { return Env{GOOS: "linux", Home: home}, nil })

	if err := inst.Uninstall(context.Background(), ModeNormal); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if exists(unitPath) {
		t.Error("the unit file still exists after Uninstall")
	}
	wantCalls := [][]string{
		{"systemctl", "--user", "disable", "--now", "marshald.service"},
		{"systemctl", "--user", "daemon-reload"},
	}
	assertCalls(t, runner.calls, wantCalls)
}

func TestSystemdInstallerStatusNotFound(t *testing.T) {
	// No unit file on disk: Status must decide "not installed" from that alone, never from
	// parsing is-enabled or is-active, and never call either.
	home := t.TempDir()
	runner := &fakeRunner{}
	inst := newSystemdInstaller(runner.run, func() (Env, error) { return Env{GOOS: "linux", Home: home}, nil })

	got, err := inst.Status(context.Background(), ModeNormal)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got.Installed {
		t.Errorf("Status = %+v, want Installed false", got)
	}
	if len(runner.calls) != 0 {
		t.Errorf("systemctl was called although the unit file does not exist: %v", runner.calls)
	}
}

func TestSystemdInstallerStatusActive(t *testing.T) {
	home := t.TempDir()
	writeFile(t, systemdUnitPath(home, ModeNormal), "placeholder")
	runner := &fakeRunner{answers: map[string]fakeAnswer{
		"systemctl --user is-enabled marshald.service": {out: []byte("enabled\n")},
		"systemctl --user is-active marshald.service":  {out: []byte("active\n")},
	}}
	inst := newSystemdInstaller(runner.run, func() (Env, error) { return Env{GOOS: "linux", Home: home}, nil })

	got, err := inst.Status(context.Background(), ModeNormal)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !got.Installed || !got.Running {
		t.Errorf("Status = %+v, want Installed and Running true", got)
	}
}

func TestSystemdInstallerStatusEnabledButInactive(t *testing.T) {
	home := t.TempDir()
	writeFile(t, systemdUnitPath(home, ModeNormal), "placeholder")
	runner := &fakeRunner{answers: map[string]fakeAnswer{
		"systemctl --user is-enabled marshald.service": {out: []byte("enabled\n")},
		"systemctl --user is-active marshald.service":  {out: []byte("inactive\n"), err: errors.New("exit status 3")},
	}}
	inst := newSystemdInstaller(runner.run, func() (Env, error) { return Env{GOOS: "linux", Home: home}, nil })

	got, err := inst.Status(context.Background(), ModeNormal)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !got.Installed || got.Running {
		t.Errorf("Status = %+v, want Installed true and Running false", got)
	}
}
