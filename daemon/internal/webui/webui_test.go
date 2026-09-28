package webui

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

// dirEntriesOf turns a MapFS's root listing into the []fs.DirEntry hasRealBuild reads, without
// this package's own real embed - a fake filesystem stands in for whatever dist/ actually holds
// on the machine running the test.
func dirEntriesOf(t *testing.T, files fstest.MapFS) []fs.DirEntry {
	t.Helper()
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		t.Fatalf("read the fake root: %v", err)
	}
	return entries
}

func TestHasRealBuildIsFalseWithOnlyThePlaceholder(t *testing.T) {
	entries := dirEntriesOf(t, fstest.MapFS{".gitkeep": {}})
	if hasRealBuild(entries) {
		t.Error("hasRealBuild = true with only .gitkeep, want false")
	}
}

func TestHasRealBuildIsTrueOnceABuildIsCopiedIn(t *testing.T) {
	entries := dirEntriesOf(t, fstest.MapFS{
		".gitkeep":   {},
		"index.html": {Data: []byte("<!doctype html>")},
	})
	if !hasRealBuild(entries) {
		t.Error("hasRealBuild = false with index.html present, want true")
	}
}

func TestHasRealBuildIsFalseWithNothingAtAll(t *testing.T) {
	if hasRealBuild(nil) {
		t.Error("hasRealBuild(nil) = true, want false")
	}
}

// TestFSServesTheEmbeddedDirectory covers FS() itself: it must resolve to dist/ inside the
// embed, whatever dist/ currently holds on this machine, so a caller can always list or open it.
func TestFSServesTheEmbeddedDirectory(t *testing.T) {
	if _, err := fs.ReadDir(FS(), "."); err != nil {
		t.Errorf("read the embedded dist/: %v", err)
	}
}

// TestAvailableReadsTheRealEmbed covers Available() end to end, against whatever dist/ actually
// holds on this machine: hasRealBuild's own tests above cover the two answers on a fake tree, this
// just proves Available() wires FS() and fs.ReadDir into it without panicking or erroring.
func TestAvailableReadsTheRealEmbed(t *testing.T) {
	entries, err := fs.ReadDir(FS(), ".")
	if err != nil {
		t.Fatalf("read the embedded dist/: %v", err)
	}
	if got, want := Available(), hasRealBuild(entries); got != want {
		t.Errorf("Available() = %v, want %v to match the embed's own listing", got, want)
	}
}
