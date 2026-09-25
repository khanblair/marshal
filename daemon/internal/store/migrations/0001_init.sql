-- Everything here is forward only. Times are INTEGER Unix milliseconds in UTC and ids are TEXT.
-- STRICT makes SQLite refuse a value of the wrong type, so a time can never be stored as text.

-- +goose Up

CREATE TABLE settings (
    key        TEXT NOT NULL PRIMARY KEY,
    value_json TEXT NOT NULL
) STRICT;

CREATE TABLE users (
    id               TEXT NOT NULL PRIMARY KEY,
    name             TEXT NOT NULL,
    email            TEXT NOT NULL DEFAULT '',
    avatar_path      TEXT NOT NULL DEFAULT '',
    time_zone        TEXT NOT NULL DEFAULT '',
    tailnet_identity TEXT NOT NULL DEFAULT '',
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL
) STRICT;

-- token_hash is the SHA-256 of the client token in lower case hex. The token itself is never stored.
CREATE TABLE devices (
    id           TEXT NOT NULL PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users (id),
    name         TEXT NOT NULL,
    kind         TEXT NOT NULL CHECK (kind IN ('web', 'desktop', 'mobile', 'cli', 'dev')),
    token_hash   TEXT NOT NULL UNIQUE,
    paired_at    INTEGER NOT NULL,
    last_seen_at INTEGER,
    revoked_at   INTEGER
) STRICT;

CREATE INDEX devices_user_id ON devices (user_id);

CREATE TABLE user_progress (
    user_id            TEXT NOT NULL PRIMARY KEY REFERENCES users (id),
    onboarding_step    INTEGER NOT NULL DEFAULT 0,
    onboarding_done_at INTEGER,
    tutorial_done_at   INTEGER,
    tutorial_skipped   INTEGER NOT NULL DEFAULT 0 CHECK (tutorial_skipped IN (0, 1))
) STRICT;
