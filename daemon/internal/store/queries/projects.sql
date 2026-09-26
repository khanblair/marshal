-- name: CreateProject :exec
INSERT INTO projects (
    id, name, repo_path, default_branch, language, dev_command,
    bypass_locked, is_monorepo, packages_json, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetProject :one
SELECT * FROM projects WHERE id = ?;

-- name: GetProjectByRepoPath :one
SELECT * FROM projects WHERE repo_path = ?;

-- name: ProjectIDExists :one
SELECT EXISTS (SELECT 1 FROM projects WHERE id = ?);

-- name: ListProjects :many
SELECT * FROM projects ORDER BY created_at, id;

-- name: UpdateProject :execrows
UPDATE projects
SET name = ?, dev_command = ?, default_branch = ?, bypass_locked = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteProject :execrows
DELETE FROM projects WHERE id = ?;

-- name: CreateBoard :exec
INSERT INTO boards (id, project_id, columns_json) VALUES (?, ?, ?);

-- name: GetBoardByProject :one
SELECT * FROM boards WHERE project_id = ?;

-- name: ReserveCardNumber :one
-- Takes the next card number of a project and moves the counter on, in one statement. Run it in
-- the same transaction as the card insert, so a failed insert gives the number back.
UPDATE projects
SET next_card_number = next_card_number + 1
WHERE id = ?
RETURNING CAST(next_card_number - 1 AS INTEGER) AS number;

-- name: CreateCard :exec
INSERT INTO cards (
    id, project_id, number, board_id, title, body, state, agent_kind, model, thinking,
    permission_mode, role, package, created_by, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetCard :one
SELECT * FROM cards WHERE id = ?;

-- name: GetCardByKey :one
SELECT * FROM cards WHERE project_id = ? AND number = ?;

-- name: ListCardsByProject :many
SELECT * FROM cards WHERE project_id = ? ORDER BY number;

-- name: UpdateCardState :execrows
UPDATE cards SET state = ?, updated_at = ? WHERE id = ?;

-- name: UpdateCardWorktree :execrows
UPDATE cards SET worktree_path = ?, branch = ?, updated_at = ? WHERE id = ?;

-- name: ListCardBranches :many
SELECT branch FROM cards WHERE project_id = ? AND branch <> '' ORDER BY number;

-- name: CountNeedsByProject :many
SELECT project_id, CAST(COUNT(*) AS INTEGER) AS needs
FROM cards
WHERE state = 'needs'
GROUP BY project_id;

-- name: CountNeedsForProject :one
SELECT CAST(COUNT(*) AS INTEGER) AS needs FROM cards WHERE project_id = ? AND state = 'needs';

-- name: UpdateCardFields :execrows
-- Writes the fields a person can change on a card. Every value is passed as it should end up, so
-- a caller that leaves one alone passes what it read.
UPDATE cards
SET number = ?, state = ?, title = ?, body = ?, agent_kind = ?, model = ?, thinking = ?, permission_mode = ?,
    role = ?, package = ?, planned_start = ?, planned_end = ?, due = ?, actual_start = ?,
    actual_end = ?, pull_request_number = ?, pull_request_url = ?, ci_state = ?,
    context_used = ?, needs_reason_kind = ?, needs_reason_text = ?, needs_since = ?,
    doing_now = ?, paused = ?, pinned = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteCard :execrows
DELETE FROM cards WHERE id = ?;

-- name: ListLabelsByProject :many
SELECT * FROM labels WHERE project_id = ? ORDER BY name;

-- name: GetLabel :one
SELECT * FROM labels WHERE id = ?;

-- name: CreateLabel :exec
INSERT INTO labels (id, project_id, name, color, created_at) VALUES (?, ?, ?, ?, ?);

-- name: UpdateLabel :execrows
UPDATE labels SET name = ?, color = ? WHERE id = ?;

-- name: DeleteLabel :execrows
DELETE FROM labels WHERE id = ?;

-- name: ListLabelsForCard :many
SELECT labels.* FROM labels
JOIN card_labels ON card_labels.label_id = labels.id
WHERE card_labels.card_id = ?
ORDER BY labels.name;

-- name: ClearCardLabels :exec
DELETE FROM card_labels WHERE card_id = ?;

-- name: AddCardLabel :exec
INSERT INTO card_labels (card_id, label_id) VALUES (?, ?);

-- name: ListCardsNeedingYou :many
-- Home's needs-you list. Ordered by how long the card has waited, longest first.
SELECT cards.*, projects.name AS project_name
FROM cards
JOIN projects ON projects.id = cards.project_id
WHERE cards.state = 'needs'
ORDER BY COALESCE(cards.needs_since, cards.updated_at), cards.id;

-- name: CountCardsByState :many
SELECT project_id, state, CAST(COUNT(*) AS INTEGER) AS cards
FROM cards
GROUP BY project_id, state;

-- name: CountCardsFinishedSince :one
SELECT CAST(COUNT(*) AS INTEGER) AS cards
FROM cards
WHERE state = 'done' AND updated_at >= ?;

-- name: ListCardLabelsByProject :many
-- Every label on every card of a project, so a board can be built without a query per card.
SELECT card_labels.card_id AS card_id, labels.id AS label_id, labels.project_id AS project_id,
       labels.name AS name, labels.color AS color, labels.created_at AS created_at
FROM card_labels
JOIN labels ON labels.id = card_labels.label_id
WHERE labels.project_id = ?
ORDER BY labels.name;

-- name: ListCardsWithLabel :many
-- The cards that carry a label, before the label is deleted.
SELECT card_id FROM card_labels WHERE label_id = ? ORDER BY card_id;

-- name: SetCardForkFields :execrows
-- Records where a card came from, so its branch starts from that card's latest commit, and the
-- "doing now" line it shows until it starts. Both are set as the card is created, which is why
-- they are not part of the plain insert that every card goes through.
UPDATE cards SET forked_from = ?, doing_now = ?, updated_at = ? WHERE id = ?;

-- name: GetCardForkedFrom :one
-- The card a fork came from, for the session manager when it makes the worktree.
SELECT forked_from FROM cards WHERE id = ?;

-- name: SetNextCardNumber :exec
-- Moves a project's card counter past a number a fixture wrote, so the next card a person adds
-- does not take a number that is already on the board. It never moves the counter backwards.
UPDATE projects SET next_card_number = ? WHERE id = ? AND next_card_number < ?;
