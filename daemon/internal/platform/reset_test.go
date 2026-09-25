package platform_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/platform"
)

func TestCheckResettableRefusesUnsafeFolders(t *testing.T) {
	home := t.TempDir()
	tests := map[string]string{
		"relative":          "Marshal-dev",
		"home":              home,
		"normal data":       filepath.Join(home, "Library", "Application Support", "Marshal"),
		"no dev suffix":     filepath.Join(home, "data", "notes"),
		"top of the disk":   filepath.Join(string(filepath.Separator), "x-dev"),
		"only two folders":  filepath.Join(string(filepath.Separator), "data", "x-dev"),
		"a dev name inside": filepath.Join(home, "x-dev", "keep"),
	}
	for name, path := range tests {
		t.Run(name, func(t *testing.T) {
			err := platform.CheckResettable(path, home)
			if !errors.Is(err, platform.ErrUnsafeReset) {
				t.Errorf("CheckResettable(%q) = %v, want ErrUnsafeReset", path, err)
			}
		})
	}
}

func TestResetDevDataDeletesOnlyTheDevFolder(t *testing.T) {
	home := t.TempDir()
	dev := filepath.Join(home, "share", "marshal-dev")
	normal := filepath.Join(home, "share", "marshal")
	for _, dir := range []string{dev, normal} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "marshal.db"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := platform.ResetDevData(dev, home); err != nil {
		t.Fatalf("ResetDevData: %v", err)
	}
	if _, err := os.Stat(dev); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dev folder still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(normal, "marshal.db")); err != nil {
		t.Errorf("normal folder was touched: %v", err)
	}
}

func TestResetDevDataRefusesANormalFolder(t *testing.T) {
	home := t.TempDir()
	normal := filepath.Join(home, "share", "marshal")
	if err := os.MkdirAll(normal, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := platform.ResetDevData(normal, home); !errors.Is(err, platform.ErrUnsafeReset) {
		t.Fatalf("ResetDevData = %v, want ErrUnsafeReset", err)
	}
	if _, err := os.Stat(normal); err != nil {
		t.Errorf("the normal folder was deleted: %v", err)
	}
}

func TestResetDevDataMissingFolderIsFine(t *testing.T) {
	home := t.TempDir()
	if err := platform.ResetDevData(filepath.Join(home, "share", "marshal-dev"), home); err != nil {
		t.Fatalf("ResetDevData on a missing folder: %v", err)
	}
}
