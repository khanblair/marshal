-- The card fields the screens show that Phase 1's model lacked (inventory N1), and labels
-- (decision D3). Forward only, like every migration. Times are INTEGER Unix milliseconds in UTC
-- and ids are TEXT. STRICT makes SQLite refuse a value of the wrong type.
--
-- nullable INTEGER time columns are the dates a person may not have set. An empty string in
-- ci_state and needs_reason_kind means "no value", the same way cards.thinking already does.

-- +goose Up

ALTER TABLE cards ADD COLUMN role TEXT NOT NULL DEFAULT '';
ALTER TABLE cards ADD COLUMN package TEXT NOT NULL DEFAULT '';
ALTER TABLE cards ADD COLUMN planned_start INTEGER;
ALTER TABLE cards ADD COLUMN planned_end INTEGER;
ALTER TABLE cards ADD COLUMN due INTEGER;
ALTER TABLE cards ADD COLUMN actual_start INTEGER;
ALTER TABLE cards ADD COLUMN actual_end INTEGER;
ALTER TABLE cards ADD COLUMN pull_request_number INTEGER;
ALTER TABLE cards ADD COLUMN pull_request_url TEXT NOT NULL DEFAULT '';
ALTER TABLE cards ADD COLUMN ci_state TEXT NOT NULL DEFAULT '';
ALTER TABLE cards ADD COLUMN context_used INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cards ADD COLUMN needs_reason_kind TEXT NOT NULL DEFAULT '';
ALTER TABLE cards ADD COLUMN needs_reason_text TEXT NOT NULL DEFAULT '';
-- needs_since is when the card entered the needs state, so Home can show how long it has waited.
-- It is cleared when the card leaves that state.
ALTER TABLE cards ADD COLUMN needs_since INTEGER;
ALTER TABLE cards ADD COLUMN doing_now TEXT NOT NULL DEFAULT '';
ALTER TABLE cards ADD COLUMN paused INTEGER NOT NULL DEFAULT 0 CHECK (paused IN (0, 1));

-- A label belongs to one project and its name is unique there. The color is one of the fixed
-- label colors, checked in Go against protocol.LabelColor, not here (see 0002 for why).
CREATE TABLE labels (
    id         TEXT NOT NULL PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    color      TEXT NOT NULL DEFAULT 'slate',
    created_at INTEGER NOT NULL,
    UNIQUE (project_id, name)
) STRICT;

CREATE INDEX labels_project ON labels (project_id, name);

-- A card has any number of a project's labels. The row exists only while both ends do.
CREATE TABLE card_labels (
    card_id  TEXT NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    label_id TEXT NOT NULL REFERENCES labels (id) ON DELETE CASCADE,
    PRIMARY KEY (card_id, label_id)
) STRICT;

CREATE INDEX card_labels_label ON card_labels (label_id);
