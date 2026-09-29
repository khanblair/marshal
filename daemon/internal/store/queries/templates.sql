-- Card templates: the reusable shape a new card starts from (migration 0024, docs/architecture.md
-- section 10's `templates` row; docs/backend-checklist.md B10.1, build-plan task 10.1). It replaces
-- the fixed five-item picklist the settings screen has today, so it is a table a person can add to.
--
-- A template is edited as one thing and read as one thing, which is why its spec is one JSON column
-- and every query here reads or writes the whole row rather than its parts.

-- name: CreateTemplate :exec
-- Makes a template. A duplicate name is refused by the service with a sentence a person can act on,
-- rather than left to the schema: two templates named the same is a person's mistake, and a
-- constraint violation would be shown as one.
INSERT INTO templates (id, name, spec_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?);

-- name: GetTemplate :one
-- One template by id. sql.ErrNoRows means there is no such template, which is how a card made from
-- one that has since been deleted is told.
SELECT * FROM templates WHERE id = ?;

-- name: UpdateTemplate :exec
-- Renames a template or replaces its spec. Both are sent together because both are edited in one
-- form, and `updated_at` moves so a screen can order templates by when they were last changed.
UPDATE templates
SET name = ?, spec_json = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteTemplate :exec
-- Removes a template. Cards already made from it keep their own copy of what they started with, so
-- deleting a template does not rewrite history - it only stops the next card using it.
DELETE FROM templates WHERE id = ?;

-- name: ListTemplates :many
-- Every template, in the order the picker shows them: by name, so a person finds theirs without
-- scanning. `id` breaks the tie so two templates named alike still come out in the same order twice.
SELECT * FROM templates ORDER BY name, id;

-- name: CountTemplates :one
-- How many templates there are. The settings screen offers the built-in five until a person makes
-- their own, and this is what tells it whether they have.
SELECT COUNT(*) FROM templates;
