package codemap

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The walk decides what the map ever sees, so it is worth a test of its own: what it leaves out is
// most of what makes the index light.

// writeTree writes every file of a tree into a fresh temporary folder and answers its root.
func writeTree(t *testing.T, files ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("make %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

func TestSourceFilesLeavesOutWhatIsNotTheProjectsOwnSource(t *testing.T) {
	root := writeTree(t,
		"a.go",
		"src/board.ts",
		"src/deep/thing.tsx",
		"node_modules/pkg/index.js",
		"vendor/lib/lib.go",
		"dist/bundle.js",
		"build/out.go",
		"target/classes/Main.java",
		".git/config",
		".venv/lib/python3.11/os.py",
		"src/.hidden.go",
		".eslintrc.json",
		"src/logo.png",
		"src/app.min.js",
		"src/bundle.bundle.js",
		"src/source.js.map",
		"go.sum",
		"README.md",
	)

	files, err := sourceFiles(root)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	want := []string{"README.md", "a.go", "src/board.ts", "src/deep/thing.tsx"}
	if !slices.Equal(files, want) {
		t.Errorf("the walk found\n%q\nwant\n%q", files, want)
	}
}

func TestSourceFilesSkipsASymlink(t *testing.T) {
	root := writeTree(t, "a.go", "real/b.go")
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "linked")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "a.go"), filepath.Join(root, "b.go")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	files, err := sourceFiles(root)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if want := []string{"a.go", "real/b.go"}; !slices.Equal(files, want) {
		t.Errorf("the walk found %q, want every real file once: %q", files, want)
	}
}

// TestSourceFilesSkipsAFileTooLargeToBeHandWritten: a megabyte of source is a bundle or a
// generated file, and handing it to ctags costs time for answers nobody wants.
func TestSourceFilesSkipsAFileTooLargeToBeHandWritten(t *testing.T) {
	root := writeTree(t, "small.go")
	big := filepath.Join(root, "big.go")
	if err := os.WriteFile(big, []byte(strings.Repeat("// padding\n", 120_000)), 0o644); err != nil {
		t.Fatalf("write the big file: %v", err)
	}

	files, err := sourceFiles(root)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if want := []string{"small.go"}; !slices.Equal(files, want) {
		t.Errorf("the walk found %q, want only the file small enough to be source: %q", files, want)
	}
}

func TestSourceFilesReportsAFolderThatIsNotThere(t *testing.T) {
	if _, err := sourceFiles(filepath.Join(t.TempDir(), "gone")); err == nil {
		t.Fatal("a folder that is not there was walked as if it were empty")
	}
}

func TestHasSuffixFoldIgnoresCase(t *testing.T) {
	if !hasSuffixFold("App.MIN.js", skipSuffix) {
		t.Error("a bundled file written in capitals was not recognised")
	}
	if hasSuffixFold("app.js", skipSuffix) {
		t.Error("an ordinary file was skipped as a bundle")
	}
}
