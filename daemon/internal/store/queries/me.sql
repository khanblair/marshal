-- The person using Marshal: their profile, avatar, progress, and preferences (internal/accounts).

-- name: GetUser :one
SELECT * FROM users WHERE id = ?;

-- name: ListUsers :many
-- Everyone who can be put on a card, by name. Solo use has one row, the owner.
SELECT * FROM users ORDER BY name, id;

-- name: UpdateUserProfile :execrows
UPDATE users SET name = ?, email = ?, time_zone = ?, updated_at = ? WHERE id = ?;

-- name: SetUserAvatar :execrows
-- avatar_path is the file name under <data>/avatars, or '' with a NULL avatar_updated_at when the
-- avatar is removed.
UPDATE users SET avatar_path = ?, avatar_updated_at = ?, updated_at = ? WHERE id = ?;

-- name: GetUserProgress :one
SELECT * FROM user_progress WHERE user_id = ?;

-- name: UpsertUserProgress :exec
INSERT INTO user_progress (
    user_id, onboarding_step, onboarding_done_at, onboarding_skipped, tutorial_done_at, tutorial_skipped
) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (user_id) DO UPDATE SET
    onboarding_step = excluded.onboarding_step,
    onboarding_done_at = excluded.onboarding_done_at,
    onboarding_skipped = excluded.onboarding_skipped,
    tutorial_done_at = excluded.tutorial_done_at,
    tutorial_skipped = excluded.tutorial_skipped;

-- name: GetUserPreferences :one
SELECT * FROM user_preferences WHERE user_id = ?;

-- name: UpsertUserPreferences :exec
INSERT INTO user_preferences (user_id, theme, list_columns_json, sort_json, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (user_id) DO UPDATE SET
    theme = excluded.theme,
    list_columns_json = excluded.list_columns_json,
    sort_json = excluded.sort_json,
    updated_at = excluded.updated_at;

-- name: ListProjectPreferences :many
SELECT * FROM project_preferences WHERE user_id = ? ORDER BY project_id;

-- name: UpsertProjectPreferences :exec
INSERT INTO project_preferences (
    user_id, project_id, last_view, filters_json, query, swimlane, collapsed_lanes_json,
    show_all_done, saved_view_id, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (user_id, project_id) DO UPDATE SET
    last_view = excluded.last_view,
    filters_json = excluded.filters_json,
    query = excluded.query,
    swimlane = excluded.swimlane,
    collapsed_lanes_json = excluded.collapsed_lanes_json,
    show_all_done = excluded.show_all_done,
    saved_view_id = excluded.saved_view_id,
    updated_at = excluded.updated_at;
