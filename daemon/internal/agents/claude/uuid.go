package claude

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// The bits of a version 4 UUID that are not random.
const (
	versionByte    = 6
	variantByte    = 8
	lowNibble      = 0x0f
	lowSixBits     = 0x3f
	version4       = 0x40
	variantRFC4122 = 0x80
	uuidLength     = 36
)

// newSessionID makes a random id in the form of a version 4 UUID, which is what Claude Code's
// --session-id and --resume flags require. internal/protocol's own ids are ULIDs, not UUIDs, so
// this is its own small helper rather than a shared one.
func newSessionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("make a session id: %w", err)
	}
	b[versionByte] = b[versionByte]&lowNibble | version4
	b[variantByte] = b[variantByte]&lowSixBits | variantRFC4122
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}

// looksLikeUUID says whether text has the shape of a UUID: eight, four, four, four, and twelve
// hex digits joined by dashes. Resume checks this before it hands the id to Claude Code's
// --resume flag, which requires a valid UUID and otherwise fails the whole process.
func looksLikeUUID(text string) bool {
	if len(text) != uuidLength {
		return false
	}
	for i, r := range text {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if r != '-' {
				return false
			}
			continue
		}
		if !isHexDigit(r) {
			return false
		}
	}
	return true
}

// isHexDigit says whether r is one hexadecimal digit.
func isHexDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}
