//go:build darwin

package platform

import "testing"

// TestNewServiceInstallerIsLaunchd only checks wiring: it must never call any method on the
// result, since that would run a real launchctl command on the machine running the test.
func TestNewServiceInstallerIsLaunchd(t *testing.T) {
	got := NewServiceInstaller()
	inst, ok := got.(*launchdInstaller)
	if !ok {
		t.Fatalf("NewServiceInstaller() = %T, want *launchdInstaller", got)
	}
	if inst.run == nil {
		t.Error("the installer has no command runner")
	}
	if inst.env == nil {
		t.Error("the installer has no environment reader")
	}
}
