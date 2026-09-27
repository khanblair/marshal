-- Installed skills (migration 0019, docs/architecture.md section 10's `skills` row, section 12's
-- `<data>/skills/`). A role names the skills it wants, the same way it names MCP servers, and this
-- table is what those names are resolved against. The skills manager and its screen belong to Phase
-- 11 (build-plan task 11.6); Phase 7 stores the registry so a role's skill names can be read back
-- and so the session start can say plainly when one is missing.

-- name: ListSkills :many
-- Every installed skill, by name.
SELECT * FROM skills ORDER BY name;

-- name: GetSkill :one
-- One skill by its own id.
SELECT * FROM skills WHERE id = ?;

-- name: GetSkillByName :one
-- One skill by the name a role names it with. sql.ErrNoRows means the role asks for a skill that is
-- not installed.
SELECT * FROM skills WHERE name = ?;

-- name: UpsertSkill :exec
-- Saves a skill's folder and where it came from, making the row the first time.
INSERT INTO skills (id, name, path, source)
VALUES (?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    name   = excluded.name,
    path   = excluded.path,
    source = excluded.source;

-- name: DeleteSkill :exec
-- Forgets a skill. Removing one that has no row is not an error, the same as every other delete in
-- this module.
DELETE FROM skills WHERE id = ?;
