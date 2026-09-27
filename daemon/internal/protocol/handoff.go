package protocol

import "slices"

// Handing a card's work off to a different agent (docs/marshal-product-scope.md section 10.5,
// build-plan 7.5).
//
// A card's session row is one per card for its whole life, and the agent that runs it is a column of
// that row (0003_sessions.sql). Handing off therefore does not open a second session: it stops the
// process the card has, rewrites which agent the row runs, and starts that agent fresh in the same
// worktree. The card's own history is untouched - it belongs to the card, not to the agent - so the
// conversation the person sees is continuous even though the agent behind it changed.
//
// A resume cannot do this: resuming continues the *same* agent's own session id (restore.go
// resumeRow), and the daemon never replays the stored history to an agent. So what makes a handoff
// "clean" is a summary: a short account of where the work got to, carried to the new agent as the
// context its first session starts with, exactly the way a fresh card's context reaches its agent.

// MaxHandoffSummaryChars is the most characters a handoff's summary may have. The daemon refuses a
// longer one with a plain sentence that names this limit, so the composer can show the same number.
const MaxHandoffSummaryChars = 20000

// HandoffRequest is the body of POST /v1/cards/{id}/handoff: continue this card's work on a
// different agent, from a summary of where it got to.
type HandoffRequest struct {
	// To is the agent the card continues on. It must be a kind Marshal knows, and it must differ
	// from the agent the card's session is running now: handing a card to the agent it already
	// runs is a restart, which is what Stop and Start are for, and is refused.
	To AgentKind `json:"to"`
	// Summary is what the new agent is told about the work so far: the goal, what was done, what is
	// left, the decisions made, and anything still open (products scope 10.5). It is the new
	// session's starting context, so it is read once, by the new agent's first turn, and never
	// again. It may be empty, and then the new agent starts with the card's own context alone.
	Summary string `json:"summary,omitempty"`
}

// HandoffRefusalReason is why a handoff was refused. It is the stable token a client switches on;
// the sentence beside it is what a person reads.
type HandoffRefusalReason string

const (
	// HandoffRefusalReasonNoSession is a handoff for a card that has no session: it was never
	// started, so there is no work to continue and no agent to hand from.
	HandoffRefusalReasonNoSession HandoffRefusalReason = "handoff_no_session"
	// HandoffRefusalReasonTurnRunning is a handoff while the agent is in the middle of a turn. The
	// process would end mid-turn, so the handoff is refused until the turn finishes.
	HandoffRefusalReasonTurnRunning HandoffRefusalReason = "handoff_turn_running"
	// HandoffRefusalReasonHoldingMessages is a handoff for a paused card that is holding a message.
	// The waiting messages live in memory with the process, so the card is resumed first.
	HandoffRefusalReasonHoldingMessages HandoffRefusalReason = "handoff_holding_messages"
	// HandoffRefusalReasonSameAgent is a handoff to the agent the card already runs.
	HandoffRefusalReasonSameAgent HandoffRefusalReason = "handoff_same_agent"
	// HandoffRefusalReasonUnknownAgent is a handoff to an agent kind Marshal has no adapter for.
	HandoffRefusalReasonUnknownAgent HandoffRefusalReason = "handoff_unknown_agent"
	// HandoffRefusalReasonCannotStart is a handoff whose new agent could not be started after the
	// old process had stopped. The card moves to Needs you, by the rule of section 5.3.
	HandoffRefusalReasonCannotStart HandoffRefusalReason = "handoff_cannot_start"
)

// HandoffRefusalReasonValues lists every reason a handoff can be refused, in the order the session
// manager checks them.
func HandoffRefusalReasonValues() []HandoffRefusalReason {
	return []HandoffRefusalReason{
		HandoffRefusalReasonNoSession, HandoffRefusalReasonTurnRunning,
		HandoffRefusalReasonHoldingMessages, HandoffRefusalReasonSameAgent,
		HandoffRefusalReasonUnknownAgent, HandoffRefusalReasonCannotStart,
	}
}

// Valid reports whether r is a handoff refusal reason.
func (r HandoffRefusalReason) Valid() bool { return slices.Contains(HandoffRefusalReasonValues(), r) }
