package security

// This file owns the one place Marshal keeps the secrets a person typed: today, the model provider
// API keys (docs/backend-checklist.md section 3, "Keys go into the keychain with the `marshal keys`
// command"). It is a real OS keychain — macOS Keychain, the Linux Secret Service, or the Windows
// Credential Manager — through `github.com/zalando/go-keyring`, never a file. The platform package's
// token file (platform/tokens.go) is a different thing for a different reason: that stores a device
// token Marshal itself minted, in a 0600 file, and cannot hold a provider key.
//
// Two shapes live here on purpose. `Keychain` is the seam: the daemon, the CLI, and every test talk
// to this interface, so a test never prompts a person for keychain access and never reads or writes
// the real one. `OSKeychain` is the only implementation that reaches the real keychain, and it holds
// go-keyring behind `keyringAPI` so its own error mapping is tested against a scripted backend
// rather than the machine's keychain.

import (
	"errors"
	"fmt"
	"sync"

	"github.com/zalando/go-keyring"
)

// ErrNoKey means no secret is stored for that provider. It is the expected answer for a provider
// nobody has saved a key for, so a caller asks with errors.Is rather than treating it as a failure.
var ErrNoKey = errors.New("no key is stored")

// Keychain stores one secret per provider id. Set replaces whatever was there, so saving a key
// twice leaves one entry rather than two, and it is what a person does when a key is rotated.
//
// The provider id is the same id the provider registry and the screens use ("anthropic",
// "openai", "ollama"). A provider with no key has no entry: `Get` answers ErrNoKey. There is no
// listing method because the OS keychains do not offer one — and do not need to: the set of
// providers Marshal knows is a fixed, known list, so "which keys are saved" is asked by trying each
// known id.
type Keychain interface {
	// Set stores secret under provider, replacing any secret already there.
	Set(provider, secret string) error
	// Get returns the secret stored for provider, or ErrNoKey when none is.
	Get(provider string) (string, error)
	// Remove deletes the secret stored for provider. Removing a provider with no secret answers
	// ErrNoKey, so a caller can tell "removed" from "there was nothing to remove".
	Remove(provider string) error
}

// keyringAPI is the part of go-keyring Marshal uses. Naming it separately lets OSKeychain be tested
// with a backend that answers with whatever the test needs — including go-keyring's ErrNotFound —
// without touching the machine's keychain.
type keyringAPI interface {
	Set(service, user, password string) error
	Get(service, user string) (string, error)
	Delete(service, user string) error
}

// osKeyring is the real backend: go-keyring's package functions, which pick the right
// implementation for the operating system the daemon was built for.
type osKeyring struct{}

func (osKeyring) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}

func (osKeyring) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}

func (osKeyring) Delete(service, user string) error {
	return keyring.Delete(service, user)
}

// OSKeychain is the real keychain. service is the name Marshal files its secrets under, which is
// platform.AppName(mode) so a dev daemon and a normal install keep their keys apart.
type OSKeychain struct {
	service string
	api     keyringAPI
}

// NewOSKeychain returns a Keychain backed by this machine's OS keychain, filing every secret under
// service. Passing an empty service is a programming error and is refused by every method rather
// than silently using an empty name, because go-keyring treats an empty service as "everything".
func NewOSKeychain(service string) *OSKeychain {
	return &OSKeychain{service: service, api: osKeyring{}}
}

// newKeychainWith is NewOSKeychain with the backend supplied, so the tests can drive the error
// mapping without a keychain on the machine.
func newKeychainWith(service string, api keyringAPI) *OSKeychain {
	return &OSKeychain{service: service, api: api}
}

// Set stores secret under provider.
func (k *OSKeychain) Set(provider, secret string) error {
	if err := k.check(provider); err != nil {
		return err
	}
	if secret == "" {
		return errors.New("keychain: a key to store is required")
	}
	if err := k.api.Set(k.service, provider, secret); err != nil {
		return fmt.Errorf("save the %s key in the keychain: %w", provider, err)
	}
	return nil
}

// Get returns the secret stored for provider, or ErrNoKey when none is.
func (k *OSKeychain) Get(provider string) (string, error) {
	if err := k.check(provider); err != nil {
		return "", err
	}
	secret, err := k.api.Get(k.service, provider)
	switch {
	case errors.Is(err, keyring.ErrNotFound):
		return "", fmt.Errorf("%w: %s", ErrNoKey, provider)
	case err != nil:
		return "", fmt.Errorf("read the %s key from the keychain: %w", provider, err)
	}
	return secret, nil
}

// Remove deletes the secret stored for provider, or answers ErrNoKey when there was none.
func (k *OSKeychain) Remove(provider string) error {
	if err := k.check(provider); err != nil {
		return err
	}
	err := k.api.Delete(k.service, provider)
	switch {
	case errors.Is(err, keyring.ErrNotFound):
		return fmt.Errorf("%w: %s", ErrNoKey, provider)
	case err != nil:
		return fmt.Errorf("remove the %s key from the keychain: %w", provider, err)
	}
	return nil
}

// check refuses a missing service or provider before anything reaches the OS keychain.
func (k *OSKeychain) check(provider string) error {
	if k.service == "" {
		return errors.New("keychain: a service name is required")
	}
	if provider == "" {
		return errors.New("keychain: a provider is required")
	}
	return nil
}

// MemoryKeychain is a Keychain kept in memory. It is a test double: the daemon never builds one,
// and nothing it holds outlives the process. Tests hand it to the provider store so a suite can
// save and remove keys without touching the machine's real keychain.
type MemoryKeychain struct {
	mu      sync.Mutex
	secrets map[string]string
}

// NewMemoryKeychain returns an empty in-memory keychain.
func NewMemoryKeychain() *MemoryKeychain {
	return &MemoryKeychain{secrets: map[string]string{}}
}

// Set stores secret under provider.
func (m *MemoryKeychain) Set(provider, secret string) error {
	if provider == "" {
		return errors.New("keychain: a provider is required")
	}
	if secret == "" {
		return errors.New("keychain: a key to store is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.secrets[provider] = secret
	return nil
}

// Get returns the secret stored for provider, or ErrNoKey when none is.
func (m *MemoryKeychain) Get(provider string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	secret, ok := m.secrets[provider]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrNoKey, provider)
	}
	return secret, nil
}

// Remove deletes the secret stored for provider, or answers ErrNoKey when there was none.
func (m *MemoryKeychain) Remove(provider string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.secrets[provider]; !ok {
		return fmt.Errorf("%w: %s", ErrNoKey, provider)
	}
	delete(m.secrets, provider)
	return nil
}
