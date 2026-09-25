//go:build windows

package platform

// NewServiceInstaller returns the schtasks-based installer: Windows runs the daemon as a per-user
// scheduled task that starts at logon, with no admin elevation.
func NewServiceInstaller() ServiceInstaller {
	return newSchtasksInstaller(runServiceCommand)
}
