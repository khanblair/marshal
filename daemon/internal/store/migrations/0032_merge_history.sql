-- What the Integrator delivered, and how it is set for each project (Integrator flow). Forward only,
-- like every migration. Times are INTEGER Unix milliseconds in UTC and ids are TEXT. STRICT makes
-- SQLite refuse a value of the wrong type.
--
-- `merge_history` has one row for every card the Integrator delivered. `commit_sha` is the commit
-- the integration branch moved to and `prev_tip` is where it was, which is all an undo needs.
-- `wip_ref` names the ref that keeps a snapshot of the owner's uncommitted work, and `folder_tree`
-- is the tree the owner's folder was left holding, when the delivery had to merge that work in; both
-- are empty for a delivery that did not. `undone_at` is 0 until the owner undoes the delivery. A
-- history row goes with its project and with its card.
--
-- `project_merge_settings` has no row until a setting changes, and a missing row reads as the
-- defaults: not paused, merging on its own. `pending_tip` is the commit on the integrator branch
-- that passed its tests but has not been delivered, empty when nothing waits.

-- +goose Up

CREATE TABLE merge_history (
    id            TEXT NOT NULL PRIMARY KEY,
    project_id    TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    card_id       TEXT NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    target        TEXT NOT NULL,
    merged_at     INTEGER NOT NULL,
    commit_sha    TEXT NOT NULL,
    prev_tip      TEXT NOT NULL,
    resolved      INTEGER NOT NULL DEFAULT 0,
    summary       TEXT NOT NULL DEFAULT '',
    backup_branch TEXT NOT NULL DEFAULT '',
    wip_ref       TEXT NOT NULL DEFAULT '',
    folder_tree   TEXT NOT NULL DEFAULT '',
    undone_at     INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE INDEX merge_history_project ON merge_history (project_id, merged_at DESC);
CREATE INDEX merge_history_card ON merge_history (card_id);

CREATE TABLE project_merge_settings (
    project_id  TEXT NOT NULL PRIMARY KEY REFERENCES projects (id) ON DELETE CASCADE,
    paused      INTEGER NOT NULL DEFAULT 0 CHECK (paused IN (0, 1)),
    auto_merge  INTEGER NOT NULL DEFAULT 1 CHECK (auto_merge IN (0, 1)),
    pending_tip TEXT NOT NULL DEFAULT '',
    updated_at  INTEGER NOT NULL
) STRICT;
