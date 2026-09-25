package fixture

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/khanblair/marshal/daemon/internal/gitx"
)

const (
	// The folder and file modes of a fixture repository. It lives inside the data folder, which
	// only its owner can enter, so it gets the same care as the rest of what Marshal stores.
	repoDirMode  = 0o750
	repoFileMode = 0o600

	// stagingPrefix starts the name of the folder a repository is built in before it is moved to
	// its final name. A start that is killed part way leaves only a staging folder, never a
	// half-made repository under the real name.
	stagingPrefix = ".staging-"

	// commitDate is the date of the fixture commit. With a fixed identity and a fixed date, the
	// same files make the same commit on every machine.
	commitDate = "2026-01-01T00:00:00Z"
)

// ErrSourceNotFound means the folder with the fixture repositories (daemon/testdata/repos) could
// not be found.
var ErrSourceNotFound = errors.New("the fixture repositories were not found")

// fixtureGit is a Git with a fixed identity and no user or system configuration, so a fixture
// commit is the same on every machine and does not depend on the person's own Git identity. It is
// only used to make the fixture repositories; the projects module has its own Git.
func fixtureGit() *gitx.Git {
	return gitx.New(gitx.WithEnv(
		"GIT_AUTHOR_NAME=Marshal Fixture", "GIT_AUTHOR_EMAIL=fixture@marshal.invalid",
		"GIT_COMMITTER_NAME=Marshal Fixture", "GIT_COMMITTER_EMAIL=fixture@marshal.invalid",
		"GIT_AUTHOR_DATE="+commitDate, "GIT_COMMITTER_DATE="+commitDate,
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
	))
}

// ensureRepo returns the folder of the seed's repository, making it first when it is not there.
// A folder that is already a repository with a commit is used as it is, so a second start does
// not change anything. A folder that is there but is not one is reported rather than replaced:
// it is not Marshal's to delete.
func (l *loader) ensureRepo(ctx context.Context, s seed) (string, error) {
	fixtures := filepath.Join(l.dataDir, fixturesFolder)
	if err := os.MkdirAll(fixtures, repoDirMode); err != nil {
		return "", fmt.Errorf("make the fixtures folder: %w", err)
	}
	dst := filepath.Join(fixtures, s.name)
	_, err := os.Stat(dst)
	switch {
	case err == nil:
		if err := l.checkRepo(ctx, dst); err != nil {
			return "", err
		}
		return dst, nil
	case !errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("look at the fixture folder: %w", err)
	}
	src, err := l.source(s.source)
	if err != nil {
		return "", err
	}
	if err := l.build(ctx, src, filepath.Join(fixtures, stagingPrefix+s.name), dst); err != nil {
		return "", fmt.Errorf("make the %s repository: %w", s.name, err)
	}
	l.log.Info("made a fixture repository", "project_id", s.id, "source", s.source)
	return dst, nil
}

// checkRepo confirms that an existing folder is a repository with at least one commit.
func (l *loader) checkRepo(ctx context.Context, dir string) error {
	info, err := l.git.Inspect(ctx, dir)
	if err != nil {
		return fmt.Errorf("the fixture folder %s exists but is not a usable repository, so delete it and start again: %w", dir, err)
	}
	if !info.HasCommits {
		return fmt.Errorf("the fixture folder %s is a repository with no commits, so delete it and start again", dir)
	}
	return nil
}

// build copies src into stage, makes it a repository with one commit, and moves it to dst. The
// staging folder is removed whatever happens, so nothing is left behind after a failure.
func (l *loader) build(ctx context.Context, src, stage, dst string) (err error) {
	// A staging folder left by an earlier start that was killed is of no use: start clean.
	if err := os.RemoveAll(stage); err != nil {
		return fmt.Errorf("clear an old staging folder: %w", err)
	}
	defer func() {
		if cleanErr := os.RemoveAll(stage); cleanErr != nil {
			err = errors.Join(err, fmt.Errorf("remove the staging folder: %w", cleanErr))
		}
	}()
	if err := copyTree(src, stage); err != nil {
		return fmt.Errorf("copy the files: %w", err)
	}
	if err := l.commitAll(ctx, stage, filepath.Base(dst)); err != nil {
		return err
	}
	if err := os.Rename(stage, dst); err != nil {
		return fmt.Errorf("move the repository into place: %w", err)
	}
	return nil
}

// commitAll makes dir a repository on the main branch with one commit of everything in it.
func (l *loader) commitAll(ctx context.Context, dir, name string) error {
	for _, args := range [][]string{
		{"init", "--quiet", "--initial-branch=main"},
		{"add", "--all"},
		{"-c", "commit.gpgsign=false", "commit", "--quiet", "--message", "Fixture: " + name},
	} {
		if _, err := l.git.Run(ctx, dir, args...); err != nil {
			return fmt.Errorf("commit the fixture files: %w", err)
		}
	}
	return nil
}

// source returns the folder of one fixture repository under the source folder.
func (l *loader) source(name string) (string, error) {
	root := l.sourceDir
	if root == "" {
		var err error
		if root, err = findSource(sourceCandidates()); err != nil {
			return "", err
		}
	}
	dir := filepath.Join(root, name)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: there is no %s folder in %s", ErrSourceNotFound, name, root)
	}
	return dir, nil
}

// sourceCandidates are the places daemon/testdata/repos may be, most likely first:
//   - next to this source file, which is right for a daemon built from a checkout on this machine
//     (pnpm dev, pnpm build, go run), wherever the program is then started from;
//   - in the checkout that the program's own folder belongs to (dist/bin/marshald is two folders
//     below the checkout's top folder), which is right when the checkout was moved after the build
//     or the build had its source paths trimmed.
func sourceCandidates() []string {
	var found []string
	if _, file, _, ok := runtime.Caller(0); ok {
		found = append(found, filepath.Join(filepath.Dir(file), "..", "..", "testdata", "repos"))
	}
	if exe, err := os.Executable(); err == nil {
		found = append(found, filepath.Join(filepath.Dir(exe), "..", "..", "daemon", "testdata", "repos"))
	}
	return found
}

// findSource returns the first candidate that holds both fixture repositories.
func findSource(candidates []string) (string, error) {
	for _, dir := range candidates {
		if isDir(filepath.Join(dir, smallRepo)) && isDir(filepath.Join(dir, monorepo)) {
			return filepath.Clean(dir), nil
		}
	}
	return "", fmt.Errorf("%w: run the daemon built from a checkout of the Marshal repository, which has daemon/testdata/repos",
		ErrSourceNotFound)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// copyTree copies the folder src to dst. It copies files and folders only: a fixture holds
// nothing else, so anything else is a mistake worth reporting.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case entry.IsDir():
			return os.MkdirAll(target, repoDirMode)
		case entry.Type().IsRegular():
			return copyFile(path, target)
		default:
			return fmt.Errorf("%s is not a plain file or folder", rel)
		}
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, repoFileMode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
