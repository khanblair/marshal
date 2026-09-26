package sample

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// The txtar reader takes the text before the first file as the message and keeps each file's
// text as it is, and it refuses a commit that could write outside the repository.
func TestParseCommit(t *testing.T) {
	c, err := parseCommit("Add a thing\n\nWhy it is added.\n-- a.txt --\none\ntwo\n-- src/b.ts --\nthree\n")
	if err != nil {
		t.Fatal(err)
	}
	if c.Message != "Add a thing\n\nWhy it is added." {
		t.Errorf("message = %q", c.Message)
	}
	if len(c.Files) != 2 || c.Files[0].Name != "a.txt" || c.Files[0].Data != "one\ntwo\n" ||
		c.Files[1].Name != "src/b.ts" || c.Files[1].Data != "three\n" {
		t.Errorf("files = %+v", c.Files)
	}
	for _, bad := range []string{
		"No files at all\n",
		"-- a.txt --\nno message\n",
		"Escape\n-- ../outside.txt --\nx\n",
		"Absolute\n-- /etc/passwd --\nx\n",
	} {
		if _, err := parseCommit(bad); err == nil {
			t.Errorf("parseCommit(%q) was accepted", bad)
		}
	}
}

// A build that finds the sample already in place, because another request finished first, uses the
// one there and leaves no staging folder behind. A folder that is there but is not a repository
// is reported, not replaced.
func TestBuildKeepsWhatIsAlreadyInPlace(t *testing.T) {
	ctx := context.Background()
	commits, err := Commits()
	if err != nil {
		t.Fatal(err)
	}
	dataDir := t.TempDir()
	dst, err := Ensure(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := build(ctx, commits, dst); err != nil {
		t.Errorf("a build that lost the race failed: %v", err)
	}
	if entries, err := os.ReadDir(filepath.Dir(dst)); err != nil || len(entries) != 1 {
		t.Errorf("the sample's parent folder holds %d entries (%v), want only the sample", len(entries), err)
	}

	other := filepath.Join(t.TempDir(), "in-the-way")
	if err := os.MkdirAll(other, dirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "mine.txt"), []byte("mine\n"), fileMode); err != nil {
		t.Fatal(err)
	}
	if err := build(ctx, commits, other); !errors.Is(err, ErrNotUsable) {
		t.Errorf("a build into a folder that is not a repository = %v, want ErrNotUsable", err)
	}
	if _, err := os.Stat(filepath.Join(other, "mine.txt")); err != nil {
		t.Errorf("the folder in the way lost its file: %v", err)
	}
}

// A commit whose files cannot be written says which one, and a sample that cannot be made says
// so instead of leaving a half-made one.
func TestFailuresAreReported(t *testing.T) {
	ctx := context.Background()

	// A file and a folder of the same name cannot both be written.
	clash := Commit{Message: "Clash", Files: []File{{Name: "a", Data: "file\n"}, {Name: "a/b", Data: "inside\n"}}}
	if err := clash.commit(ctx, t.TempDir(), firstCommit()); err == nil {
		t.Error("a commit that cannot write its files was accepted")
	}

	// The sample's parent is a file, so the sample's place cannot even be looked at.
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, parentFolder), []byte("not a folder\n"), fileMode); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(ctx, dataDir); err == nil || errors.Is(err, ErrNotUsable) {
		t.Errorf("Ensure with a file in the parent's place = %v, want a plain error", err)
	}

	// A folder that cannot be written to stops the build at the first step, before Git runs.
	if os.Geteuid() == 0 {
		t.Skip("a file mode does not stop the root user")
	}
	readOnly := t.TempDir()
	if err := os.Chmod(readOnly, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnly, dirMode) })
	if _, err := Ensure(ctx, filepath.Join(readOnly, "data")); err == nil {
		t.Error("Ensure in a folder that cannot be written to made a sample")
	}
	sealed := t.TempDir()
	if err := os.MkdirAll(filepath.Join(sealed, parentFolder), dirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(sealed, parentFolder), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(sealed, parentFolder), dirMode) })
	if _, err := Ensure(ctx, sealed); err == nil {
		t.Error("Ensure with a parent folder that cannot be written to made a sample")
	}
}
