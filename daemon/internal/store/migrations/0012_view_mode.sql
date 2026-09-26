-- The view a card's agent runs in (docs/architecture.md 4.3, docs/backend-checklist.md B2.7). A
-- card's agent runs in the chat view, which is its structured mode, or in the terminal view, which
-- is its own interactive command in a pseudo-terminal. One process cannot serve both, so switching
-- stops the process and resumes the same agent session id in the other mode. The mode is stored
-- with the session so that it survives a restart: the restore reads it from the row it already
-- holds and resumes the session in the mode it was left in, and the wire card reads it from here.
--
-- Forward only, like every migration. Every session that already exists was in the chat view, so
-- the default is 'chat' and nothing else changes. A session that stops is put back in the chat view
-- by the query that writes its state (queries/sessions.sql, UpdateSessionRuntime), not by a trigger,
-- so a row is only ever read as 'terminal' while its process runs in one, or was running in one
-- when the daemon stopped.
--
-- A project chat's session (its chat_id is set) has no terminal view: the check allows 'terminal'
-- only on a card's session. A check on a column may name the other columns of its row, which is
-- what lets ALTER TABLE ADD COLUMN state the rule, since SQLite cannot add a table constraint to an
-- existing table.

-- +goose Up

ALTER TABLE sessions ADD COLUMN view_mode TEXT NOT NULL DEFAULT 'chat'
    CHECK (view_mode IN ('chat', 'terminal') AND (view_mode = 'chat' OR chat_id IS NULL));
