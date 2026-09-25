-- name: GetOwner :one
SELECT * FROM users ORDER BY created_at, id LIMIT 1;

-- name: CreateUser :one
INSERT INTO users (id, name, email, avatar_path, time_zone, tailnet_identity, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: CreateDevice :exec
INSERT INTO devices (id, user_id, name, kind, token_hash, paired_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetActiveDeviceByTokenHash :one
SELECT * FROM devices WHERE token_hash = ? AND revoked_at IS NULL;

-- name: ListDevices :many
SELECT id, user_id, name, kind, paired_at, last_seen_at, revoked_at
FROM devices
WHERE user_id = ?
ORDER BY paired_at, id;

-- name: TouchDevice :exec
UPDATE devices SET last_seen_at = CAST(sqlc.arg(last_seen_at) AS INTEGER)
WHERE id = sqlc.arg(id) AND revoked_at IS NULL;

-- name: RevokeDevice :execrows
UPDATE devices SET revoked_at = CAST(sqlc.arg(revoked_at) AS INTEGER)
WHERE id = sqlc.arg(id) AND revoked_at IS NULL;
