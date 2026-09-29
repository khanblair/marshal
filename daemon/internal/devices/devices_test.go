package devices

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A code is six characters from an alphabet with no letter that reads as a digit, grouped three
// and three so it can be read aloud. Every code can be typed back in exactly: dashes, spaces, and
// case are not part of it, and I, L, and O read as the digits they are mistaken for.
func TestACodeIsReadableAndComesBackTheSame(t *testing.T) {
	for range 200 {
		raw := newCode()
		if len(raw) != codeLength {
			t.Fatalf("a code is %d characters, want %d", len(raw), codeLength)
		}
		for _, r := range raw {
			if !strings.ContainsRune(codeAlphabet, r) {
				t.Fatalf("%q is not in the alphabet %q", r, codeAlphabet)
			}
		}
		shown := formatCode(raw)
		if want := raw[:3] + "-" + raw[3:]; shown != want {
			t.Fatalf("formatCode(%q) = %q, want %q", raw, shown, want)
		}
		for _, typed := range []string{
			shown,
			strings.ToLower(shown),
			strings.ReplaceAll(shown, "-", ""),
			strings.ReplaceAll(shown, "-", " "),
			strings.ReplaceAll(shown, "-", "  "),
		} {
			if got := normalizeCode(typed); got != raw {
				t.Fatalf("normalizeCode(%q) = %q, want %q", typed, got, raw)
			}
		}
	}
}

// The letters a person reading a code off a screen mistakes for digits are typed back as those
// digits, so a code still works however it was read.
func TestANormalizedCodeFoldsTheLettersAMistakes(t *testing.T) {
	tests := []struct{ typed, want string }{
		{"il0o", ""},  // too short to be a code at all
		{"I1L0O", ""}, // five characters: still not a code
		{"ABC-DEF", "ABCDEF"},
		// L and O and I are not in the alphabet at all, so they can only have been a digit read
		// wrong: they come back as the digit they are mistaken for.
		{"7QX-2LD", "7QX21D"},
		{"7QX-1LD", "7QX11D"},
	}
	for _, tc := range tests {
		if got := normalizeCode(tc.typed); got != tc.want {
			t.Errorf("normalizeCode(%q) = %q, want %q", tc.typed, got, tc.want)
		}
	}
	// A code typed entirely in the letters that stand for digits comes back as that code.
	if got := normalizeCode("ilo-ilo"); got != "110110" {
		t.Errorf("normalizeCode(%q) = %q, want %q", "ilo-ilo", got, "110110")
	}
}

// Anything that is not six characters of the alphabet is not a code, so a guess cannot be a
// partial one.
func TestANormalizedCodeRefusesWhatCannotBeOne(t *testing.T) {
	for _, given := range []string{"", "-", "ABC-DEF-GHI", "ABC-DE!", "ABCDEFGHIJKLMNOP", "  "} {
		if got := normalizeCode(given); got != "" && len(got) != codeLength {
			t.Errorf("normalizeCode(%q) = %q, want it to be refused", given, got)
		}
		if got := normalizeCode(given); got != "" {
			for _, r := range got {
				if !strings.ContainsRune(codeAlphabet, r) {
					t.Errorf("normalizeCode(%q) = %q holds %q, which is not in the alphabet", given, got, r)
				}
			}
		}
	}
}

// Only a phone, a tablet, or a browser may be paired. The kinds the daemon writes itself are
// refused by name rather than by being absent from a list, so the sentence says what to do.
func TestOnlyTheKindsAPhoneCanPairAreAllowed(t *testing.T) {
	allowed := []protocol.DeviceKind{protocol.DeviceKindMobile, protocol.DeviceKindWeb}
	for _, kind := range allowed {
		if _, err := deviceName("A phone", kind); err != nil {
			t.Errorf("%s was refused: %v", kind, err)
		}
	}
	owned := []protocol.DeviceKind{
		protocol.DeviceKindCLI, protocol.DeviceKindDev, protocol.DeviceKindDesktop, "smartwatch", "",
	}
	for _, kind := range owned {
		_, err := deviceName("A phone", kind)
		if err == nil {
			t.Errorf("%s was allowed; the daemon owns that kind", kind)
			continue
		}
		var perr *protocol.Error
		if !errors.As(err, &perr) {
			t.Errorf("%s: %v is not the one error shape", kind, err)
			continue
		}
		if !strings.Contains(perr.Message, "Pair a phone") {
			t.Errorf("%s: message = %q, want it to say what to pair", kind, perr.Message)
		}
	}
}

// A device needs a name that can be told apart from the others, and one that is not absurd.
func TestADeviceNeedsAUsableName(t *testing.T) {
	for _, name := range []string{"", "   ", "\t\n"} {
		if _, err := deviceName(name, protocol.DeviceKindMobile); err == nil {
			t.Errorf("%q was accepted; a device needs a name", name)
		}
	}
	if _, err := deviceName(strings.Repeat("x", maxDeviceNameBytes), protocol.DeviceKindMobile); err != nil {
		t.Errorf("a name of exactly %d characters was refused: %v", maxDeviceNameBytes, err)
	}
	long, err := deviceName(strings.Repeat("x", maxDeviceNameBytes+1), protocol.DeviceKindMobile)
	if err == nil {
		t.Errorf("a name of %d characters was accepted, and it is %q", maxDeviceNameBytes+1, long)
	}
	// The name the person typed is kept, minus the spaces around it.
	got, err := deviceName("  Blair's iPhone  ", protocol.DeviceKindMobile)
	if err != nil || got != "Blair's iPhone" {
		t.Errorf("deviceName = %q, %v; want the name as typed, trimmed", got, err)
	}
}

// The clock is the service's own, so a code's life can be moved without waiting it out.
func TestTheServiceUsesTheClockItWasGiven(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	clock := now
	svc := NewService(nil, WithClock(func() time.Time { return clock }))
	code := svc.IssueCode()
	if !code.ExpiresAt.Time().Equal(now.Add(CodeTTL)) {
		t.Errorf("expiresAt = %v, want %v", code.ExpiresAt.Time(), now.Add(CodeTTL))
	}
	clock = now.Add(time.Minute)
	again := svc.IssueCode()
	if again.Code == code.Code {
		t.Error("a second code came back identical; a live code is replaced, not returned")
	}
	if again.ExpiresAt.Time().Equal(code.ExpiresAt.Time()) {
		t.Errorf("expiresAt = %v, want it to move with the clock to %v",
			again.ExpiresAt.Time(), clock.Add(CodeTTL))
	}
}
