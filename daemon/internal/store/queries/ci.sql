-- Workflow runs and the state they are in (docs/architecture.md sections 9 and 10, docs/
-- backend-checklist.md B6.2, build-plan 6.2). The table and its columns are described in migration
-- 0018; what matters here is that a row is a workflow's *state* on one branch and not a history, so
-- a run that happens again replaces the row it was.
--
-- internal/ci is the only writer. It writes from a verified webhook, from the polling backup that
-- covers a missed delivery, and from a simulated failure; no route writes a run.

-- name: GetCiRun :one
-- One run by its id, which is the forge's own run id. sql.ErrNoRows means Marshal has no row for it,
-- which is the ordinary state of a run the daemon has not heard about.
SELECT * FROM ci_runs WHERE id = ?;

-- name: GetCiRunFor :one
-- The newest run of one workflow on one branch: the row a delivery lands on. The unique key of the
-- table is exactly these three columns, so this is the lookup the upsert conflicts on.
SELECT * FROM ci_runs WHERE project_id = ? AND branch = ? AND workflow = ?;

-- name: ListCiRuns :many
-- Every run Marshal holds, grouped by project and newest first inside each, which is the order the
-- CI health page draws.
SELECT * FROM ci_runs ORDER BY project_id, updated_at DESC, id;

-- name: ListCiRunsForProject :many
-- One project's runs, newest first, which is what a project's board and its CI panel read.
SELECT * FROM ci_runs WHERE project_id = ? ORDER BY updated_at DESC, id;

-- name: ListCiRunsForCard :many
-- One card's runs, newest first, which is what the card's badge and the fix loop read.
SELECT * FROM ci_runs WHERE card_id = ? ORDER BY updated_at DESC, id;

-- name: ListCiRunsWaiting :many
-- The runs Marshal is still waiting on that it has not heard about for a while: the queued or
-- running ones whose last change is older than the cutoff. This is the polling backup's question
-- (section 9): a run the forge finished but whose delivery never arrived stays exactly here. A run
-- Marshal is waiting on is a small set at any moment, so the backup's work does not grow with the
-- number of runs the daemon has ever seen.
SELECT * FROM ci_runs WHERE status IN ('queued', 'running') AND updated_at < ?
ORDER BY project_id, branch, updated_at, id;

-- name: UpsertCiRun :exec
-- Records the newest state of one workflow on one branch, making the row the first time. A run that
-- happens again keeps its own run id and lands on the same row, so `rerun_at` - Marshal's memory of
-- having already asked for the failed jobs once (section 9) - and `fix_sent_at` - its memory of
-- having already sent that run's failure to the card's session - are both kept when the id is
-- unchanged and started over at 0 when this is a new run. `card_id` is set to whatever this delivery
-- resolved it to, including to NULL for a branch that is not a card's, so a card that is deleted and
-- a branch that stops being a card's both end up with the honest answer.
INSERT INTO ci_runs (
    id, project_id, card_id, branch, workflow, status, url, started_at, updated_at, rerun_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (project_id, branch, workflow) DO UPDATE SET
    id          = excluded.id,
    card_id     = excluded.card_id,
    status      = excluded.status,
    url         = excluded.url,
    started_at  = excluded.started_at,
    updated_at  = excluded.updated_at,
    rerun_at    = CASE WHEN ci_runs.id = excluded.id THEN ci_runs.rerun_at ELSE 0 END,
    fix_sent_at = CASE WHEN ci_runs.id = excluded.id THEN ci_runs.fix_sent_at ELSE 0 END;

-- name: SetCiRunRerun :exec
-- Remembers that Marshal asked the forge to run this run's failed jobs again, so a second failure of
-- the same run sends the trimmed log to the card's session instead of rerunning it forever
-- (section 9).
UPDATE ci_runs SET rerun_at = ? WHERE id = ?;

-- name: SetCiRunFixSent :exec
-- Remembers that Marshal sent this run's failed-step log to the card's session, so every later
-- delivery of the same failed run records its state without sending the same log again (section 9).
UPDATE ci_runs SET fix_sent_at = ? WHERE id = ?;
