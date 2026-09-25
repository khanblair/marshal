//go:build darwin

package platform

// NewServiceInstaller returns the launchd-based installer: macOS runs the daemon as a per-user
// launch agent.
func NewServiceInstaller() ServiceInstaller {
	return newLaunchdInstaller(runServiceCommand, CurrentEnv)
}
