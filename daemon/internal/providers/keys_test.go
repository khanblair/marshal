package providers

import (
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
)

// testLocalAddress is a loopback address on the discard port. It is what a test stores for a
// provider that runs on this machine, and it reaches nothing.
const testLocalAddress = "http://127.0.0.1:9/v1"

// newTestService returns a service over an in-memory keychain, so no test can read or write the
// real one, and a logger that throws its lines away.
func newTestService(t *testing.T, keys security.Keychain) *Service {
	t.Helper()
	if keys == nil {
		keys = security.NewMemoryKeychain()
	}
	s, err := New(keys, Options{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// brokenKeychain is a keychain that cannot be read. A test uses it to prove a keychain failure is
// not mistaken for "this provider is not set up".
type brokenKeychain struct{ err error }

func (b brokenKeychain) Set(string, string) error   { return b.err }
func (b brokenKeychain) Get(string) (string, error) { return "", b.err }
func (b brokenKeychain) Remove(string) error        { return b.err }

func TestMaskKeepsTheKeysKindAndItsEnding(t *testing.T) {
	cases := []struct {
		name   string
		secret string
		want   string
	}{
		{"an Anthropic key", "sk-ant-api03-Ab12Cd34Ef56Gh784f2a", "sk-ant-…4f2a"},
		{"an OpenAI key", "sk-proj-Ab12Cd34Ef56Gh78Ab91cd", "sk-proj-…91cd"},
		{"an OpenRouter key", "sk-or-v1-Ab12Cd34Ef56Gh780b33", "sk-or-…0b33"},
		{"a Google key, which has no hyphen", "AIzaSyD9x8Y7w6V5uT4s3R27Qe0", "AIza…7Qe0"},
		{"a key with no hyphen at all", "Ab12Cd34Ef56Gh78", "Ab12…Gh78"},
		{"a key too short to show a prefix of", "sk-abcdefg", "…g"},
		{"one character", "x", "…x"},
		{"nothing stored", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Mask(c.secret)
			if got != c.want {
				t.Errorf("Mask(%q) = %q, want %q", c.secret, got, c.want)
			}
			if c.secret != "" && got == c.secret {
				t.Errorf("Mask(%q) returned the key itself", c.secret)
			}
		})
	}
}

func TestMaskedValueShowsALocalProvidersAddress(t *testing.T) {
	ollama, ok := Lookup(OllamaID)
	if !ok {
		t.Fatal("Ollama is not in the table")
	}
	if got := ollama.MaskedValue(testLocalAddress); got != testLocalAddress {
		t.Errorf("a local provider's address was masked to %q, want it as it is", got)
	}
	anthropic, _ := Lookup(AnthropicID)
	key := "sk-ant-api03-Ab12Cd34Ef56Gh784f2a"
	if got := anthropic.MaskedValue(key); got == key || got != "sk-ant-…4f2a" {
		t.Errorf("a hosted provider's key was shown as %q", got)
	}
}

func TestSaveTellsTheListWhichProvidersAreSetUp(t *testing.T) {
	s := newTestService(t, nil)
	key := "sk-ant-api03-Ab12Cd34Ef56Gh784f2a"

	rows, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != len(Known()) {
		t.Errorf("List has %d rows, want one for each of the %d providers", len(rows), len(Known()))
	}
	for _, row := range rows {
		if row.Status != protocol.ProviderStatusEmpty {
			t.Errorf("%s is %s on a fresh install, want empty", row.ID, row.Status)
		}
		if row.Masked != "" {
			t.Errorf("%s shows %q with nothing stored", row.ID, row.Masked)
		}
	}

	if err := s.Save(AnthropicID, key); err != nil {
		t.Fatalf("Save: %v", err)
	}
	row := rowFor(t, s, AnthropicID)
	if row.Status != protocol.ProviderStatusSaved {
		t.Errorf("after saving a key, %s is %s, want saved", AnthropicID, row.Status)
	}
	if row.Masked != "sk-ant-…4f2a" {
		t.Errorf("the saved key is shown as %q, want it masked", row.Masked)
	}
	if row.Name == "" || row.Models == "" || row.Local {
		t.Errorf("the row lost its own fields: %+v", row)
	}
	other := rowFor(t, s, OpenAIID)
	if other.Status != protocol.ProviderStatusEmpty {
		t.Errorf("saving one provider set up another: %s is %s", other.ID, other.Status)
	}
}

// TestListNeverCarriesTheKey is the rule the whole keychain exists for: the key leaves the daemon
// only as a masked value.
func TestListNeverCarriesTheKey(t *testing.T) {
	s := newTestService(t, nil)
	key := "sk-ant-api03-Ab12Cd34Ef56Gh784f2a"
	if err := s.Save(AnthropicID, key); err != nil {
		t.Fatalf("Save: %v", err)
	}
	rows, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, row := range rows {
		if strings.Contains(row.Masked, key) {
			t.Fatalf("%s carries the key itself", row.ID)
		}
	}
}

func TestSaveRefusesValuesThatCouldNotWork(t *testing.T) {
	s := newTestService(t, nil)
	cases := []struct {
		name string
		id   string
		key  string
	}{
		{"an empty key", AnthropicID, ""},
		{"a key with a space on the end", AnthropicID, "sk-ant-api03-Ab12Cd34Ef564f2a "},
		{"a key with a space in front", AnthropicID, " sk-ant-api03-Ab12Cd34Ef564f2a"},
		{"a local provider with no address", OllamaID, ""},
		{"a local provider given a bare host", OllamaID, "127.0.0.1:11434"},
		{"a local provider given something that is not an address", OllamaID, "not an address"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := s.Save(c.id, c.key)
			if err == nil {
				t.Fatalf("Save accepted %q", c.key)
			}
			var answer *protocol.Error
			if !errors.As(err, &answer) || answer.Code != protocol.ErrorCodeInvalidArgument {
				t.Fatalf("Save refused %q with %v, want a plain sentence for the person", c.key, err)
			}
			if answer.Message == "" {
				t.Error("the refusal has no sentence to show")
			}
			setUp, err := s.Has(c.id)
			if err != nil {
				t.Fatalf("Has: %v", err)
			}
			if setUp {
				t.Errorf("a refused value was stored for %s", c.id)
			}
		})
	}
}

func TestSaveAndRemoveRefuseAnUnknownProvider(t *testing.T) {
	s := newTestService(t, nil)
	for _, err := range []error{s.Save("gemini-cli", "x"), s.Remove("gemini-cli")} {
		var answer *protocol.Error
		if !errors.As(err, &answer) || answer.Code != protocol.ErrorCodeNotFound {
			t.Errorf("an unknown provider answered %v, want not found", err)
		}
	}
}

func TestSaveReplacesAKeyRatherThanAddingOne(t *testing.T) {
	s := newTestService(t, nil)
	if err := s.Save(AnthropicID, "sk-ant-api03-Ab12Cd34Ef561111"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Save(AnthropicID, "sk-ant-api03-Ab12Cd34Ef562222"); err != nil {
		t.Fatalf("Save again: %v", err)
	}
	if got := rowFor(t, s, AnthropicID).Masked; got != "sk-ant-…2222" {
		t.Errorf("after saving twice the row shows %q, want the second key masked", got)
	}
}

func TestRemoveTellsRemovedFromNothingToRemove(t *testing.T) {
	s := newTestService(t, nil)
	if err := s.Save(AnthropicID, "sk-ant-api03-Ab12Cd34Ef564f2a"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Remove(AnthropicID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if got := rowFor(t, s, AnthropicID); got.Status != protocol.ProviderStatusEmpty || got.Masked != "" {
		t.Errorf("after removing the key the row is %+v, want empty", got)
	}
	if err := s.Remove(AnthropicID); !errors.Is(err, security.ErrNoKey) {
		t.Errorf("removing a provider with nothing stored answered %v, want ErrNoKey", err)
	}
}

func TestSaveAndRemoveLeaveTheKeychainToItself(t *testing.T) {
	keys := security.NewMemoryKeychain()
	s := newTestService(t, keys)
	key := "sk-ant-api03-Ab12Cd34Ef564f2a"
	if err := s.Save(AnthropicID, key); err != nil {
		t.Fatalf("Save: %v", err)
	}
	stored, err := keys.Get(AnthropicID)
	if err != nil {
		t.Fatalf("the key was not stored under the provider's id: %v", err)
	}
	if stored != key {
		t.Errorf("the keychain holds %q, want the key as it was typed", stored)
	}
	if err := s.Save(OllamaID, testLocalAddress); err != nil {
		t.Fatalf("Save a local address: %v", err)
	}
	if stored, err := keys.Get(OllamaID); err != nil || stored != testLocalAddress {
		t.Errorf("a local provider's address is stored as %q, %v", stored, err)
	}
}

func TestHasReportsWhatIsStored(t *testing.T) {
	s := newTestService(t, nil)
	setUp, err := s.Has(AnthropicID)
	if err != nil {
		t.Fatalf("Has: %v", err)
	}
	if setUp {
		t.Error("a provider with no key reports itself set up")
	}
	if _, err := s.Has("not-a-provider"); err == nil {
		t.Error("Has accepted a provider Marshal does not know")
	}
}

// TestAKeychainFailureIsNotAnEmptyProvider is why Service.secret tells ErrNoKey apart from every
// other error: a keychain that cannot be read must be reported, not quietly called "not set up".
func TestAKeychainFailureIsNotAnEmptyProvider(t *testing.T) {
	failure := errors.New("the keychain is locked")
	s := newTestService(t, brokenKeychain{err: failure})

	if setUp, err := s.Has(AnthropicID); !errors.Is(err, failure) {
		t.Errorf("Has answered %v with %v, want the keychain's own failure", setUp, err)
	} else if setUp {
		t.Error("Has called an unreadable keychain a set-up provider")
	}
	if _, err := s.List(); !errors.Is(err, failure) {
		t.Errorf("List answered %v, want the keychain's own failure", err)
	}
	if _, err := s.Resolve(DefaultModel); !errors.Is(err, failure) {
		t.Errorf("Resolve answered %v, want the keychain's own failure", err)
	}
	if got := s.Models(); len(got) != 0 {
		t.Errorf("Models offered %v from a keychain it could not read", got)
	}
}

// rowFor returns the row for a provider, failing when it is missing.
func rowFor(t *testing.T, s *Service, id string) protocol.Provider {
	t.Helper()
	rows, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, row := range rows {
		if row.ID == id {
			return row
		}
	}
	t.Fatalf("List has no row for %s", id)
	return protocol.Provider{}
}
