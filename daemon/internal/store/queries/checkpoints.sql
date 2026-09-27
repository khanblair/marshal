-- A card's checkpoints: the restore points Marshal makes before an agent turn, before a merge, and
-- whenever a person asks (docs/architecture.md section 10, docs/backend-checklist.md B5.3). A
-- checkpoint is a Git commit on a hidden ref and a row that names it; the card's list is read newest
-- first, and a card is removed with its checkpoints (the foreign key in migration 0016).
--
-- Nothing here updates a checkpoint: a restore point is a fact about a moment, and the only way one
-- changes is by being removed with the card. The id is an opaque id whose first ten characters are
-- the time, and created_at, then the id, is what breaks a tie between two made in one millisecond.

-- name: InsertCheckpoint :exec
-- Records one restore point. `git_ref` is the hidden ref the commit is kept on, so a checkpoint can
-- be found again after the row is read even if the branch has moved on, and `label` says what it was
-- made before ("before turn 3"), and is empty when there is nothing to say.
INSERT INTO checkpoints (id, card_id, git_ref, label, created_at)
VALUES (?, ?, ?, ?, ?);

-- name: ListCheckpointsForCard :many
-- One card's restore points, newest first, which is the order the card panel shows them in.
SELECT * FROM checkpoints
WHERE card_id = ?
ORDER BY created_at DESC, id DESC
LIMIT ?;

-- name: GetCheckpoint :one
-- One restore point of one card, for a restore asked for by id. The card is part of the lookup so a
-- checkpoint of another card can never be restored through this card's address.
SELECT * FROM checkpoints
WHERE card_id = ? AND id = ?;

-- name: CountCheckpointsForCard :one
-- How many restore points a card has. A restore point is made before every turn, and a card
-- stopped and resumed many times would otherwise grow a list with no end, so a caller reads this to
-- decide whether to trim the oldest.
SELECT CAST(COUNT(*) AS INTEGER) AS count FROM checkpoints WHERE card_id = ?;

-- name: DeleteCheckpoint :exec
-- Removes one restore point's row. Trimming a card's list goes one row at a time so each removed
-- row's hidden ref can be deleted with it, and the commit it pointed at is left for Git's own
-- garbage collection. A checkpoint of another card is never removed through this card's address.
DELETE FROM checkpoints WHERE card_id = ? AND id = ?;
