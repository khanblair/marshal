-- Files and packages a card has claimed (migration 0019, docs/architecture.md section 10's
-- `file_claims` row and section 11.4's `claim_files` and `release_files`; docs/backend-checklist.md
-- B7.2, build-plan task 7.3). internal/memory is the only caller.
--
-- A claim is a claim on a thing, not a lock on it. Two cards may hold the same path; what the
-- daemon does about that is warn both of them early (B7.2, task 7.3), which is why the overlap read
-- below looks for the other card's claim rather than refusing the second one.

-- name: InsertFileClaim :exec
-- Claims one path for one card. The primary key is (card_id, path_or_package), so claiming a path
-- the card already holds is not an error and does not move the claim's time - the same claim is
-- still the same claim, and its age is how long the card has had it.
INSERT INTO file_claims (card_id, project_id, path_or_package, claimed_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (card_id, path_or_package) DO NOTHING;

-- name: DeleteFileClaim :exec
-- Releases one claim. Releasing one that is not held is not an error: the answer is the same either
-- way, which is what `release_files` needs when an agent releases a path twice.
DELETE FROM file_claims WHERE card_id = ? AND path_or_package = ?;

-- name: DeleteFileClaimsOfCard :exec
-- Releases every claim one card holds, for the end of a card's work and for a card that is being
-- reset. A card deleted outright needs none of this: the foreign key takes its claims with it.
DELETE FROM file_claims WHERE card_id = ?;

-- name: ListFileClaimsOfCard :many
-- One card's claims, oldest first, which is the order an agent claimed them in.
SELECT * FROM file_claims WHERE card_id = ? ORDER BY claimed_at, path_or_package;

-- name: ListFileClaimsOfProject :many
-- One project's claims, for the board-awareness summary (B7.2, task 7.2) and for the memory
-- screens.
SELECT * FROM file_claims WHERE project_id = ? ORDER BY claimed_at, path_or_package;

-- name: ListFileClaimsOnPath :many
-- Who else holds a path in this project. The term `card_id <> ?` is not decoration: it is what
-- makes the answer "the other cards" and not "everyone including the card asking", and it lets the
-- caller pass "" to ask about every card at once.
SELECT * FROM file_claims
WHERE project_id = ? AND path_or_package = ? AND card_id <> ?;
