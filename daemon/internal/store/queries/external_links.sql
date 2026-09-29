-- name: LinkExternal :exec
INSERT INTO external_links (card_id, kind, external_id) VALUES (?, ?, ?)
ON CONFLICT (card_id, kind) DO UPDATE SET external_id = excluded.external_id;

-- name: ExternalCard :one
SELECT card_id FROM external_links WHERE kind = ? AND external_id = ? LIMIT 1;

-- name: CardExternalID :one
SELECT external_id FROM external_links WHERE card_id = ? AND kind = ? LIMIT 1;

-- name: UnlinkExternal :exec
DELETE FROM external_links WHERE card_id = ? AND kind = ?;
