//go:build linux

package platform

// NewServiceInstaller returns the systemd-based installer: Linux runs the daemon as a per-user
// systemd unit.
func NewServiceInstaller() ServiceInstaller {
	return newSystemdInstaller(runServiceCommand, CurrentEnv)
}
