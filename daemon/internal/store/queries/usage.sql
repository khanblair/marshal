-- What Marshal's own model calls cost (migration 0014, docs/architecture.md section 10, inventory
-- B4.4). The store's usage writer (internal/providers) inserts one row per provider call and, in the
-- same transaction, adds the same cost to that day's daily_stats.cost_micros, so the Home chart and
-- the usage detail can never disagree. Nothing here updates or deletes: spend is append-only.

-- name: InsertUsage :exec
-- Records one model call: which provider and model answered, which card, project, and role it ran
-- for, what it read and wrote, what that cost in micro-dollars, and when. `id` is an opaque id its
-- caller makes, so a caller that retries can recognise the row it already wrote.
INSERT INTO usage (
    id, card_id, project_id, role_id, provider, model,
    input_tokens, output_tokens, cost_micros, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListUsageForCard :many
-- One card's spend, newest first. The card's own page and the card's usage tab read this. The id
-- breaks a tie between two calls written in the same millisecond, so a page boundary is stable.
SELECT * FROM usage
WHERE card_id = ?
ORDER BY created_at DESC, id DESC
LIMIT ?;

-- name: ListUsageForProject :many
-- One project's spend, newest first, which is what the Home cost-per-project chart drills into.
SELECT * FROM usage
WHERE project_id = ?
ORDER BY created_at DESC, id DESC
LIMIT ?;

-- name: SumUsageForCard :one
-- One card's total spend in micro-dollars, which is what the harness reads against a role's own
-- cost ceiling (B5.3, docs/architecture.md section 9 step 4). A card with no provider call yet sums
-- to zero, so a caller needs no special case for a card that has not spent anything.
SELECT CAST(COALESCE(SUM(cost_micros), 0) AS INTEGER) AS cost_micros FROM usage WHERE card_id = ?;
