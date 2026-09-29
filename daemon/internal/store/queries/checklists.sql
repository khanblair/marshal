-- Named checklists and their items, plus the people on a card (migration 0024,
-- docs/architecture.md sections 10 and 19; docs/backend-checklist.md B10.5 and B10.6, build-plan
-- tasks 10.10 to 10.13).
--
-- The rules live in the service, not here: which checklists gate a merge, that a people-only one
-- refuses an agent's tick, and that evidence is checked before a tick is saved. What the queries
-- own is the order things are drawn in and the cascade that takes a card's checklists with it.

-- name: CreateChecklist :exec
-- Makes a checklist on a card. `position` is where it is drawn among the card's others; a new one
-- goes last, which the service works out by counting what is already there.
INSERT INTO checklists (id, card_id, name, position, required, people_only, hide_checked, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetChecklist :one
SELECT * FROM checklists WHERE id = ?;

-- name: UpdateChecklist :exec
-- Renames a checklist and sets its three flags. They are sent together because the screen edits
-- them in one place, and because "required" and "people only" are the two that change what an agent
-- is allowed to do - they belong in one place a person can read back.
UPDATE checklists
SET name = ?, required = ?, people_only = ?, hide_checked = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteChecklist :exec
-- Removes a checklist and, by the cascade, its items. Deleting a checklist is not a step toward
-- finishing a card; it is a person deciding the checks were wrong.
DELETE FROM checklists WHERE id = ?;

-- name: ListCardChecklists :many
-- One card's checklists, in the order they are drawn.
SELECT * FROM checklists WHERE card_id = ? ORDER BY position, id;

-- name: CountCardChecklists :one
-- How many checklists a card has. A new one is put after them.
SELECT COUNT(*) FROM checklists WHERE card_id = ?;

-- name: CreateChecklistItem :exec
INSERT INTO checklist_items (id, checklist_id, text, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetChecklistItem :one
SELECT * FROM checklist_items WHERE id = ?;

-- name: UpdateChecklistItemText :exec
-- Edits an item's words. The tick is untouched: rewording a check that has been ticked does not
-- untick it, because what was proven has not changed.
UPDATE checklist_items
SET text = ?, updated_at = ?
WHERE id = ?;

-- name: UpdateChecklistItemPosition :exec
-- Moves one item. Reordering is its own call because dragging writes many positions and one item's
-- text is nobody's business while that is happening.
UPDATE checklist_items
SET position = ?, updated_at = ?
WHERE id = ?;

-- name: TickChecklistItem :exec
-- Ticks an item and records who ticked it and what proved it, or unticks it by passing a null
-- `done_at` and an empty evidence. This is the one place a tick is written, so "who ticked this"
-- and "with what evidence" can never disagree with `done`.
UPDATE checklist_items
SET done = ?, done_by_kind = ?, done_by_id = ?, evidence_json = ?, done_at = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteChecklistItem :exec
DELETE FROM checklist_items WHERE id = ?;

-- name: ListChecklistItems :many
-- One checklist's items, in the order they are drawn.
SELECT * FROM checklist_items WHERE checklist_id = ? ORDER BY position, id;

-- name: ListCardChecklistItems :many
-- Every item on a card, with its checklist's id and flags beside it. It is one read for the whole
-- card rather than one per checklist, because a card's panel draws all of them at once and because
-- the merge gate asks the same question over all of them.
SELECT item.*, checklist.card_id AS card_id, checklist.required AS required,
       checklist.people_only AS people_only
FROM checklist_items AS item
JOIN checklists AS checklist ON checklist.id = item.checklist_id
WHERE checklist.card_id = ?
ORDER BY checklist.position, checklist.id, item.position, item.id;

-- name: CountOpenRequiredItems :one
-- How many open items sit on a card's required checklists. This is the number the merge queue is
-- blocked by (docs/architecture.md section 19): a card cannot move to Ready to merge while it is
-- more than zero, and when everything else is ready the card is sent to Needs you saying so.
SELECT COUNT(*)
FROM checklist_items AS item
JOIN checklists AS checklist ON checklist.id = item.checklist_id
WHERE checklist.card_id = ? AND checklist.required = 1 AND item.done = 0;

-- name: AddCardMember :exec
-- Puts a person on a card. Adding one who is already on it is not an error: the card's membership is
-- a set, not an append, and a second press from a second screen means the same thing.
INSERT INTO card_members (card_id, user_id, added_at) VALUES (?, ?, ?)
ON CONFLICT (card_id, user_id) DO NOTHING;

-- name: RemoveCardMember :exec
-- Takes a person off a card. Removing one who is not on it is not an answer worth failing over.
DELETE FROM card_members WHERE card_id = ? AND user_id = ?;

-- name: ListCardMembers :many
-- Who is on a card, oldest first, for the card's own row of people chips.
SELECT * FROM card_members WHERE card_id = ? ORDER BY added_at, user_id;

-- name: ListUserCardMemberships :many
-- Which cards a person is on. It is what "my cards" reads, and what a member notice is sent to.
SELECT * FROM card_members WHERE user_id = ? ORDER BY added_at, card_id;
