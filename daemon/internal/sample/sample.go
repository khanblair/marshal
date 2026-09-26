// Package sample makes the sample repository that a person can add as a project to try Marshal on
// (docs/backend-checklist.md B2.12, docs/backend-inventory.md N24): a small todo service in
// TypeScript, with a README and a short history, that is safe to try things on. It ships inside
// the daemon, so it needs no network and no checkout of the Marshal repository, and it is written
// into the data folder the first time someone asks for it.
//
// Each commit is one txtar file under commits/, in name order. The text before the first file is
// the commit message, and every file after it is written, made or replaced, before the commit.
// The commits are made by a Git with a fixed identity, fixed dates, and none of the person's own
// Git settings, so the sample needs no Git identity on the machine, never records the person's,
// and is the same repository, commit for commit, on every machine. The identity is only in the
// environment of those commits: nothing is written into the sample's Git configuration, so the
// person's own commits in it, and in its card worktrees, carry their own name as usual.
package sample

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/gitx"
)

const (
	// FolderName is the name of the sample's folder, and so the name a project made from it gets
	// when the request names none.
	FolderName = "marshal-sample"
	// parentFolder is the folder under the data folder that holds the sample.
	parentFolder = "sample"
	// Branch is the sample's only branch.
	Branch = "main"

	// The sample lives inside the data folder, which only its owner can enter, so its files get the
	// same care as the rest of what Marshal stores. Git records them as ordinary files either way.
	dirMode  = 0o750
	fileMode = 0o600

	// stagingPrefix starts the name of the folder the sample is built in before it is moved to its
	// final name. A start that is killed part way leaves only a staging folder, never a half-made
	// sample under the real name.
	stagingPrefix = ".staging-"

	// authorName and authorEmail are who the sample's commits are by. The address is on a domain
	// that cannot exist, so it never reaches anyone.
	authorName  = "Marshal"
	authorEmail = "sample@marshal.invalid"
)

// firstCommit is when the first commit of the sample was made. Each later one is a day after the
// one before, so the history reads like a real one and is the same on every machine.
func firstCommit() time.Time {
	return time.Date(2026, time.January, 5, 9, 0, 0, 0, time.UTC)
}

// ErrNotUsable means the sample's folder is there but is not a repository with commits. It is left
// as it is: it may hold a person's work, so it is not Marshal's to delete.
var ErrNotUsable = errors.New("the sample folder is there but is not a usable repository")

//go:embed commits/*.txtar
var commitFiles embed.FS

// Folder is where the sample repository lives under the data folder.
func Folder(dataDir string) string {
	return filepath.Join(dataDir, parentFolder, FolderName)
}

// Ensure returns the top folder of the sample repository under dataDir, making it first when it
// is not there. A folder that is already a repository with a commit is used as it is, with
// whatever was done in it since, so asking twice changes nothing. A folder that is there but is
// not one is reported with ErrNotUsable. Two calls at once are safe: each builds in its own
// staging folder, and the one that finishes second uses the sample the first one made.
func Ensure(ctx context.Context, dataDir string) (string, error) {
	dst := Folder(dataDir)
	_, err := os.Stat(dst)
	switch {
	case err == nil:
		if err := checkRepo(ctx, dst); err != nil {
			return "", err
		}
		return dst, nil
	case !errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("look at the sample folder: %w", err)
	}
	commits, err := Commits()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dst), dirMode); err != nil {
		return "", fmt.Errorf("make the sample's parent folder: %w", err)
	}
	if err := build(ctx, commits, dst); err != nil {
		return "", fmt.Errorf("make the sample repository: %w", err)
	}
	return dst, nil
}

// checkRepo confirms that an existing folder is a repository with at least one commit.
func checkRepo(ctx context.Context, dir string) error {
	info, err := sampleGit(firstCommit()).Inspect(ctx, dir)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNotUsable, err)
	}
	if !info.HasCommits {
		return fmt.Errorf("%w: it has no commits", ErrNotUsable)
	}
	return nil
}

// build makes the repository in a staging folder next to dst and moves it into place. The staging
// folder is removed whatever happens. When another call put a sample in place first, that one is
// kept and used.
func build(ctx context.Context, commits []Commit, dst string) (err error) {
	stage, err := os.MkdirTemp(filepath.Dir(dst), stagingPrefix)
	if err != nil {
		return fmt.Errorf("make a staging folder: %w", err)
	}
	defer func() {
		if cleanErr := os.RemoveAll(stage); cleanErr != nil {
			err = errors.Join(err, fmt.Errorf("remove the staging folder: %w", cleanErr))
		}
	}()
	if err := os.Chmod(stage, dirMode); err != nil {
		return fmt.Errorf("set the staging folder's mode: %w", err)
	}
	if _, err := sampleGit(firstCommit()).Run(ctx, stage, "init", "--quiet", "--initial-branch="+Branch); err != nil {
		return fmt.Errorf("start the repository: %w", err)
	}
	for i, c := range commits {
		if err := c.commit(ctx, stage, firstCommit().AddDate(0, 0, i)); err != nil {
			return err
		}
	}
	if err := os.Rename(stage, dst); err != nil {
		if _, statErr := os.Stat(dst); statErr == nil {
			return checkRepo(ctx, dst)
		}
		return fmt.Errorf("move the sample into place: %w", err)
	}
	return nil
}

// commit writes the commit's files into the repository at dir and commits them, dated at.
func (c Commit) commit(ctx context.Context, dir string, at time.Time) error {
	for _, f := range c.Files {
		target := filepath.Join(dir, filepath.FromSlash(f.Name))
		if err := os.MkdirAll(filepath.Dir(target), dirMode); err != nil {
			return fmt.Errorf("make the folder of %s: %w", f.Name, err)
		}
		if err := os.WriteFile(target, []byte(f.Data), fileMode); err != nil {
			return fmt.Errorf("write %s: %w", f.Name, err)
		}
	}
	git := sampleGit(at)
	if _, err := git.Run(ctx, dir, "add", "--all"); err != nil {
		return fmt.Errorf("stage the files of %q: %w", c.Message, err)
	}
	if _, err := git.Run(ctx, dir, "-c", "commit.gpgsign=false", "commit", "--quiet", "--message", c.Message); err != nil {
		return fmt.Errorf("commit %q: %w", c.Message, err)
	}
	return nil
}

// sampleGit is a Git with the sample's identity and a fixed date, and without the machine's own
// Git settings: no hooks, no signing, no templates of the person's run while the sample is made.
func sampleGit(at time.Time) *gitx.Git {
	date := at.Format(time.RFC3339)
	return gitx.New(gitx.WithEnv(
		"GIT_AUTHOR_NAME="+authorName, "GIT_AUTHOR_EMAIL="+authorEmail,
		"GIT_COMMITTER_NAME="+authorName, "GIT_COMMITTER_EMAIL="+authorEmail,
		"GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date,
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
	))
}

// Commit is one commit of the sample: its message and the files it writes.
type Commit struct {
	// Message is the whole commit message: its subject line, and a body after a blank line.
	Message string
	// Files are the files the commit makes or replaces, in the order the txtar file lists them.
	Files []File
}

// File is one file of a commit.
type File struct {
	// Name is the file's path in the repository, with forward slashes.
	Name string
	// Data is the file's whole content.
	Data string
}

// Commits returns the sample's commits, oldest first.
func Commits() ([]Commit, error) {
	entries, err := fs.ReadDir(commitFiles, "commits")
	if err != nil {
		return nil, fmt.Errorf("list the sample's commits: %w", err)
	}
	commits := make([]Commit, 0, len(entries))
	for _, entry := range entries {
		data, err := fs.ReadFile(commitFiles, path.Join("commits", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read the sample commit %s: %w", entry.Name(), err)
		}
		c, err := parseCommit(string(data))
		if err != nil {
			return nil, fmt.Errorf("read the sample commit %s: %w", entry.Name(), err)
		}
		commits = append(commits, c)
	}
	return commits, nil
}

// parseCommit reads one txtar file: the text before the first "-- name --" line is the message,
// and each marker starts a file that runs to the next marker.
func parseCommit(text string) (Commit, error) {
	var c Commit
	var message strings.Builder
	for _, line := range strings.SplitAfter(text, "\n") {
		if name, ok := markerName(line); ok {
			if !filepath.IsLocal(filepath.FromSlash(name)) {
				return Commit{}, fmt.Errorf("the file %q is not inside the repository", name)
			}
			c.Files = append(c.Files, File{Name: name})
			continue
		}
		if len(c.Files) == 0 {
			message.WriteString(line)
		} else {
			c.Files[len(c.Files)-1].Data += line
		}
	}
	c.Message = strings.TrimSpace(message.String())
	if c.Message == "" || len(c.Files) == 0 {
		return Commit{}, errors.New("a sample commit needs a message and at least one file")
	}
	return c, nil
}

// markerName reads a txtar file marker, "-- name --", and returns the name in it.
func markerName(line string) (string, bool) {
	line = strings.TrimRight(line, "\r\n")
	name, ok := strings.CutPrefix(line, "-- ")
	if !ok {
		return "", false
	}
	name, ok = strings.CutSuffix(name, " --")
	name = strings.TrimSpace(name)
	return name, ok && name != ""
}
