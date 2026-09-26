-- Project chats and their own sessions (docs/architecture.md 16.2, docs/backend-checklist.md B2.10,
-- inventory N13). Forward only, like every migration. Times are INTEGER Unix milliseconds in UTC
-- and ids are TEXT. STRICT makes SQLite refuse a value of the wrong type.
--
-- A chat belongs to one project and talks to one target: the Orchestrator, a role, or a card's own
-- agent (the `target_kind` and `target_id` of architecture.md section 10). It keeps its own lasting
-- session, stored in `sessions` with `chat_id` set rather than `card_id`, so the two tables below
-- are made in one migration: SQLite cannot drop the NOT NULL on `sessions.card_id` without
-- rebuilding the table, and a session must be able to belong to a chat instead of a card.
--
-- The words `target_kind`, `agent_kind`, `thinking`, and `permission_mode` can hold are checked in
-- Go against the lists in the protocol package, not here, for the same reason cards.state is not
-- checked here (see 0002_projects.sql).

-- +goose Up
-- +goose NO TRANSACTION

-- The statements below run without a wrapping transaction, and with foreign keys off, because
-- rebuilding `sessions` means dropping a table that `session_events` points at: with foreign keys
-- on, SQLite answers DROP TABLE on a parent by deleting its children's rows first, which would
-- silently empty a card's stored history. Every connection opens with foreign_keys(1) (see
-- dsn in store.go) and the writer pool holds one connection, so the pragma below reaches every
-- statement of this migration and is turned back on at the end, in the same connection.
PRAGMA foreign_keys = OFF;

-- A chat's title is "New chat" until its first message gives it a name (a cheap model writes it in
-- Phase 4, B4.7; until then the first words are used). `archived_at` is NULL while the chat is in
-- the main list and holds the moment it was archived otherwise, so restore is one NULL.
--
-- `agent_kind`, `model`, `thinking`, and `permission_mode` are the settings its session starts
-- with, copied onto the session row when the session first starts. They are here rather than in a
-- second settings table because a chat has exactly one session for its whole life.
CREATE TABLE chats (
    id              TEXT NOT NULL PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    target_kind     TEXT NOT NULL DEFAULT 'orchestrator',
    target_id       TEXT NOT NULL DEFAULT '',
    agent_kind      TEXT NOT NULL DEFAULT 'claude',
    model           TEXT NOT NULL DEFAULT '',
    thinking        TEXT NOT NULL DEFAULT '',
    permission_mode TEXT NOT NULL DEFAULT '',
    archived_at     INTEGER,
    last_active_at  INTEGER NOT NULL,
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
) STRICT;

-- A project's chats are read newest-active first, and the archived ones are filtered out of the
-- main list, so both halves of the list come from this one index.
CREATE INDEX chats_project_active ON chats (project_id, archived_at, last_active_at);

-- The sessions of Phase 1 belonged to a card and only a card: `card_id` was NOT NULL UNIQUE. A
-- session now belongs to a card or a chat, so the unique rule moves into a partial index where
-- `card_id` is set and a second one gives a chat the same rule. SQLite cannot drop a UNIQUE
-- constraint in place, so the table is rebuilt once here and the rows are copied as they are.
-- Every other column, its order, and its defaults are unchanged.
--
-- The one difference a Go reader will notice: `card_id` stays NOT NULL, with an empty string as
-- "this session belongs to a chat, not a card", rather than becoming a nullable column. That is
-- deliberate. `sessions` is read all over the daemon (internal/session, internal/history,
-- internal/cardhistory, internal/dashboard), and a nullable column would make the generated
-- `db.Session.CardID` a *string and change every one of those call sites in a slice about chats.
-- An empty string is distinguishable from NULL in SQLite, so the invariant is exact: the CHECK
-- below allows a row with a card or a row with a chat, and never one with neither or both.
-- card_id carries no REFERENCES of its own: a foreign key on a NOT NULL column requires every
-- value, including the empty-string sentinel a chat-owned session stores, to match a row in
-- `cards` (only NULL is ever exempt), so the constraint that satisfied a real card would reject
-- every chat. The cascade a REFERENCES ... ON DELETE CASCADE gave a card-owned session before is
-- done by the trigger below instead, on the same condition (a card that had a session, deleted).
CREATE TABLE sessions_new (
    id                TEXT NOT NULL PRIMARY KEY,
    card_id           TEXT NOT NULL DEFAULT '',
    chat_id           TEXT REFERENCES chats (id) ON DELETE CASCADE,
    agent_kind        TEXT NOT NULL,
    agent_session_id  TEXT NOT NULL DEFAULT '',
    state             TEXT NOT NULL DEFAULT 'starting',
    model             TEXT NOT NULL DEFAULT '',
    thinking          TEXT NOT NULL DEFAULT '',
    permission_mode   TEXT NOT NULL DEFAULT '',
    last_active_at    INTEGER NOT NULL,
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    CHECK ((card_id = '') <> (chat_id IS NULL))
) STRICT;

INSERT INTO sessions_new (
    id, card_id, chat_id, agent_kind, agent_session_id, state, model, thinking,
    permission_mode, last_active_at, created_at, updated_at
)
SELECT
    id, card_id, NULL, agent_kind, agent_session_id, state, model, thinking,
    permission_mode, last_active_at, created_at, updated_at
FROM sessions;

DROP TABLE sessions;
ALTER TABLE sessions_new RENAME TO sessions;

CREATE UNIQUE INDEX sessions_card ON sessions (card_id) WHERE card_id <> '';
CREATE UNIQUE INDEX sessions_chat ON sessions (chat_id) WHERE chat_id IS NOT NULL;
CREATE INDEX sessions_state ON sessions (state);

-- Replaces the cascade the old card_id foreign key gave for free: a card-owned session must not
-- outlive its card. The StatementBegin/End pair keeps goose from splitting this on the semicolon
-- inside the trigger body.
-- +goose StatementBegin
CREATE TRIGGER sessions_card_cascade_delete AFTER DELETE ON cards
BEGIN
    DELETE FROM sessions WHERE card_id = OLD.id;
END;
-- +goose StatementEnd

PRAGMA foreign_keys = ON;
