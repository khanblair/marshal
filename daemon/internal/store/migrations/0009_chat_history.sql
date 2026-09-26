-- A chat's history (docs/backend-checklist.md B2.10, slice C's message half): the typed events of a
-- chat's session go into the same session_events index a card's do, so a chat's messages are paged
-- exactly as a card's are. Forward only, like every migration. Times are INTEGER Unix milliseconds
-- in UTC and ids are TEXT. STRICT makes SQLite refuse a value of the wrong type.
--
-- session_events was made for cards alone: `card_id` was NOT NULL and pointed at `cards`, and
-- UNIQUE (card_id, seq) numbered a card's events. A row now belongs to a card or to a chat, the way
-- 0007_chats.sql made a session belong to either, so the table is rebuilt once here and its rows are
-- copied as they are. SQLite cannot change a column's constraints in place.
--
-- The shape follows 0007 on purpose. `card_id` stays NOT NULL, with an empty string for "this event
-- is a chat's", so db.SessionEvent.CardID stays a plain string at every card call site; `chat_id` is
-- the chat's id and NULL on a card's event; the CHECK allows exactly one of the two. `card_id` loses
-- its REFERENCES, because a foreign key on a NOT NULL column would require the empty sentinel to be
-- a card. The two triggers at the end give back what that foreign key did: an event cannot name a
-- card that is not there, and a card's events go with the card. (The second also follows from the
-- session: every event points at its session, and a session is removed with its card by the trigger
-- of 0007, which removes its events in the same step.)
--
-- Each owner numbers its own events from 1, so the unique rule becomes two partial indexes, one per
-- kind of owner. A query must carry the index's own condition to use a partial index (SQLite only
-- picks one when the query's WHERE contains the index's WHERE), which is why the card queries in
-- queries/session_events.sql say `card_id <> ''` beside `card_id = ?`.
--
-- Nothing references session_events, so dropping it deletes no other row, and this migration runs
-- in goose's usual transaction with foreign keys on.

-- +goose Up

CREATE TABLE session_events_new (
    id          TEXT NOT NULL PRIMARY KEY,
    card_id     TEXT NOT NULL DEFAULT '',
    chat_id     TEXT REFERENCES chats (id) ON DELETE CASCADE,
    session_id  TEXT NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
    seq         INTEGER NOT NULL CHECK (seq >= 1),
    kind        TEXT NOT NULL,
    state       TEXT NOT NULL DEFAULT '',
    summary     TEXT NOT NULL DEFAULT '',
    detail_json TEXT NOT NULL DEFAULT '',
    log_ref     TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    CHECK ((card_id = '') <> (chat_id IS NULL))
) STRICT;

INSERT INTO session_events_new (
    id, card_id, chat_id, session_id, seq, kind, state, summary, detail_json, log_ref, created_at
)
SELECT
    id, card_id, NULL, session_id, seq, kind, state, summary, detail_json, log_ref, created_at
FROM session_events;

DROP TABLE session_events;
ALTER TABLE session_events_new RENAME TO session_events;

-- The unique numbering of each owner's events, and the index that answers "the newest N before
-- this cursor" for each: a page reads one of these backwards from its cursor.
CREATE UNIQUE INDEX session_events_card_seq ON session_events (card_id, seq) WHERE card_id <> '';
CREATE UNIQUE INDEX session_events_chat_seq ON session_events (chat_id, seq) WHERE chat_id IS NOT NULL;
-- One kind of a card's activity pages on its own, as before.
CREATE INDEX session_events_card_kind ON session_events (card_id, kind, seq);

-- The two halves of the old card_id foreign key. The StatementBegin/End pairs keep goose from
-- splitting a trigger on the semicolon inside its body. The first refuses an event for a card that
-- is not there, with the same error text a foreign key gives; the second removes a card's events
-- with the card.
-- +goose StatementBegin
CREATE TRIGGER session_events_card_must_exist BEFORE INSERT ON session_events
WHEN NEW.card_id <> '' AND NOT EXISTS (SELECT 1 FROM cards WHERE id = NEW.card_id)
BEGIN
    SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER session_events_card_cascade_delete AFTER DELETE ON cards
BEGIN
    DELETE FROM session_events WHERE card_id = OLD.id AND card_id <> '';
END;
-- +goose StatementEnd
