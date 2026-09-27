package gitx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// fileMode is the permission of a file Marshals writes itself.
const fileMode = 0o644

// CommitFile writes one file inside a working folder and commits it, answering the new commit's id.
// It is how the real mode of a simulated CI failure marks a card's branch: one file, one commit,
// and nothing else in the working folder is touched.
//
// The path is always relative to the working folder and may not leave it, so a caller cannot write
// outside the card's own worktree by mistake. A file whose contents are what the branch already
// holds is not a commit - committing nothing would answer a commit id for a change that does not
// exist - so the commit the branch is already on is answered instead, and the caller can say the
// branch was already marked.
//
// The repository's own hooks run: this is Marshal committing, not an agent, and the secret scanner
// a project has set up is as much a check on Marshal as on anyone. A hook that refuses the commit
// is returned to the caller.
func (g *Git) CommitFile(ctx context.Context, dir, path, content, message string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", newOpError(ErrBadPath, "a file needs a working folder", nil)
	}
	if strings.TrimSpace(message) == "" {
		return "", newOpError(ErrBadPath, "a commit needs a message", nil)
	}
	relative, err := worktreeFile(path)
	if err != nil {
		return "", err
	}
	full := filepath.Join(dir, filepath.FromSlash(relative))
	if parent := filepath.Dir(full); parent != "" {
		if err := os.MkdirAll(parent, dirMode); err != nil {
			return "", newOpError(ErrBadPath, "the file's folder cannot be made", err)
		}
	}
	if err := os.WriteFile(full, []byte(content), fileMode); err != nil {
		return "", newOpError(ErrBadPath, "the file cannot be written", err)
	}
	if _, err := g.Run(ctx, dir, "add", "--", relative); err != nil {
		return "", err
	}
	// `git diff --cached --quiet` answers with its exit code: 0 for a staging area that holds the
	// commit already, 1 for one with something in it.
	unchanged, err := g.ask(ctx, dir, "diff", "--cached", "--quiet")
	if err != nil {
		return "", err
	}
	if !unchanged {
		if _, err := g.Run(ctx, dir, "commit", "-m", message, "--", relative); err != nil {
			return "", err
		}
	}
	return g.Run(ctx, dir, "rev-parse", "HEAD")
}

// worktreeFile checks a file's path inside a working folder and answers it with forward slashes, the
// way Git names a path. It refuses an absolute path and any ".." segment, so what is written is
// always inside the folder it was given.
func worktreeFile(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", newOpError(ErrBadPath, "the file needs a path", nil)
	}
	if filepath.IsAbs(path) {
		return "", newOpError(ErrBadPath, "the file's path must be relative to the working folder", nil)
	}
	for _, part := range strings.FieldsFunc(path, isSeparator) {
		if part == ".." {
			return "", newOpError(ErrBadPath, "the file's path must not contain \"..\"", nil)
		}
	}
	return filepath.ToSlash(filepath.Clean(path)), nil
}
