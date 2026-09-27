-- Connected services and the result of the last connection test (docs/architecture.md section 10's
-- `integrations` row, and section 18 "Connection tests"). Forward only, like every migration. Times
-- are INTEGER Unix milliseconds in UTC and ids are TEXT. STRICT makes SQLite refuse a value of the
-- wrong type.
--
-- One table serves every kind of connection Marshal can be set up with or signed in to - a model
-- provider today, GitHub, Trello, Calendar, Gmail, Telegram, Discord, and MCP servers in later
-- phases - because section 18 gives them all the same mechanics: connect, test, save the result,
-- and cool down before the next test. `kind` says which sort of connection a row is ("provider"
-- today), and `id` is that connection's own id (for a provider, its provider id, such as
-- "anthropic").
--
-- The table is deliberately thin. `config_json` holds the non-secret settings a connection needs (a
-- repository, a label, a calendar); `keychain_ref` names where its secret lives in the OS keychain
-- and is never the secret itself; and the two `last_test_*` columns are the last test's time and its
-- whole result, in the shape the protocol's TestResult is sent as. A result is replaced in place -
-- only the newest matters, and the cooldown reads `last_test_at` alone.
--
-- No foreign key: a connection is owned by no project and no user, and this table outlives the rest
-- of the data the way `settings` does. `last_test_at` is 0, never NULL, for a connection that has
-- never been tested, so the generated parameter and row types stay plain integers.

-- +goose Up

CREATE TABLE integrations (
    id                    TEXT NOT NULL PRIMARY KEY,
    kind                  TEXT NOT NULL,
    config_json           TEXT NOT NULL DEFAULT '',
    keychain_ref          TEXT NOT NULL DEFAULT '',
    last_test_at          INTEGER NOT NULL DEFAULT 0 CHECK (last_test_at >= 0),
    last_test_result_json TEXT NOT NULL DEFAULT ''
) STRICT;

-- A test's cooldown and a "last checked" line read one row by its id, which the primary key serves.
-- A screen that lists the connections of one kind ("every provider", "every integration") reads by
-- kind, which is the one other index.
CREATE INDEX integrations_kind ON integrations (kind);
