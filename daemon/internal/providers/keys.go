package providers

// This file owns the writing half of a provider's secret: storing a key, removing one, and the
// masking that lets a screen show which key is stored without ever holding it. The secret itself
// lives in the OS keychain, behind security.Keychain (internal/security/keychain.go); nothing here
// writes a file and nothing here logs a key.

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const (
	// maskMin is the shortest secret whose prefix Mask shows. A shorter value is not a key any
	// provider issues - the settings form's own shortest is 12 - and showing most of it would
	// defeat the point of masking.
	maskMin = 12
	// maskTail is how many characters from the end of a key Mask keeps. Four is enough to tell two
	// keys of the same kind apart, which is all the masked value is for.
	maskTail = 4
	// maskPrefixLimit is how far into a key Mask looks for the hyphen that ends its kind prefix.
	// The providers put their key's kind there ("sk-ant-", "sk-proj-", "sk-or-"), and it always
	// appears in the first few characters.
	maskPrefixLimit = 8
)

// Mask hides the middle of a secret so a screen can show which key is stored without holding the
// key. It keeps the key's own kind prefix - the part up to the last hyphen in its first few
// characters, which is where providers say what kind of key it is - and its last four characters,
// so "sk-ant-api03-Ab12Cd34Ef564f2a" reads as "sk-ant-…4f2a" and "AIzaSyD9x8Y7w6V5u7Qe0" as
// "AIza…7Qe0". A value too short to hide a middle is shown as nothing but its last character.
//
// This is the only form of a key that ever leaves the daemon (docs/backend-inventory.md N18: the
// key never reaches the client, only a masked value does). A provider that runs on this machine has
// no key to mask; its stored value is a server address and is shown as it is (see MaskedValue).
func Mask(secret string) string {
	runes := []rune(secret)
	switch {
	case len(runes) == 0:
		return ""
	case len(runes) < maskMin:
		return "…" + string(runes[len(runes)-1:])
	}
	return maskPrefix(runes) + "…" + string(runes[len(runes)-maskTail:])
}

// maskPrefix is the leading part of a key that names its kind: up to and including the last hyphen
// in its first maskPrefixLimit characters, or its first four characters when there is no such
// hyphen.
func maskPrefix(runes []rune) string {
	limit := min(len(runes), maskPrefixLimit)
	end := 0
	for i := 0; i < limit; i++ {
		if runes[i] == '-' {
			end = i + 1
		}
	}
	if end > 0 {
		return string(runes[:end])
	}
	return string(runes[:min(len(runes), 4)])
}

// MaskedValue is what a screen is shown for a stored value: the address itself for a provider that
// runs on this machine, because an address is not a secret and a person needs to read it, and the
// masked form for a key.
func (i Info) MaskedValue(secret string) string {
	if i.Local {
		return secret
	}
	return Mask(secret)
}

// Save stores a provider's secret, replacing whatever was there: an API key, or the address of the
// server a local provider runs on. Replacing is what rotating a key does, so saving twice leaves
// one entry rather than two.
//
// The value is checked first, so one that could never work is refused with a sentence rather than
// stored and failing on the first call. Nothing is written until it passes.
func (s *Service) Save(id, secret string) error {
	info, ok := Lookup(id)
	if !ok {
		return unknownProvider(id)
	}
	if err := info.checkSecret(secret); err != nil {
		return err
	}
	if err := s.keys.Set(id, secret); err != nil {
		return fmt.Errorf("save the %s key in the keychain: %w", id, err)
	}
	s.forgetClient(id)
	return nil
}

// Remove deletes a provider's stored value. A provider that had none answers security.ErrNoKey, so
// a caller can tell "removed" from "there was nothing to remove"; the error is passed through
// unwrapped for that.
func (s *Service) Remove(id string) error {
	if _, ok := Lookup(id); !ok {
		return unknownProvider(id)
	}
	if err := s.keys.Remove(id); err != nil {
		return err
	}
	s.forgetClient(id)
	return nil
}

// checkSecret refuses a value that could not be a key for this provider. The sentences are the ones
// a person reads, so they name the provider and say what is wrong.
func (i Info) checkSecret(secret string) error {
	if strings.TrimSpace(secret) != secret {
		return badValue(fmt.Sprintf("The %s key cannot start or end with a space.", i.Name))
	}
	if secret == "" {
		if i.Local {
			return badValue(fmt.Sprintf("Enter the address of the %s server, such as %s.",
				i.Name, i.BaseURL))
		}
		return badValue(fmt.Sprintf("Enter the %s API key.", i.Name))
	}
	if i.Local {
		if err := checkServerAddress(secret); err != nil {
			return badValue(fmt.Sprintf("The %s address %s.", i.Name, err))
		}
	}
	return nil
}

// checkServerAddress refuses anything that is not an http or https address with a host, so a person
// who pastes a key into a local provider's field is told straight away rather than getting a client
// that fails on every call.
func checkServerAddress(value string) error {
	u, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("is not a web address")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("is not an http address, like http://127.0.0.1:1234/v1")
	}
	return nil
}

// badValue is the answer for a value the person typed that Marshal will not store. It is an answer
// the client sees, in the words of docs/ui-rules.md, not the daemon's own failure.
func badValue(message string) error {
	return protocol.InvalidArgument(message)
}
