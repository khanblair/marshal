-- What the Integrator delivered, and the settings it merges by (migration 0032, Integrator flow).

-- name: InsertMergeHistory :exec
INSERT INTO merge_history (
    id, project_id, card_id, target, merged_at, commit_sha, prev_tip, resolved, summary,
    backup_branch, wip_ref, folder_tree
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListMergeHistory :many
-- The newest deliveries of a project, with the card's number and title for the Integration view.
SELECT h.id, h.project_id, h.card_id, h.target, h.merged_at, h.commit_sha, h.prev_tip, h.resolved,
       h.summary, h.backup_branch, h.wip_ref, h.folder_tree, h.undone_at,
       c.number AS card_number, c.title AS card_title
FROM merge_history h
JOIN cards c ON c.id = h.card_id
WHERE h.project_id = ?
ORDER BY h.merged_at DESC, h.id DESC
LIMIT ?;

-- name: GetLatestMergeHistoryForCard :one
SELECT * FROM merge_history WHERE card_id = ? ORDER BY merged_at DESC, id DESC LIMIT 1;

-- name: MarkMergeHistoryUndone :execrows
-- Marks a delivery undone, once: a row that is already undone is left alone and reports 0 rows.
UPDATE merge_history SET undone_at = ? WHERE id = ? AND undone_at = 0;

-- name: GetMergeSettings :one
SELECT * FROM project_merge_settings WHERE project_id = ?;

-- name: SetMergePaused :exec
INSERT INTO project_merge_settings (project_id, paused, updated_at) VALUES (?, ?, ?)
ON CONFLICT (project_id) DO UPDATE SET paused = excluded.paused, updated_at = excluded.updated_at;

-- name: SetMergeAutoMerge :exec
INSERT INTO project_merge_settings (project_id, auto_merge, updated_at) VALUES (?, ?, ?)
ON CONFLICT (project_id) DO UPDATE SET auto_merge = excluded.auto_merge, updated_at = excluded.updated_at;

-- name: SetMergePendingTip :exec
INSERT INTO project_merge_settings (project_id, pending_tip, updated_at) VALUES (?, ?, ?)
ON CONFLICT (project_id) DO UPDATE SET pending_tip = excluded.pending_tip, updated_at = excluded.updated_at;
