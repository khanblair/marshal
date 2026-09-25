package platform

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// readFile is a small wrapper the service generation tests use to check that a file was written,
// without caring about its content beyond that a read succeeds.
func readFile(t *testing.T, path string) (string, error) {
	t.Helper()
	data, err := os.ReadFile(path)
	return string(data), err
}

// writeFile writes content at path, making its folder first, for tests that need a file already
// there before they call Uninstall or Status.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// exists reports whether a path is there.
func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// uidString mirrors launchdGUIDomain's own os.Getuid() call, so a test can build the exact
// command line it expects without duplicating that knowledge as a hardcoded number.
func uidString() string {
	return strconv.Itoa(os.Getuid())
}

// cmdKey builds the same lookup key fakeRunner.run computes from a call, so a test can register a
// canned answer for a command without hand-joining (and risking mis-quoting) the argument list
// itself.
func cmdKey(name string, args ...string) string {
	return strings.Join(append([]string{name}, args...), " ")
}

// assertCalls checks that a fakeRunner saw exactly the given commands, in order.
func assertCalls(t *testing.T, got, want [][]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("ran %d commands, want %d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if !slices.Equal(got[i], want[i]) {
			t.Errorf("call %d = %v, want %v", i, got[i], want[i])
		}
	}
}
