-- What a brief is made of, and where it goes (the starter schedules). Forward only, like every
-- migration. STRICT makes SQLite refuse a value of the wrong type.
--
-- `template` names the starter a schedule began as ("morning", "wind-down", "weekly", and so on), or
-- is empty for one made from nothing. It only picks the title, the window a brief looks back over, and
-- the wording; a person can change every other field and the schedule stays what they made.
-- `sections_json` is the list of parts a brief is built from, and `deliver_json` the chat channels it
-- is sent to as well as being kept in the run history. `quiet_when_empty` keeps a brief with nothing
-- to report from being sent at all; it is still recorded as a run. A schedule that already exists
-- keeps its old behavior: no sections means "the way briefs were made before", and no channels means
-- the run history only.

-- +goose Up

ALTER TABLE schedules ADD COLUMN template TEXT NOT NULL DEFAULT '';
ALTER TABLE schedules ADD COLUMN sections_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE schedules ADD COLUMN deliver_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE schedules ADD COLUMN quiet_when_empty INTEGER NOT NULL DEFAULT 0 CHECK (quiet_when_empty IN (0, 1));
