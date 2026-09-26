-- Home's stored numbers: the pre-computed per-day dashboard counts (docs/architecture.md 16.3,
-- docs/backend-checklist.md B2.3, inventory N17) and the activity stream the Recent activity list
-- and its view-all page draw. Forward only, like every migration. Times and days are INTEGER Unix
-- milliseconds in UTC and ids are TEXT. STRICT makes SQLite refuse a value of the wrong type.
--
-- Both tables are written by the dashboard module's event subscriber (internal/dashboard) from the
-- events the daemon already publishes, and are read by the Home answer and the activity route. Home
-- therefore never scans every card to draw a chart or a feed: a chart is one indexed range read and
-- a feed page is one indexed key read.
--
-- The words `kind` and `subject_kind` can hold are checked in Go against the lists in the protocol
-- package (FeedKind) and the constants in internal/dashboard, not here, for the same reason
-- cards.state is not checked here (see 0002_projects.sql).

-- +goose Up

-- One row per day and project: what happened that day. `day` is midnight at the start of the day in
-- the daemon's own clock, in Unix milliseconds, so the same day arithmetic as Home's "merged today"
-- (internal/dashboard.startOfDay) reads a range with two integer comparisons.
--
-- (day, project_id) is the primary key and therefore the index a range read needs: a read of seven,
-- thirty, or ninety days is one range scan in day order, and grouping a day's rows sums a handful of
-- per-project rows rather than every card. No second index is made on the same columns.
--
-- Every column is a total, not a snapshot: a subscriber adds to it as the day goes on, which is what
-- the upsert in queries/daily_stats.sql does. cost_micros is the same micro-dollar unit the rest of
-- the daemon uses for money; nothing writes it yet (usage and cost are Phase 4, B4.4).
CREATE TABLE daily_stats (
    day            INTEGER NOT NULL,
    project_id     TEXT NOT NULL,
    cards_finished INTEGER NOT NULL DEFAULT 0 CHECK (cards_finished >= 0),
    merges         INTEGER NOT NULL DEFAULT 0 CHECK (merges >= 0),
    ci_failures    INTEGER NOT NULL DEFAULT 0 CHECK (ci_failures >= 0),
    cost_micros    INTEGER NOT NULL DEFAULT 0 CHECK (cost_micros >= 0),
    PRIMARY KEY (day, project_id)
) STRICT;

-- The Home activity stream: one append-only row per thing the feed shows (a merge, a brief, a
-- schedule run, an approval, a plan, a CI result, a tool or system event). Rows are never updated
-- and are deleted only by the ninety-day trim below, so a cursor stays true. `seq` numbers the
-- whole stream from 1 in the order things happened, so a page is read by cursor and never by offset.
--
-- `summary` is the one line the feed row shows. The subject says what the row is about and how a
-- client opens it: subject_kind is 'card', 'schedule', 'project', or empty; subject_id is that
-- thing's own id; subject_key is the way the screens name it, `<projectId>#<number>` for a card and
-- empty otherwise. The key is stored rather than joined from cards at read time for the same reason
-- card.deleted carries it: a card can be gone while its history still shows, and a client cannot turn
-- an opaque id back into a key by itself.
--
-- `project_id` is empty for an entry that belongs to no project (a brief, or a removed project's own
-- line) and is the project's short id otherwise. It is deliberately not a foreign key: an empty
-- string is not a project, and the dashboard module owns this table, so it removes a project's rows
-- itself when the project.removed event arrives (see the subscriber) rather than leaning on a
-- cascade. An empty string rather than NULL keeps the generated parameter and row types plain
-- strings, the way sessions.card_id stays NOT NULL with '' for "no card".
CREATE TABLE activity (
    id           TEXT NOT NULL PRIMARY KEY,
    seq          INTEGER NOT NULL CHECK (seq >= 1),
    project_id   TEXT NOT NULL DEFAULT '',
    kind         TEXT NOT NULL,
    subject_kind TEXT NOT NULL DEFAULT '',
    subject_id   TEXT NOT NULL DEFAULT '',
    subject_key  TEXT NOT NULL DEFAULT '',
    summary      TEXT NOT NULL DEFAULT '',
    created_at   INTEGER NOT NULL,
    UNIQUE (seq)
) STRICT;

-- A page of one kind (the page's kind pills) and a page of one project (its project select) each
-- read in stream order without scanning the other kinds or projects.
CREATE INDEX activity_kind_seq ON activity (kind, seq);
CREATE INDEX activity_project_seq ON activity (project_id, seq);
-- The ninety-day trim deletes by age, and the oldest rows are the only ones it looks at.
CREATE INDEX activity_created_at ON activity (created_at);
