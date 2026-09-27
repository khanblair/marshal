-- The MCP servers Marshal is configured with (migration 0019, docs/architecture.md section 10's
-- `mcp_servers` row, section 11.4). The internal MCP server the daemon serves every agent is not
-- one of these: it is built into the daemon and always attached (section 11.4), while this table is
-- the outside servers a person or a role adds. A role names the ones it wants by name
-- (`apps/web/src/mock/seed/settings.ts` seeds `mcp: ["marshal", "github"]`), which is why the name
-- is unique and a lookup by name is here beside the lookup by id.

-- name: ListMCPServers :many
-- Every configured server, by name, which is the order a settings screen shows them in.
SELECT * FROM mcp_servers ORDER BY name;

-- name: GetMCPServer :one
-- One server by its own id.
SELECT * FROM mcp_servers WHERE id = ?;

-- name: GetMCPServerByName :one
-- One server by the name a role names it with. sql.ErrNoRows means a role asks for a server Marshal
-- has no row for, which the session start reports rather than silently leaving out.
SELECT * FROM mcp_servers WHERE name = ?;

-- name: UpsertMCPServer :exec
-- Saves a server's transport, making the row the first time. It never touches the two health
-- columns: what a server is set up as and what the last check found are separate facts, the same
-- way a connection's settings and its last test are (migration 0015).
INSERT INTO mcp_servers (id, name, transport_json)
VALUES (?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    name           = excluded.name,
    transport_json = excluded.transport_json;

-- name: SetMCPServerHealth :exec
-- Records one check's answer, making the row the first time so a check of a server nobody saved is
-- remembered rather than lost. On conflict the name and the transport are left alone.
INSERT INTO mcp_servers (id, name, health, last_checked_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    health          = excluded.health,
    last_checked_at = excluded.last_checked_at;

-- name: DeleteMCPServer :exec
-- Forgets a server entirely. Removing one that has no row is not an error: the answer is the same
-- list either way.
DELETE FROM mcp_servers WHERE id = ?;
