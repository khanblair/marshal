package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/platform"
	"github.com/khanblair/marshal/daemon/internal/providers"
	"github.com/khanblair/marshal/daemon/internal/security"
)

// These keys are synthetic and reach nothing. Every test here runs against an in-memory keychain,
// so no test reads or writes the machine's real one and no provider is ever called.
const (
	keysTestAnthropic = "sk-ant-api03-not-real-7c41"
	keysTestMasked    = "sk-ant-…7c41"
	keysTestOllama    = "http://127.0.0.1:11434/v1"
)

// keysMachine is a fake machine with a home folder and no environment variables, so the keys
// commands never reach a real data folder and config.Load sees only the defaults.
func keysMachine(t *testing.T) platform.Env {
	t.Helper()
	return platform.Env{GOOS: "linux", Home: t.TempDir(), Getenv: func(string) string { return "" }}
}

// keyStore is the injected keychain seam: it hands every command the same in-memory keychain and
// records which mode it was opened for, the way a test of the real one would check that a dev
// command never touches a normal install's secrets.
type keyStore struct {
	kc   *security.MemoryKeychain
	mode platform.Mode
	open int
}

func newKeyStore() *keyStore { return &keyStore{kc: security.NewMemoryKeychain()} }

func (k *keyStore) factory(mode platform.Mode) security.Keychain {
	k.mode = mode
	k.open++
	return k.kc
}

func runKeysCLI(t *testing.T, ks *keyStore, env platform.Env, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = runKeys(env, args, terminal{stdin: strings.NewReader(stdin), stdout: &out, stderr: &errOut}, ks.factory)
	return code, out.String(), errOut.String()
}

func TestKeysListShowsEveryProviderAndNothingStored(t *testing.T) {
	ks := newKeyStore()
	code, out, errOut := runKeysCLI(t, ks, keysMachine(t), "", "list")
	if code != exitOK {
		t.Fatalf("keys list = %d %q %q", code, out, errOut)
	}
	if lines := strings.Count(out, "\n"); lines != len(providers.Known()) {
		t.Errorf("keys list printed %d lines, want one per known provider (%d)", lines, len(providers.Known()))
	}
	for _, info := range providers.Known() {
		if !strings.Contains(out, info.ID+" ") {
			t.Errorf("keys list does not mention %q:\n%s", info.ID, out)
		}
	}
	if !strings.Contains(out, "not set") {
		t.Errorf("keys list on an empty keychain = %q, want every row to say not set", out)
	}
}

func TestKeysSetStoresAKeyAndTheListShowsItMasked(t *testing.T) {
	ks := newKeyStore()
	env := keysMachine(t)
	code, out, errOut := runKeysCLI(t, ks, env, keysTestAnthropic+"\n", "set", "anthropic")
	if code != exitOK {
		t.Fatalf("keys set = %d %q %q", code, out, errOut)
	}
	if !strings.Contains(out, "Saved the Anthropic key.") {
		t.Errorf("keys set stdout = %q, want a confirmation", out)
	}
	if stored, _ := ks.kc.Get("anthropic"); stored != keysTestAnthropic {
		t.Errorf("stored key = %q, want the key that was piped in", stored)
	}
	code, out, errOut = runKeysCLI(t, ks, env, "", "list")
	if code != exitOK {
		t.Fatalf("keys list = %d %q %q", code, out, errOut)
	}
	if !strings.Contains(out, keysTestMasked) {
		t.Errorf("keys list = %q, want the masked key %q", out, keysTestMasked)
	}
	if strings.Contains(out+errOut, keysTestAnthropic) {
		t.Errorf("keys list printed the key itself:\n%s%s", out, errOut)
	}
}

// TestKeysSetNeverTakesTheKeyAsAnArgument proves the key cannot be passed on the command line,
// where it would land in the shell history: set takes exactly one argument, the provider id.
func TestKeysSetNeverTakesTheKeyAsAnArgument(t *testing.T) {
	ks := newKeyStore()
	code, _, errOut := runKeysCLI(t, ks, keysMachine(t), "", "set", "anthropic", keysTestAnthropic)
	if code != exitBadInput || !strings.Contains(errOut, "Usage: marshal keys") {
		t.Fatalf("keys set with the key as an argument = %d %q, want a usage answer", code, errOut)
	}
	if _, err := ks.kc.Get("anthropic"); err == nil {
		t.Error("a key passed as an argument was stored")
	}
	if ks.open != 0 {
		t.Error("the keychain was opened although the arguments were wrong")
	}
}

func TestKeysSetRefusesAKeyWithSurroundingSpace(t *testing.T) {
	ks := newKeyStore()
	code, _, errOut := runKeysCLI(t, ks, keysMachine(t), " "+keysTestAnthropic+"\n", "set", "anthropic")
	if code != exitFailed || !strings.Contains(errOut, "The Anthropic key cannot start or end with a space.") {
		t.Fatalf("keys set with a padded key = %d %q", code, errOut)
	}
	if _, err := ks.kc.Get("anthropic"); err == nil {
		t.Error("a padded key was stored")
	}
}

func TestKeysSetRefusesAnEmptyKey(t *testing.T) {
	ks := newKeyStore()
	code, _, errOut := runKeysCLI(t, ks, keysMachine(t), "\n", "set", "anthropic")
	if code != exitFailed || !strings.Contains(errOut, "Enter the Anthropic API key.") {
		t.Fatalf("keys set with an empty key = %d %q", code, errOut)
	}
	if _, err := ks.kc.Get("anthropic"); err == nil {
		t.Error("an empty key was stored")
	}
}

func TestKeysSetAnUnknownProviderNamesItAndTheOnesItKnows(t *testing.T) {
	ks := newKeyStore()
	code, _, errOut := runKeysCLI(t, ks, keysMachine(t), "whatever\n", "set", "groq")
	if code != exitBadInput {
		t.Fatalf("keys set groq = %d %q, want %d", code, errOut, exitBadInput)
	}
	if !strings.Contains(errOut, `Marshal does not know a provider called "groq".`) {
		t.Errorf("stderr = %q, want it to name the unknown provider", errOut)
	}
	if !strings.Contains(errOut, "anthropic") {
		t.Errorf("stderr = %q, want it to list the providers Marshal knows", errOut)
	}
}

// TestKeysSetALocalProviderStoresAnAddressAndShowsIt covers the local providers: their stored value
// is a server address, not a key, and a person needs to read it, so the list shows it as it is.
func TestKeysSetALocalProviderStoresAnAddressAndShowsIt(t *testing.T) {
	ks := newKeyStore()
	env := keysMachine(t)
	code, out, errOut := runKeysCLI(t, ks, env, keysTestOllama+"\n", "set", "ollama")
	if code != exitOK {
		t.Fatalf("keys set ollama = %d %q %q", code, out, errOut)
	}
	if !strings.Contains(errOut, "Paste the address of the Ollama server") {
		t.Errorf("keys set stderr = %q, want an address prompt for a local provider", errOut)
	}
	_, out, _ = runKeysCLI(t, ks, env, "", "list")
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(line, "ollama") {
			if !strings.Contains(line, keysTestOllama) {
				t.Errorf("the ollama row = %q, want the stored address shown unmasked", line)
			}
			return
		}
	}
	t.Errorf("keys list has no ollama row:\n%s", out)
}

func TestKeysSetALocalProviderRefusesAKey(t *testing.T) {
	ks := newKeyStore()
	code, _, errOut := runKeysCLI(t, ks, keysMachine(t), keysTestAnthropic+"\n", "set", "ollama")
	if code != exitFailed || !strings.Contains(errOut, "The Ollama address") {
		t.Fatalf("keys set ollama with a key = %d %q", code, errOut)
	}
	if _, err := ks.kc.Get("ollama"); err == nil {
		t.Error("a key pasted into a local provider's address field was stored")
	}
}

func TestKeysRemoveDeletesAKey(t *testing.T) {
	ks := newKeyStore()
	env := keysMachine(t)
	if code, _, errOut := runKeysCLI(t, ks, env, keysTestAnthropic+"\n", "set", "anthropic"); code != exitOK {
		t.Fatalf("keys set = %d %q", code, errOut)
	}
	code, out, errOut := runKeysCLI(t, ks, env, "", "remove", "anthropic")
	if code != exitOK || !strings.Contains(out, "Removed the Anthropic key.") {
		t.Fatalf("keys remove = %d %q %q", code, out, errOut)
	}
	if _, err := ks.kc.Get("anthropic"); err == nil {
		t.Error("the key was still stored after remove")
	}
}

func TestKeysRemoveWhenThereIsNoneIsNotAnError(t *testing.T) {
	ks := newKeyStore()
	code, out, errOut := runKeysCLI(t, ks, keysMachine(t), "", "remove", "anthropic")
	if code != exitOK || !strings.Contains(out, "There is no Anthropic key stored.") || errOut != "" {
		t.Fatalf("keys remove with nothing stored = %d %q %q", code, out, errOut)
	}
}

func TestKeysRemoveAnUnknownProvider(t *testing.T) {
	ks := newKeyStore()
	code, _, errOut := runKeysCLI(t, ks, keysMachine(t), "", "remove", "groq")
	if code != exitBadInput || !strings.Contains(errOut, `Marshal does not know a provider called "groq".`) {
		t.Fatalf("keys remove groq = %d %q", code, errOut)
	}
}

func TestKeysDevUsesTheDevKeychain(t *testing.T) {
	ks := newKeyStore()
	if code, _, errOut := runKeysCLI(t, ks, keysMachine(t), "", "list", "--dev"); code != exitOK {
		t.Fatalf("keys list --dev = %d %q", code, errOut)
	}
	if ks.mode != platform.ModeDev {
		t.Errorf("keychain opened for mode %v, want dev", ks.mode)
	}
}

func TestKeysWithNoSubcommandShowsUsage(t *testing.T) {
	ks := newKeyStore()
	code, _, errOut := runKeysCLI(t, ks, keysMachine(t), "")
	if code != exitBadInput || !strings.Contains(errOut, "Usage: marshal keys") {
		t.Errorf("keys (no subcommand) = %d %q", code, errOut)
	}
}

func TestKeysUnknownSubcommandShowsUsage(t *testing.T) {
	ks := newKeyStore()
	code, _, errOut := runKeysCLI(t, ks, keysMachine(t), "", "explode")
	if code != exitBadInput || !strings.Contains(errOut, "Unknown keys command") {
		t.Errorf("keys explode = %d %q", code, errOut)
	}
}

func TestKeysSetWithNoProviderShowsUsage(t *testing.T) {
	ks := newKeyStore()
	code, _, errOut := runKeysCLI(t, ks, keysMachine(t), "key\n", "set")
	if code != exitBadInput || !strings.Contains(errOut, "Usage: marshal keys") {
		t.Errorf("keys set (no provider) = %d %q", code, errOut)
	}
	if ks.open != 0 {
		t.Error("the keychain was opened although no provider was named")
	}
}

func TestKeysListRejectsStrayArguments(t *testing.T) {
	ks := newKeyStore()
	code, _, errOut := runKeysCLI(t, ks, keysMachine(t), "", "list", "anthropic")
	if code != exitBadInput || !strings.Contains(errOut, "Usage: marshal keys") {
		t.Errorf("keys list with a stray argument = %d %q", code, errOut)
	}
}

// TestKeysIsWiredIntoTheCommandLine proves runWith sends `keys` here. It names no provider, so it
// stops at the usage line and never opens the machine's real keychain.
func TestKeysIsWiredIntoTheCommandLine(t *testing.T) {
	if !strings.Contains(usage, "keys set|list|remove") {
		t.Errorf("the command list does not mention keys:\n%s", usage)
	}
	code, _, errOut := runDevWith(keysMachine(t), "", "keys")
	if code != exitBadInput || !strings.Contains(errOut, "Usage: marshal keys") {
		t.Errorf("marshal keys = %d %q, want the keys usage", code, errOut)
	}
}
