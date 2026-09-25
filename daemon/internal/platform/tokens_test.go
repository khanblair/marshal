package platform_test

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/platform"
)

func TestTokenPathDependsOnMode(t *testing.T) {
	dir := filepath.Join("data", "marshal")
	if got, want := platform.TokenPath(dir, platform.ModeDev), filepath.Join(dir, "dev-token"); got != want {
		t.Errorf("dev path = %q, want %q", got, want)
	}
	if got, want := platform.TokenPath(dir, platform.ModeNormal), filepath.Join(dir, "owner-token"); got != want {
		t.Errorf("normal path = %q, want %q", got, want)
	}
}

func TestNewTokenIsRandomAndSafeInAHeader(t *testing.T) {
	first, err := platform.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := platform.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Error("two tokens are equal")
	}
	// 32 bytes are 43 characters of unpadded URL-safe base64.
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`).MatchString(first) {
		t.Errorf("token %q is not 43 URL-safe characters", first)
	}
}

func TestCreateTokenFileIsPrivateAndNeverReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owner-token")
	if err := platform.CreateTokenFile(path, "first-token"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 && filepath.Separator == '/' {
		t.Errorf("token file mode is %v, want no group or other access", perm)
	}
	if err := platform.CreateTokenFile(path, "second-token"); !errors.Is(err, os.ErrExist) {
		t.Errorf("second create = %v, want an error that satisfies os.ErrExist", err)
	}
	got, err := platform.ReadTokenFile(path)
	if err != nil || got != "first-token" {
		t.Errorf("ReadTokenFile = %q, %v; want the first token", got, err)
	}
}

func TestReadTokenFileMissingAndEmpty(t *testing.T) {
	dir := t.TempDir()
	if _, err := platform.ReadTokenFile(filepath.Join(dir, "none")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file = %v, want os.ErrNotExist", err)
	}
	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := platform.ReadTokenFile(empty); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("empty file = %v, want an error", err)
	}
}
