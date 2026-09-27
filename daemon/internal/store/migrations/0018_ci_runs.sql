-- Workflow runs and the state they are in (docs/architecture.md section 10's `ci_runs` row, section
-- 9's CI failure loop, docs/backend-checklist.md B6.2, build-plan 6.2). Forward only, like every
-- migration. Times are INTEGER Unix milliseconds in UTC and ids are TEXT. STRICT makes SQLite refuse
-- a value of the wrong type.
--
-- The table holds the *state* of a workflow and not its history: one row per project, branch, and
-- workflow, replaced when the same workflow runs again on the same branch. That is what the CI
-- health page, the project board, and a card's badge show, and it is what keeps the table from
-- growing without end on a repository that runs CI all day. `branch` is what ties a row to a card:
-- a card's own branch is `cards.branch`, so a card is found from a run by matching the two, and the
-- card id is stored beside it for the reads that go the other way.
--
-- `status` is one of the wire CIState values (protocol.CIStateValues): queued, running, passed,
-- failed, cancelled. `started_at` is 0, never NULL, for a run that is still queued - the same
-- convention `integrations.last_test_at` uses - so the generated row type stays a plain integer and
-- the wire layer decides what a zero means. `updated_at` is when Marshal last heard about the run,
-- and it is what a screen counts "4 min ago" from.
--
-- `rerun_at` is when Marshal last asked the forge to run this run's failed jobs again, and 0 for one
-- it has not. It is what keeps section 9's "rerun the failed jobs once" true across a restart. A
-- re-run keeps the forge's run id and lands on the same row, so the upsert puts this column back
-- when the id is unchanged and starts it over at 0 when the row is a new run.
--
-- `fix_sent_at` is when Marshal last sent this run's failed-step log to the card's session, and 0 for
-- one it has not. It is what keeps section 9's third step from happening twice: a run fails again,
-- its log is sent once, and every later delivery of the same run records the state without sending
-- the same log a second time. It is preserved across an upsert for the same reason `rerun_at` is.
--
-- A card is removed with its runs. The project is not a foreign key: a run is Marshal's memory of
-- what a forge said, and it is written from a delivery that arrives with no project row in hand.

-- +goose Up

CREATE TABLE ci_runs (
    id          TEXT NOT NULL PRIMARY KEY,
    project_id  TEXT NOT NULL,
    card_id     TEXT REFERENCES cards (id) ON DELETE CASCADE,
    branch      TEXT NOT NULL,
    workflow    TEXT NOT NULL,
    status      TEXT NOT NULL,
    url         TEXT NOT NULL DEFAULT '',
    started_at  INTEGER NOT NULL DEFAULT 0 CHECK (started_at >= 0),
    updated_at  INTEGER NOT NULL,
    rerun_at    INTEGER NOT NULL DEFAULT 0 CHECK (rerun_at >= 0),
    fix_sent_at INTEGER NOT NULL DEFAULT 0 CHECK (fix_sent_at >= 0),
    UNIQUE (project_id, branch, workflow)
) STRICT;

-- The CI health page reads every run of every project, newest first, and a project's board reads
-- its own. The unique key above already serves a lookup by project, branch, and workflow; this
-- index serves the ordering those two reads want.
CREATE INDEX ci_runs_project ON ci_runs (project_id, updated_at DESC);

-- A card's badge, and the fix loop's "is this run on a card's branch", read a card's own runs.
CREATE INDEX ci_runs_card ON ci_runs (card_id);
