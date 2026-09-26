package history

import "slices"

// Kind names what one stored event is. The words are this package's own, not the wire's: the API
// layer maps them to the chat message kinds a screen draws (docs/backend-inventory.md 4.3, N13)
// when it serves a page, so the wire enum can grow without a migration.
type Kind string

const (
	// KindUser is a message a person sent into the session.
	KindUser Kind = "user"
	// KindAgent is a piece of the agent's answer. Pieces of one answer join into one message.
	KindAgent Kind = "agent"
	// KindThought is a piece of the agent's reasoning, when the agent shares it.
	KindThought Kind = "thought"
	// KindToolCall starts a tool call, with the detail a chat block draws.
	KindToolCall Kind = "tool_call"
	// KindToolCallUpdate is progress on a tool call that is already stored.
	KindToolCallUpdate Kind = "tool_call_update"
	// KindPlan replaces the agent's plan in a plan-first session.
	KindPlan Kind = "plan"
	// KindApproval is a permission the agent asked for. It holds StateWaiting until it is
	// answered, which is the approvals work of a later phase.
	KindApproval Kind = "approval"
	// KindSystem is a note the daemon wrote about the session itself, such as a failure.
	KindSystem Kind = "system"
	// KindDiffSummary is the files a turn changed, beside the diff itself. Nothing writes it yet:
	// it is reserved for the turn-end diff of a later slice (inventory N15), the way the protocol
	// package reserves an event type it does not publish.
	KindDiffSummary Kind = "diff_summary"
	// KindCardReference is a card an agent made or named. Nothing writes it yet: the tool that
	// makes a card is the internal MCP server of a later phase.
	KindCardReference Kind = "card_reference"
)

// KindValues lists every kind, in the order of the constants above.
func KindValues() []Kind {
	return []Kind{
		KindUser, KindAgent, KindThought, KindToolCall, KindToolCallUpdate, KindPlan,
		KindApproval, KindSystem, KindDiffSummary, KindCardReference,
	}
}

// Valid reports whether k is a kind this package stores.
func (k Kind) Valid() bool { return slices.Contains(KindValues(), k) }

// State says how an entry of a card's activity list ended (docs/backend-inventory.md 4.4, N14).
// The empty state means the entry is a chat message rather than an activity item.
type State string

const (
	// StateOK is work that finished.
	StateOK State = "ok"
	// StateRunning is work that is still going.
	StateRunning State = "running"
	// StateFailed is work that ended badly.
	StateFailed State = "failed"
	// StateWaiting is work blocked on a person, such as a permission request.
	StateWaiting State = "waiting"
)

// StateValues lists every activity state, in the order of the constants above.
func StateValues() []State { return []State{StateOK, StateRunning, StateFailed, StateWaiting} }

// Valid reports whether s is a state an entry can hold. The empty state is valid: it means the
// entry is not an activity item.
func (s State) Valid() bool { return s == "" || slices.Contains(StateValues(), s) }
