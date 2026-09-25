package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/platform"
)

// fakeInstaller is a platform.ServiceInstaller that never runs a real command. Every test in this
// file uses one instead of platform.NewServiceInstaller, since the real one would register or
// query a login item on the machine running the test.
type fakeInstaller struct {
	installErr   error
	uninstallErr error
	status       platform.ServiceStatus
	statusErr    error

	installCalled   bool
	installSpec     platform.ServiceSpec
	uninstallCalled bool
	uninstallMode   platform.Mode
	statusCalled    bool
	statusMode      platform.Mode
}

func (f *fakeInstaller) Install(_ context.Context, spec platform.ServiceSpec) error {
	f.installCalled = true
	f.installSpec = spec
	return f.installErr
}

func (f *fakeInstaller) Uninstall(_ context.Context, mode platform.Mode) error {
	f.uninstallCalled = true
	f.uninstallMode = mode
	return f.uninstallErr
}

func (f *fakeInstaller) Status(_ context.Context, mode platform.Mode) (platform.ServiceStatus, error) {
	f.statusCalled = true
	f.statusMode = mode
	return f.status, f.statusErr
}

func runServiceCLI(t *testing.T, inst *fakeInstaller, selfExecutable func() (string, error), args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	newInstaller := func() platform.ServiceInstaller { return inst }
	code = runService(args, terminal{stdout: &out, stderr: &errOut}, newInstaller, selfExecutable)
	return code, out.String(), errOut.String()
}

// realDir resolves a temp folder's own symlinks, the way daemonExecutablePath resolves the
// program path built from it. On macOS, t.TempDir() sits under /var, itself a symlink to
// /private/var, so a path built from the raw temp dir and one built from a resolved program path
// can otherwise look unequal despite being the same folder.
func realDir(t *testing.T, dir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// daemondSibling makes a temp folder with a fake "marshal" program and a fake "marshald" beside
// it, and returns marshal's path, so tests can give runServiceInstall a selfExecutable that
// behaves like a real install: marshald sitting right next to marshal.
func daemondSibling(t *testing.T) (selfPath string) {
	t.Helper()
	dir := realDir(t, t.TempDir())
	name := "marshald"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	self := filepath.Join(dir, "marshal")
	if err := os.WriteFile(self, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return self
}

func TestServiceWithNoSubcommandShowsUsage(t *testing.T) {
	inst := &fakeInstaller{}
	code, _, errOut := runServiceCLI(t, inst, os.Executable)
	if code != exitBadInput || !strings.Contains(errOut, "Usage: marshal service") {
		t.Errorf("service (no subcommand) = %d %q", code, errOut)
	}
}

func TestServiceUnknownSubcommand(t *testing.T) {
	inst := &fakeInstaller{}
	code, _, errOut := runServiceCLI(t, inst, os.Executable, "explode")
	if code != exitBadInput || !strings.Contains(errOut, "Unknown service command") {
		t.Errorf("service explode = %d %q", code, errOut)
	}
}

func TestServiceInstallInstallsMarshaldNextToMarshal(t *testing.T) {
	self := daemondSibling(t)
	inst := &fakeInstaller{}
	code, out, errOut := runServiceCLI(t, inst, func() (string, error) { return self, nil }, "install")
	if code != exitOK {
		t.Fatalf("service install = %d %q %q", code, out, errOut)
	}
	if !inst.installCalled {
		t.Fatal("Install was not called")
	}
	wantExe := filepath.Join(filepath.Dir(self), "marshald")
	if inst.installSpec.ExecutablePath != wantExe {
		t.Errorf("ExecutablePath = %q, want %q", inst.installSpec.ExecutablePath, wantExe)
	}
	if inst.installSpec.Mode != platform.ModeNormal || len(inst.installSpec.Args) != 0 {
		t.Errorf("spec = %+v, want normal mode and no arguments", inst.installSpec)
	}
	if !strings.Contains(out, "Installed") {
		t.Errorf("stdout = %q, want a confirmation", out)
	}
}

func TestServiceInstallDevPassesTheDevFlagAndMode(t *testing.T) {
	self := daemondSibling(t)
	inst := &fakeInstaller{}
	code, _, errOut := runServiceCLI(t, inst, func() (string, error) { return self, nil }, "install", "--dev")
	if code != exitOK {
		t.Fatalf("service install --dev = %d %q", code, errOut)
	}
	if inst.installSpec.Mode != platform.ModeDev {
		t.Errorf("Mode = %v, want dev", inst.installSpec.Mode)
	}
	if len(inst.installSpec.Args) != 1 || inst.installSpec.Args[0] != "--dev" {
		t.Errorf("Args = %v, want [--dev]", inst.installSpec.Args)
	}
}

func TestServiceInstallFailsWhenMarshaldIsMissing(t *testing.T) {
	self := filepath.Join(t.TempDir(), "marshal")
	if err := os.WriteFile(self, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	inst := &fakeInstaller{}
	code, _, errOut := runServiceCLI(t, inst, func() (string, error) { return self, nil }, "install")
	if code != exitFailed || !strings.Contains(errOut, "marshald was not found") {
		t.Errorf("service install with no marshald = %d %q", code, errOut)
	}
	if inst.installCalled {
		t.Error("Install was called although marshald could not be found")
	}
}

func TestServiceInstallFailsWhenSelfExecutableErrors(t *testing.T) {
	inst := &fakeInstaller{}
	code, _, errOut := runServiceCLI(t, inst, func() (string, error) { return "", errors.New("no such process") }, "install")
	if code != exitFailed || !strings.Contains(errOut, "own program location") {
		t.Errorf("service install with a failing selfExecutable = %d %q", code, errOut)
	}
	if inst.installCalled {
		t.Error("Install was called although selfExecutable failed")
	}
}

func TestServiceInstallReportsTheInstallerError(t *testing.T) {
	self := daemondSibling(t)
	inst := &fakeInstaller{installErr: errors.New("marshal is already running for this user")}
	code, _, errOut := runServiceCLI(t, inst, func() (string, error) { return self, nil }, "install")
	want := "The service could not be installed: marshal is already running for this user.\n"
	if code != exitFailed || errOut != want {
		t.Errorf("service install with a failing installer = %d %q, want %d %q", code, errOut, exitFailed, want)
	}
}

// TestServiceInstallWarnsButSucceedsWhenLingerFails covers the one case Install can fail on
// without failing the whole command: lingering could not be enabled. The service is already
// installed and running, so this is a warning on stderr, exit 0, and the usual confirmation.
func TestServiceInstallWarnsButSucceedsWhenLingerFails(t *testing.T) {
	self := daemondSibling(t)
	inst := &fakeInstaller{installErr: fmt.Errorf("%w: access denied", platform.ErrLingerNotEnabled)}
	code, out, errOut := runServiceCLI(t, inst, func() (string, error) { return self, nil }, "install")
	if code != exitOK {
		t.Fatalf("service install with a failing linger call = %d %q %q", code, out, errOut)
	}
	want := "Warning: the service is installed and running, but could not enable lingering for this account: access denied.\n"
	if errOut != want {
		t.Errorf("stderr = %q, want %q", errOut, want)
	}
	if !strings.Contains(out, "Installed") {
		t.Errorf("stdout = %q, want the usual confirmation", out)
	}
}

func TestServiceUninstallCallsUninstallWithTheRightMode(t *testing.T) {
	tests := []struct {
		args []string
		want platform.Mode
	}{
		{nil, platform.ModeNormal},
		{[]string{"--dev"}, platform.ModeDev},
	}
	for _, tc := range tests {
		inst := &fakeInstaller{}
		args := append([]string{"uninstall"}, tc.args...)
		code, out, errOut := runServiceCLI(t, inst, os.Executable, args...)
		if code != exitOK {
			t.Fatalf("service uninstall %v = %d %q %q", tc.args, code, out, errOut)
		}
		if !inst.uninstallCalled || inst.uninstallMode != tc.want {
			t.Errorf("Uninstall mode = %v (called %v), want %v", inst.uninstallMode, inst.uninstallCalled, tc.want)
		}
		if !strings.Contains(out, "Removed") {
			t.Errorf("stdout = %q, want a confirmation", out)
		}
	}
}

func TestServiceUninstallReportsTheInstallerError(t *testing.T) {
	inst := &fakeInstaller{uninstallErr: errors.New("boom")}
	code, _, errOut := runServiceCLI(t, inst, os.Executable, "uninstall")
	want := "The service could not be removed: boom.\n"
	if code != exitFailed || errOut != want {
		t.Errorf("service uninstall with a failing installer = %d %q, want %d %q", code, errOut, exitFailed, want)
	}
}

func TestServiceStatusMessages(t *testing.T) {
	tests := []struct {
		name   string
		status platform.ServiceStatus
		want   string
	}{
		{"not installed", platform.ServiceStatus{}, "not installed"},
		{"running", platform.ServiceStatus{Installed: true, Running: true}, "installed and running"},
		{"stopped", platform.ServiceStatus{Installed: true, Running: false}, "installed but not running"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inst := &fakeInstaller{status: tc.status}
			code, out, errOut := runServiceCLI(t, inst, os.Executable, "status")
			if code != exitOK {
				t.Fatalf("service status = %d %q %q", code, out, errOut)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("stdout = %q, want it to mention %q", out, tc.want)
			}
		})
	}
}

func TestServiceStatusDevPassesDevMode(t *testing.T) {
	inst := &fakeInstaller{}
	if _, _, _ = runServiceCLI(t, inst, os.Executable, "status", "--dev"); inst.statusMode != platform.ModeDev {
		t.Errorf("Status mode = %v, want dev", inst.statusMode)
	}
}

func TestServiceStatusReportsTheInstallerError(t *testing.T) {
	inst := &fakeInstaller{statusErr: errors.New("boom")}
	code, _, errOut := runServiceCLI(t, inst, os.Executable, "status")
	want := "The service status could not be checked: boom.\n"
	if code != exitFailed || errOut != want {
		t.Errorf("service status with a failing installer = %d %q, want %d %q", code, errOut, exitFailed, want)
	}
}

func TestDaemonExecutablePathResolvesASymlink(t *testing.T) {
	dir := realDir(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(dir, "marshald"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "marshal")
	if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "marshal-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("this machine cannot make symbolic links: %v", err)
	}
	got, err := daemonExecutablePath(link)
	if err != nil {
		t.Fatalf("daemonExecutablePath: %v", err)
	}
	want := filepath.Join(dir, "marshald")
	if got != want {
		t.Errorf("daemonExecutablePath(%q) = %q, want %q", link, got, want)
	}
}

func TestDaemonExecutablePathFailsOnAMissingSelf(t *testing.T) {
	if _, err := daemonExecutablePath(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Error("daemonExecutablePath on a missing self path = nil, want an error")
	}
}
