-- The branch finished cards merge into, and where a card is in the merge (Integrator flow). Forward
-- only. An empty integration_branch reads as the project's default branch, so existing projects
-- behave as before until the owner chooses one.

-- +goose Up

ALTER TABLE projects ADD COLUMN integration_branch TEXT NOT NULL DEFAULT '';
ALTER TABLE cards ADD COLUMN merge_phase TEXT NOT NULL DEFAULT '';
ALTER TABLE cards ADD COLUMN merge_note TEXT NOT NULL DEFAULT '';
