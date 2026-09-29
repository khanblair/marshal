-- Card comments and their attachments (migration 0024, docs/architecture.md section 10's
-- `comments` and `attachments` rows and section 19's rules; docs/backend-checklist.md B10.6,
-- build-plan tasks 10.12 and 10.13).
--
-- A comment is the one place a person writes to the agent outside the chat, and the only content on
-- the card that can carry an attachment. The rules that matter - that an attachment is data and
-- never run, that a reply wakes the session if needed, that only the author may edit their own
-- comment - live in the service; these queries own the order, the cascade, and the agent's read
-- marker.

-- name: CreateComment :exec
-- Makes a comment. `mentions_json` is written with it because a mention is part of what was said,
-- not something derived later from the text - a person's handle may be renamed, and the comment
-- still has to remember who it named.
INSERT INTO comments (id, card_id, author_kind, author_id, body, mentions_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetComment :one
SELECT * FROM comments WHERE id = ?;

-- name: UpdateCommentBody :exec
-- Edits a comment's words, keeping `created_at` and moving `edited_at`. The mentions are left alone
-- on purpose: an edit is to what was written, not to who was named, and re-reading the body for
-- mentions would let an edit add an address nobody meant to send to.
UPDATE comments
SET body = ?, edited_at = ?
WHERE id = ?;

-- name: DeleteComment :exec
-- Removes a comment and, by the cascade, its attachments. Deleting a card removes its comments the
-- same way (docs/architecture.md section 19).
DELETE FROM comments WHERE id = ?;

-- name: ListCardComments :many
-- One card's comments, oldest first, which is the order a conversation is read in.
SELECT * FROM comments WHERE card_id = ? ORDER BY created_at, id;

-- name: ListUnreadComments :many
-- The comments on a card the card's agent has not picked up yet, oldest first. This is what
-- `read_comments` hands the agent at the start of a turn, and what marks them read on the way past.
SELECT * FROM comments
WHERE card_id = ? AND agent_read_at IS NULL
ORDER BY created_at, id;

-- name: MarkCommentsReadByAgent :exec
-- Records that the card's agent has read what was waiting. It stamps everything the card has that
-- is unread, so a marker never lags the answer the agent just acted on.
UPDATE comments
SET agent_read_at = ?
WHERE card_id = ? AND agent_read_at IS NULL;

-- name: CountUnreadComments :one
-- How many comments are waiting for the agent. The card's "N new" hint reads this, and it is cheap
-- enough to ask whenever the card is drawn.
SELECT COUNT(*) FROM comments WHERE card_id = ? AND agent_read_at IS NULL;

-- name: CreateAttachment :exec
-- Records a file on a comment. The row is written after the file is on disk, so a row that exists
-- points at a file that does too; the service writes the file first and this second, and cleans up
-- the file if this fails.
INSERT INTO attachments (id, comment_id, file_name, mime_type, size_bytes, path, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListCommentAttachments :many
-- The files on one comment, oldest first.
SELECT * FROM attachments WHERE comment_id = ? ORDER BY created_at, id;

-- name: ListCardAttachments :many
-- Every file on a card, for the agent's context and for a card's own file list. It is the join the
-- single-comment read does not cover: attachments are asked for per card far more often than per
-- comment.
SELECT attachment.*
FROM attachments AS attachment
JOIN comments AS comment ON comment.id = attachment.comment_id
WHERE comment.card_id = ?
ORDER BY attachment.created_at, attachment.id;

-- name: DeleteAttachment :exec
-- Removes one file's row. The file on disk is removed by the service that holds the path; the query
-- only ever owns the row, so nothing here reaches outside the database.
DELETE FROM attachments WHERE id = ?;
