-- The Trello apply flow (docs/marshal-product-scope.md section 19.2, docs/architecture.md section
-- 19.4). Marshal reads a linked board and works on its own copy, so it must remember what Trello held
-- when it last looked: the difference between that and the card now is what is pending. Forward only,
-- like every migration.
--
-- `trello_snapshots` holds one card's Trello side as it was last read or last written: its list,
-- title, description, checklists, and the comments Marshal already knows. `from_trello` says the
-- card was first made in Trello, not pushed to it later. The Marshal card's own ids for checklists,
-- items, and comments are matched to Trello's in `trello_item_links`, so a comment that came from
-- Trello is never sent back, and a ticked item is the same item on both sides.
--
-- `trello_dismissed` keeps a change the person chose not to send. The id changes when the change
-- does, so an edited change is shown again. `trello_sync_state` is when a project's board was last
-- read and what the board is called.

-- +goose Up

CREATE TABLE trello_snapshots (
    card_id        TEXT NOT NULL PRIMARY KEY REFERENCES cards (id) ON DELETE CASCADE,
    trello_card_id TEXT NOT NULL,
    url            TEXT NOT NULL DEFAULT '',
    from_trello    INTEGER NOT NULL DEFAULT 1 CHECK (from_trello IN (0, 1)),
    snapshot_json  TEXT NOT NULL,
    fetched_at     INTEGER NOT NULL
) STRICT;

CREATE TABLE trello_item_links (
    card_id    TEXT NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK (kind IN ('checklist', 'item', 'comment')),
    marshal_id TEXT NOT NULL,
    trello_id  TEXT NOT NULL,
    PRIMARY KEY (card_id, kind, marshal_id)
) STRICT;

CREATE UNIQUE INDEX trello_item_links_trello ON trello_item_links (card_id, kind, trello_id);

CREATE TABLE trello_dismissed (
    project_id   TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    change_id    TEXT NOT NULL,
    dismissed_at INTEGER NOT NULL,
    PRIMARY KEY (project_id, change_id)
) STRICT;

CREATE TABLE trello_sync_state (
    project_id TEXT NOT NULL PRIMARY KEY REFERENCES projects (id) ON DELETE CASCADE,
    board_name TEXT NOT NULL DEFAULT '',
    fetched_at INTEGER NOT NULL
) STRICT;
