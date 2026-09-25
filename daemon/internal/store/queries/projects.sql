-- name: CreateProject :exec
INSERT INTO projects (
    id, name, repo_path, default_branch, language, dev_command,
    bypass_locked, is_monorepo, packages_json, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetProject :one
SELECT * FROM projects WHERE id = ?;

-- name: GetProjectByRepoPath :one
SELECT * FROM projects WHERE repo_path = ?;

-- name: ProjectIDExists :one
SELECT EXISTS (SELECT 1 FROM projects WHERE id = ?);

-- name: ListProjects :many
SELECT * FROM projects ORDER BY created_at, id;

-- name: UpdateProject :execrows
UPDATE projects
SET name = ?, dev_command = ?, default_branch = ?, bypass_locked = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteProject :execrows
DELETE FROM projects WHERE id = ?;

-- name: CreateBoard :exec
INSERT INTO boards (id, project_id, columns_json) VALUES (?, ?, ?);

-- name: GetBoardByProject :one
SELECT * FROM boards WHERE project_id = ?;

-- name: ReserveCardNumber :one
-- Takes the next card number of a project and moves the counter on, in one statement. Run it in
-- the same transaction as the card insert, so a failed insert gives the number back.
UPDATE projects
SET next_card_number = next_card_number + 1
WHERE id = ?
RETURNING CAST(next_card_number - 1 AS INTEGER) AS number;

-- name: CreateCard :exec
INSERT INTO cards (
    id, project_id, number, board_id, title, body, state, agent_kind, model, thinking,
    permission_mode, created_by, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetCard :one
SELECT * FROM cards WHERE id = ?;

-- name: GetCardByKey :one
SELECT * FROM cards WHERE project_id = ? AND number = ?;

-- name: ListCardsByProject :many
SELECT * FROM cards WHERE project_id = ? ORDER BY number;

-- name: UpdateCardState :execrows
UPDATE cards SET state = ?, updated_at = ? WHERE id = ?;

-- name: UpdateCardWorktree :execrows
UPDATE cards SET worktree_path = ?, branch = ?, updated_at = ? WHERE id = ?;

-- name: ListCardBranches :many
SELECT branch FROM cards WHERE project_id = ? AND branch <> '' ORDER BY number;

-- name: CountNeedsByProject :many
SELECT project_id, CAST(COUNT(*) AS INTEGER) AS needs
FROM cards
WHERE state = 'needs'
GROUP BY project_id;

-- name: CountNeedsForProject :one
SELECT CAST(COUNT(*) AS INTEGER) AS needs FROM cards WHERE project_id = ? AND state = 'needs';
