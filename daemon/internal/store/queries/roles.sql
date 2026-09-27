-- Roles and per-project role overrides (migration 0016, docs/architecture.md section 10, inventory
-- N18). A role is a template the screens edit and a project can keep its own version of. The spec
-- is one JSON document and is never read or written piecemeal.

-- name: ListRoles :many
-- Every role, in the order the screens show them: Marshal's starter roles first in the order they
-- were seeded, then roles a person made in the order they were made. Rowid is that order - a role is
-- never moved, and neither a name nor an id orders the list the way the seed does (two ids made in
-- the same millisecond differ only in random bits).
SELECT * FROM roles ORDER BY rowid;

-- name: GetRole :one
-- One role by its own id. ErrNoSuchRole (sql.ErrNoRows) means no such role.
SELECT * FROM roles WHERE id = ?;

-- name: GetRoleByName :one
-- One role by its name, which is unique. This is how the API layer reaches a role, because that is
-- what a route, a card, and a chat target name.
SELECT * FROM roles WHERE name = ?;

-- name: CreateRole :exec
-- Adds a role. A name that is taken is a constraint failure; the service checks first so it can
-- answer with a sentence.
INSERT INTO roles (id, name, is_starter, spec_json) VALUES (?, ?, ?, ?);

-- name: CreateStarterRole :execrows
-- Adds a starter role if its name is not in use, and does nothing if it is. This is how the daemon
-- seeds its starter roles at start-up: a name already there is a role a person has, so their edits
-- are left alone and a role is never doubled. It answers how many rows were added, so the seeder can
-- say how many were really added.
INSERT INTO roles (id, name, is_starter, spec_json) VALUES (?, ?, 1, ?)
ON CONFLICT (name) DO NOTHING;

-- name: UpdateRole :execrows
-- Renames a role, replaces its spec, or both. Answers how many rows changed, so a role that went
-- away between the read and the write is reported as not found.
UPDATE roles SET name = ?, spec_json = ? WHERE id = ?;

-- name: DeleteRole :execrows
-- Removes a role. Every project's override of it goes with it.
DELETE FROM roles WHERE id = ?;

-- name: ListRoleOverridesByProject :many
-- One project's overrides of every role, so a role list can say which roles this project overrides
-- without asking once per role.
SELECT * FROM role_overrides WHERE project_id = ?;

-- name: GetRoleOverride :one
-- One project's version of one role. ErrNoRows means the project keeps no override and the role's
-- own spec is what it uses.
SELECT * FROM role_overrides WHERE role_id = ? AND project_id = ?;

-- name: SetRoleOverride :exec
-- Sets or replaces one project's version of one role, so saving twice leaves one row.
INSERT INTO role_overrides (role_id, project_id, spec_json)
VALUES (?, ?, ?)
ON CONFLICT (role_id, project_id) DO UPDATE SET spec_json = excluded.spec_json;

-- name: DeleteRoleOverride :exec
-- Removes one project's version of one role. Removing an override that is not set is not an error:
-- a reset that is asked for twice answers the same list both times.
DELETE FROM role_overrides WHERE role_id = ? AND project_id = ?;
