// Package testutil holds helpers that only tests use: golden files and fixture repositories.
package testutil

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const (
	updateEnv  = "UPDATE_GOLDEN"
	goldenMode = 0o644
	dirMode    = 0o755
)

// moduleDir returns the daemon module folder, found from this file's own location.
func moduleDir(t testing.TB) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot find the module folder")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// TestdataPath returns a path inside daemon/testdata.
func TestdataPath(t testing.TB, elem ...string) string {
	t.Helper()
	return filepath.Join(append([]string{moduleDir(t), "testdata"}, elem...)...)
}

// Golden compares the JSON form of value with daemon/testdata/golden/<name>.json. The client's
// mapper tests read the same file, so a change to a wire type shows up on both sides. Run the
// tests with UPDATE_GOLDEN=1 to write the file after a deliberate change.
func Golden(t testing.TB, name string, value any) {
	t.Helper()
	got, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("encode %s: %v", name, err)
	}
	got = append(got, '\n')
	path := TestdataPath(t, "golden", name+".json")
	if os.Getenv(updateEnv) == "1" {
		if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
			t.Fatalf("make the golden folder: %v", err)
		}
		if err := os.WriteFile(path, got, goldenMode); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s (run with %s=1 to create it): %v", path, updateEnv, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s does not match the golden file.\n got:\n%s\nwant:\n%s\nRun with %s=1 after a deliberate change.", name, got, want, updateEnv)
	}
}
