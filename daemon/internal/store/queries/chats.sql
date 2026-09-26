-- name: CreateChat :exec
INSERT INTO chats (
    id, project_id, title, target_kind, target_id, agent_kind, model, thinking,
    permission_mode, archived_at, last_active_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetChat :one
SELECT * FROM chats WHERE id = ?;

-- name: ListChatsByProject :many
-- A project's live chats: the ones in the main list, most recently active first.
SELECT * FROM chats
WHERE project_id = ? AND archived_at IS NULL
ORDER BY last_active_at DESC, id;

-- name: ListArchivedChatsByProject :many
-- A project's archived chats, most recently active first. The live list and this one are two
-- queries rather than one query with an optional archived_at, so neither has to cope with a NULL
-- test whose answer is always the same.
SELECT * FROM chats
WHERE project_id = ? AND archived_at IS NOT NULL
ORDER BY last_active_at DESC, id;

-- name: RenameChat :execrows
UPDATE chats SET title = ?, updated_at = ? WHERE id = ?;

-- name: ArchiveChat :execrows
UPDATE chats SET archived_at = ?, updated_at = ? WHERE id = ?;

-- name: RestoreChat :execrows
UPDATE chats SET archived_at = NULL, updated_at = ? WHERE id = ?;

-- name: TouchChat :execrows
-- Moves a chat to the top of its project's list, once a message has been written to its session.
UPDATE chats SET last_active_at = ?, updated_at = ? WHERE id = ?;

-- name: DeleteChat :execrows
DELETE FROM chats WHERE id = ?;
