package protocol

import (
	"fmt"
	"io"
	"time"
)

// crockford is Crockford's base32 alphabet: digits and capital letters without I, L, O, and U.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

const (
	// idLength is the length of an opaque id: 10 characters of time and 16 of randomness.
	idLength         = 26
	idTimeLength     = 10
	idHalfLength     = 8
	idEntropyBytes   = 10
	idHalfBytes      = idEntropyBytes / 2
	idBitsPerChar    = 5
	idCharMask       = 1<<idBitsPerChar - 1
	idMaxMillis      = 1<<48 - 1
	idFirstCharLimit = '7'
	bitsPerByte      = 8
)

// NewID makes an opaque id: a 26-character ULID in Crockford base32. The first 10 characters
// are the time in milliseconds, so ids sort by creation time (to the millisecond), and the last
// 16 are 80 random bits read from entropy. Use crypto/rand.Reader for entropy in the daemon and
// a fixed reader in tests. Chats, sessions, devices, approvals, and cards all use these ids.
func NewID(now time.Time, entropy io.Reader) (string, error) {
	millis := now.UnixMilli()
	if millis < 0 || millis > idMaxMillis {
		return "", fmt.Errorf("make an id: the time %s cannot be an id time", now.UTC().Format(time.RFC3339))
	}
	var random [idEntropyBytes]byte
	if _, err := io.ReadFull(entropy, random[:]); err != nil {
		return "", fmt.Errorf("make an id: read random bytes: %w", err)
	}
	var out [idLength]byte
	putBase32(out[:idTimeLength], uint64(millis))
	putBase32(out[idTimeLength:idTimeLength+idHalfLength], beUint40(random[:idHalfBytes]))
	putBase32(out[idTimeLength+idHalfLength:], beUint40(random[idHalfBytes:]))
	return string(out[:]), nil
}

// ValidID reports whether id has the shape of an id made by NewID.
func ValidID(id string) bool {
	if len(id) != idLength || id[0] > idFirstCharLimit {
		return false
	}
	for i := 0; i < len(id); i++ {
		if !isCrockford(id[i]) {
			return false
		}
	}
	return true
}

func isCrockford(c byte) bool {
	for i := 0; i < len(crockford); i++ {
		if crockford[i] == c {
			return true
		}
	}
	return false
}

// putBase32 writes v into dst as base32, most significant character first.
func putBase32(dst []byte, v uint64) {
	for i := len(dst) - 1; i >= 0; i-- {
		dst[i] = crockford[v&idCharMask]
		v >>= idBitsPerChar
	}
}

// beUint40 reads five bytes as a big-endian number.
func beUint40(b []byte) uint64 {
	var v uint64
	for _, x := range b[:idHalfBytes] {
		v = v<<bitsPerByte | uint64(x)
	}
	return v
}
