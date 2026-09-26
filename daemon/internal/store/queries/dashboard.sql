-- The Home dashboard's own reads. Slice A builds the first version: the cards that wait on a
-- person, the cards with a running session, and the tile counts. Slice D adds the stored daily
-- numbers and the activity stream, each in its own file.

-- name: ListAwakeCards :many
-- The cards with a session that is awake, working, or waking, oldest session first, so Home shows
-- the ones that have run longest at the top.
SELECT cards.id AS card_id, cards.project_id AS project_id, projects.name AS project_name,
       cards.number AS number, cards.title AS title, cards.state AS state,
       cards.doing_now AS doing_now, cards.pinned AS pinned, cards.paused AS paused,
       cards.context_used AS context_used, sessions.state AS session_state,
       sessions.created_at AS session_started_at
FROM sessions
JOIN cards ON cards.id = sessions.card_id
JOIN projects ON projects.id = cards.project_id
WHERE sessions.state IN ('awake', 'working', 'waking')
ORDER BY sessions.created_at, cards.id;
