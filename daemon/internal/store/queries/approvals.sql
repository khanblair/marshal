-- name: CreateApproval :exec
-- The insert the session pump makes the moment an agent asks for permission, before the request is
-- announced. decision and decided_by keep their defaults (empty, meaning nobody has answered yet).
INSERT INTO approvals (id, session_id, request_json) VALUES (?, ?, ?);

-- name: GetApproval :one
SELECT * FROM approvals WHERE id = ?;

-- name: GetPendingApprovalBySession :one
-- The one approval a session is waiting on right now, if it has one (S8b): a session blocks on at
-- most one at a time, so this is at most one row. It is how a card's own wire NeedsReason carries
-- the approval's id for Home's needs-you list (protocol.NeedsReason.ApprovalID, set by
-- session.StoredStates from this), derived fresh on every read rather than stored on the card, so
-- it can never go stale once the approval is answered: decision turns non-empty and this simply
-- stops matching.
SELECT * FROM approvals WHERE session_id = ? AND decision = '' ORDER BY id DESC LIMIT 1;

-- name: ListPendingApprovalsByProject :many
-- GetPendingApprovalBySession's twin for a whole project's cards at once, so a board read does not
-- run one query per card the way GetPendingApprovalBySession would if it were called in a loop
-- (see cardWithLabels's own doc comment on that rule; this is the batched read the doc comment
-- means).
SELECT sessions.card_id AS card_id, approvals.id AS approval_id
FROM approvals
JOIN sessions ON sessions.id = approvals.session_id
JOIN cards ON cards.id = sessions.card_id
WHERE cards.project_id = ? AND approvals.decision = '';

-- name: DecideApproval :execrows
-- The one write that answers a request. The `decision = ''` guard makes it the only answer: a second
-- call for a request that is already decided changes nothing and reports zero rows, which the caller
-- treats as "already answered" rather than overwriting the first decision.
UPDATE approvals SET decision = sqlc.arg(decision), decided_by = sqlc.arg(decided_by)
WHERE id = sqlc.arg(id) AND decision = '';
