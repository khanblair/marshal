-- name: UpsertTrelloSnapshot :exec
INSERT INTO trello_snapshots (card_id, trello_card_id, url, from_trello, snapshot_json, fetched_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (card_id) DO UPDATE SET
    trello_card_id = excluded.trello_card_id,
    url = excluded.url,
    snapshot_json = excluded.snapshot_json,
    fetched_at = excluded.fetched_at;

-- name: GetTrelloSnapshot :one
SELECT card_id, trello_card_id, url, from_trello, snapshot_json, fetched_at
FROM trello_snapshots WHERE card_id = ?;

-- name: ListProjectTrelloSnapshots :many
SELECT s.card_id, s.trello_card_id, s.url, s.from_trello, s.snapshot_json, s.fetched_at
FROM trello_snapshots s
JOIN cards c ON c.id = s.card_id
WHERE c.project_id = ?;

-- name: UpsertTrelloItemLink :exec
INSERT INTO trello_item_links (card_id, kind, marshal_id, trello_id) VALUES (?, ?, ?, ?)
ON CONFLICT (card_id, kind, marshal_id) DO UPDATE SET trello_id = excluded.trello_id;

-- name: ListTrelloItemLinks :many
SELECT card_id, kind, marshal_id, trello_id FROM trello_item_links WHERE card_id = ?;

-- name: DismissTrelloChange :exec
INSERT INTO trello_dismissed (project_id, change_id, dismissed_at) VALUES (?, ?, ?)
ON CONFLICT (project_id, change_id) DO NOTHING;

-- name: ListDismissedTrelloChanges :many
SELECT change_id FROM trello_dismissed WHERE project_id = ?;

-- name: UpsertTrelloSyncState :exec
INSERT INTO trello_sync_state (project_id, board_name, fetched_at) VALUES (?, ?, ?)
ON CONFLICT (project_id) DO UPDATE SET board_name = excluded.board_name, fetched_at = excluded.fetched_at;

-- name: GetTrelloSyncState :one
SELECT project_id, board_name, fetched_at FROM trello_sync_state WHERE project_id = ?;
