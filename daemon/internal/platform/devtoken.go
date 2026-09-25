package platform

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	tokenBytes    = 32
	tokenFileMode = 0o600
)

// EnsureToken returns the token stored in the file, and creates the file with a new random
// token when it does not exist. Only the owner can read it. The dev daemon accepts this token
// from localhost only, so no real credential is ever needed in development.
func EnsureToken(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		if token := strings.TrimSpace(string(data)); token != "" {
			return token, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read the token file: %w", err)
	}
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("make a token: %w", err)
	}
	token := hex.EncodeToString(raw)
	if err := os.WriteFile(path, []byte(token+"\n"), tokenFileMode); err != nil {
		return "", fmt.Errorf("write the token file: %w", err)
	}
	return token, nil
}
