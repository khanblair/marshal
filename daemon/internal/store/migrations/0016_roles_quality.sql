-- Roles, per-project role overrides, checkpoints, and the smell-check tables (docs/architecture.md
-- section 10; docs/backend-checklist.md Phase 5 items B5.1 "Roles", B5.3 "Limits, stuck detection,
-- and checkpoints", and B5.8 "Quality module"). Forward only, like every migration. Times are
-- INTEGER Unix milliseconds in UTC and ids are TEXT. STRICT makes SQLite refuse a value of the wrong
-- type.
--
-- The words these tables can hold that are meant to grow - a smell's family, its severity, and
-- whether a finding is open, fixed, or dismissed - are checked in Go against the lists in the
-- protocol package and not here, for the same reason cards.state is not checked here (see
-- 0002_projects.sql): SQLite cannot change a CHECK without rebuilding the table.
--
-- `roles` is Marshal's library of role templates: the eight starter roles it ships, and any role a
-- person makes by duplicating or importing one. A role is a name and a spec, where the spec is the
-- whole editable body (its description, instructions, agent, model, thinking, permission mode,
-- strength, backup model, skills, MCP servers, and its own time, cost, and round limits) kept as one
-- JSON document so the shape can grow without a migration. `is_starter` marks a role Marshal shipped
-- rather than one a person made: the editor shows a starter role a "Reset to starter" button and a
-- custom role a "Delete role" button (apps/web/src/views/settings/RoleEditor.tsx).
--
-- `role_overrides` is one project's own version of a role. A role is global; a project that needs
-- different instructions or a cheaper model for one role gets a row here rather than a second role.
-- The pair (role_id, project_id) is the primary key, so a project holds at most one override of each
-- role, and the row is what makes the role read as "overridden" for that project. Resetting a role
-- removes only this row: it clears the flag and leaves the role's own spec alone, which is exactly
-- what the prototype's confirmResetRole does (apps/web/src/views/settings/role-actions.ts).
--
-- `checkpoints` is a card's restore points (B5.3, architecture.md 9.9): a Git commit on a hidden ref
-- that Marshal made before an agent turn, before a big edit, or before a merge, with a label saying
-- what it was made before. A card's checkpoints are read newest first, and restoring one puts the
-- worktree - and, on request, the conversation - back to that commit.
--
-- `smell_profiles` is one project's smell-check settings (B5.8, architecture.md section 17): which
-- built-in families are on, their thresholds, and which severities block. It is one JSON document
-- per project, keyed by the project, and a project with no row uses the built-in defaults.
--
-- `smell_findings` is what a check found in one card's diff: one row per finding, tied to the commit
-- it was found in so a cached answer is per commit and an old finding is never blamed on a newer
-- one. `status` is open, fixed, or dismissed, and a dismissed finding keeps the reason in
-- `dismiss_reason`, so the trail of why a warning was waved away stays true. `family` is which of
-- the nine smell families the finding belongs to (product scope 15.4), and `smell` is the rule's own
-- name: one of Marshal's built-in checks, or a project linter's rule.

-- +goose Up

-- Role templates. `name` is unique: the screens list roles by name and a card names its role by
-- name, so two roles with one name would be unusable. `spec_json` is never empty.
CREATE TABLE roles (
    id         TEXT NOT NULL PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    is_starter INTEGER NOT NULL DEFAULT 0 CHECK (is_starter IN (0, 1)),
    spec_json  TEXT NOT NULL
) STRICT;

-- One project's version of one role. The project owns the row: removing a project takes its
-- overrides with it, and removing a role takes every project's override of it.
CREATE TABLE role_overrides (
    role_id    TEXT NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    spec_json  TEXT NOT NULL,
    PRIMARY KEY (role_id, project_id)
) STRICT;

-- A card's restore point. `git_ref` is the commit Marshal made; `label` is what it was made before
-- ("before turn 3", "before merge"), and is empty when there is nothing to say. created_at is when
-- it was made, in UTC milliseconds, and the card's own list is newest first.
CREATE TABLE checkpoints (
    id         TEXT NOT NULL PRIMARY KEY,
    card_id    TEXT NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    git_ref    TEXT NOT NULL,
    label      TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
) STRICT;

-- A card's checkpoints are read newest first, and a card is removed with its checkpoints. The id
-- is an opaque id whose first ten characters are the time, but two checkpoints made in the same
-- millisecond sort by nothing in the id, so created_at leads the index.
CREATE INDEX checkpoints_card_created ON checkpoints (card_id, created_at DESC, id DESC);

-- One project's smell-check settings. A project with no row uses the built-in defaults, so this is
-- empty on a fresh install.
CREATE TABLE smell_profiles (
    project_id TEXT NOT NULL PRIMARY KEY REFERENCES projects (id) ON DELETE CASCADE,
    spec_json  TEXT NOT NULL
) STRICT;

-- One smell a check found in a card's diff. `commit_sha` is the commit the finding was found in, so
-- a check's answer is cached per commit and a finding in an older commit is not re-blamed on a newer
-- one; it is empty for a finding that came from a check of uncommitted work. `family` is which of
-- the nine smell families the finding belongs to, `smell` is the rule's own name, `severity` is how
-- much it matters, and `status` is open, fixed, or dismissed. A dismissed finding keeps why in
-- `dismiss_reason`.
--
-- The column is `commit_sha` and not `commit` because COMMIT is a SQL keyword; the data model's row
-- calls it `commit` (architecture.md section 10) and this is the same thing under a name SQLite
-- reads without quoting.
CREATE TABLE smell_findings (
    id             TEXT NOT NULL PRIMARY KEY,
    card_id        TEXT NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    commit_sha     TEXT NOT NULL DEFAULT '',
    family         TEXT NOT NULL,
    smell          TEXT NOT NULL,
    file           TEXT NOT NULL,
    line           INTEGER NOT NULL DEFAULT 0 CHECK (line >= 0),
    severity       TEXT NOT NULL,
    message        TEXT NOT NULL DEFAULT '',
    suggestion     TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'open',
    dismiss_reason TEXT NOT NULL DEFAULT ''
) STRICT;

-- A card's findings are read by card and by commit, and a check reads the ones already there for a
-- commit before it runs again. The status is what the card's panel filters on, so it follows the
-- commit in the index.
CREATE INDEX smell_findings_card_commit ON smell_findings (card_id, commit_sha, status);
