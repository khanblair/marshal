// Package history stores and reads the typed history and activity of a card's session: the rows
// of the session_events table (migration 0006).
//
// The session manager appends to it as each agent event arrives, so a card's chat survives a
// restart and pages by cursor instead of replaying a log file; the API layer reads it back. A
// caller appends Record values and never makes an id or a sequence number itself.
//
// The kinds and states are this package's own words, not the wire's. The API layer maps them onto
// the chat message kinds of docs/backend-inventory.md 4.3 (N13) and the activity kinds and states
// of 4.4 (N14) when it serves a page, so a wire enum can change without a migration.
//
// The package depends on the store, on the agents package (to turn an agent event into records),
// and on nothing else. Modules that use it reach it through a narrow interface of their own
// (internal/session defines HistoryRecorder), never through its tables.
package history
