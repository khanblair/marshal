package platform_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/platform"
)

func TestEnsureTokenCreatesThenReuses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dev-token")
	first, err := platform.EnsureToken(path)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if len(first) < 32 {
		t.Errorf("token %q is too short", first)
	}
	second, err := platform.EnsureToken(path)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if first != second {
		t.Errorf("token changed between calls: %q then %q", first, second)
	}
}

func TestEnsureTokenReplacesAnEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dev-token")
	if err := os.WriteFile(path, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	token, err := platform.EnsureToken(path)
	if err != nil || token == "" {
		t.Fatalf("EnsureToken = %q, %v; want a new token", token, err)
	}
}

func TestEnsureTokenFileIsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dev-token")
	if _, err := platform.EnsureToken(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 && filepath.Separator == '/' {
		t.Errorf("token file mode is %v, want no group or other access", perm)
	}
}
