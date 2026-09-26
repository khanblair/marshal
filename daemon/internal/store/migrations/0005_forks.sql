-- A card that was forked from another records where it came from, so its branch starts from that
-- card's latest commit instead of the project's default branch. Forward only, like every
-- migration.

-- +goose Up

ALTER TABLE cards ADD COLUMN forked_from TEXT NOT NULL DEFAULT '';
