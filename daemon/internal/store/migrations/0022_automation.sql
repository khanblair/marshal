-- Automation and schedules: scheduled jobs, briefs, and execution logs (docs/backend-checklist.md B8.1).
-- Forward only, like every migration. STRICT mode enabled.

-- +goose Up

CREATE TABLE schedules (
    id            TEXT NOT NULL PRIMARY KEY,
    project_id    TEXT NOT NULL DEFAULT '',
    name          TEXT NOT NULL,
    kind          TEXT NOT NULL,
    icon          TEXT NOT NULL DEFAULT '',
    trigger_type  TEXT NOT NULL,
    when_text     TEXT NOT NULL DEFAULT '',
    cron_expr     TEXT NOT NULL DEFAULT '',
    time_str      TEXT NOT NULL DEFAULT '',
    days_json     TEXT NOT NULL DEFAULT '[]',
    action        TEXT NOT NULL DEFAULT '',
    enabled       INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    missed_policy TEXT NOT NULL DEFAULT 'run_now',
    created_at    INTEGER NOT NULL DEFAULT 0 CHECK (created_at >= 0),
    updated_at    INTEGER NOT NULL DEFAULT 0 CHECK (updated_at >= 0),
    last_run_at   INTEGER NOT NULL DEFAULT 0 CHECK (last_run_at >= 0)
) STRICT;

CREATE INDEX schedules_project ON schedules (project_id);

CREATE TABLE schedule_runs (
    id          TEXT NOT NULL PRIMARY KEY,
    schedule_id TEXT NOT NULL REFERENCES schedules (id) ON DELETE CASCADE,
    run_at      INTEGER NOT NULL CHECK (run_at >= 0),
    status      TEXT NOT NULL,
    details     TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE INDEX schedule_runs_schedule ON schedule_runs (schedule_id, run_at DESC);
