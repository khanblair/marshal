-- What Marshal is allowed to spend and how many cards it may keep awake (migration 0014,
-- docs/architecture.md section 10, inventory B4.5). A limit is a ceiling, not a running total: the
-- running total is daily_stats.cost_micros (cost) and the awake list itself (awake). This table is
-- tiny and is replaced in place, and a limit the person clears leaves no row.

-- name: GetLimit :one
-- One scope's ceiling of one kind. ErrNoSuchLimit (sql.ErrNoRows) means the scope has no ceiling of
-- that kind, which is the shipped state: a fresh install limits nothing.
SELECT value FROM limits WHERE scope = ? AND kind = ?;

-- name: ListLimits :many
-- Every ceiling, ordered by (scope, kind) so a reader's answer is stable. This is deliberately not
-- the order the settings form shows them in: "global" must come before the project ids and the kinds
-- go in cost-day, cost-month, awake order, and both are screen decisions that belong with the
-- protocol's own constants, not in SQL (internal/providers/limits.go's sortLimits). The Limits
-- section (S26b) reads this to draw the form, and the check that runs before a model call reads it
-- once and keeps the answer for the calls that follow.
SELECT * FROM limits ORDER BY scope, kind;

-- name: SetLimit :exec
-- Sets or replaces one scope's ceiling of one kind, so saving twice leaves one row.
INSERT INTO limits (scope, kind, value)
VALUES (?, ?, ?)
ON CONFLICT (scope, kind) DO UPDATE SET value = excluded.value;

-- name: DeleteLimit :exec
-- Removes one scope's ceiling of one kind. Removing a ceiling that is not set is not an error: the
-- answer to every limits call is the whole list, so a client that clears a field twice reads the
-- same answer both times and there is no separate "there was nothing there" to report.
DELETE FROM limits WHERE scope = ? AND kind = ?;
