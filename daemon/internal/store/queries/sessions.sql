-- name: CreateCardSession :exec
-- The insert the session manager makes for a card, with the chat side left NULL.
INSERT INTO sessions (
    id, card_id, agent_kind, agent_session_id, state, model, thinking,
    permission_mode, last_active_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: CreateChatSession :exec
-- The insert the chats service makes when a chat is created, with the card side left NULL. A chat's
-- own session row exists from the moment the chat does; the session manager resumes it when the
-- chat is opened (slice C).
INSERT INTO sessions (
    id, chat_id, agent_kind, agent_session_id, state, model, thinking,
    permission_mode, last_active_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetSession :one
SELECT * FROM sessions WHERE id = ?;

-- name: GetSessionByCard :one
-- The second term is not redundant. A chat's session stores '' as its card, so without it an empty
-- card id would find a chat's session; and the index on card_id is partial (WHERE card_id <> ''),
-- which SQLite only uses when the query carries that same term.
SELECT * FROM sessions WHERE card_id = ? AND card_id <> '';

-- name: GetSessionByChat :one
SELECT * FROM sessions WHERE chat_id = ?;

-- name: ListResumableSessions :many
-- The card sessions that still need a resume after a restart. 'asleep' is deliberately not one of
-- the states: sleep is a person's decision (B2.15), and only a person wakes it. 'waking' is one of
-- them, because a daemon that stopped in the middle of a wake left nothing running and the card
-- would otherwise be stranded as waking forever. Nothing writes 'waiting-approval' or
-- 'sleep-warning' yet (Phase 3 and Phase 5). A chat's session is never in this list: it starts on
-- demand, so its next message resumes it through its saved id, and a chat that nobody opens starts
-- nothing at restart.
SELECT * FROM sessions
WHERE card_id <> '' AND state IN ('starting', 'awake', 'working', 'waking')
ORDER BY card_id;

-- name: ListCardSessionStatesByProject :many
-- The stored state and view of every card session of one project, for the session and viewMode
-- fields of the wire card, so a board reads them all at once. A chat's session belongs to no card
-- and is not joined.
SELECT sessions.card_id AS card_id, sessions.state AS state, sessions.view_mode AS view_mode
FROM sessions
JOIN cards ON cards.id = sessions.card_id
WHERE cards.project_id = ?;

-- name: UpdateSessionRuntime :execrows
-- A session that stops is back in the chat view: nothing runs for it, and the next start of the card
-- resumes it in the structured mode. Every way a session ends goes through this one write, so the
-- rule is stated here once. The other states keep the view the session is in.
UPDATE sessions
SET state = sqlc.arg(state), agent_session_id = sqlc.arg(agent_session_id),
    last_active_at = sqlc.arg(last_active_at), updated_at = sqlc.arg(updated_at),
    view_mode = CASE WHEN sqlc.arg(state) = 'stopped' THEN 'chat' ELSE view_mode END
WHERE id = sqlc.arg(id);

-- name: UpdateSessionView :execrows
-- The view switch: the session now runs in this mode. The session manager writes it after the new
-- process has started and before the old one is forgotten, so a restart resumes what is running.
UPDATE sessions
SET view_mode = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteSession :execrows
DELETE FROM sessions WHERE id = ?;
