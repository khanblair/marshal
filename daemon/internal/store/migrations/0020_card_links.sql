-- Card dependencies: which cards a card waits for (docs/marshal-product-scope.md section 10.2's
-- goal-to-plan flow and the Orchestrator's "create cards with the right roles and dependencies";
-- docs/backend-checklist.md B7.3; build-plan task 7.4). The mock has drawn these as lines between
-- cards on the Timeline since before the daemon existed (apps/web/src/mock/types.ts's `deps`, and
-- views/timeline/timeline-geometry.ts's dependencyLines); this is where the daemon records them.
--
-- An edge points from a card to the card it depends on, which is the direction the plan is written
-- in: "this card cannot start until that one is done". The pair is the primary key, so saying the
-- same thing twice is one edge and not two. Both ends are cards of the same project - nothing in
-- the schema can say so, since a foreign key names a table and not a column, so the projects service
-- checks it when the edge is made.
--
-- No cycle can be made: an edge is only ever written for a card that is being created, and it may
-- only name cards that already exist. A card therefore never depends on something newer than itself,
-- and a chain of edges always walks backwards in creation order and so ends. That is a property of
-- how the rows are written rather than a rule the schema enforces, so it is written down here for
-- whoever later adds a way to edit a card's dependencies.
--
-- Both ends cascade: a card that goes away takes the edges that name it with it, in either
-- direction, so no edge is left pointing at a card that is not there. Forward only, like every
-- migration. The ids are TEXT and there are no times on an edge: what a card was created at is when
-- the relation was made.

-- +goose Up

CREATE TABLE card_links (
    card_id       TEXT NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    depends_on_id TEXT NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    PRIMARY KEY (card_id, depends_on_id)
) STRICT;

-- The primary key covers "what does this card wait for", which is the read a card's own panel and
-- the Orchestrator both do. This index covers the other direction - "what is waiting on this card" -
-- which is what tells a person whose work is holding up the board, and what a card that has just
-- finished would use to release the cards behind it.
CREATE INDEX card_links_depends_on ON card_links (depends_on_id);
