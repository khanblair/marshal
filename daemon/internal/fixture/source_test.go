package fixture

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// makeSource makes a folder that holds both fixture repositories and returns it.
func makeSource(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{smallRepo, monorepo} {
		if err := os.MkdirAll(filepath.Join(root, name), repoDirMode); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestFindSourcePicksTheFirstCandidateThatHoldsBothRepositories(t *testing.T) {
	good := makeSource(t)
	onlyOne := t.TempDir()
	if err := os.MkdirAll(filepath.Join(onlyOne, smallRepo), repoDirMode); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		candidates []string
		want       string
		wantErr    error
	}{
		{name: "the only good one", candidates: []string{good}, want: good},
		{name: "skips a missing folder", candidates: []string{filepath.Join(good, "missing"), good}, want: good},
		{name: "skips a folder with one repository", candidates: []string{onlyOne, good}, want: good},
		{name: "none at all", wantErr: ErrSourceNotFound},
		{name: "none that fit", candidates: []string{onlyOne}, wantErr: ErrSourceNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := findSource(tc.candidates)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("source = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSourceCandidatesIncludeTheCheckoutOfThisSourceFile(t *testing.T) {
	got, err := findSource(sourceCandidates())
	if err != nil {
		t.Fatalf("the checkout's fixture repositories were not found: %v", err)
	}
	if !isDir(filepath.Join(got, smallRepo)) {
		t.Errorf("%s has no %s", got, smallRepo)
	}
}

func TestCopyTreeRefusesWhatIsNotAPlainFileOrFolder(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), repoFileMode); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.txt", filepath.Join(src, "link")); err != nil {
		t.Skipf("this machine cannot make symbolic links: %v", err)
	}
	if err := copyTree(src, filepath.Join(t.TempDir(), "dst")); err == nil {
		t.Error("a symbolic link was copied")
	}
}
