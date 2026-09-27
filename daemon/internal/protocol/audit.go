package protocol

// The audit log on the wire (docs/architecture.md section 10, docs/backend-checklist.md B3.5,
// docs/marshal-product-scope.md section 14.7): every action worth accounting for, in the order it
// was written. It is read-only: nothing in the API creates, edits, or deletes a row through a route.
// The daemon writes rows as things happen (see internal/audit for the writer and the action names);
// these types are only how a row and a query's answer travel.

// AuditEntry is one row of the audit log.
type AuditEntry struct {
	// ID is the row's own opaque id. It is made by the writer and sorts by time like every other id.
	ID string `json:"id"`
	// SessionID is the session the action belonged to, when it belonged to one.
	SessionID string `json:"sessionId,omitempty"`
	// Actor is who acted: "person", "agent", or "daemon".
	Actor string `json:"actor"`
	// Action is what they did, as one of internal/audit's action names: "approve", "deny",
	// "bypass.on", "bypass.off", "commit.blocked", and so on. It is a plain string, not an enum, so a
	// new kind of action needs no wire change and an older client shows it by its name.
	Action string `json:"action"`
	// Target is what the action was about, when there is one: an approval id, a card id, a commit.
	Target string `json:"target,omitempty"`
	// Detail is whatever else the writer recorded, as JSON. It is left out when the writer recorded
	// nothing.
	Detail map[string]any `json:"detail,omitempty"`
	// At is when it happened, in UTC.
	At Timestamp `json:"at"`
}

// AuditExport is the answer to the export route: everything the query matched, newest first, not
// paged and not counted against the page limit, because an export is asked for as a whole. It
// carries the query it answers so the file says what it is.
type AuditExport struct {
	// Query is the search that produced this export, and is empty for an export of everything.
	Query string `json:"query,omitempty"`
	// Entries are the rows the query matched, newest first.
	Entries []AuditEntry `json:"entries"`
	// Total is how many entries there are, so a reader knows whether it has all of them.
	Total int `json:"total"`
	// ServerTime is the daemon's time when the export was made.
	ServerTime Timestamp `json:"serverTime"`
}
