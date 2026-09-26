package protocol

// A card's chat and its activity, as the screens draw them (docs/backend-inventory.md 4.3 and
// 4.4, N13 and N14). Both come from the same stored rows: the session_events index of
// internal/history holds one row per moment a session emitted, and the API maps a row to the chat
// message it draws and, when the row carries a state, to the activity item it draws too.
//
// The message kinds are a fixed list in enums.go (ChatMessageKind). A list route answers with
// Page[ChatMessage] or Page[ActivityItem], newest first (Page, page.go). A message that has more
// to show than one line - a tool call's output and its diffs - keeps that detail out of the page
// and serves it from its own route (ChatMessageDetail), so a page of a hundred tool calls stays
// small.

// ChatMessage is one item of a card's chat, newest first. Kind says which part below is set; the
// parts that do not belong to the kind are null, never left out, so a client always reads the
// same fields.
type ChatMessage struct {
	// ID is the stored event's opaque id. It is the id the message's detail route takes.
	ID string `json:"id"`
	// Kind is what this message is.
	Kind ChatMessageKind `json:"kind"`
	// Seq is the message's place in the card's history, counting from 1, newest first.
	Seq int64 `json:"seq"`
	// At is when the session emitted it, in UTC.
	At Timestamp `json:"at"`
	// Text is a user or agent message's text, a system note, or the one line that stands for the
	// message when Kind is diff, plan, approval, or card. Empty when the message is only its part
	// below (an agent's tool call).
	Text string `json:"text"`
	// Tool is the tool call, for Kind tool, and null for every other kind.
	Tool *ChatToolCall `json:"tool" tstype:"ChatToolCall | null"`
	// Diff is the summary of the files a turn changed, for Kind diff, and null otherwise.
	Diff *ChatDiffSummary `json:"diff" tstype:"ChatDiffSummary | null"`
	// Plan is the plan block, for Kind plan, and null otherwise. Nothing writes it yet (Phase 5).
	Plan *ChatPlan `json:"plan" tstype:"ChatPlan | null"`
	// Approval is the approval block, for Kind approval, and null otherwise. Nothing writes it yet
	// (Phase 3).
	Approval *ChatApproval `json:"approval" tstype:"ChatApproval | null"`
	// Card is the card reference, for Kind card, and null otherwise.
	Card *ChatCardRef `json:"card" tstype:"ChatCardRef | null"`
}

// ChatToolCall is the tool call block of a card's chat: the line it shows, how the call is going,
// and whether there is more to load. The tool's output and its file diffs are never in the page:
// they are read from GET /v1/cards/{id}/messages/{messageId} when the block is opened (B2.6).
type ChatToolCall struct {
	// ID names the call inside the session. An update that only reports output shares the id of
	// the call it updates, so a client matches the two and keeps what an update leaves out.
	ID string `json:"id"`
	// Title is the one line the block shows, such as "Edited internal/upstream/conn.go". An update
	// that does not rename the call leaves it empty.
	Title string `json:"title"`
	// ToolKind is the agent's own word for the tool: read, edit, delete, move, search, execute,
	// think, fetch, switch_mode, or other. Only the call that starts the tool sets it.
	ToolKind string `json:"toolKind"`
	// State is how the call is going. It is null when this event did not report a state (an
	// update that only carried output), which means the state did not change.
	State *ActivityState `json:"state" tstype:"ActivityState | null"`
	// HasDetail is true when the tool's output and diffs can be read from the detail route. It is
	// false when the one line says everything the daemon stored.
	HasDetail bool `json:"hasDetail"`
}

// ChatDiffSummary is what one turn changed, beside the diff itself (N15): how many files, and how
// many lines went in and came out.
type ChatDiffSummary struct {
	// Files is how many files the turn changed.
	Files int `json:"files"`
	// Additions is how many lines were added.
	Additions int `json:"additions"`
	// Deletions is how many lines were removed.
	Deletions int `json:"deletions"`
}

// ChatPlan is a plan block of a card's chat (docs/backend-inventory.md 4.3). The kind and the
// shape are defined now so the app can be written against them; nothing writes a plan message
// until the plan-first session of Phase 5 (B5.2).
type ChatPlan struct {
	// State is where the plan stands.
	State ChatPlanState `json:"state"`
	// Steps are the plan's steps, in order.
	Steps []string `json:"steps"`
	// Files are the files the plan says it will touch.
	Files []string `json:"files"`
	// Risks are the risks the plan names.
	Risks []string `json:"risks"`
	// Checks are the checks the plan says it will run.
	Checks []string `json:"checks"`
}

// ChatApproval is an approval block of a card's chat (docs/backend-inventory.md 4.3). The kind
// and the shape are defined now so the app can be written against them; nothing writes an
// approval message until the permission flow of Phase 3 (B3.4).
type ChatApproval struct {
	// State is where the request stands.
	State ChatApprovalState `json:"state"`
	// Command is the command or the action the agent asked to run. Empty when the request has
	// none.
	Command string `json:"command"`
	// Reason is why the agent asked, in the agent's own words. Empty when it gave none.
	Reason string `json:"reason"`
}

// ChatCardRef is a card reference: the cards an agent or the Orchestrator made or named, in the
// order they appeared (docs/backend-inventory.md 4.3).
type ChatCardRef struct {
	// Cards are the cards the message names. Never null.
	Cards []CardKey `json:"cards"`
}

// ChatMessageDetail is the answer to GET /v1/cards/{id}/messages/{messageId}: one message with
// the parts the page leaves out. Only the tool call has such a part today (its output and its
// diffs); every other kind answers with the message alone.
type ChatMessageDetail struct {
	// Message is the same message the page carries, so a client that opened a row has its kind,
	// its text, and its id.
	Message ChatMessage `json:"message"`
	// Tool is the tool call in full, for Kind tool, and null otherwise.
	Tool *ChatToolDetail `json:"tool" tstype:"ChatToolDetail | null"`
	// ServerTime is the daemon's time when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}

// ChatToolDetail is the whole of one tool call: the output the agent kept, the files it changed,
// and whether the adapter had to cut either short. The full untruncated text belongs to the
// session log on disk (docs/architecture.md section 10).
type ChatToolDetail struct {
	// ID names the call inside the session.
	ID string `json:"id"`
	// Title is the line the block shows.
	Title string `json:"title"`
	// ToolKind is the agent's own word for the tool.
	ToolKind string `json:"toolKind"`
	// Path is the file the call is about. Empty when it has none.
	Path string `json:"path"`
	// Command is the command line of an execute call. Empty when it has none.
	Command string `json:"command"`
	// Content is the output the tool produced, cut to agents.MaxContentBytes.
	Content string `json:"content"`
	// Diffs are the changes the tool made to files. Never null.
	Diffs []FileDiff `json:"diffs"`
	// Truncated is true when Content or Diffs were cut short.
	Truncated bool `json:"truncated"`
}

// ActivityItem is one entry of a card's Activity tab, newest first (docs/backend-inventory.md 4.4,
// N14). It is a stored row that carries a state: a row with no state is a chat message and is
// never served here.
type ActivityItem struct {
	// ID is the stored event's opaque id.
	ID string `json:"id"`
	// Kind is what happened.
	Kind ActivityKind `json:"kind"`
	// Seq is the entry's place in the card's history, counting from 1, newest first.
	Seq int64 `json:"seq"`
	// At is when it happened, in UTC.
	At Timestamp `json:"at"`
	// Text is the one line the entry shows, such as "Edited internal/upstream/conn.go".
	Text string `json:"text"`
	// Result is the short line beside the text, such as "+4 -4". Empty when the daemon has
	// nothing to add to the one line.
	Result string `json:"result"`
	// State is how the entry ended.
	State ActivityState `json:"state"`
}
