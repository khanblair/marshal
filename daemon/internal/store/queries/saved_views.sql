-- A project's saved views (internal/projects).

-- name: ListSavedViewsByProject :many
-- The order the views were last saved in, which is the order the board's chips are drawn in.
SELECT * FROM saved_views WHERE project_id = ? ORDER BY updated_at, id;

-- name: GetSavedView :one
SELECT * FROM saved_views WHERE id = ?;

-- name: CreateSavedView :exec
INSERT INTO saved_views (id, project_id, name, filters_json, swimlane, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: UpdateSavedView :execrows
UPDATE saved_views SET name = ?, filters_json = ?, swimlane = ?, updated_at = ? WHERE id = ?;

-- name: DeleteSavedView :execrows
DELETE FROM saved_views WHERE id = ?;
