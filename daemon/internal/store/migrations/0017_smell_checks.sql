-- The code-smell checks' memory of what they have already checked (docs/architecture.md section
-- 17.3, docs/backend-checklist.md B5.8, build-plan 5.20). The rule is that the same commit is never
-- checked twice; a finding is not the only thing a check can leave behind, because a check that
-- found nothing must be remembered too, or a clean commit would be re-checked on every read.
--
-- One row per card and commit, written when a check finishes. The commit is empty for a check of
-- uncommitted work, which is the worktree as it stands before an agent has committed anything.
--
-- A card is removed with its rows. `checked_at` is when the check finished, in UTC milliseconds, and
-- is what the card's own checks panel shows as "last checked".

-- +goose Up

CREATE TABLE smell_checks (
    card_id    TEXT NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    commit_sha TEXT NOT NULL DEFAULT '',
    checked_at INTEGER NOT NULL,
    PRIMARY KEY (card_id, commit_sha)
) STRICT;
