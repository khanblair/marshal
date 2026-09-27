-- Approvals and the audit log (docs/architecture.md section 10, docs/backend-checklist.md Phase 3
-- items B3.4 and B3.5). Forward only, like every migration. Times are INTEGER Unix milliseconds in
-- UTC and ids are TEXT. STRICT makes SQLite refuse a value of the wrong type.
--
-- `approvals` holds one row per permission an agent asked a person for, from the moment the request
-- is announced until it is answered. The full request (the tool call's title, kind, path, command,
-- and the options the agent offered) is kept as JSON in request_json, so the row can be drawn again
-- after a restart and the exact option the person chose can be delivered to the agent unchanged. A
-- row whose decision is still empty is a request nobody has answered: that is the pending approval
-- the Approvals view (S8b) shows and the one POST /v1/approvals/{id} answers. decision and
-- decided_by are set together, once, by that call.
--
-- The id is an opaque id (a ULID, architecture.md 11.5), whose first ten characters are the time,
-- so rows sort by when they were asked without a separate created_at column. When a row was
-- decided is written to audit_log (below), which every decision also writes.

-- +goose Up

-- session_id is the session that asked, and it owns the row: a card's session being removed takes
-- its approvals with it. A chat's session can ask too (one row, many views: N7), and is cascaded
-- the same way.
CREATE TABLE approvals (
    id           TEXT NOT NULL PRIMARY KEY,
    session_id   TEXT NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
    request_json TEXT NOT NULL,
    decision     TEXT NOT NULL DEFAULT '',
    decided_by   TEXT NOT NULL DEFAULT ''
) STRICT;

-- The two questions asked of this table are "the pending approvals of these sessions", newest
-- first, and "this one by id" (the primary key). The partial index answers the first without
-- carrying the decided rows, which are only ever read by id or through the audit log.
CREATE INDEX approvals_pending ON approvals (session_id, id) WHERE decision = '';

-- One row per action a person, the daemon, or an agent took that a person may need to account for:
-- approving or denying a request, turning bypass on or off, a commit the secret scanner blocked,
-- and the rest of Phase 3 (docs/architecture.md section 10). Read only from the API: a row is
-- written here and never edited or deleted, so the trail stays true.
--
-- session_id is empty for an action that belongs to no session (turning bypass on, for example), so
-- it carries no foreign key: '' is not a session id. actor is who did it ("person", "agent", or
-- "daemon"), action is the short verb ("approve", "deny", "bypass.on", ...), target is what it was
-- done to (a card id, a tool call id, a path), and detail_json is the structured payload the action
-- needs to be read in full. created_at is when it happened, in UTC milliseconds.
CREATE TABLE audit_log (
    id          TEXT NOT NULL PRIMARY KEY,
    session_id  TEXT NOT NULL DEFAULT '',
    actor       TEXT NOT NULL,
    action      TEXT NOT NULL,
    target      TEXT NOT NULL DEFAULT '',
    detail_json TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL
) STRICT;

-- The audit log is read newest first: the whole list, one action's list, and a search. The time is
-- what a person filters by, so each index leads with it. The order itself is the rowid's, not the
-- time's: two rows written in the same millisecond have the same time, and an id does not order them
-- either, because two ids made in the same millisecond differ only in random bits. Rowid is what
-- orders them, and it is the only thing in the table that does. (A rowid cannot be named in an index,
-- so these two are for the filters; the list's own order needs no index.)
CREATE INDEX audit_log_recent ON audit_log (created_at DESC, id DESC);
CREATE INDEX audit_log_action ON audit_log (action, created_at DESC, id DESC);
