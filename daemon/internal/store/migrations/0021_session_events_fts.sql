-- The full-text index over a card's past sessions, the second half of what docs/architecture.md
-- section 10 asks for: "Session search uses SQLite full-text search over `session_events.summary`
-- and card notes" (docs/backend-checklist.md B7.4, build-plan task 7.10). `notes_fts` (migration
-- 0019) is the first half; this is the second, so `GET /v1/search?q=` can find past work by the
-- words that were kept for it instead of reading every event of every card.
--
-- It indexes `session_events.summary`, which is the one line the daemon stores for a moment of a
-- session (migration 0006; the column is rebuilt by 0009). Nothing else about an event is indexed:
-- `detail_json` is a tool call's payload and `log_ref` points at terminal output, and neither is
-- text anybody searches for.
--
-- ONLY A CARD'S EVENTS ARE INDEXED. A project chat's events store '' as their card (0009) and are
-- found through the Chats kind of the same search, so indexing them here would answer one chat
-- twice. An event with no summary is left out for the same reason the empty word is not a match:
-- it could never be found, and the index would carry a row per event for nothing.
--
-- This is a plain FTS5 table holding its own copy of the summary rather than an external-content
-- one, exactly as `notes_fts` is and for the same reason: a row is addressed by the content table's
-- `rowid`, which is a rowid `session_events` does not put on the wire - a search hit has to name the
-- event's own TEXT id. Keeping the id in an UNINDEXED column is what lets a row be removed by id and
-- lets the query join back to `session_events` for the rest of the event.
--
-- `session_events` is append-only and never pruned, so this index inherits that growth: it is one
-- row per stored event with a summary, for as long as the event is kept. Pruning old events is its
-- own decision (they are a card's history, and how long that is kept is the owner's), and this index
-- does not make it worse - it is proportional to the events themselves. The three triggers keep it
-- in step whichever module writes an event; the fourth removes a deleted card's rows explicitly,
-- matching what migration 0019 measured about a cascade and a delete trigger.
--
-- +goose Up

CREATE VIRTUAL TABLE session_events_fts USING fts5(
    id UNINDEXED,
    card_id UNINDEXED,
    summary
);

-- +goose StatementBegin
CREATE TRIGGER session_events_fts_insert AFTER INSERT ON session_events
WHEN NEW.card_id <> '' AND NEW.summary <> ''
BEGIN
    INSERT INTO session_events_fts (id, card_id, summary)
    VALUES (NEW.id, NEW.card_id, NEW.summary);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER session_events_fts_update AFTER UPDATE ON session_events
BEGIN
    DELETE FROM session_events_fts WHERE id = OLD.id;
    INSERT INTO session_events_fts (id, card_id, summary)
    SELECT NEW.id, NEW.card_id, NEW.summary
    WHERE NEW.card_id <> '' AND NEW.summary <> '';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER session_events_fts_delete AFTER DELETE ON session_events
BEGIN
    DELETE FROM session_events_fts WHERE id = OLD.id;
END;
-- +goose StatementEnd

-- A deleted card takes its events with it through the foreign key, and the delete trigger above
-- with them. This one is written anyway, for the reason 0019 gives: it removes the indexed rows by
-- the card, so the index is cleaned up whether or not the cascade fires the per-row delete trigger.
-- +goose StatementBegin
CREATE TRIGGER cards_session_events_fts_delete AFTER DELETE ON cards
BEGIN
    DELETE FROM session_events_fts WHERE card_id = OLD.id;
END;
-- +goose StatementEnd
