-- Memory: the card notes and their search index, file claims, and the two registries Phase 7 needs
-- (docs/architecture.md section 10's `notes`, `file_claims`, `mcp_servers`, and `skills` rows,
-- section 12's vault layout; docs/backend-checklist.md B7.2, B7.4, and B7.5; build-plan tasks 7.3,
-- 7.6, 7.7, 7.9, and 7.12). Forward only, like every migration. Times are INTEGER Unix milliseconds
-- in UTC and ids are TEXT. STRICT makes SQLite refuse a value of the wrong type.
--
-- Memory really lives on disk. Section 12 puts it in the vault: `<vault>/<project>/memory` for the
-- rules, architecture notes, and decisions, `lessons` for the lessons an agent writes after a fix,
-- `cards/<card>.md` for one note per card, and `briefs/` for the daily briefs. A person also edits
-- those files by hand in Obsidian, which is the point of the layout. These tables are the *index* of
-- the parts the daemon must find without walking the vault - the card notes, and the files a card
-- has claimed - so a read is a query and not a directory scan, and so search has something to match
-- against. The vault is written first and the row second, in one call; the vault watcher (task 7.8)
-- is what brings an edit made in Obsidian back into the row.
--
-- `notes` holds two kinds of vault markdown, one row per file, told apart by `kind`: ONE NOTE PER
-- CARD (`kind = 'card_note'`), which is what the Notes tab shows (`apps/web/src/views/card/
-- card-note.ts` reads one string and `saveNote` overwrites the whole thing, B7.4, N11, task 7.12),
-- and a project's LESSONS (`kind = 'lesson'`, task 7.6) - what an agent learned worth remembering
-- next time. The owner decided lessons are file-only and indexed the same way notes already are,
-- rather than a second, parallel table: one file-backed store, not two (progress-tracker.md decisions
-- log, 2026-09-28). `slug` and `title` are a lesson's identity - its file name and its heading - and
-- are empty for a card note, which is identified by `card_id` and titled by its card instead.
--
-- A card note's unique key is (project_id, card_id) and a lesson's is (project_id, slug); neither
-- constraint can be written as a single UNIQUE, so both are partial indexes below, scoped by `kind`.
-- That is also why `card_id` cannot stay a foreign key: a lesson has no card, and this schema never
-- spells "no card" with SQL NULL (no column anywhere in this database is nullable; a missing value is
-- always a typed default, such as `mcp_servers.last_checked_at`'s 0 below) - so a lesson's `card_id`
-- is `''`, which a FOREIGN KEY would reject outright. A card's note taking the card's own deletion
-- with it - `TestDeletingACardTakesItsMemory` in internal/store's migration test - still has to hold,
-- so the trigger below does by hand what the foreign key's ON DELETE CASCADE used to do for free:
-- it is scoped to `kind = 'card_note'` so a lesson, which never has a matching card_id, is never
-- touched by it, and its own delete of a `notes` row fires `notes_fts_delete` beneath it, the same
-- as any other delete of one. What the foreign key is not replaced for is rejecting a note written
-- against a card_id no card has; nothing in this codebase does that today, and a lesson's own rows
-- being real rows with no card is the same shape, so there was nothing to keep. `kind` has no CHECK
-- constraint the way `author` does not either, for the reason 0016 gives about smell families
-- (SQLite cannot change a CHECK without rebuilding the table) - but unlike `author`, which a person's
-- own save request carries as a word Go must validate, `kind` is never taken from outside the daemon:
-- every writer names its own literal ('card_note' by the column's default, 'lesson' in UpsertLesson's
-- INSERT), so there is no input to check. The same is true of `author`, in the two words
-- the rest of the schema uses for the same question (`audit_log.actor` is "person", "agent", or
-- "daemon", migration 0013): "person" for the owner, "agent" for the card's agent - true of a
-- lesson's author as well as a note's.
--
-- The mock's note path drops the project segment (`vault/cards/<card>.md`); the vault the daemon
-- writes follows section 12 (`<vault>/<project>/cards/<card>.md`). The mock is the side that is
-- slightly wrong here, and this migration follows the doc.
--
-- `notes_fts` is the full-text index section 10 calls for ("Session search uses SQLite full-text
-- search over `session_events.summary` and card notes"), and now over lessons too, the same way.
-- It is an ordinary FTS5 table holding its own copy of the title and the body rather than an
-- external-content one, because a note's id is TEXT and an external-content FTS5 table is addressed
-- by the content table's rowid; keeping the id in an UNINDEXED column is what lets a row be replaced
-- and removed by id. `kind` is UNINDEXED too: it is never matched against a person's words, only
-- filtered on, so a search can ask for card notes or lessons without a second table. The tokenizer is
-- FTS5's default (unicode61), so matching is word-based and a prefix match is written `word*` by the
-- caller. The three triggers keep it in step with `notes` whichever module writes a row, and the
-- fourth removes a deleted card's rows explicitly, so the index is correct no matter how a card goes
-- away. `notes_fts` cannot be STRICT: it is a virtual table.
--
-- `file_claims` is what a card is working on right now (section 11.4's `claim_files` and
-- `release_files`, B7.2, task 7.3). The primary key is (card_id, path_or_package), so the same card
-- claiming the same path twice is one claim and not two, and releasing one is a delete. The column
-- is named `path_or_package` because a claim may name either a file or a whole package (section
-- 11.4), and the conflict rule that reads it is written in the memory module. `project_id` is stored
-- beside the card even though section 10 lists only the card: the board-awareness summary and the
-- overlap warning both read a project's claims, and a copy of the project is what keeps that read
-- from joining `cards`, which the projects module owns.
--
-- `mcp_servers` and `skills` are the two registries the phase's own tools need. A role already
-- names the servers it wants (`apps/web/src/mock/seed/settings.ts` seeds `mcp: ["marshal",
-- "github"]`), so a server is looked up by name and the name is unique. `health` is one of the wire
-- MCP health values (checked in Go, like every other word this schema stores that is meant to grow)
-- and `last_checked_at` is 0, never NULL, for a server that has never been checked - the same
-- convention `integrations.last_test_at` uses. `skills.path` is the skill's own folder under
-- `<data>/skills` (section 12) and `source` says where it came from.

-- +goose Up

CREATE TABLE notes (
    id         TEXT NOT NULL PRIMARY KEY,
    project_id TEXT NOT NULL,
    -- '' for a lesson, which has no card. See the header comment for why this is not a foreign key
    -- and not NULL.
    card_id    TEXT NOT NULL DEFAULT '',
    -- 'card_note' or 'lesson'. Never taken from outside the daemon (see the header comment), so
    -- there is nothing here for Go to validate the way author.Valid() validates a note's author.
    kind       TEXT NOT NULL DEFAULT 'card_note',
    -- A lesson's file-name-safe identifier, '' for a card note (identified by card_id instead).
    slug       TEXT NOT NULL DEFAULT '',
    -- A lesson's heading, '' for a card note (titled by its card instead).
    title      TEXT NOT NULL DEFAULT '',
    author     TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

-- "One note per card": true only among card notes, where card_id is always set.
CREATE UNIQUE INDEX notes_card_note_unique ON notes (project_id, card_id) WHERE kind = 'card_note';
-- "One lesson per slug per project": true only among lessons, where card_id is never set.
CREATE UNIQUE INDEX notes_lesson_slug_unique ON notes (project_id, slug) WHERE kind = 'lesson';
-- The Notes tab knows the card it is drawing and not the project, so the read by card alone needs
-- its own index; the two partial unique indexes above serve the reads that start from a project.
CREATE INDEX notes_card ON notes (card_id);

CREATE VIRTUAL TABLE notes_fts USING fts5(
    id UNINDEXED,
    project_id UNINDEXED,
    card_id UNINDEXED,
    kind UNINDEXED,
    title,
    body
);

-- +goose StatementBegin
CREATE TRIGGER notes_fts_insert AFTER INSERT ON notes
BEGIN
    INSERT INTO notes_fts (id, project_id, card_id, kind, title, body)
    VALUES (NEW.id, NEW.project_id, NEW.card_id, NEW.kind, NEW.title, NEW.body);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER notes_fts_update AFTER UPDATE ON notes
BEGIN
    DELETE FROM notes_fts WHERE id = OLD.id;
    INSERT INTO notes_fts (id, project_id, card_id, kind, title, body)
    VALUES (NEW.id, NEW.project_id, NEW.card_id, NEW.kind, NEW.title, NEW.body);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER notes_fts_delete AFTER DELETE ON notes
BEGIN
    DELETE FROM notes_fts WHERE id = OLD.id;
END;
-- +goose StatementEnd

-- cards_notes_delete is what does by hand what ON DELETE CASCADE used to do for `notes.card_id`
-- before it lost its foreign key (see the header comment): a card's own deletion still has to take
-- its note with it, which `TestDeletingACardTakesItsMemory` (internal/store) proves by deleting a
-- card and checking `notes` for it. Scoped to `kind = 'card_note'` even though a lesson's card_id is
-- always '' and could never match `OLD.id` anyway - naming the scope is cheaper than trusting that.
-- Deleting the row here fires notes_fts_delete beneath it the same as any other delete of one, so
-- the FTS half needs no trigger of its own the way it would have if this deleted straight from
-- notes_fts (nested trigger firing needs no `recursive_triggers`; that pragma only guards a trigger
-- whose own action would fire itself again, not a trigger on one table firing another's).
-- +goose StatementBegin
CREATE TRIGGER cards_notes_delete AFTER DELETE ON cards
BEGIN
    DELETE FROM notes WHERE card_id = OLD.id AND kind = 'card_note';
END;
-- +goose StatementEnd

CREATE TABLE file_claims (
    card_id         TEXT NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    project_id      TEXT NOT NULL,
    path_or_package TEXT NOT NULL,
    claimed_at      INTEGER NOT NULL,
    PRIMARY KEY (card_id, path_or_package)
) STRICT;

-- The overlap warning reads a project's claims, and the awareness summary lists them, so this
-- index serves both: `project_id` is its prefix for the listing, and the path makes the "is anyone
-- else on this file" lookup a seek rather than a scan.
CREATE INDEX file_claims_project_path ON file_claims (project_id, path_or_package);

CREATE TABLE mcp_servers (
    id              TEXT NOT NULL PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    transport_json  TEXT NOT NULL DEFAULT '',
    health          TEXT NOT NULL DEFAULT 'unknown',
    last_checked_at INTEGER NOT NULL DEFAULT 0 CHECK (last_checked_at >= 0)
) STRICT;

CREATE TABLE skills (
    id     TEXT NOT NULL PRIMARY KEY,
    name   TEXT NOT NULL,
    path   TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT ''
) STRICT;
