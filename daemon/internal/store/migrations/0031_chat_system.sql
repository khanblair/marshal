-- A chat Marshal keeps for the project itself, such as the pinned Integrator chat. Forward only.
-- `system` is empty for a chat a person made; one project has at most one chat of each system kind.

-- +goose Up

ALTER TABLE chats ADD COLUMN system TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX chats_one_system_per_project ON chats (project_id, system) WHERE system != '';
