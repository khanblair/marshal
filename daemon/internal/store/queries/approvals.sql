-- name: CreateApproval :exec
-- The insert the session pump makes the moment an agent asks for permission, before the request is
-- announced. decision and decided_by keep their defaults (empty, meaning nobody has answered yet).
INSERT INTO approvals (id, session_id, request_json) VALUES (?, ?, ?);

-- name: GetApproval :one
SELECT * FROM approvals WHERE id = ?;

-- name: DecideApproval :execrows
-- The one write that answers a request. The `decision = ''` guard makes it the only answer: a second
-- call for a request that is already decided changes nothing and reports zero rows, which the caller
-- treats as "already answered" rather than overwriting the first decision.
UPDATE approvals SET decision = sqlc.arg(decision), decided_by = sqlc.arg(decided_by)
WHERE id = sqlc.arg(id) AND decision = '';
