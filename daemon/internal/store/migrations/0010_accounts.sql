-- The person using Marshal and a project's saved views (docs/backend-checklist.md B2.2, B2.5, and
-- B2.13, inventory N20, N27, and N30, decision D2). Forward only, like every migration. Times are
-- INTEGER Unix milliseconds in UTC and ids are TEXT. STRICT makes SQLite refuse a value of the wrong
-- type.
--
-- Two modules own what is here: internal/accounts owns the avatar and progress columns and the two
-- preferences tables, and internal/projects owns `saved_views`. They share one file because this
-- slice was given one migration number; each module reads and writes only its own tables.
--
-- The words `theme`, `last_view`, and `swimlane` can hold are checked in Go against the lists in
-- the protocol package, not here, for the same reason cards.state is not (see 0002_projects.sql).

-- +goose Up

-- `users.avatar_path` (0001) holds the avatar's file name under <data>/avatars, or '' when there is
-- none. `avatar_updated_at` is when the image last changed, which versions the avatar's address so a
-- new image is never served from a stale cache.
ALTER TABLE users ADD COLUMN avatar_updated_at INTEGER;

-- `user_progress` (0001) has `tutorial_skipped` but no way to tell a finished onboarding from a
-- skipped one. `onboarding_done_at` is set for both, as `tutorial_done_at` is.
ALTER TABLE user_progress ADD COLUMN onboarding_skipped INTEGER NOT NULL DEFAULT 0
    CHECK (onboarding_skipped IN (0, 1));

-- A project's saved views: a name, the filter chips, and a swimlane. The name is unique in its
-- project, and saving under a name in use replaces that view. The list is read in the order the
-- views were last saved.
CREATE TABLE saved_views (
    id           TEXT NOT NULL PRIMARY KEY,
    project_id   TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    filters_json TEXT NOT NULL DEFAULT '[]',
    swimlane     TEXT NOT NULL DEFAULT 'none',
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL,
    UNIQUE (project_id, name)
) STRICT;

CREATE INDEX saved_views_project ON saved_views (project_id, updated_at);

-- A person's screen preferences that follow them between devices (D2). There is no row until the
-- first change, and a missing row reads as the defaults. `list_columns_json` is an object of the
-- List columns the person showed or hid, by the column's key; `sort_json` holds the two tables'
-- orders, each absent until changed.
CREATE TABLE user_preferences (
    user_id           TEXT NOT NULL PRIMARY KEY REFERENCES users (id),
    theme             TEXT NOT NULL DEFAULT 'system',
    list_columns_json TEXT NOT NULL DEFAULT '{}',
    sort_json         TEXT NOT NULL DEFAULT '{}',
    updated_at        INTEGER NOT NULL
) STRICT;

-- One project's screen preferences for one person. The row goes with its project. A saved view in
-- use that is deleted leaves no view in use, rather than an id that points nowhere.
CREATE TABLE project_preferences (
    user_id              TEXT NOT NULL REFERENCES users (id),
    project_id           TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    last_view            TEXT NOT NULL DEFAULT 'board',
    filters_json         TEXT NOT NULL DEFAULT '[]',
    query                TEXT NOT NULL DEFAULT '',
    swimlane             TEXT NOT NULL DEFAULT 'none',
    collapsed_lanes_json TEXT NOT NULL DEFAULT '[]',
    show_all_done        INTEGER NOT NULL DEFAULT 0 CHECK (show_all_done IN (0, 1)),
    saved_view_id        TEXT REFERENCES saved_views (id) ON DELETE SET NULL,
    updated_at           INTEGER NOT NULL,
    PRIMARY KEY (user_id, project_id)
) STRICT;

-- SQLite finds the rows a removed project or a deleted saved view changes through these.
CREATE INDEX project_preferences_project ON project_preferences (project_id);
CREATE INDEX project_preferences_saved_view ON project_preferences (saved_view_id);
