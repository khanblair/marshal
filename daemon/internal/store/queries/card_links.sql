-- Card dependencies: the edges of a plan (docs/architecture.md section 10, migration 0020,
-- build-plan task 7.4). The Orchestrator makes them as it creates cards from a goal; the board and
-- a card's own panel read them back.

-- name: InsertCardLink :exec
-- Writing the same edge twice is not an error: a card's dependencies are set, not appended, and a
-- caller that repeats one is saying the same thing.
INSERT INTO card_links (card_id, depends_on_id) VALUES (?, ?) ON CONFLICT DO NOTHING;

-- name: ListCardDependencyNumbers :many
-- The numbers of the cards one card waits for, in number order. Keys are built from the project id
-- and a number (protocol.CardKey), so the number is what the caller needs and the join is what
-- gives it.
SELECT dep.number
FROM card_links AS link
JOIN cards AS dep ON dep.id = link.depends_on_id
WHERE link.card_id = ?
ORDER BY dep.number;

-- name: ListProjectDependencies :many
-- Every edge in one project, as the pair (the card, the number of the card it waits for). It is one
-- query for the whole board rather than one per card, because the board summary and the awareness
-- summary both read every card's dependencies at once.
SELECT link.card_id AS card_id, dep.number AS depends_on_number
FROM card_links AS link
JOIN cards AS card ON card.id = link.card_id
JOIN cards AS dep ON dep.id = link.depends_on_id
WHERE card.project_id = ?
ORDER BY card.number, dep.number;
