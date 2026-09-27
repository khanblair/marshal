-- What the model providers cost and what they are allowed to cost (docs/architecture.md section 10,
-- docs/backend-checklist.md Phase 4 items B4.4 "Usage and cost" and B4.5 "Limits"). Forward only,
-- like every migration. Times are INTEGER Unix milliseconds in UTC and ids are TEXT. STRICT makes
-- SQLite refuse a value of the wrong type.
--
-- Two tables, and each answers one question:
--
-- `usage` is what happened: one row per model call Marshal made, with the tokens the provider
-- reported and what those tokens cost in micro-dollars. Nothing updates or deletes a row, so the
-- record of spend is append-only and a number on a screen can always be traced back to the calls
-- that made it. This is the table task 4.11's "usage and cost per card, role, and model" reads, and
-- it is where the cost that lands in daily_stats.cost_micros comes from (see the store's usage
-- writer, internal/providers): the same call writes the usage row and adds its cost to the day, so
-- the chart and the detail can never disagree.
--
-- `limits` is what is allowed: a daily or monthly cost ceiling or an awake ceiling, for everything
-- at once or for one project. A limit is a ceiling, not a running total - the running total is
-- daily_stats.cost_micros and the usage rows - so this table is tiny, is replaced in place, and a
-- limit the person removes leaves no row at all.
--
-- `card_id` is empty for a call that belongs to no card (a chat, or a connection test), so it carries
-- no foreign key: '' is not a card. `project_id` is likewise empty when there is no project, and is
-- deliberately not a foreign key for the same reason activity.project_id is not one (see
-- 0008_home_stats.sql): the usage record of spend outlives a card or a project being removed, and a
-- removed project's rows are removed by whoever owns this table rather than by a cascade.

-- +goose Up

-- One model call: which provider and model answered, what it read and wrote, what it cost, and when
-- it happened. `id` is an opaque id (a ULID, architecture.md 11.5) whose first ten characters are
-- the time, so rows sort by when they were made without a separate ordering column.
--
-- `role_id` names the role the call ran as (a card's role, or empty for a chat or a bare call), which
-- is the third way the usage screens group: per card, per role, and per model. It is TEXT and not a
-- foreign key because a role can be renamed or removed while the spend it caused stays true.
--
-- `project_id` is what the cost-per-project chart groups by, and it is written for a call that has
-- no card (a chat in a project) just as it is for a card's call. It is empty, never NULL, so the
-- generated parameter and row types stay plain strings. It is here rather than joined from cards at
-- read time for the same reason activity.subject_key is stored: a card can be gone while the spend
-- it caused still shows on Home, and a removed card cannot be joined back.
--
-- Tokens and cost are never negative: an adapter that cannot read a provider's usage leaves them
-- zero rather than guessing, and every CHECK makes a wrong sign impossible rather than merely
-- unlikely.
CREATE TABLE usage (
    id            TEXT NOT NULL PRIMARY KEY,
    card_id       TEXT NOT NULL DEFAULT '',
    project_id    TEXT NOT NULL DEFAULT '',
    role_id       TEXT NOT NULL DEFAULT '',
    provider      TEXT NOT NULL,
    model         TEXT NOT NULL,
    input_tokens  INTEGER NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens INTEGER NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    cost_micros   INTEGER NOT NULL DEFAULT 0 CHECK (cost_micros >= 0),
    created_at    INTEGER NOT NULL
) STRICT;

-- The usage screens read newest first, and "this card's spend" is one card's rows newest first:
-- both are the same index with a different first column, so one leads with the card, one with the
-- project, and one with the time alone. A day's or a month's spend is a range of `created_at`, which
-- the third index serves.
CREATE INDEX usage_card_created ON usage (card_id, created_at DESC, id DESC);
CREATE INDEX usage_project_created ON usage (project_id, created_at DESC, id DESC);
CREATE INDEX usage_created ON usage (created_at);

-- A ceiling: the most a scope may spend in a day or a month, or the most cards it may keep awake at
-- once. `scope` is the literal 'global' for the whole install or a project id for one project, and
-- `kind` is 'cost-day', 'cost-month', or 'awake'; the words all three can hold are checked in Go
-- against the constants in the protocol package, not here, for the same reason cards.state is not
-- checked here (see 0002_projects.sql).
--
-- `value` is one INTEGER because every ceiling measures in an integer unit - a cost in micro-dollars,
-- the same unit as usage.cost_micros and daily_stats.cost_micros, and the awake limit in cards, the
-- same count the awake list is - so one column holds any of them and a reader knows which unit it is
-- from `kind`. The pair (scope, kind) is the primary key and therefore the index every read uses: a
-- limit is looked up by exactly those two things, and one scope holds its three ceilings side by
-- side.
CREATE TABLE limits (
    scope TEXT NOT NULL,
    kind  TEXT NOT NULL,
    value INTEGER NOT NULL CHECK (value >= 0),
    PRIMARY KEY (scope, kind)
) STRICT;
