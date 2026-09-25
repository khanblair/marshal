// Package session owns an agent session's life cycle for a card: starting it (with its worktree),
// sending it messages, stopping it, resuming it after the daemon restarts, and its logs
// (docs/architecture.md section 5, B1.9 to B1.11 of docs/backend-checklist.md).
//
// Phase 1 gives a card at most one session for its whole life: chats, which need many sessions per
// project, and sleep, wake, pause, pin, approvals, and bypass are all later phases, and their code
// does not exist yet. This package only ever moves a card between Backlog, Working, and Needs you
// (docs/architecture.md section 6); every other card state is another module's to set.
package session
