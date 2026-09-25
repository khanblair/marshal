//go:build linux

package platform

import "testing"

// TestNewServiceInstallerIsSystemd only checks wiring: it must never call any method on the
// result, since that would run a real systemctl command on the machine running the test.
func TestNewServiceInstallerIsSystemd(t *testing.T) {
	got := NewServiceInstaller()
	inst, ok := got.(*systemdInstaller)
	if !ok {
		t.Fatalf("NewServiceInstaller() = %T, want *systemdInstaller", got)
	}
	if inst.run == nil {
		t.Error("the installer has no command runner")
	}
	if inst.env == nil {
		t.Error("the installer has no environment reader")
	}
}
