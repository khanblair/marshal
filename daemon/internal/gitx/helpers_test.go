package gitx_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/goleak"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// testGit is a Git with an identity and no user or system configuration, so tests behave the same
// on every machine. A CI runner has no Git identity, and a developer's own settings, such as a
// hooks folder or signed commits, must not leak in.
func testGit() *gitx.Git {
	return gitx.New(gitx.WithEnv(
		"GIT_AUTHOR_NAME=Marshal Test", "GIT_AUTHOR_EMAIL=test@marshal.invalid",
		"GIT_COMMITTER_NAME=Marshal Test", "GIT_COMMITTER_EMAIL=test@marshal.invalid",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
	))
}

// fixture copies a fixture repository into a temp folder and returns its path.
func fixture(t *testing.T, name string) string {
	t.Helper()
	return testutil.Fixture(t, name)
}

// git runs a Git command that must succeed, and returns what it printed.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := testGit().Run(context.Background(), dir, args...)
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return out
}

// commitFile writes a file in a repository or worktree and commits it.
func commitFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "--all")
	git(t, dir, "-c", "commit.gpgsign=false", "commit", "--quiet", "--message", "Add "+name)
}

// newEmptyRepo makes a repository with no commits, with main as its first branch.
func newEmptyRepo(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "empty")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "--quiet", "--initial-branch=main")
	return dir
}

// sameFolder reports whether two paths lead to the same folder, however they are spelled.
func sameFolder(t *testing.T, a, b string) bool {
	t.Helper()
	infoA, err := os.Stat(a)
	if err != nil {
		t.Fatalf("stat %s: %v", a, err)
	}
	infoB, err := os.Stat(b)
	if err != nil {
		t.Fatalf("stat %s: %v", b, err)
	}
	return os.SameFile(infoA, infoB)
}

// exists reports whether a path is there.
func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// symlink makes a link, and skips the test when the machine does not allow links, as a Windows
// machine without developer mode does not.
func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("this machine cannot make symbolic links: %v", err)
	}
}
