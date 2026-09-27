-- The code-smell checks (docs/architecture.md section 17, docs/backend-checklist.md B5.8, build-plan
-- 5.20): one project's profile of which checks run at what thresholds, and the findings a check made
-- in one card's diff. The two tables and the columns they hold are described in migration 0016.
--
-- A finding is written once, when a check runs for a commit, and a check of the same commit does not
-- run again: the row in smell_checks is what remembers that a commit was checked, even when the
-- check found nothing. After that a finding changes in only two ways - it is dismissed with a reason,
-- or it is marked fixed - so nothing here updates a finding's file, line, or words: those are facts
-- about the commit it was found in.
--
-- The id is an opaque id, and the profile is one JSON document per project; a project with no row
-- uses the built-in defaults, which is why GetSmellProfile answers nothing rather than an error for a
-- fresh install.

-- name: InsertSmellFinding :exec
-- Records one smell a check found. `commit_sha` is the commit it was found in and is empty for a
-- finding from uncommitted work; `family` is the kind of smell, `smell` is the rule's own name, and
-- `status` starts open. A finding is inserted with its words and its place, which never change.
INSERT INTO smell_findings (
    id, card_id, commit_sha, family, smell, file, line, severity, message, suggestion, status, dismiss_reason
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListSmellFindingsForCardCommit :many
-- One card's findings as of one commit: the rows a card's checks panel shows, and the rows the move
-- to review counts blocking findings from. Findings from an older commit are left where they are,
-- because a check of a newer commit must never re-blame a smell on work the card has since changed.
-- Ordered by place, so a screen reads them top to bottom of the diff.
SELECT * FROM smell_findings
WHERE card_id = ? AND commit_sha = ?
ORDER BY file, line, id;

-- name: GetSmellFinding :one
-- One finding of one card, for the call that acts on it. The card is part of the lookup so a finding
-- of another card can never be fixed or dismissed through this card's address.
SELECT * FROM smell_findings
WHERE card_id = ? AND id = ?;

-- name: SetSmellFindingStatus :exec
-- Marks one finding fixed or dismissed, and keeps why in `dismiss_reason` for a dismissal (empty for
-- a fix). Nothing else about a finding changes: its place and its words are facts about its commit.
UPDATE smell_findings
SET status = ?, dismiss_reason = ?
WHERE card_id = ? AND id = ?;

-- name: GetSmellProfile :one
-- One project's smell profile. A project that has none reads nothing, and the caller uses the
-- built-in defaults.
SELECT * FROM smell_profiles WHERE project_id = ?;

-- name: UpsertSmellProfile :exec
-- Saves one project's smell profile, replacing whatever was there. One profile per project, so this
-- is the only write the profile table has.
INSERT INTO smell_profiles (project_id, spec_json)
VALUES (?, ?)
ON CONFLICT (project_id) DO UPDATE SET spec_json = excluded.spec_json;

-- name: RecordSmellCheck :exec
-- Remembers that the checks have run for one card and commit, so the same commit is never checked
-- twice even when the checks found nothing. A second run of the same commit replaces the time
-- rather than writing a second row, which is what the primary key is for.
INSERT INTO smell_checks (card_id, commit_sha, checked_at)
VALUES (?, ?, ?)
ON CONFLICT (card_id, commit_sha) DO UPDATE SET checked_at = excluded.checked_at;

-- name: GetSmellCheck :one
-- When the checks last ran for one card and commit, for the per-commit cache. A commit with no row
-- has never been checked and is checked now.
SELECT checked_at FROM smell_checks WHERE card_id = ? AND commit_sha = ?;
