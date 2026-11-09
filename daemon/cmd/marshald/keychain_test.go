package main

import (
	"errors"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/platform"
	"github.com/khanblair/marshal/daemon/internal/security"
)

func TestDaemonKeychainIsolatesOnlyDevStubFixtures(t *testing.T) {
	cases := []struct {
		name     string
		settings config.Settings
		memory   bool
	}{
		{"fixture stub", config.Settings{Mode: platform.ModeDev, Agent: config.AgentStub, Fixture: "prototype"}, true},
		{"normal", config.Settings{Mode: platform.ModeNormal, Agent: config.AgentReal}, false},
		{"normal cannot bypass", config.Settings{Mode: platform.ModeNormal, Agent: config.AgentStub, Fixture: "prototype"}, false},
		{"real dev fixture", config.Settings{Mode: platform.ModeDev, Agent: config.AgentReal, Fixture: "prototype"}, false},
		{"dev without fixture", config.Settings{Mode: platform.ModeDev, Agent: config.AgentStub}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			keychain := daemonKeychain(c.settings)
			_, memory := keychain.(*security.MemoryKeychain)
			if memory != c.memory {
				t.Fatalf("keychain %T, memory wanted %v", keychain, c.memory)
			}
			if !c.memory {
				if _, ok := keychain.(*security.OSKeychain); !ok {
					t.Fatalf("unexpected real keychain %T", keychain)
				}
			}
		})
	}
}

func TestDaemonKeychainFixtureStartsEmptyAndDoesNotPersistSecrets(t *testing.T) {
	settings := config.Settings{Mode: platform.ModeDev, Agent: config.AgentStub, Fixture: "prototype"}
	first := daemonKeychain(settings)
	if _, err := first.Get("anthropic"); !errors.Is(err, security.ErrNoKey) {
		t.Fatalf("fixture read: %v", err)
	}
	if err := first.Set("anthropic", "fixture-value"); err != nil {
		t.Fatal(err)
	}
	if secret, err := first.Get("anthropic"); err != nil || secret != "fixture-value" {
		t.Fatalf("fixture save: %v", err)
	}
	if _, err := daemonKeychain(settings).Get("anthropic"); !errors.Is(err, security.ErrNoKey) {
		t.Fatalf("new fixture retained credentials: %v", err)
	}
}
