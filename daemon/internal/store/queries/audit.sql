-- name: InsertAuditLog :exec
-- One row per action worth accounting for (docs/architecture.md section 10). Append only: the API
-- never edits or deletes one. The id is an opaque id made by the caller, so it sorts by time like
-- every other id.
INSERT INTO audit_log (id, session_id, actor, action, target, detail_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListAuditLog :many
-- The newest rows first. The order is the rowid's, which is the order the rows were written in: two
-- rows written in the same millisecond share a time, and their ids share everything but random bits,
-- so neither orders them. A test reads this to prove a row was written.
SELECT * FROM audit_log ORDER BY rowid DESC LIMIT ?;

-- name: ListAuditLogPage :many
-- One page of the audit log, newest first, paged by (created_at, id) - the pair the table already
-- indexes - rather than by rowid, which a query may order by but not compare (SQLite's rowid is not
-- a declared column, so sqlc refuses it in a WHERE). The pair is unique because the id is, so the
-- order is total and stable. The newest page asks for rows before the end of time with an empty id,
-- which is every row.
SELECT * FROM audit_log
WHERE created_at < sqlc.arg(before_time)
   OR (created_at = sqlc.arg(before_time) AND id < sqlc.arg(before_id))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

-- name: SearchAuditLogPage :many
-- The rows a search matched, newest first, paged the same way as ListAuditLogPage. The search reads
-- the four columns a person can search by - actor, action, target, and the recorded detail - and is
-- told the pattern to use ('%word%'). A `%` or `_` in a person's words acts as the SQL wildcard it
-- looks like, which widens a search rather than failing it.
SELECT * FROM audit_log
WHERE (created_at < sqlc.arg(before_time)
       OR (created_at = sqlc.arg(before_time) AND id < sqlc.arg(before_id)))
  AND (actor LIKE sqlc.arg(pattern) OR action LIKE sqlc.arg(pattern)
       OR target LIKE sqlc.arg(pattern) OR detail_json LIKE sqlc.arg(pattern))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

-- name: ExportAuditLog :many
-- Every row, newest first, for an export: no paging, so the file holds the whole answer. The limit
-- is a ceiling on how much one export may carry, not a page size.
SELECT * FROM audit_log ORDER BY created_at DESC, id DESC LIMIT sqlc.arg(export_limit);

-- name: SearchAuditLogExport :many
-- Every row a search matched, newest first, for an export: the same search as SearchAuditLogPage,
-- with no paging.
SELECT * FROM audit_log
WHERE actor LIKE sqlc.arg(pattern) OR action LIKE sqlc.arg(pattern)
   OR target LIKE sqlc.arg(pattern) OR detail_json LIKE sqlc.arg(pattern)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(export_limit);
