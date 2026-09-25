-- Agent sessions, one per card for its whole life in Phase 1 (chats, which need many sessions per
-- project, are a later phase). Forward only, like every migration. Times are INTEGER Unix
-- milliseconds in UTC and ids are TEXT. STRICT makes SQLite refuse a value of the wrong type.
--
-- The words a session can hold in state are checked in Go against the list in the protocol
-- package, not here, for the same reason cards.state is not checked here (see 0002_projects.sql).

-- +goose Up

-- agent_session_id is the agent's own id for the session (what Resume needs); it is set once
-- Start succeeds. card_id is UNIQUE: a card has at most one session for its whole life here.
CREATE TABLE sessions (
    id                TEXT NOT NULL PRIMARY KEY,
    card_id           TEXT NOT NULL UNIQUE REFERENCES cards (id) ON DELETE CASCADE,
    agent_kind        TEXT NOT NULL,
    agent_session_id  TEXT NOT NULL DEFAULT '',
    state             TEXT NOT NULL DEFAULT 'starting',
    model             TEXT NOT NULL DEFAULT '',
    thinking          TEXT NOT NULL DEFAULT '',
    permission_mode   TEXT NOT NULL DEFAULT '',
    last_active_at    INTEGER NOT NULL,
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
) STRICT;
CREATE INDEX sessions_state ON sessions (state);
