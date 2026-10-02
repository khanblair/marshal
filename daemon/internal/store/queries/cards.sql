-- The merge queue's reads and writes of the cards table (Integrator flow). The projects module owns
-- a card's state and fields; the Integrator writes only the three columns that show a merge in
-- progress, and reads the cards that are waiting for, in, or stopped by a merge.

-- name: SetCardMergeProgress :execrows
-- Sets where a card's merge is: the phase, the one sentence about it, and the card's "doing now"
-- line. An empty phase means no merge is in progress. Every exit of a merge calls it, so a phase
-- never outlives the merge. It leaves updated_at alone: the queue is ordered by when a card became
-- ready, and the merge.progress event, not a new timestamp, tells clients the phase moved.
UPDATE cards SET merge_phase = ?, merge_note = ?, doing_now = ? WHERE id = ?;

-- name: ListMergeQueue :many
-- The cards of a project that wait for a merge or are being merged: the one being merged first, the
-- rest in the order they became ready.
SELECT id, project_id, number, title, state, branch, merge_phase, merge_note, updated_at
FROM cards
WHERE project_id = ? AND state IN ('ready', 'merging')
ORDER BY CASE state WHEN 'merging' THEN 0 ELSE 1 END, updated_at, number;

-- name: ListMergeStalled :many
-- The cards of a project that a merge stopped and a person has to look at, newest stop first. A
-- stopped merge leaves the phase "stopped" and one of its two reasons on the card, so a card that
-- was stopped once and now waits for another reason is not listed.
SELECT id, project_id, number, title, state, branch, merge_phase, merge_note, updated_at,
       needs_reason_kind, needs_reason_text
FROM cards
WHERE project_id = ? AND state = 'needs' AND merge_phase = 'stopped'
  AND needs_reason_kind IN ('conflict', 'ci-failed')
ORDER BY updated_at DESC, number DESC;

-- name: ListMergeBranches :many
-- The cards of a project that have a branch and are not done, with their state. It is how the
-- Integrator finds which cards the work waiting on its branch belongs to, and which of them have
-- been pulled back since.
SELECT id, state, branch, merge_phase
FROM cards
WHERE project_id = ? AND branch <> '' AND state <> 'done'
ORDER BY number;

-- name: ListMergingCards :many
-- Every card, in any project, that is marked as being merged. After a restart none is, so these are
-- the cards a crash left behind.
SELECT id, project_id FROM cards WHERE state = 'merging' ORDER BY project_id, number;
