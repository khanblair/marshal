-- name: InsertOutbox :exec
INSERT INTO outbox (id, kind, target, payload_json, created_at)
VALUES (?, ?, ?, ?, ?);

-- name: ListOutbox :many
SELECT id, kind, target, payload_json, attempts, last_error, created_at
FROM outbox ORDER BY created_at, id LIMIT ?;

-- name: CountOutbox :one
SELECT COUNT(*) FROM outbox;

-- name: DeleteOutbox :exec
DELETE FROM outbox WHERE id = ?;

-- name: FailOutbox :exec
UPDATE outbox SET attempts = attempts + 1, last_error = ? WHERE id = ?;

-- name: TrimOutbox :exec
DELETE FROM outbox WHERE id IN (
    SELECT id FROM outbox ORDER BY created_at, id LIMIT MAX((SELECT COUNT(*) FROM outbox) - ?, 0)
);
