-- The Home activity stream (migration 0008, docs/backend-checklist.md B2.3, inventory N17). The
-- dashboard module's subscriber appends rows as the daemon's events arrive and trims them after
-- ninety days; the view-all route reads them newest first, one page at a time, filtered by kind
-- and by project. A page is read by cursor (`seq`), never by offset, so a row appended while a
-- client pages does not shift what it has already seen.

-- name: InsertActivity :exec
-- Appends one row of the stream. Run it in the same transaction as NextActivitySeq and the
-- daily_stats update that goes with it, so the stream's order and the numbers stay together.
INSERT INTO activity (id, seq, project_id, kind, subject_kind, subject_id, subject_key, summary, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: NextActivitySeq :one
-- The seq the next row of the stream takes. One stream covers every project, so this is the
-- newest seq in the table and not a per-project number. Run it in the same transaction as the
-- insert, so two rows cannot take one number.
SELECT CAST(COALESCE(MAX(seq), 0) + 1 AS INTEGER) AS seq FROM activity;

-- name: TrimActivity :exec
-- Drops the rows older than the moment given, which is what "trimmed after ninety days" is. It is
-- one range delete on activity_created_at, and it is what keeps the stream bounded.
DELETE FROM activity WHERE created_at < ?;

-- name: DeleteActivityForProject :exec
-- Drops a project's rows when the project is removed from Marshal. The table has no foreign key on
-- project_id (an empty string means "no project"), so the module that owns the stream removes them
-- itself when project.removed arrives.
DELETE FROM activity WHERE project_id = ?;

-- Every read below selects the whole table, so sqlc gives them all the table's own row type
-- (`db.Activity`) and one Go mapper serves them. `seq < ?` is the cursor: pass a number larger than
-- any seq for the newest page, and the seq the previous page ended on for the page after it. A
-- filter that is not wanted is left out of the WHERE clause rather than passed as a wildcard, so
-- each kind of page uses its own index.

-- name: ListActivity :many
SELECT * FROM activity
WHERE seq < ?
ORDER BY seq DESC
LIMIT ?;

-- name: ListActivityByKind :many
SELECT * FROM activity
WHERE kind = ? AND seq < ?
ORDER BY seq DESC
LIMIT ?;

-- name: ListActivityByProject :many
SELECT * FROM activity
WHERE project_id = ? AND seq < ?
ORDER BY seq DESC
LIMIT ?;

-- name: ListActivityByKindAndProject :many
SELECT * FROM activity
WHERE kind = ? AND project_id = ? AND seq < ?
ORDER BY seq DESC
LIMIT ?;
