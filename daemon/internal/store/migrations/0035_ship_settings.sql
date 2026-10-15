-- How a project ships its cards: whether a finished card goes through review, checks and the merge
-- by itself, and whether anything may leave the machine. The columns are on the table that already
-- holds how a project merges. Forward only, like every migration.
--
-- Everything is on except pushing: a project that never opened this screen ships locally and
-- publishes nothing.
-- +goose Up

ALTER TABLE project_merge_settings ADD COLUMN auto_ship INTEGER NOT NULL DEFAULT 1 CHECK (auto_ship IN (0, 1));
ALTER TABLE project_merge_settings ADD COLUMN review INTEGER NOT NULL DEFAULT 1 CHECK (review IN (0, 1));
ALTER TABLE project_merge_settings ADD COLUMN checks INTEGER NOT NULL DEFAULT 1 CHECK (checks IN (0, 1));
ALTER TABLE project_merge_settings ADD COLUMN allow_push INTEGER NOT NULL DEFAULT 0 CHECK (allow_push IN (0, 1));
