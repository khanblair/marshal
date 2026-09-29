-- name: ListSchedules :many
SELECT * FROM schedules ORDER BY created_at ASC;

-- name: ListSchedulesByProject :many
SELECT * FROM schedules WHERE project_id = ? ORDER BY created_at ASC;

-- name: GetSchedule :one
SELECT * FROM schedules WHERE id = ? LIMIT 1;

-- name: CreateSchedule :one
INSERT INTO schedules (
    id, project_id, name, kind, icon, trigger_type, when_text, cron_expr, time_str, days_json, action, enabled, missed_policy, created_at, updated_at, last_run_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
) RETURNING *;

-- name: UpdateSchedule :one
UPDATE schedules SET
    name = ?,
    when_text = ?,
    cron_expr = ?,
    time_str = ?,
    days_json = ?,
    action = ?,
    enabled = ?,
    missed_policy = ?,
    updated_at = ?
WHERE id = ?
RETURNING *;

-- name: UpdateScheduleLastRun :exec
UPDATE schedules SET last_run_at = ?, updated_at = ? WHERE id = ?;

-- name: DisableSchedule :exec
UPDATE schedules SET enabled = 0, updated_at = ? WHERE id = ?;

-- name: DeleteSchedule :exec
DELETE FROM schedules WHERE id = ?;

-- name: CreateScheduleRun :one
INSERT INTO schedule_runs (
    id, schedule_id, run_at, status, details
) VALUES (
    ?, ?, ?, ?, ?
) RETURNING *;

-- name: ListScheduleRuns :many
SELECT * FROM schedule_runs WHERE schedule_id = ? ORDER BY run_at DESC LIMIT ?;
