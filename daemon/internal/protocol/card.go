package protocol

// Card is a task on a project's board, as clients see it. A card is known by its opaque ID, and
// by its Key (the project and the number) when a person reads or types it.
type Card struct {
	// ID is the card's opaque id. Routes use it.
	ID string `json:"id"`
	// ProjectID is the short id of the project the card belongs to.
	ProjectID string `json:"projectId"`
	// Number counts the project's cards from 1. It is the "41" in "#41". Two projects each have a
	// card 12, so a list that mixes projects shows the project name too.
	Number int `json:"number"`
	// Key is the project id and the number as text, such as "web-dashboard#12".
	Key string `json:"key"`
	// Title is the card's short name.
	Title string `json:"title"`
	// Body is the longer description the agent starts from. It may be empty.
	Body string `json:"body"`
	// State is where the card is on the board.
	State CardState `json:"state"`
	// Agent is which agent program works on the card.
	Agent AgentKind `json:"agent"`
	// Model is the model the agent uses. Empty means the agent's own default.
	Model string `json:"model"`
	// Thinking is how much the agent thinks. It is null when the card has no thinking setting.
	// The tstype tag makes the generated TypeScript `ThinkingMode | null` and required, because
	// this field is always sent, as null when there is no setting: a pointer without `omitempty`
	// encodes nothing away, so the type must not make it optional.
	Thinking *ThinkingMode `json:"thinking" tstype:"ThinkingMode | null"`
	// PermissionMode is how much the agent may do without asking.
	PermissionMode PermissionMode `json:"permissionMode"`
	// Role is the role's name, such as "Implementer". Phase 5 turns this into a role id; until
	// then it is the text name of a starter role, and it may be empty.
	Role string `json:"role"`
	// Labels are the project's labels on this card, in the order they were added. Never null.
	Labels []Label `json:"labels"`
	// Package is the monorepo package the card works in. Empty when the project is not one.
	Package string `json:"package"`
	// PlannedStart and PlannedEnd are when the card is meant to run (the Timeline draws these for
	// a card that has not started). Null when the person set no plan.
	PlannedStart *Timestamp `json:"plannedStart" tstype:"Timestamp | null"`
	PlannedEnd   *Timestamp `json:"plannedEnd" tstype:"Timestamp | null"`
	// Due is when the card is due. Null when it has no date.
	Due *Timestamp `json:"due" tstype:"Timestamp | null"`
	// ActualStart and ActualEnd are when the card really ran (the Timeline draws these once it
	// has started, decision D1). Null before the card starts, and before it ends.
	ActualStart *Timestamp `json:"actualStart" tstype:"Timestamp | null"`
	ActualEnd   *Timestamp `json:"actualEnd" tstype:"Timestamp | null"`
	// PullRequest is the card's pull request. Null when it has none. Phase 1 has no CI or forge
	// integration, so it is only ever set by hand or by the fixture.
	PullRequest *PullRequest `json:"pullRequest" tstype:"PullRequest | null"`
	// CI is the state of the card's CI run. Null when the card has no CI data (nothing invents
	// one: architecture.md section 11.5 and the design-port deviations list).
	CI *CIState `json:"ci" tstype:"CIState | null"`
	// ContextUsed is how full the agent's context window is, as a percentage from 0 to 100.
	ContextUsed int `json:"contextUsed"`
	// NeedsReason is why the card waits on a person. Null unless State is needs.
	NeedsReason *NeedsReason `json:"needsReason" tstype:"NeedsReason | null"`
	// DoingNow is the agent's one-line "doing now", empty when it is not working.
	DoingNow string `json:"doingNow"`
	// Paused is true while a pause holds the card between turns. Slice G of Phase 2 writes it.
	Paused bool `json:"paused"`
	// Pinned is true while the card is kept from sleeping on its own. Automatic sleep is Phase 5,
	// so today it only records what the person chose.
	Pinned bool `json:"pinned"`
	// Session is the state of the card's session as it was last stored: awake, working, asleep,
	// waking, and the rest. It is null when the card has never had a session. It is the stored
	// state rather than whether a process runs in the daemon right now, so it survives a restart:
	// a card put to sleep reads asleep after the daemon starts again, and one that was awake reads
	// awake until the restore resumes it (docs/architecture.md 5.3). Every change to it is also
	// announced as card.updated on the project topic, beside session.state_changed on the card's
	// own topic. It is always sent, as null when there is no session: a pointer without `omitempty`
	// encodes nothing away, and the `required` flag of the tstype tag makes the generated type
	// `SessionState | null`, where a bare pointer would be generated as an optional field.
	Session *SessionState `json:"session" tstype:"SessionState | null,required"`
	// ViewMode is the view the card's agent runs in: chat, or terminal (docs/architecture.md 4.3).
	// It is stored with the session, so it survives a restart, and it is chat for a card with no
	// session and for one whose session has stopped. It is always sent. Every change to it is
	// announced as card.updated, like the session state it goes with.
	ViewMode CardViewMode `json:"viewMode"`
	// Branch is the Git branch of the card's work. Empty until the card starts.
	Branch string `json:"branch"`
	// CreatedAt is when the card was made.
	CreatedAt Timestamp `json:"createdAt"`
	// UpdatedAt is when the card last changed.
	UpdatedAt Timestamp `json:"updatedAt"`
}

// PullRequest is a card's pull request: the number a person reads and the address it lives at.
type PullRequest struct {
	// Number is the pull request number, such as 287.
	Number int `json:"number"`
	// URL is the address of the pull request. Empty when only the number is known.
	URL string `json:"url"`
}

// NeedsReason is why a card waits on a person: a kind a client can act on, and the sentence the
// app shows.
type NeedsReason struct {
	// Kind is the reason.
	Kind NeedsReasonKind `json:"kind"`
	// Text is the plain sentence shown under the card. It may be empty.
	Text string `json:"text"`
}

// BoardSnapshot is the answer to GET /v1/projects/{id}/board: the columns and every card.
type BoardSnapshot struct {
	// ProjectID is the project the board belongs to.
	ProjectID string `json:"projectId"`
	// Columns are the card states that have a column, in board order. A card in the "merging"
	// state has no column of its own: it is shown in the "ready" column.
	Columns []CardState `json:"columns"`
	// Cards are all the project's cards, in number order.
	Cards []Card `json:"cards"`
	// ServerTime is the daemon's time when the board was made.
	ServerTime Timestamp `json:"serverTime"`
}

// CreateCardRequest is the body of POST /v1/projects/{id}/cards. The card starts in the backlog
// unless StartState names another of the three states a person can add a card in.
type CreateCardRequest struct {
	// Title is the card's name. It cannot be empty.
	Title string `json:"title"`
	// Body is the description. It may be empty.
	Body string `json:"body,omitempty"`
	// Agent is which agent works on the card. Empty means Claude Code.
	Agent AgentKind `json:"agent,omitempty"`
	// Model is the model to use. Empty means the agent's own default.
	Model string `json:"model,omitempty"`
	// Thinking is how much the agent thinks. Empty means no setting.
	Thinking ThinkingMode `json:"thinking,omitempty"`
	// PermissionMode is how much the agent may do without asking. Empty means auto-accept edits.
	PermissionMode PermissionMode `json:"permissionMode,omitempty"`
	// Role is the role's name. Empty means no role.
	Role string `json:"role,omitempty"`
	// Package is the monorepo package. Empty means none.
	Package string `json:"package,omitempty"`
	// StartState is the column the card is added in: backlog (the default when empty), planning,
	// or working. Any other value is refused, because a card reaches the other columns by what
	// happens to it, not by being added there.
	//
	// The card row is written in the backlog and its session is started straight after, so nothing
	// is ever shown as working before its agent exists. If the agent cannot start, the card is
	// removed again and the request fails: adding a card either works or leaves nothing behind.
	StartState CardState `json:"startState,omitempty"`
}

// UpdateCardRequest is the body of PATCH /v1/cards/{id}. A field that is not set is left as it
// is, so a client sends only what the person changed. Labels replaces the whole list when it is
// not null.
type UpdateCardRequest struct {
	// Title renames the card. Null leaves the name.
	Title *string `json:"title,omitempty"`
	// Body replaces the description. Null leaves it.
	Body *string `json:"body,omitempty"`
	// Agent changes the agent program. Null leaves it.
	Agent *AgentKind `json:"agent,omitempty"`
	// Model changes the model. Null leaves it; the empty string clears it.
	Model *string `json:"model,omitempty"`
	// Thinking changes the thinking mode. Null leaves it; the empty string clears it, which is what
	// a model that cannot be told how hard to think leaves on a card.
	Thinking *ThinkingMode `json:"thinking,omitempty" tstype:"ThinkingMode | ''"`
	// PermissionMode changes how much the agent may do. Null leaves it.
	PermissionMode *PermissionMode `json:"permissionMode,omitempty"`
	// Role changes the role name. Null leaves it.
	Role *string `json:"role,omitempty"`
	// Package changes the package. Null leaves it.
	Package *string `json:"package,omitempty"`
	// Labels replaces the card's labels with exactly these ids. Null leaves them.
	Labels *[]string `json:"labels,omitempty"`
	// PlannedStart, PlannedEnd, Due, ActualStart, and ActualEnd set dates. Null leaves the date.
	// A DateChange clears it instead (there is no way to tell "clear" from "absent" with a
	// pointer alone).
	PlannedStart *DateChange `json:"plannedStart,omitempty"`
	PlannedEnd   *DateChange `json:"plannedEnd,omitempty"`
	Due          *DateChange `json:"due,omitempty"`
	ActualStart  *DateChange `json:"actualStart,omitempty"`
	ActualEnd    *DateChange `json:"actualEnd,omitempty"`
	// DoingNow replaces the "doing now" line. Null leaves it.
	DoingNow *string `json:"doingNow,omitempty"`
	// NeedsReason sets or clears why the card waits on a person. Null leaves it.
	NeedsReason *NeedsReason `json:"needsReason,omitempty"`
}

// DateChange sets a card's date, or clears it. It exists because a JSON null and an absent field
// are the same to Go's decoder, so "clear this date" needs its own shape.
type DateChange struct {
	// Clear removes the date. When it is false, At is the new date.
	Clear bool `json:"clear,omitempty"`
	// At is the new date, in UTC with milliseconds, and null when the date is being cleared. It is
	// a pointer because a Timestamp is a struct: `omitempty` cannot leave one out, and a Timestamp
	// that was never set refuses to encode at all.
	At *Timestamp `json:"at,omitempty" tstype:"Timestamp | null"`
}

// MoveCardRequest is the body of POST /v1/cards/{id}/move: the column a person dragged the card
// to. It carries no reason, because the rules of docs/architecture.md 6.1 refuse every manual move
// to Needs you (rule 3): a card waits on a person because an agent is waiting, never because
// somebody dragged it there. The reason a card carries is written by the daemon on its own path,
// through UpdateCardRequest or the session manager.
type MoveCardRequest struct {
	// State is the column to move to. It must be one of the board's columns.
	State CardState `json:"state"`
}
