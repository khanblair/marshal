package gitx

import (
	"context"
	"errors"
	"os/exec"
)

// The cases a caller has to handle. Match them with errors.Is. Their text is a plain sentence, and
// the API layer turns them into the sentences a user sees. The Git error underneath, when there
// is one, is still reachable with errors.As, for the logs.
var (
	// ErrNotARepo means a folder is not the top folder of a Git working tree.
	ErrNotARepo = errors.New("this folder is not the top folder of a Git repository")
	// ErrBranchExists means a branch with that name is already there.
	ErrBranchExists = errors.New("a branch with that name already exists")
	// ErrBadBranchName means a name cannot be a branch name.
	ErrBadBranchName = errors.New("that is not a usable branch name")
	// ErrWorktreeExists means the place for a new worktree is already taken.
	ErrWorktreeExists = errors.New("a worktree or another folder is already at that place")
	// ErrDirty means a folder holds changes that are not committed, and would be lost.
	ErrDirty = errors.New("there are changes that are not committed")
	// ErrOutsideRoot means a path is not inside the folder Marshal keeps its worktrees in.
	ErrOutsideRoot = errors.New("that path is outside the worktrees folder")
	// ErrBadURL means an address is not one Marshal will clone from.
	ErrBadURL = errors.New("that is not an address Marshal can clone from")
	// ErrBadPath means a path cannot be used for what it was asked to do.
	ErrBadPath = errors.New("that path cannot be used")
)

// opError is one of the sentinel errors with a short detail and the error underneath.
type opError struct {
	kind   error
	detail string
	cause  error
}

func newOpError(kind error, detail string, cause error) error {
	return &opError{kind: kind, detail: detail, cause: cause}
}

func (e *opError) Error() string {
	if e.detail == "" {
		return e.kind.Error()
	}
	return e.kind.Error() + ": " + e.detail
}

func (e *opError) Unwrap() []error {
	if e.cause == nil {
		return []error{e.kind}
	}
	return []error{e.kind, e.cause}
}

// The exit codes Git uses: 1 is the "no" of a yes or no command, and 128 is a fatal error, such as
// running in a folder that is not a repository.
const (
	exitFalse = 1
	exitFatal = 128
)

// exitCode returns the exit code of a Git command that ran and failed, and -1 for any other error.
func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// ask runs a Git command that answers a yes or no question with its exit code: 0 is yes, 1 is no,
// and anything else is a real error.
func (g *Git) ask(ctx context.Context, dir string, args ...string) (bool, error) {
	_, err := g.Run(ctx, dir, args...)
	switch {
	case err == nil:
		return true, nil
	case exitCode(err) == exitFalse:
		return false, nil
	default:
		return false, err
	}
}
