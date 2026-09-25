package platform

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The token files live directly in the data folder. A dev daemon uses the dev token, and a
// normal daemon uses the owner token. The desktop app and the `marshal` command read them from
// here, so the names are shared.
const (
	// DevTokenFile is the file with the dev token. Only a dev daemon reads it.
	DevTokenFile = "dev-token"
	// OwnerTokenFile is the file with the token of the owner's command line device.
	OwnerTokenFile = "owner-token"
)

// TokenPath returns the token file that clients of a daemon in this mode read.
func TokenPath(dataDir string, mode Mode) string {
	if mode == ModeDev {
		return filepath.Join(dataDir, DevTokenFile)
	}
	return filepath.Join(dataDir, OwnerTokenFile)
}

// NewToken makes a random token: 32 bytes from the system's random source, as URL-safe base64
// without padding. That form is safe in a header and in a WebSocket subprotocol name.
func NewToken() (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("make a token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// CreateTokenFile writes a token to a new file that only the owner can read. It never replaces a
// file: if the file exists the error satisfies errors.Is(err, os.ErrExist), so a token that a
// client already holds cannot be lost.
func CreateTokenFile(path, token string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, tokenFileMode)
	if err != nil {
		return fmt.Errorf("create the token file: %w", err)
	}
	if _, err := file.WriteString(token + "\n"); err != nil {
		return errors.Join(fmt.Errorf("write the token file: %w", err), file.Close())
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("write the token file: %w", err)
	}
	return nil
}

// ReadTokenFile returns the token in a file. A missing file is an error that satisfies
// errors.Is(err, os.ErrNotExist), and so is a file with no token in it.
func ReadTokenFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read the token file: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("read the token file: it is empty: %w", os.ErrNotExist)
	}
	return token, nil
}
