-- Advanced cards: templates, sub-cards, checklists, checklist items, card members, comments,
-- attachments, and acceptance checks (docs/backend-checklist.md B10.1, B10.2, B10.5, B10.6;
-- docs/architecture.md section 10's table list and section 19's rules; build-plan tasks 10.1 to
-- 10.13).
--
-- Dependencies are already here (`card_links`, migration 0020) and so are file claims (0009), so
-- this migration adds only what Phase 10 leaves open: the two columns `cards` needs for templates
-- and sub-cards, and the six tables above them. Everything below is columned in
-- `docs/architecture.md` section 10 already, so nothing here is invented - with two exceptions this
-- phase's brief calls out: `checklist_items.evidence_json` and `card_checks.run_ref`, which are
-- what makes "a failing test unticks the item it proved" know which run the evidence came from.
--
-- Design rules, held by every table here:
--   - TEXT ids, INTEGER millisecond times, STRICT tables, and one migration that only adds - the
--     same as every other migration in this folder.
--   - A card that goes away takes its checklists, members, comments, checks, and template usage
--     with it (ON DELETE CASCADE), so nothing is ever left pointing at a card that is not there.
--   - A comment that goes away takes its attachments with it, for the same reason.
--   - "kind" columns are the two words `person` and `agent`, kept as TEXT rather than a lookup so a
--     later kind never needs a schema change; the service checks the value it writes.
--   - Positions are INTEGER and compared with "order by position, id" so two things at the same
--     position still have a stable order.
--
-- Forward only, like every migration.

-- +goose Up

-- A card's template: the reusable shape a new card starts from (B10.1, build-plan 10.1). It
-- replaces the fixed five-item picklist the settings screen has today, so it is a table rather than
-- an enum, and a person can make their own.
--
-- `spec_json` is the whole template as one document - the title pattern, body, role, agent kind,
-- permission mode, checklist seeds - because a template is edited as one thing on one screen and is
-- read as one thing when a card is made from it. Nothing reads half a template, so nothing here is
-- split into columns that would have to be migrated whenever the template gains a field.
CREATE TABLE templates (
    id         TEXT NOT NULL PRIMARY KEY,
    name       TEXT NOT NULL,
    spec_json  TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

-- A second template with the same name is refused by the service, not by the schema: two templates
-- named "Bug fix" would be a person's mistake to make, and a rename past one is the kind of thing
-- the screen wants a plain sentence for rather than a constraint violation.

-- Named checklists on a card (B10.5, section 19). `required` is what gates the merge queue: a card
-- cannot go to Ready to merge while a required checklist still has an open item. `people_only`
-- refuses an agent's tick, so the agent cannot tick its own way past a check a person set.
-- `hide_checked` is a reading preference kept with the checklist rather than in a client, because
-- the same person reads it on a phone and a laptop and expects the same board on both.
CREATE TABLE checklists (
    id           TEXT NOT NULL PRIMARY KEY,
    card_id      TEXT NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    position     INTEGER NOT NULL,
    required     INTEGER NOT NULL DEFAULT 0 CHECK (required IN (0, 1)),
    people_only  INTEGER NOT NULL DEFAULT 0 CHECK (people_only IN (0, 1)),
    hide_checked INTEGER NOT NULL DEFAULT 0 CHECK (hide_checked IN (0, 1)),
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
) STRICT;

-- A card's own panel reads its checklists in the order they are drawn, and a whole card's worth of
-- them is the only read that exists.
CREATE INDEX checklists_card ON checklists (card_id, position);

-- The items inside one checklist (B10.5, section 19).
--
-- `evidence_json` is what a tick was proven by: a check result, a commit, or a run id, as one small
-- document. The harness verifies the evidence exists before the tick is saved, and the item is
-- unticked when that evidence stops being true - which is why the evidence is stored rather than
-- recomputed: an item's tick has to outlive the run that proved it, and then be judged against it.
--
-- `done_by_kind` and `done_by_id` answer "who ticked this" - a person or the card's agent - which
-- the screen shows as the row's own line and which nobody else may forge.
CREATE TABLE checklist_items (
    id            TEXT NOT NULL PRIMARY KEY,
    checklist_id  TEXT NOT NULL REFERENCES checklists (id) ON DELETE CASCADE,
    text          TEXT NOT NULL,
    position      INTEGER NOT NULL,
    done          INTEGER NOT NULL DEFAULT 0 CHECK (done IN (0, 1)),
    done_by_kind  TEXT NOT NULL DEFAULT '' CHECK (done_by_kind IN ('', 'person', 'agent')),
    done_by_id    TEXT NOT NULL DEFAULT '',
    evidence_json TEXT NOT NULL DEFAULT '',
    done_at       INTEGER,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
) STRICT;

CREATE INDEX checklist_items_list ON checklist_items (checklist_id, position);

-- People on a card (B10.6, section 19). The card's agent is not here: it comes from the card's own
-- session settings, so the same query does not have to tell a person apart from a running program.
CREATE TABLE card_members (
    card_id  TEXT NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    user_id  TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    added_at INTEGER NOT NULL,
    PRIMARY KEY (card_id, user_id)
) STRICT;

-- The other direction is the one a person's own "my cards" list makes.
CREATE INDEX card_members_user ON card_members (user_id);

-- Card comments (B10.6, section 19).
--
-- `mentions_json` is who the comment named - `@agent` and a person's own handle - as an array of
-- ids, stored with the comment so a mention survives the person being renamed. It is what the reply
-- path reads: a comment that mentions the agent, or that ends in a question mark, gets a reply from
-- the card's session, waking it if it is asleep.
--
-- `agent_read_at` is when the card's agent picked the comment up. The UI shows it as "Agent read
-- this", and it is the one part of a comment the daemon owns rather than a person.
CREATE TABLE comments (
    id            TEXT NOT NULL PRIMARY KEY,
    card_id       TEXT NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    author_kind   TEXT NOT NULL CHECK (author_kind IN ('person', 'agent')),
    author_id     TEXT NOT NULL,
    body          TEXT NOT NULL,
    mentions_json TEXT NOT NULL DEFAULT '[]',
    agent_read_at INTEGER,
    created_at    INTEGER NOT NULL,
    edited_at     INTEGER
) STRICT;

CREATE INDEX comments_card ON comments (card_id, created_at);

-- Files and images on a comment (B10.6, section 19).
--
-- `path` is where the file lives on disk, under `<data>/attachments/<project>/<card>/`, and is
-- relative: a backup moved to another machine must not read an absolute path from the old one. The
-- file is data the agent is shown and never run, whatever its mime type says.
CREATE TABLE attachments (
    id          TEXT NOT NULL PRIMARY KEY,
    comment_id  TEXT NOT NULL REFERENCES comments (id) ON DELETE CASCADE,
    file_name   TEXT NOT NULL,
    mime_type   TEXT NOT NULL,
    size_bytes  INTEGER NOT NULL,
    path        TEXT NOT NULL,
    created_at  INTEGER NOT NULL
) STRICT;

CREATE INDEX attachments_comment ON attachments (comment_id);

-- Acceptance checks (B10.2, section 19, N9).
--
-- `status` is `pending`, `passed`, or `failed`. A card cannot finish until every check has passed,
-- and a check's own result is the tick evidence a checklist item can be ticked with.
--
-- `run_ref` is what the current answer came from: the run id, or the commit it judged. It is the
-- column this phase's brief asks for explicitly, because "a failing test unticks the item it
-- proved" has to know which run proved it - a `status` alone cannot tell a person whether what they
-- are looking at is still the answer.
CREATE TABLE card_checks (
    id        TEXT NOT NULL PRIMARY KEY,
    card_id   TEXT NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    kind      TEXT NOT NULL,
    spec_json TEXT NOT NULL,
    status    TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'passed', 'failed')),
    run_ref   TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

CREATE INDEX card_checks_card ON card_checks (card_id, status);

-- A new card's template and its parent, added to `cards` itself rather than to a table of their
-- own: both are facts about one card, never read without it, and both are nullable because most
-- cards have neither.
--
-- `parent_id` is what makes a sub-card one: a parent card shows its sub-cards' combined progress,
-- and the sub-card knows whose it is. The FK cascades so deleting a parent takes the sub-cards with
-- it, the same as everything else a card owns.
ALTER TABLE cards ADD COLUMN template_id TEXT NOT NULL DEFAULT '';
ALTER TABLE cards ADD COLUMN parent_id TEXT REFERENCES cards (id) ON DELETE CASCADE;

-- The parent's own panel lists its sub-cards, which is the only read these two columns exist for.
CREATE INDEX cards_parent ON cards (parent_id) WHERE parent_id IS NOT NULL;
