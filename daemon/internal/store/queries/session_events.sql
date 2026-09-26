-- The append-only history and activity of a session (migration 0006, and 0009 for a chat's). A
-- page is read by cursor, newest first: pass the seq the previous page ended on, or 0 for the newest
-- page. seq is the owner's own sequence (a card's, or a chat's), so the cursor never skips or
-- repeats a row while the session appends.
--
-- A card's queries say `card_id <> ''` beside `card_id = ?`. The term is not redundant: a chat's
-- event stores '' as its card, and the index that numbers a card's events is partial (WHERE
-- card_id <> ''), which SQLite only uses for a query that carries that same term.

-- name: InsertSessionEvent :exec
INSERT INTO session_events (
    id, card_id, session_id, seq, kind, state, summary, detail_json, log_ref, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: NextSessionEventSeq :one
-- The seq the next event of a card takes. Run it in the same transaction as the insert, so two
-- events cannot take one number.
SELECT CAST(COALESCE(MAX(seq), 0) + 1 AS INTEGER) AS seq FROM session_events
WHERE card_id = ? AND card_id <> '';

-- name: ListSessionEvents :many
SELECT * FROM session_events
WHERE card_id = ? AND card_id <> '' AND seq < ?
ORDER BY seq DESC
LIMIT ?;

-- name: ListSessionEventsByKind :many
SELECT * FROM session_events
WHERE card_id = ? AND card_id <> '' AND kind = ? AND seq < ?
ORDER BY seq DESC
LIMIT ?;

-- name: GetSessionEvent :one
-- One event in full, for the detail a row opens on demand (a tool call's content and diffs).
SELECT * FROM session_events WHERE card_id = ? AND card_id <> '' AND id = ?;

-- name: InsertChatEvent :exec
-- A chat's event: the card side is left at its empty default.
INSERT INTO session_events (
    id, chat_id, session_id, seq, kind, state, summary, detail_json, log_ref, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: NextChatEventSeq :one
-- The seq the next event of a chat takes, in the same transaction as the insert.
SELECT CAST(COALESCE(MAX(seq), 0) + 1 AS INTEGER) AS seq FROM session_events WHERE chat_id = ?;

-- name: ListChatEvents :many
SELECT * FROM session_events
WHERE chat_id = ? AND seq < ?
ORDER BY seq DESC
LIMIT ?;

-- name: GetChatEvent :one
-- One of a chat's events in full, for the detail a row opens on demand.
SELECT * FROM session_events WHERE chat_id = ? AND id = ?;
