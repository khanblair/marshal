package security

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

// scriptedKeyring is a keyringAPI whose answers a test decides. Nothing here reaches the machine's
// keychain, so no test can prompt a person for access or leave an entry behind.
type scriptedKeyring struct {
	setService, setUser, setPassword string
	setErr                           error
	getSecret                        string
	getErr                           error
	deleteErr                        error
	deletedService, deletedUser      string
}

func (s *scriptedKeyring) Set(service, user, password string) error {
	s.setService, s.setUser, s.setPassword = service, user, password
	return s.setErr
}

func (s *scriptedKeyring) Get(service, user string) (string, error) {
	return s.getSecret, s.getErr
}

func (s *scriptedKeyring) Delete(service, user string) error {
	s.deletedService, s.deletedUser = service, user
	return s.deleteErr
}

func TestTheRealKeychainFilesAKeyUnderTheServiceAndProvider(t *testing.T) {
	fake := &scriptedKeyring{}
	k := newKeychainWith("Marshal", fake)

	if err := k.Set("anthropic", "sk-ant-secret"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if fake.setService != "Marshal" || fake.setUser != "anthropic" || fake.setPassword != "sk-ant-secret" {
		t.Errorf("Set stored (%q, %q, %q), want (Marshal, anthropic, sk-ant-secret)",
			fake.setService, fake.setUser, fake.setPassword)
	}

	fake.getSecret = "sk-ant-secret"
	secret, err := k.Get("anthropic")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if secret != "sk-ant-secret" {
		t.Errorf("Get = %q, want %q", secret, "sk-ant-secret")
	}
}

func TestAMissingKeyIsNotAFailure(t *testing.T) {
	k := newKeychainWith("Marshal", &scriptedKeyring{getErr: keyring.ErrNotFound})

	_, err := k.Get("openai")
	if !errors.Is(err, ErrNoKey) {
		t.Fatalf("Get error = %v, want ErrNoKey", err)
	}
}

func TestRemovingAKeyThatWasNeverSavedSaysSo(t *testing.T) {
	k := newKeychainWith("Marshal", &scriptedKeyring{deleteErr: keyring.ErrNotFound})

	err := k.Remove("deepseek")
	if !errors.Is(err, ErrNoKey) {
		t.Fatalf("Remove error = %v, want ErrNoKey", err)
	}
}

func TestRemovingAKeyDeletesItUnderTheRightProvider(t *testing.T) {
	fake := &scriptedKeyring{}
	k := newKeychainWith("Marshal", fake)

	if err := k.Remove("gemini"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if fake.deletedService != "Marshal" || fake.deletedUser != "gemini" {
		t.Errorf("Remove deleted (%q, %q), want (Marshal, gemini)", fake.deletedService, fake.deletedUser)
	}
}

func TestAnUnexpectedKeychainErrorIsPassedOn(t *testing.T) {
	want := errors.New("the keychain is locked")
	k := newKeychainWith("Marshal", &scriptedKeyring{setErr: want})

	err := k.Set("anthropic", "sk-ant-secret")
	if !errors.Is(err, want) {
		t.Fatalf("Set error = %v, want it to wrap %v", err, want)
	}
	if errors.Is(err, ErrNoKey) {
		t.Error("a locked keychain was reported as a missing key")
	}
}

func TestTheKeychainRefusesAnEmptyServiceOrProvider(t *testing.T) {
	t.Run("no service", func(t *testing.T) {
		k := newKeychainWith("", &scriptedKeyring{})
		if err := k.Set("anthropic", "k"); err == nil {
			t.Error("Set with no service name was allowed")
		}
		if _, err := k.Get("anthropic"); err == nil {
			t.Error("Get with no service name was allowed")
		}
	})
	t.Run("no provider", func(t *testing.T) {
		k := newKeychainWith("Marshal", &scriptedKeyring{})
		if err := k.Set("", "k"); err == nil {
			t.Error("Set with no provider was allowed")
		}
	})
	t.Run("no key", func(t *testing.T) {
		k := newKeychainWith("Marshal", &scriptedKeyring{})
		if err := k.Set("anthropic", ""); err == nil {
			t.Error("Set with an empty key was allowed")
		}
	})
}

func TestTheInMemoryKeychainBehavesLikeTheRealOne(t *testing.T) {
	k := NewMemoryKeychain()

	if _, err := k.Get("anthropic"); !errors.Is(err, ErrNoKey) {
		t.Fatalf("Get before any save = %v, want ErrNoKey", err)
	}
	if err := k.Set("anthropic", "first"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := k.Set("anthropic", "second"); err != nil {
		t.Fatalf("Set again: %v", err)
	}
	got, err := k.Get("anthropic")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "second" {
		t.Errorf("Get = %q, want the key that was saved last (%q)", got, "second")
	}
	if err := k.Set("openai", "other"); err != nil {
		t.Fatalf("Set on another provider: %v", err)
	}
	if err := k.Remove("anthropic"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := k.Get("anthropic"); !errors.Is(err, ErrNoKey) {
		t.Errorf("Get after Remove = %v, want ErrNoKey", err)
	}
	if got, err := k.Get("openai"); err != nil || got != "other" {
		t.Errorf("removing one key changed another: Get(openai) = %q, %v", got, err)
	}
	if err := k.Remove("anthropic"); !errors.Is(err, ErrNoKey) {
		t.Errorf("Remove twice = %v, want ErrNoKey", err)
	}
}
