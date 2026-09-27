-- Connected services and their last connection test (migration 0015, docs/architecture.md section
-- 18). internal/connectiontest is the only writer of the two last_test_* columns; a later phase's
-- integration module owns config_json and keychain_ref.

-- name: GetIntegration :one
-- One connection by id. sql.ErrNoRows means Marshal has no row for it yet, which is the state of a
-- connection that has never been connected - a provider nobody has saved a key for.
SELECT * FROM integrations WHERE id = ?;

-- name: ListIntegrations :many
-- Every connection of one kind, by id. The providers Marshal knows are the catalog's own list
-- (internal/providers/catalog.go); this is the ones there is a row for, which is the ones that have
-- been saved or tested at least once.
SELECT * FROM integrations WHERE kind = ? ORDER BY id;

-- name: SetIntegrationTest :exec
-- Records the result of one connection test, making the row the first time. On conflict only the two
-- test columns change, so a test never clobbers a connection's settings or its keychain reference.
-- The kind is written on insert and left alone afterwards, so the row keeps the kind it was made
-- with.
INSERT INTO integrations (id, kind, last_test_at, last_test_result_json)
VALUES (?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    last_test_at          = excluded.last_test_at,
    last_test_result_json = excluded.last_test_result_json;

-- name: UpsertIntegration :exec
-- Saves a connection's own settings and the name of its secret, making the row the first time. It
-- never touches the two test columns: what is set up and what the last test found are separate
-- facts, and saving a new key does not make the last result about the old key disappear - the
-- automatic test that follows a save replaces it a moment later (section 18). The kind is written
-- on insert so a row always says what sort of connection it is.
INSERT INTO integrations (id, kind, config_json, keychain_ref)
VALUES (?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    kind         = excluded.kind,
    config_json  = excluded.config_json,
    keychain_ref = excluded.keychain_ref;

-- name: DeleteIntegration :exec
-- Forgets a connection entirely, settings and last result alike. Removing a connection that has no
-- row is not an error: the answer is the same list either way, which is how removing a provider key
-- that is not there behaves.
DELETE FROM integrations WHERE id = ?;
