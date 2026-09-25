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
	Thinking *ThinkingMode `json:"thinking"`
	// PermissionMode is how much the agent may do without asking.
	PermissionMode PermissionMode `json:"permissionMode"`
	// Branch is the Git branch of the card's work. Empty until the card starts.
	Branch string `json:"branch"`
	// CreatedAt is when the card was made.
	CreatedAt Timestamp `json:"createdAt"`
	// UpdatedAt is when the card last changed.
	UpdatedAt Timestamp `json:"updatedAt"`
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

// CreateCardRequest is the body of POST /v1/projects/{id}/cards. The card starts in the backlog.
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
}
