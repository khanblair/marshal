-- name: ListActivityCountsByMinute :many
-- Rebuilds Home's daily numbers in a new zone (dashboard Recut): rows are grouped by the minute here
-- and given their day in Go, since a minute never straddles any zone's midnight. This counts the rows
-- of one kind at or after `since`; for the merge kind that is the cards counted as finished.
SELECT project_id,
       CAST(created_at / 60000 * 60000 AS INTEGER) AS minute_ms,
       CAST(COUNT(*) AS INTEGER) AS entries
FROM activity
WHERE kind = sqlc.arg(kind) AND created_at >= sqlc.arg(since)
GROUP BY project_id, created_at / 60000
ORDER BY project_id, minute_ms;

-- name: ListUsageCostByMinute :many
-- What the paid model calls at or after `since` cost, per project and minute. A call with no cost
-- adds nothing to a day, so it is left out. A removed project's calls are left out too: its
-- daily_stats went with it (DeleteDailyStatsForProject) but its usage rows stay.
SELECT usage.project_id,
       CAST(usage.created_at / 60000 * 60000 AS INTEGER) AS minute_ms,
       CAST(SUM(usage.cost_micros) AS INTEGER) AS cost_micros
FROM usage
WHERE usage.created_at >= sqlc.arg(since)
  AND usage.cost_micros > 0
  AND (usage.project_id = '' OR usage.project_id IN (SELECT id FROM projects))
GROUP BY usage.project_id, usage.created_at / 60000
ORDER BY usage.project_id, minute_ms;

-- name: DeleteDailyStatsSince :exec
-- Drops every stored day from `from_day` on, the days a re-cut builds again.
DELETE FROM daily_stats WHERE day >= sqlc.arg(from_day);
