package gitx

import (
	"context"
	"strings"
)

// Push pushes one branch of a working folder to a named remote, answering nothing on success.
//
// It is the only push Marshal runs by itself (the real mode of a simulated CI failure, B6.4). Every
// push an agent runs goes through the harness's own check first; this one is checked the same way,
// on the argument list it is about to run, so what Marshal refuses is
//   - a force-push, in any spelling, and the forms that push every branch (-f, --force,
//     --force-with-lease, --force-if-includes, --mirror, --all, --delete);
//   - any argument whose destination is the project's main branch.
//
// The branch is the card's own and `main` is the project's default branch, so a project whose
// default branch cannot be read must not be pushed to at all: an empty main makes the rule unable
// to recognize it, and this refuses rather than guesses.
func (g *Git) Push(ctx context.Context, dir, remote, branch, main string) error {
	remote, branch, main = strings.TrimSpace(remote), strings.TrimSpace(branch), strings.TrimSpace(main)
	if remote == "" || branch == "" {
		return newOpError(ErrBadPath, "a push needs a remote and a branch", nil)
	}
	if main == "" {
		return newOpError(ErrBadPath, "a push needs the project's main branch to stay off", nil)
	}
	if err := NewContainment(dir, dir, branch, main).CheckCommand([]string{"push", remote, branch}); err != nil {
		return err
	}
	_, err := g.Run(ctx, dir, "push", remote, branch)
	return err
}
