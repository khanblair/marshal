-- The typed history and activity of a card's session: one append-only row per moment a session
-- emits (inventory N13) or that a card's activity list shows (N14). Forward only, like every
-- migration. Times are INTEGER Unix milliseconds in UTC and ids are TEXT. STRICT makes SQLite
-- refuse a value of the wrong type.
--
-- The full output also stays in the session's log files on disk (docs/architecture.md section 10);
-- this table is the index the daemon pages through after a restart. A row is never updated and
-- never deleted except by the cascade below, so a cursor stays true.
--
-- The words `kind` and `state` can hold are checked in Go against the lists in internal/history,
-- not here, for the same reason cards.state is not checked here (see 0002_projects.sql).

-- +goose Up

-- seq numbers a card's events from 1, in the order they happened, so a page is read by cursor and
-- never by offset. UNIQUE (card_id, seq) is what makes that sequence trustworthy (a concurrent
-- append cannot reuse a number) and its own index is what answers "the newest N before this
-- cursor", so no second index on the same columns is made.
--
-- summary is the one line the history and the activity list show (a message chunk's text, a tool
-- call's title, a command line). detail_json is the structured payload a row needs to be drawn in
-- full, which is what "tool detail loaded on demand" reads back by id; it is empty when the line
-- alone says everything. log_ref is where the event sits in the on-disk session log, for the full
-- untruncated text; nothing writes it yet, because a log segment has no line numbers until a
-- reader needs them, and detail_json already carries what the adapter kept.
CREATE TABLE session_events (
    id          TEXT NOT NULL PRIMARY KEY,
    card_id     TEXT NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    session_id  TEXT NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
    seq         INTEGER NOT NULL CHECK (seq >= 1),
    kind        TEXT NOT NULL,
    state       TEXT NOT NULL DEFAULT '',
    summary     TEXT NOT NULL DEFAULT '',
    detail_json TEXT NOT NULL DEFAULT '',
    log_ref     TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    UNIQUE (card_id, seq)
) STRICT;

-- One kind of a card's activity (files changed, commands, tests) pages on its own, newest first,
-- beside the whole history a chat screen pages through.
CREATE INDEX session_events_card_kind ON session_events (card_id, kind, seq);
