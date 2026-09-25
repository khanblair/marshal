//go:build windows

package platform

import "testing"

// TestNewServiceInstallerIsSchtasks only checks wiring: it must never call any method on the
// result, since that would run a real schtasks command on the machine running the test.
func TestNewServiceInstallerIsSchtasks(t *testing.T) {
	got := NewServiceInstaller()
	inst, ok := got.(*schtasksInstaller)
	if !ok {
		t.Fatalf("NewServiceInstaller() = %T, want *schtasksInstaller", got)
	}
	if inst.run == nil {
		t.Error("the installer has no command runner")
	}
}
