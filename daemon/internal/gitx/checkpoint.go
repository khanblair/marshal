package gitx

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Checkpoints (docs/architecture.md section 10, docs/backend-checklist B5.3, build-plan 5.21): a
// restore point Marshal makes before an agent turn, before a merge, or when a person asks, so a
// card's work can be put back to a known state. This file is the Git half; the row that names the
// commit and the label a person reads live in the store, and the session manager is what makes one.
//
// A checkpoint is a real commit on the card's own branch, not a stash or a copy of the tree: Marshal
// commits whatever the worktree holds, so the commit is a complete state the branch can be put back
// to with an ordinary reset. The commit is kept on a hidden ref, named by the checkpoint's own id,
// so it survives the branch moving on and so Git's garbage collection can reclaim a checkpoint that
// was trimmed.

// CheckpointRefPrefix is the hidden ref namespace checkpoints are kept under. Nothing else uses it,
// so a checkpoint is never confused with a branch a person or an agent made.
const CheckpointRefPrefix = "refs/marshal/checkpoints/"

// CheckpointRef is the hidden ref one checkpoint's commit is kept on, named by the checkpoint's own
// id. The id is opaque and unique, so the ref is too.
func CheckpointRef(id string) string { return CheckpointRefPrefix + id }

// CheckpointState is what making a checkpoint found.
type CheckpointState struct {
	// SHA is the commit the checkpoint points at, full length. It is the commit the card's worktree
	// is put back to when the checkpoint is restored.
	SHA string
	// Created is true when Marshal made a new commit, and false when the branch was already clean
	// and its HEAD is the restore point. Both are a checkpoint; they differ only in whether the
	// agent's pending work was committed to reach it.
	Created bool
}

// Checkpoint commits whatever the card's worktree holds, points ref at the result, and reports the
// commit. `message` is the checkpoint's label, kept as the commit's message as well as on the row,
// so a person who reads the branch's own log sees why Marshal made it.
//
// A clean worktree is not a failure: there is nothing to commit, and the checkpoint points at the
// branch's own head. Whether there is anything to commit is asked of Git directly rather than read
// from a commit that would fail, because `git commit` writes "nothing to commit" to its output and
// not to its error stream, where a failure here is read from.
//
// The commit is made with Marshal's own identity and with signing and hooks turned off, so a machine
// with no Git identity, a signing key, or a pre-commit hook still makes a checkpoint.
func (g *Git) Checkpoint(ctx context.Context, worktree, ref, message string) (CheckpointState, error) {
	if strings.TrimSpace(worktree) == "" || strings.TrimSpace(ref) == "" {
		return CheckpointState{}, errors.New("a checkpoint needs a worktree and a ref")
	}
	if _, err := g.Run(ctx, worktree, "add", "-A"); err != nil {
		return CheckpointState{}, fmt.Errorf("stage a card's work for a checkpoint: %w", err)
	}
	// `git diff --cached --quiet` answers with its exit code: 0 is "the index matches the head",
	// which is what a clean worktree means, and 1 is "there are staged differences".
	clean, err := g.ask(ctx, worktree, "diff", "--cached", "--quiet")
	if err != nil {
		return CheckpointState{}, fmt.Errorf("see whether a card's worktree has anything to commit: %w", err)
	}
	created := !clean
	if created {
		args := []string{
			"-c", "user.name=Marshal", "-c", "user.email=marshal@localhost",
			"-c", "commit.gpgsign=false", "commit", "-m", message, "--no-verify",
		}
		if _, err := g.Run(ctx, worktree, args...); err != nil {
			return CheckpointState{}, fmt.Errorf("make a checkpoint commit: %w", err)
		}
	}
	sha, err := g.Run(ctx, worktree, "rev-parse", "HEAD")
	if err != nil {
		return CheckpointState{}, fmt.Errorf("read a checkpoint's commit: %w", err)
	}
	if _, err := g.Run(ctx, worktree, "update-ref", ref, sha); err != nil {
		return CheckpointState{}, fmt.Errorf("point a hidden ref at a checkpoint: %w", err)
	}
	return CheckpointState{SHA: sha, Created: created}, nil
}

// RestoreCheckpoint puts a card's worktree and branch back to a checkpoint's commit: the tracked
// files are reset to that commit, and files the worktree holds that the commit does not are removed.
// Ignored files are left alone, because a build folder is not what a restore point is about.
//
// It never touches a ref other than the branch the worktree is on, and it never pushes: a restore
// is local to one card's own worktree.
func (g *Git) RestoreCheckpoint(ctx context.Context, worktree, sha string) error {
	if strings.TrimSpace(worktree) == "" || strings.TrimSpace(sha) == "" {
		return errors.New("a restore needs a worktree and a commit")
	}
	if _, err := g.Run(ctx, worktree, "reset", "--hard", sha); err != nil {
		return fmt.Errorf("reset a card's worktree to a checkpoint: %w", err)
	}
	if _, err := g.Run(ctx, worktree, "clean", "-fd"); err != nil {
		return fmt.Errorf("remove files a restore point does not hold: %w", err)
	}
	return nil
}

// CheckpointCommit reads the commit a checkpoint's hidden ref points at. It is how a checkpoint whose
// row is gone, or whose commit was moved, can still be found, and how a test proves a checkpoint
// points where it says.
func (g *Git) CheckpointCommit(ctx context.Context, worktree, ref string) (string, error) {
	sha, err := g.Run(ctx, worktree, "rev-parse", "--verify", ref)
	if err != nil {
		return "", fmt.Errorf("read the commit of a checkpoint ref: %w", err)
	}
	return sha, nil
}

// DeleteCheckpointRef removes a checkpoint's hidden ref, keeping the commit itself, which Git's own
// garbage collection reclaims once nothing points at it. It is what trims a card's checkpoint list
// without rewriting the branch it was made from.
func (g *Git) DeleteCheckpointRef(ctx context.Context, repo, ref string) error {
	if _, err := g.Run(ctx, repo, "update-ref", "-d", ref); err != nil {
		return fmt.Errorf("delete a checkpoint ref: %w", err)
	}
	return nil
}
