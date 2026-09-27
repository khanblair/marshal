package protocol

import "slices"

// The approval flow (docs/architecture.md section 11.4, docs/backend-checklist.md B3.4, N7): an
// agent that can ask before it uses a tool (protocol.AgentCapabilities.Approvals) blocks its turn,
// and a person answers. This file is the one shape that answer travels in.
//
// There is one row and many views (N7). The approval a card's chat shows, the same approval in a
// project chat that asked about the same card, and the approval in the Approvals view are all this
// one Approval, keyed by its own id. Answering it once updates every view, because every view
// follows the same two events (approval.requested and approval.resolved) on the card's topic.

// Approval is one permission an agent asked a person for, from the moment it is announced until it
// is answered. State is where it stands: waiting, approved, or denied.
type Approval struct {
	// ID is the approval's own opaque id. It is what POST /v1/approvals/{id} answers.
	ID string `json:"id"`
	// CardID is the card whose session asked for permission. It is empty when a chat's session did.
	CardID string `json:"cardId"`
	// ChatID is the chat whose session asked for permission. It is left out when a card's did.
	ChatID string `json:"chatId,omitempty"`
	// SessionID is the session's own opaque id, for the audit trail.
	SessionID string `json:"sessionId"`
	// ToolCallID names the tool call the request is about, when the agent says. Empty when it does
	// not.
	ToolCallID string `json:"toolCallId,omitempty"`
	// Title is the one line the request shows, in the agent's own words.
	Title string `json:"title"`
	// Kind is the agent's word for the kind of tool: read, edit, delete, move, search, execute,
	// think, fetch, switch_mode, or other. Empty when the agent gave none.
	Kind string `json:"kind,omitempty"`
	// Path is the file the request is about, when it has one.
	Path string `json:"path,omitempty"`
	// Command is the command line of an execute request, when it has one.
	Command string `json:"command,omitempty"`
	// Options are the answers the agent offered, in the order the agent gave them. Never null.
	Options []ApprovalOption `json:"options"`
	// State is where the request stands.
	State ChatApprovalState `json:"state"`
	// DecidedBy names who answered, and is empty while the request is waiting. It is "person" for an
	// answer given from a screen, or "daemon" for one the daemon gave on a person's behalf (a
	// bypassed mode, or a request withdrawn when the session ended).
	DecidedBy string `json:"decidedBy,omitempty"`
	// At is when the agent asked, in UTC. It is read back from the approval's own id (IDTime), so it
	// survives a restart without a stored column of its own.
	At Timestamp `json:"at"`
}

// ApprovalOption is one answer an agent offered for an approval, on the wire. Kind is one of the
// agents.Option constants: allow_once, allow_always, reject_once, or reject_always.
type ApprovalOption struct {
	// ID is the option's id inside the request, as the agent named it. It is what the agent is
	// given back, so it is passed through unchanged.
	ID string `json:"id"`
	// Name is the option's label, in the agent's own words.
	Name string `json:"name"`
	// Kind groups the options: allow_once and allow_always allow, reject_once and reject_always
	// refuse.
	Kind string `json:"kind"`
}

// ApprovalDecision is the answer a person gives to an Approval. It is the state the request moves
// to, and the two words match ChatApprovalState, so a decided approval needs no translation.
type ApprovalDecision string

const (
	// ApprovalDecisionApproved is an approval a person allowed.
	ApprovalDecisionApproved ApprovalDecision = "approved"
	// ApprovalDecisionDenied is an approval a person refused.
	ApprovalDecisionDenied ApprovalDecision = "denied"
)

// ApprovalDecisionValues lists every decision, in the order of the constants above.
func ApprovalDecisionValues() []ApprovalDecision {
	return []ApprovalDecision{ApprovalDecisionApproved, ApprovalDecisionDenied}
}

// Valid reports whether d is a decision.
func (d ApprovalDecision) Valid() bool { return slices.Contains(ApprovalDecisionValues(), d) }

// State maps a decision to the state the approval moves to.
func (d ApprovalDecision) State() ChatApprovalState {
	if d == ApprovalDecisionApproved {
		return ChatApprovalStateApproved
	}
	return ChatApprovalStateDenied
}

// DecideApprovalRequest is the body of POST /v1/approvals/{id}: the answer to one approval.
type DecideApprovalRequest struct {
	// Decision is approve or deny. It is required.
	Decision ApprovalDecision `json:"decision"`
	// OptionID picks the exact option the agent offered (allow_always rather than allow_once, for
	// example). It is optional: when it is empty the daemon picks the plain option for the decision
	// (allow_once to approve, reject_once to deny), and when it is set the agent must have offered
	// it.
	OptionID string `json:"optionId,omitempty"`
}

// ApprovalRequestedEventData is the payload of approval.requested, on the card's or the chat's
// topic: the whole Approval, so a view that is opened after it (or that missed it) can draw it
// without another call.
type ApprovalRequestedEventData struct {
	// CardID is the card whose session asked. It is empty when a chat's did.
	CardID string `json:"cardId"`
	// ChatID is the chat whose session asked. It is left out when a card's did.
	ChatID string `json:"chatId,omitempty"`
	// Approval is the request, in the waiting state.
	Approval Approval `json:"approval"`
}

// ApprovalResolvedEventData is the payload of approval.resolved: which approval was answered and
// where it now stands, so every view of the same row updates together (N7).
type ApprovalResolvedEventData struct {
	// CardID is the card whose session asked. It is empty when a chat's did.
	CardID string `json:"cardId"`
	// ChatID is the chat whose session asked. It is left out when a card's did.
	ChatID string `json:"chatId,omitempty"`
	// ApprovalID is the approval that was answered.
	ApprovalID string `json:"approvalId"`
	// State is where it now stands: approved or denied.
	State ChatApprovalState `json:"state"`
	// DecidedBy names who answered ("person" or "daemon").
	DecidedBy string `json:"decidedBy,omitempty"`
	// At is when it was answered, in UTC.
	At Timestamp `json:"at"`
}
