-- name: CreateSession :exec
INSERT INTO sessions (
    id, card_id, agent_kind, agent_session_id, state, model, thinking,
    permission_mode, last_active_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetSession :one
SELECT * FROM sessions WHERE id = ?;

-- name: GetSessionByCard :one
SELECT * FROM sessions WHERE card_id = ?;

-- name: ListResumableSessions :many
-- The states a live session can be in while the daemon runs, in Phase 1: the other states in
-- protocol.SessionStateValues (waiting-approval, sleep-warning, asleep, waking) belong to later
-- phases and nothing writes them yet.
SELECT * FROM sessions WHERE state IN ('starting', 'awake', 'working') ORDER BY card_id;

-- name: UpdateSessionRuntime :execrows
UPDATE sessions
SET state = ?, agent_session_id = ?, last_active_at = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteSession :execrows
DELETE FROM sessions WHERE id = ?;
