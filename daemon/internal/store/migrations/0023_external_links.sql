-- Card to outside item: which Marshal card a Trello card (or a later provider's item) is
-- (docs/architecture.md section 10's `external_links` row; docs/marshal-product-scope.md section
-- 19.2's "one Marshal project links to one Trello board"; docs/backend-checklist.md B8.2, build-plan
-- 8.2). It is what makes the sync idempotent: a delivery that arrives twice, or a card that exists on
-- both sides, is recognized by its outside id instead of being imported a second time.
--
-- `kind` is the connection's own kind ("trello" today, and a later provider's own word), so one table
-- serves every integration; `external_id` is that provider's id for the item - for Trello, the card's
-- own `id`, which is what every delivery carries.
--
-- The primary key on (card_id, kind) says one card has at most one outside item per provider. The
-- unique index the other way says one outside item belongs to exactly one card, which is the
-- direction that stops a replayed delivery from making a second card. Both are wanted, and they are
-- not the same constraint.
--
-- A card that goes away takes its links with it, the same way it takes its note and its file claims.
-- There are no times on a link and no foreign key on `kind`: what a card was linked at is when the
-- item was first seen, and a link is a fact about a pair of ids, not a row anyone reads on its own.
-- Forward only, like every migration.

-- +goose Up

CREATE TABLE external_links (
    card_id     TEXT NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    external_id TEXT NOT NULL,
    PRIMARY KEY (card_id, kind)
) STRICT;

-- The reverse read - "which Marshal card is this outside item?" - is the one a delivery makes, so it
-- is the one that needs an index. It is UNIQUE for the reason in the header: one outside item is one
-- Marshal card.
CREATE UNIQUE INDEX external_links_lookup ON external_links (kind, external_id);
