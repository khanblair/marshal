package gitx

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// CloneOptions changes how Clone works.
type CloneOptions struct {
	// Branch is the branch to check out. Leave it empty for the repository's default branch.
	Branch string
	// AllowLocal lets the address be a folder or a file:// address. Tests and dev mode use it.
	// A user or a chat never should, because it reads any repository on this machine.
	AllowLocal bool
}

// Clone copies a repository into dest, which must be a full path that is missing or is an empty
// folder. The address is checked with ParseCloneURL first, and the context bounds the time. When
// the address holds a token, the token is kept out of every error and out of the new
// repository's configuration.
func (g *Git) Clone(ctx context.Context, rawURL, dest string, opts CloneOptions) error {
	u, err := ParseCloneURL(rawURL, opts.AllowLocal)
	if err != nil {
		return err
	}
	dir, err := checkAbsolute(dest)
	if err != nil {
		return err
	}
	existed, err := emptyOrMissing(dir)
	if errors.Is(err, errFolderNotEmpty) {
		return newOpError(ErrBadPath, "that folder already has files in it", nil)
	}
	if err != nil {
		return err
	}
	if opts.Branch != "" {
		if err := g.ValidBranchName(ctx, opts.Branch); err != nil {
			return err
		}
	}
	if _, err := g.Run(ctx, "", cloneArgs(u, dir, opts)...); err != nil {
		err = fmt.Errorf("clone %s: %w", u.Redacted, redactError(err, u))
		return errors.Join(err, discardClone(dir, existed))
	}
	if err := g.finishClone(ctx, u, dir); err != nil {
		return errors.Join(err, discardClone(dir, existed))
	}
	return nil
}

// cloneArgs builds the arguments for `git clone`. Only the protocols that are needed are allowed,
// so a redirect or a helper cannot send Git somewhere else, and the address and folder come after
// "--", so neither can be read as an option.
func cloneArgs(u CloneURL, dest string, opts CloneOptions) []string {
	file := "protocol.file.allow=never"
	if opts.AllowLocal {
		file = "protocol.file.allow=always"
	}
	args := []string{
		"-c", "protocol.allow=never",
		"-c", "protocol.ext.allow=never",
		"-c", "protocol.https.allow=always",
		"-c", "protocol.ssh.allow=always",
		"-c", file,
	}
	if u.HasCredentials {
		// After a successful request Git offers the user and password in the address to every
		// credential helper, and one of them can write a token to disk in plain text. An empty
		// helper clears the list, so the token stays with the caller and the keychain module.
		args = append(args, "-c", "credential.helper=")
	}
	args = append(args, "clone", "--quiet")
	if opts.Branch != "" {
		args = append(args, "--branch", opts.Branch)
	}
	return append(args, "--", u.Raw, dest)
}

// finishClone keeps a token out of the new repository, and checks that the clone is a repository.
func (g *Git) finishClone(ctx context.Context, u CloneURL, dir string) error {
	if u.HasCredentials {
		// Git keeps the address it cloned from in the repository's own configuration, in plain text.
		if _, err := g.Run(ctx, dir, "remote", "set-url", "origin", u.Redacted); err != nil {
			return fmt.Errorf("remove the credentials from the clone: %w", redactError(err, u))
		}
	}
	if _, err := g.Inspect(ctx, dir); err != nil {
		return fmt.Errorf("check the clone: %w", err)
	}
	return nil
}

// redactError returns a copy of a Git error with credentials removed from what it quotes, since
// Git's errors carry the arguments it was run with and what it printed. Other errors are
// returned as they are.
func redactError(err error, known CloneURL) error {
	var gitErr *Error
	if !errors.As(err, &gitErr) {
		return err
	}
	args := make([]string, len(gitErr.Args))
	for i, arg := range gitErr.Args {
		args[i] = redactText(arg, known)
	}
	return &Error{Args: args, Stderr: redactText(gitErr.Stderr, known), Err: gitErr.Err}
}

// discardClone removes what a clone that failed or was cancelled left behind, so the destination
// is free for another try. Git cleans up after itself, but not when it is killed. A folder that
// was there before is put back empty.
func discardClone(dir string, existed bool) error {
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove what the clone left in %s: %w", dir, err)
	}
	if existed {
		if err := os.Mkdir(dir, dirMode); err != nil {
			return fmt.Errorf("put back the empty folder %s: %w", dir, err)
		}
	}
	return nil
}
