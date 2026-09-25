-- Projects, their boards, and their cards. Forward only, like every migration. Times are INTEGER
-- Unix milliseconds in UTC and ids are TEXT. STRICT makes SQLite refuse a value of the wrong type.
--
-- The words a card can hold in state, agent_kind, thinking, and permission_mode are checked in Go
-- against the lists in the protocol package, not here. SQLite cannot change a CHECK without
-- rebuilding the table, and those lists are meant to grow.

-- +goose Up

-- repo_path is the top folder of the repository, absolute and cleaned. Two projects cannot share
-- one. next_card_number is the number the next card gets: each project counts its own cards, and
-- a number is never used twice, so a card key (project id and number) stays true for good.
CREATE TABLE projects (
    id               TEXT NOT NULL PRIMARY KEY,
    name             TEXT NOT NULL,
    repo_path        TEXT NOT NULL UNIQUE,
    default_branch   TEXT NOT NULL DEFAULT '',
    language         TEXT NOT NULL DEFAULT '',
    dev_command      TEXT NOT NULL DEFAULT '',
    bypass_locked    INTEGER NOT NULL DEFAULT 0 CHECK (bypass_locked IN (0, 1)),
    is_monorepo      INTEGER NOT NULL DEFAULT 0 CHECK (is_monorepo IN (0, 1)),
    packages_json    TEXT NOT NULL DEFAULT '[]',
    next_card_number INTEGER NOT NULL DEFAULT 1 CHECK (next_card_number >= 1),
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL
) STRICT;

-- columns_json is a JSON list of the card states that have a column, in board order.
CREATE TABLE boards (
    id           TEXT NOT NULL PRIMARY KEY,
    project_id   TEXT NOT NULL UNIQUE REFERENCES projects (id) ON DELETE CASCADE,
    columns_json TEXT NOT NULL
) STRICT;

-- An empty thinking means the card has no thinking setting. worktree_path and branch are empty
-- until the card starts. role_id, pinned, and created_by are here for the modules that come next.
CREATE TABLE cards (
    id              TEXT NOT NULL PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    number          INTEGER NOT NULL CHECK (number >= 1),
    board_id        TEXT NOT NULL REFERENCES boards (id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    body            TEXT NOT NULL DEFAULT '',
    state           TEXT NOT NULL DEFAULT 'backlog',
    role_id         TEXT NOT NULL DEFAULT '',
    agent_kind      TEXT NOT NULL DEFAULT 'claude',
    model           TEXT NOT NULL DEFAULT '',
    thinking        TEXT NOT NULL DEFAULT '',
    permission_mode TEXT NOT NULL DEFAULT 'auto-edits',
    branch          TEXT NOT NULL DEFAULT '',
    worktree_path   TEXT NOT NULL DEFAULT '',
    pinned          INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0, 1)),
    created_by      TEXT NOT NULL DEFAULT '',
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,
    UNIQUE (project_id, number)
) STRICT;

CREATE INDEX cards_project_state ON cards (project_id, state);
