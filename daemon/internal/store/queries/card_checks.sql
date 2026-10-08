-- A card's acceptance checks (migration 0024, docs/architecture.md section 10's `card_checks` row
-- and section 19; docs/backend-checklist.md B10.2, build-plan task 10.4, inventory N9).
--
-- A check is one thing Marshal runs about a card - a test, a linter, a link - and the gate a card
-- has to pass before it can finish. Its `status` is also the evidence a checklist item can be
-- ticked with, which is why `run_ref` travels with it: a tick proven by run 118 must know it is
-- still looking at run 118 and not at run 119.

-- name: CreateCardCheck :exec
INSERT INTO card_checks (id, card_id, kind, spec_json, status, run_ref, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetCardCheck :one
SELECT * FROM card_checks WHERE id = ?;

-- name: UpdateCardCheckStatus :exec
-- Records what a run found, and which run it was. `run_ref` moves with the status in the same
-- statement, so a status can never be left describing an older answer than the reference beside it.
UPDATE card_checks
SET status = ?, run_ref = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteCardCheck :exec
DELETE FROM card_checks WHERE id = ?;

-- name: ListCardChecks :many
-- One card's checks, in the order they were added, which is the order they are drawn.
SELECT * FROM card_checks WHERE card_id = ? ORDER BY rowid;

-- name: CountCardChecks :one
-- How many checks a card has, for a card that is about to be created from a template.
SELECT COUNT(*) FROM card_checks WHERE card_id = ?;

-- name: CountUnpassedChecks :one
-- How many of a card's checks have not passed. This is the whole of the gate: a card cannot finish
-- until it is zero, and the number is what a screen shows when it says what is left.
SELECT COUNT(*) FROM card_checks WHERE card_id = ? AND status <> 'passed';
