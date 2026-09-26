-- The pre-computed Home numbers (migration 0008, docs/architecture.md 16.3). Slice D's subscriber
-- (internal/dashboard) adds to a day as the daemon's events arrive, and the Home answer reads a
-- range of days in one indexed scan. Nothing here ever counts every card.

-- name: UpsertDailyStat :exec
-- Adds one day's numbers to the row for that day and project, making the row when it is the first
-- thing that day. Every column is a delta, so the same call can add one finished card, one merge,
-- or a whole turn's cost, and a day that is already there keeps what it had. Run in the same
-- transaction as the activity row it goes with, so the two never disagree.
INSERT INTO daily_stats (day, project_id, cards_finished, merges, ci_failures, cost_micros)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (day, project_id) DO UPDATE SET
    cards_finished = daily_stats.cards_finished + excluded.cards_finished,
    merges = daily_stats.merges + excluded.merges,
    ci_failures = daily_stats.ci_failures + excluded.ci_failures,
    cost_micros = daily_stats.cost_micros + excluded.cost_micros;

-- name: ListDailyStats :many
-- Every stored day from `from_day` to `to_day`, both midnight in the daemon's own clock, oldest
-- first and each day's projects together. The range is inclusive at both ends. The bounds are named
-- so the generated parameters are `FromDay` and `ToDay` rather than two parameters called `Day`.
-- `SELECT *` keeps the row type the table's own model (`db.DailyStat`), which is what the Home
-- answer and the subscriber both read.
SELECT * FROM daily_stats
WHERE day >= sqlc.arg(from_day) AND day <= sqlc.arg(to_day)
ORDER BY day, project_id;
