package protocol

import "time"

// Two things the app's own Role interface spells out and a mapper turns into the shape the screens
// read (apps/web/src/mock/settings-types.ts): Marshal ships starter role templates, a person may
// edit any field of any of them, and a project may keep its own version of one. Nothing here is an
// enum: the agent, the model, the thinking label, the permission label, and the strength are the
// words the screens show, so they are kept as text and the story of which of them is legal lives in
// the app's pickers, not in a list the daemon would have to grow in step with.

// RoleLimits is a role's own ceilings, the three numbers the role editor's limits row holds: how
// long a turn may run (minutes), what a card may cost (whole dollars), and how many turns it may
// take. Zero means the role sets no ceiling.
//
// They are not the install's limits (protocol.Limit, internal/providers/limits.go): a limit is what
// Marshal may spend, these are what one role's cards aim for, and Phase 5's harness moves a card to
// Needs you when it passes one.
type RoleLimits struct {
	// Time is how long one turn may run, in minutes.
	Time int `json:"time"`
	// Cost is the most one card of this role may cost, in whole dollars.
	Cost int `json:"cost"`
	// Rounds is the most turns one card of this role may take.
	Rounds int `json:"rounds"`
}

// RoleSpec is the editable body of a role: everything but its name and the two flags that say where
// it came from. It is stored as the role's own JSON document (roles.spec_json) and, in the same
// shape, is what a role's export writes and its import reads, so a role moves between machines
// without a second format to keep in step.
//
// Skills and MCP are lists of names and never null: a role with none has an empty list, so the JSON
// has [] and the screens can map over it without a check.
type RoleSpec struct {
	// Skills are the skill names the role's agent loads.
	Skills []string `json:"skills"`
	// MCP are the MCP server names the role's agent may use.
	MCP []string `json:"mcp"`
	// Limits are the role's own ceilings.
	Limits RoleLimits `json:"limits"`
	// Backup is the model to fall back to when the main model is unavailable. Empty means none.
	Backup string `json:"backup"`
	// Desc is one plain sentence saying what the role is for.
	Desc string `json:"desc"`
	// Agent is the agent program the role runs, as the agent picker names it ("Claude Code").
	Agent string `json:"agent"`
	// Model is the model the role runs, as the model picker names it.
	Model string `json:"model"`
	// Think is the thinking label the role uses ("High"). Empty means the model's own default.
	Think string `json:"think"`
	// Perm is the permission label the role runs under ("Plan only").
	Perm string `json:"perm"`
	// Strength is the model strength the role is meant for ("Strong").
	Strength string `json:"strength"`
	// Instr is the role's system prompt: the instructions its agent is given.
	Instr string `json:"instr"`
}

// Role is one role template as a screen sees it: its name, whether Marshal shipped it, whether the
// project being looked at keeps its own version of it, and the editable body.
type Role struct {
	// ID is the role's own opaque id. It never changes, so a rename keeps every reference to the
	// role good. The screens address a role by its name, which is unique; the id is here for a
	// client that wants something stable to hold on to.
	ID string `json:"id"`
	// Name is the role's name, unique across every role Marshal knows. A card names its role by
	// this, and so does a project chat that talks to a role.
	Name string `json:"name"`
	// Starter is true for a role Marshal shipped. A starter role is reset rather than deleted; a
	// role a person made is deleted. Reset clears only the overridden flag, it does not restore
	// the starter text.
	Starter bool `json:"starter"`
	// Overridden is true when the project this list was asked about keeps its own version of the
	// role. A list asked for without a project reports false for every role, because no project
	// is being looked at.
	Overridden bool `json:"overridden"`
	// Spec is the role's editable body.
	Spec RoleSpec `json:"spec"`
}

// RoleList is the answer to GET /v1/roles and to a change that returns the new list: every role
// Marshal knows, in the order the screens show them.
type RoleList struct {
	// Roles has one entry for every role, starters first in the order Marshal ships them and
	// roles a person made after them in the order they were made.
	Roles []Role `json:"roles"`
	// ServerTime is the daemon's time when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}

// NewRoleList makes an answer stamped with the daemon's time. A nil list becomes an empty one, so
// the JSON has [] and never null.
func NewRoleList(roles []Role, now time.Time) RoleList {
	out := make([]Role, len(roles))
	copy(out, roles)
	return RoleList{Roles: out, ServerTime: NewTimestamp(now)}
}

// CreateRoleRequest is the body of POST /v1/roles: a new role, or a role being imported. It is the
// export document with the two flags left out, so a role that was exported from one machine can be
// posted here as it is: the id, the starter flag, and the overridden flag are the daemon's to set
// and are ignored when they are sent.
type CreateRoleRequest struct {
	// Name is what to call the new role. A name another role already has is a conflict.
	Name string `json:"name"`
	// Spec is the role's body. A missing skills or MCP list becomes an empty one.
	Spec RoleSpec `json:"spec"`
}

// UpdateRoleRequest is the body of PATCH /v1/roles/{name}: the fields to change. A nil field is
// left as it is, so a client that sends only a new name renames the role and changes nothing else.
type UpdateRoleRequest struct {
	// Name is a new name for the role. A name another role already has is a conflict.
	Name *string `json:"name,omitempty"`
	// Spec replaces the role's body when it is sent.
	Spec *RoleSpec `json:"spec,omitempty"`
}
