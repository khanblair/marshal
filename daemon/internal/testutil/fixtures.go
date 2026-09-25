package testutil

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
)

const fixtureFileMode = 0o644

// testGit is a Git with an identity, and no user or system configuration, so a fixture repo
// commits the same way on every machine. A CI runner has no Git identity of its own.
func testGit() *gitx.Git {
	return gitx.New(gitx.WithEnv(
		"GIT_AUTHOR_NAME=Marshal Test", "GIT_AUTHOR_EMAIL=test@marshal.invalid",
		"GIT_COMMITTER_NAME=Marshal Test", "GIT_COMMITTER_EMAIL=test@marshal.invalid",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
	))
}

// Fixture copies daemon/testdata/repos/<name> into a temp folder, makes it a Git repository with
// one commit on the main branch, and returns its path. The fixture itself is never changed. The
// fixtures are stored as plain files, because a repository cannot contain another one.
func Fixture(t testing.TB, name string) string {
	t.Helper()
	src := TestdataPath(t, "repos", name)
	dst := filepath.Join(t.TempDir(), name)
	copyTree(t, src, dst)
	ctx := context.Background()
	git := testGit()
	for _, args := range [][]string{
		{"init", "--quiet", "--initial-branch=main"},
		{"add", "--all"},
		{"-c", "commit.gpgsign=false", "commit", "--quiet", "--message", "Fixture: " + name},
	} {
		if _, err := git.Run(ctx, dst, args...); err != nil {
			t.Fatalf("prepare fixture %s: %v", name, err)
		}
	}
	return dst
}

func copyTree(t testing.TB, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, dirMode)
		}
		return copyFile(path, target)
	})
	if err != nil {
		t.Fatalf("copy fixture %s: %v", src, err)
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fixtureFileMode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
