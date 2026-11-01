-- The outbox (docs/architecture.md section 19.5). A notice or an email brief that could not be sent
-- because the internet was down is kept here and sent when it is back, oldest first. Forward only,
-- like every migration.
--
-- `kind` says what the payload is: a chat or email `notice`, or a `brief`. `target` is the channel it
-- was going to, such as "telegram" or "resend". `attempts` counts sends that failed for a reason other
-- than the network, so a notice a service keeps refusing is dropped after a few tries. A row older than
-- a day is dropped when the outbox is flushed, so a long outage does not end in a flood.

-- +goose Up

CREATE TABLE outbox (
    id           TEXT NOT NULL PRIMARY KEY,
    kind         TEXT NOT NULL CHECK (kind IN ('notice', 'brief')),
    target       TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    attempts     INTEGER NOT NULL DEFAULT 0,
    last_error   TEXT NOT NULL DEFAULT '',
    created_at   INTEGER NOT NULL
) STRICT;

CREATE INDEX outbox_created ON outbox (created_at, id);
