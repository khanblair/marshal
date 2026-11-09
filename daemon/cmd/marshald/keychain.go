package main

import (
	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/platform"
	"github.com/khanblair/marshal/daemon/internal/security"
)

// Fixture stub daemons must not require a desktop secret service or touch the owner's keys.
// Every other mode retains the real OS keychain and its error behavior.
func daemonKeychain(settings config.Settings) security.Keychain {
	if settings.Dev() && settings.Agent == config.AgentStub && settings.Fixture != "" {
		return security.NewMemoryKeychain()
	}
	return security.NewOSKeychain(platform.AppName(settings.Mode))
}
