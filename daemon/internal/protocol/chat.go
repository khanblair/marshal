package protocol

// A project chat (docs/architecture.md 16.2, docs/backend-checklist.md B2.10, inventory N13). A
// chat belongs to one project, talks to one target (the Orchestrator, a role, or a card's own
// agent), and keeps its own lasting session: the session row carries `chat_id` where a card's
// carries `card_id`, and it follows the same sleep, wake, and resume rules.
//
// A chat's messages are not a new wire type: they are the same ChatMessage, ChatToolCall, and
// ChatMessageDetail a card's chat already sends (history.go), so a client draws both screens with
// one set of shapes.

// ChatTarget is who a chat talks to. Kind says which of the two fields below means something: an
// orchestrator target has no id, a role target's id is the role name, and a card target's id is the
// card's opaque id - not its key, so the client turns it into a key the way it does everywhere else.
type ChatTarget struct {
	// Kind is what the target is. It is one of the fixed ChatTargetKind values.
	Kind ChatTargetKind `json:"kind"`
	// ID is the role name or the card's opaque id. Empty for the Orchestrator.
	ID string `json:"id"`
}

// Chat is one project chat, as clients see it.
type Chat struct {
	// ID is the chat's opaque id. Routes and the `chat:<id>` topic use it.
	ID string `json:"id"`
	// ProjectID is the project the chat belongs to. A chat is only ever listed for its project.
	ProjectID string `json:"projectId"`
	// Title is the name the app shows. A new chat is "New chat" until its first message names it;
	// a person can rename it at any time.
	Title string `json:"title"`
	// Target is who the chat talks to.
	Target ChatTarget `json:"target"`
	// AgentKind is the agent program its session starts, such as "claude". A chats' sessions are
	// started in slice C; the settings are stored here from the moment the chat is made so the
	// session has them.
	AgentKind AgentKind `json:"agentKind"`
	// Model is the model its session starts with. Empty means the agent's own default.
	Model string `json:"model"`
	// Thinking is the thinking mode its session starts with, and null when none is set.
	Thinking *ThinkingMode `json:"thinking" tstype:"ThinkingMode | null"`
	// PermissionMode is the permission mode its session starts with.
	PermissionMode PermissionMode `json:"permissionMode"`
	// ArchivedAt is when the chat was archived, and null while it is in the main list. Restoring a
	// chat puts it back to null.
	ArchivedAt *Timestamp `json:"archivedAt" tstype:"Timestamp | null"`
	// LastActiveAt is when the chat last had a message, which is the order the list draws it in.
	LastActiveAt Timestamp `json:"lastActiveAt"`
	// CreatedAt is when the chat was made.
	CreatedAt Timestamp `json:"createdAt"`
}

// ChatListSnapshot is the answer to GET /v1/projects/{id}/chats: one project's chats, most recently
// active first, with the archived ones kept only when the caller asked for them.
type ChatListSnapshot struct {
	// ProjectID is the project the chats belong to.
	ProjectID string `json:"projectId"`
	// Chats are the project's chats, most recently active first. Never null, so a project with no
	// chats sends an empty list rather than null.
	Chats []Chat `json:"chats"`
	// ServerTime is the daemon's time when the list was made.
	ServerTime Timestamp `json:"serverTime"`
}

// ChatEventData is the payload of chat.created and chat.updated. It carries the chat as it is now,
// not a difference, so a client that applies one twice, or after a replay, ends in the same place.
type ChatEventData struct {
	// Chat is the chat as it is now.
	Chat Chat `json:"chat"`
}

// ChatArchivedEventData is the payload of chat.archived. It carries the chat as it is now, so the
// same event serves archiving and restoring; a client reads `Chat.ArchivedAt` to tell which.
type ChatArchivedEventData struct {
	// Chat is the chat as it is now, with ArchivedAt set or cleared.
	Chat Chat `json:"chat"`
}

// ChatDeletedEventData is the payload of chat.deleted. It is critical: a client that missed it
// would keep drawing a chat that is gone.
type ChatDeletedEventData struct {
	// ChatID is the opaque id of the chat that is gone, which is what routes and the list use.
	ChatID string `json:"chatId"`
	// ProjectID is the project it belonged to, so a client knows which chat list to redraw.
	ProjectID string `json:"projectId"`
}

// CreateChatRequest is the body of POST /v1/projects/{id}/chats. Everything but the target is
// optional: a chat with no title is "New chat", and one with no agent settings uses the project's
// own defaults.
type CreateChatRequest struct {
	// Title is the name to start with. Empty means "New chat". A title that is only spaces is
	// refused.
	Title string `json:"title,omitempty"`
	// Target is who the chat talks to. Null means the Orchestrator, which is what the app opens a
	// new chat with.
	Target *ChatTarget `json:"target,omitempty" tstype:"ChatTarget | null"`
	// AgentKind is the agent program its session starts. Empty means the project's default.
	AgentKind AgentKind `json:"agentKind,omitempty"`
	// Model is the model its session starts with. Empty means the agent's own default.
	Model string `json:"model,omitempty"`
	// Thinking is the thinking mode its session starts with. Null means none.
	Thinking *ThinkingMode `json:"thinking,omitempty"`
	// PermissionMode is the permission mode its session starts with. Empty means ask.
	PermissionMode PermissionMode `json:"permissionMode,omitempty"`
}

// UpdateChatRequest is the body of PATCH /v1/chats/{id}: rename a chat. A field that is left out is
// not changed, so a client sends only what changed.
type UpdateChatRequest struct {
	// Title is the new name. Null leaves it, and a title that is only spaces is refused.
	Title *string `json:"title,omitempty"`
}
