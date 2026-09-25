package gitx_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/gitx"
)

var local = gitx.CloneOptions{AllowLocal: true}

func TestCloneFromALocalRepository(t *testing.T) {
	ctx := context.Background()
	src := fixture(t, "small-repo")
	dest := filepath.Join(t.TempDir(), "clones", "my copy")
	if err := testGit().Clone(ctx, src, dest, local); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "src", "util.js")); err != nil {
		t.Errorf("the clone is missing a file: %v", err)
	}
	info, err := testGit().Inspect(ctx, dest)
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasCommits || !info.Clean || info.CurrentBranch != "main" || info.DefaultBranch != "main" {
		t.Errorf("Inspect(clone) = %+v", info)
	}
	if len(info.Remotes) != 1 || info.Remotes[0].Name != "origin" {
		t.Errorf("Remotes = %+v, want origin", info.Remotes)
	}
}

func TestCloneFromAFileAddress(t *testing.T) {
	src := fixture(t, "small-repo")
	address := "file://" + filepath.ToSlash(src)
	if !strings.HasPrefix(filepath.ToSlash(src), "/") {
		address = "file:///" + filepath.ToSlash(src)
	}
	dest := filepath.Join(t.TempDir(), "copy")
	if err := testGit().Clone(context.Background(), address, dest, local); err != nil {
		t.Fatalf("Clone(%q): %v", address, err)
	}
}

func TestCloneIntoAnEmptyFolder(t *testing.T) {
	src := fixture(t, "small-repo")
	dest := filepath.Join(t.TempDir(), "ready")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := testGit().Clone(context.Background(), src, dest+string(os.PathSeparator), local); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if !exists(filepath.Join(dest, "README.md")) {
		t.Error("the clone is empty")
	}
}

func TestCloneABranch(t *testing.T) {
	ctx := context.Background()
	src := fixture(t, "small-repo")
	git(t, src, "switch", "--quiet", "--create", "release/1.0")
	commitFile(t, src, "release.txt", "release")
	git(t, src, "switch", "--quiet", "main")

	dest := filepath.Join(t.TempDir(), "copy")
	if err := testGit().Clone(ctx, src, dest, gitx.CloneOptions{Branch: "release/1.0", AllowLocal: true}); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if !exists(filepath.Join(dest, "release.txt")) {
		t.Error("the clone is not on the branch")
	}

	missing := filepath.Join(t.TempDir(), "missing")
	err := testGit().Clone(ctx, src, missing, gitx.CloneOptions{Branch: "no-such-branch", AllowLocal: true})
	if err == nil || errors.Is(err, gitx.ErrBadBranchName) {
		t.Errorf("Clone of a missing branch = %v, want a plain failure", err)
	}
	if exists(missing) {
		t.Error("a failed clone left its folder behind")
	}
	if err := testGit().Clone(ctx, src, missing, gitx.CloneOptions{Branch: "--upload-pack=x", AllowLocal: true}); !errors.Is(err, gitx.ErrBadBranchName) {
		t.Errorf("Clone with an option as the branch = %v, want ErrBadBranchName", err)
	}
}

func TestCloneRefusesBadDestinations(t *testing.T) {
	src := fixture(t, "small-repo")
	taken := filepath.Join(t.TempDir(), "taken")
	if err := os.Mkdir(taken, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(taken, "mine.txt"), "mine")
	file := filepath.Join(t.TempDir(), "file")
	writeFile(t, file, "a file")
	sep := string(os.PathSeparator)
	tests := map[string]string{
		"a folder with files": taken,
		"a file":              file,
		"a relative path":     "copy",
		"an empty path":       "",
		"a dot dot segment":   filepath.Join(t.TempDir(), "a") + sep + ".." + sep + "b",
	}
	for name, dest := range tests {
		t.Run(name, func(t *testing.T) {
			err := testGit().Clone(context.Background(), src, dest, local)
			if !errors.Is(err, gitx.ErrBadPath) {
				t.Errorf("Clone into %q = %v, want ErrBadPath", dest, err)
			}
			assertPlainMessage(t, err)
		})
	}
	if !exists(filepath.Join(taken, "mine.txt")) {
		t.Error("a refused clone touched the folder that was in the way")
	}
}

func TestCloneRefusesLocalAddressesUnlessAllowed(t *testing.T) {
	src := fixture(t, "small-repo")
	dest := filepath.Join(t.TempDir(), "copy")
	for _, address := range []string{src, "file://" + src, "ext::sh -c touch% " + dest, "--upload-pack=touch " + dest} {
		err := testGit().Clone(context.Background(), address, dest, gitx.CloneOptions{})
		if !errors.Is(err, gitx.ErrBadURL) {
			t.Errorf("Clone(%q) = %v, want ErrBadURL", address, err)
		}
	}
	if exists(dest) {
		t.Error("a refused address still created the destination")
	}
}

func TestCloneStopsWhenTheContextIsCancelled(t *testing.T) {
	src := fixture(t, "small-repo")
	dest := filepath.Join(t.TempDir(), "copy")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := testGit().Clone(ctx, src, dest, local)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Clone = %v, want a cancel", err)
	}
	if exists(dest) {
		t.Error("a cancelled clone left its folder behind")
	}
}

func TestFailedCloneLeavesAnExistingEmptyFolderEmpty(t *testing.T) {
	src := fixture(t, "small-repo")
	dest := filepath.Join(t.TempDir(), "ready")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	err := testGit().Clone(context.Background(), src, dest, gitx.CloneOptions{Branch: "no-such-branch", AllowLocal: true})
	if err == nil {
		t.Fatal("Clone of a missing branch worked")
	}
	entries, readErr := os.ReadDir(dest)
	if readErr != nil || len(entries) != 0 {
		t.Errorf("the folder = %v, %v; want it to exist and be empty", entries, readErr)
	}
}

// closedPort returns a port on this machine where nothing is listening, so a connection to it
// fails quickly without using the network.
func closedPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot open a local port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func TestCloneErrorsNeverShowCredentials(t *testing.T) {
	const token = "s3cr3tT0kenValue"
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	port := closedPort(t)
	addresses := []string{
		fmt.Sprintf("https://someone:%s@127.0.0.1:%d/acme/web.git", token, port),
		fmt.Sprintf("https://%s@127.0.0.1:%d/acme/web.git", token, port),
	}
	for _, address := range addresses {
		dest := filepath.Join(t.TempDir(), "copy")
		err := testGit().Clone(ctx, address, dest, gitx.CloneOptions{})
		if err == nil {
			t.Fatalf("Clone(%q) worked with nothing listening", address)
		}
		if strings.Contains(err.Error(), token) {
			t.Errorf("the error shows the token: %v", err)
		}
		var gitErr *gitx.Error
		if !errors.As(err, &gitErr) {
			t.Fatalf("error %v does not wrap a Git error", err)
		}
		if strings.Contains(strings.Join(gitErr.Args, " "), token) || strings.Contains(gitErr.Stderr, token) {
			t.Errorf("the Git error underneath shows the token: %+v", gitErr)
		}
		if !strings.Contains(err.Error(), "127.0.0.1") {
			t.Errorf("the error should still say where it tried: %v", err)
		}
		if exists(dest) {
			t.Error("a failed clone left its folder behind")
		}
	}
}
