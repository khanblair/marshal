package protocol

import "slices"

// The fixed lists of allowed values. Each list is one Go type with one const block. tygo turns
// each into a TypeScript union, and scripts/gen-protocol.mjs adds a readonly array named
// <Type>Values, so the app and the daemon cannot disagree. Go code uses <Type>Values() and
// Valid(). Keep each const block in the same order as its Values function; a test checks it.
// The words shown to people (labels) belong to the app, not here.

// CardState is where a card is in its life, in board order.
type CardState string

const (
	// CardStateBacklog is a card that has not been started.
	CardStateBacklog CardState = "backlog"
	// CardStatePlanning is a card whose agent is writing a plan.
	CardStatePlanning CardState = "planning"
	// CardStateWorking is a card whose agent is working.
	CardStateWorking CardState = "working"
	// CardStateNeeds is a card that waits for a person ("Needs you" in the app).
	CardStateNeeds CardState = "needs"
	// CardStateReview is a card whose work is being reviewed.
	CardStateReview CardState = "review"
	// CardStateReady is a card that is ready to merge.
	CardStateReady CardState = "ready"
	// CardStateMerging is a card that is being merged. It shows in the Ready column.
	CardStateMerging CardState = "merging"
	// CardStateDone is a card that is merged or closed.
	CardStateDone CardState = "done"
)

// CardStateValues lists every card state in board order.
func CardStateValues() []CardState {
	return []CardState{
		CardStateBacklog, CardStatePlanning, CardStateWorking, CardStateNeeds,
		CardStateReview, CardStateReady, CardStateMerging, CardStateDone,
	}
}

// Valid reports whether s is a card state.
func (s CardState) Valid() bool { return slices.Contains(CardStateValues(), s) }

// PermissionMode says how much an agent may do without asking.
type PermissionMode string

const (
	// PermissionModeAsk asks before every edit and command ("Ask" in the app).
	PermissionModeAsk PermissionMode = "ask"
	// PermissionModeAutoEdits allows file edits and asks for commands ("Auto-accept edits").
	PermissionModeAutoEdits PermissionMode = "auto-edits"
	// PermissionModePlan only plans and changes nothing ("Plan only").
	PermissionModePlan PermissionMode = "plan"
	// PermissionModeFullAuto allows what the rules allow without asking ("Full auto").
	PermissionModeFullAuto PermissionMode = "full-auto"
	// PermissionModeBypass skips permission checks inside the card's worktree ("Bypass permissions").
	PermissionModeBypass PermissionMode = "bypass"
)

// PermissionModeValues lists every permission mode, from the most careful to the least.
func PermissionModeValues() []PermissionMode {
	return []PermissionMode{
		PermissionModeAsk, PermissionModeAutoEdits, PermissionModePlan,
		PermissionModeFullAuto, PermissionModeBypass,
	}
}

// Valid reports whether m is a permission mode.
func (m PermissionMode) Valid() bool { return slices.Contains(PermissionModeValues(), m) }

// ThinkingMode is how much an agent thinks before it answers.
type ThinkingMode string

const (
	// ThinkingModeLow thinks a little ("Low" in the app).
	ThinkingModeLow ThinkingMode = "low"
	// ThinkingModeMedium is the middle setting ("Medium").
	ThinkingModeMedium ThinkingMode = "medium"
	// ThinkingModeHigh thinks a lot ("High").
	ThinkingModeHigh ThinkingMode = "high"
	// ThinkingModeExtraHigh thinks the most ("Extra high").
	ThinkingModeExtraHigh ThinkingMode = "extra-high"
)

// ThinkingModeValues lists every thinking mode, from the least to the most.
func ThinkingModeValues() []ThinkingMode {
	return []ThinkingMode{ThinkingModeLow, ThinkingModeMedium, ThinkingModeHigh, ThinkingModeExtraHigh}
}

// Valid reports whether m is a thinking mode.
func (m ThinkingMode) Valid() bool { return slices.Contains(ThinkingModeValues(), m) }

// AgentKind is which agent program does the work.
type AgentKind string

const (
	// AgentKindClaude is Claude Code.
	AgentKindClaude AgentKind = "claude"
	// AgentKindGemini is Gemini CLI.
	AgentKindGemini AgentKind = "gemini"
	// AgentKindCodex is Codex.
	AgentKindCodex AgentKind = "codex"
	// AgentKindBuiltin is Marshal's built-in agent.
	AgentKindBuiltin AgentKind = "builtin"
)

// AgentKindValues lists every agent kind.
func AgentKindValues() []AgentKind {
	return []AgentKind{AgentKindClaude, AgentKindGemini, AgentKindCodex, AgentKindBuiltin}
}

// Valid reports whether k is an agent kind.
func (k AgentKind) Valid() bool { return slices.Contains(AgentKindValues(), k) }

// AgentStatus says whether an agent can be used on this machine.
type AgentStatus string

const (
	// AgentStatusSupported is an installed agent at a version Marshal has been tested with.
	AgentStatusSupported AgentStatus = "supported"
	// AgentStatusUntested is an installed agent at a version Marshal has not been tested with.
	AgentStatusUntested AgentStatus = "untested"
	// AgentStatusMissing is an agent that is not installed.
	AgentStatusMissing AgentStatus = "missing"
)

// AgentStatusValues lists every agent status.
func AgentStatusValues() []AgentStatus {
	return []AgentStatus{AgentStatusSupported, AgentStatusUntested, AgentStatusMissing}
}

// Valid reports whether s is an agent status.
func (s AgentStatus) Valid() bool { return slices.Contains(AgentStatusValues(), s) }

// SessionState is where an agent session is in its life (docs/architecture.md section 5.1).
type SessionState string

const (
	// SessionStateStarting is a session that is being started.
	SessionStateStarting SessionState = "starting"
	// SessionStateAwake is a running session that is waiting for a message.
	SessionStateAwake SessionState = "awake"
	// SessionStateWorking is a session that is busy with a turn.
	SessionStateWorking SessionState = "working"
	// SessionStateWaitingApproval is a session that asked for permission and waits for a person.
	SessionStateWaitingApproval SessionState = "waiting-approval"
	// SessionStateSleepWarning is an idle session that will sleep soon.
	SessionStateSleepWarning SessionState = "sleep-warning"
	// SessionStateAsleep is a stopped process that keeps its session id for waking.
	SessionStateAsleep SessionState = "asleep"
	// SessionStateWaking is a session that is being resumed.
	SessionStateWaking SessionState = "waking"
	// SessionStateStopped is a session that ended because its card closed.
	SessionStateStopped SessionState = "stopped"
)

// SessionStateValues lists every session state in life-cycle order.
func SessionStateValues() []SessionState {
	return []SessionState{
		SessionStateStarting, SessionStateAwake, SessionStateWorking, SessionStateWaitingApproval,
		SessionStateSleepWarning, SessionStateAsleep, SessionStateWaking, SessionStateStopped,
	}
}

// Valid reports whether s is a session state.
func (s SessionState) Valid() bool { return slices.Contains(SessionStateValues(), s) }

// FeedKind is the kind of an entry in the Home activity feed.
type FeedKind string

const (
	// FeedKindBrief is a morning or evening brief.
	FeedKindBrief FeedKind = "brief"
	// FeedKindMerge is a merged card.
	FeedKindMerge FeedKind = "merge"
	// FeedKindSchedule is a scheduled job that ran.
	FeedKindSchedule FeedKind = "schedule"
	// FeedKindApproval is an approval that was asked for or answered.
	FeedKindApproval FeedKind = "approval"
	// FeedKindPlan is a plan that is ready for review.
	FeedKindPlan FeedKind = "plan"
	// FeedKindCI is a CI result.
	FeedKindCI FeedKind = "ci"
	// FeedKindTool is any other agent or system event.
	FeedKindTool FeedKind = "tool"
)

// FeedKindValues lists every feed kind.
func FeedKindValues() []FeedKind {
	return []FeedKind{
		FeedKindBrief, FeedKindMerge, FeedKindSchedule, FeedKindApproval,
		FeedKindPlan, FeedKindCI, FeedKindTool,
	}
}

// Valid reports whether k is a feed kind.
func (k FeedKind) Valid() bool { return slices.Contains(FeedKindValues(), k) }

// NoticeKind is the kind of a notice in the notice list.
type NoticeKind string

const (
	// NoticeKindSleep is a group of idle cards that will sleep soon.
	NoticeKindSleep NoticeKind = "sleep"
	// NoticeKindCIMain is a CI failure on a project's main branch.
	NoticeKindCIMain NoticeKind = "ci-main"
	// NoticeKindCost is a cost warning.
	NoticeKindCost NoticeKind = "cost"
	// NoticeKindPlan is a plan that waits for review.
	NoticeKindPlan NoticeKind = "plan"
	// NoticeKindCI is a CI failure on a card.
	NoticeKindCI NoticeKind = "ci"
)

// NoticeKindValues lists every notice kind.
func NoticeKindValues() []NoticeKind {
	return []NoticeKind{NoticeKindSleep, NoticeKindCIMain, NoticeKindCost, NoticeKindPlan, NoticeKindCI}
}

// Valid reports whether k is a notice kind.
func (k NoticeKind) Valid() bool { return slices.Contains(NoticeKindValues(), k) }

// ActivityKind is the kind of an entry on a card's Activity tab.
type ActivityKind string

const (
	// ActivityKindFile is a file that was read, edited, or created.
	ActivityKindFile ActivityKind = "file"
	// ActivityKindCommand is a command that ran.
	ActivityKindCommand ActivityKind = "command"
	// ActivityKindTest is a test or type check that ran.
	ActivityKindTest ActivityKind = "test"
	// ActivityKindTool is any other tool call or system note.
	ActivityKindTool ActivityKind = "tool"
	// ActivityKindApproval is a request for permission.
	ActivityKindApproval ActivityKind = "approval"
)

// ActivityKindValues lists every activity kind.
func ActivityKindValues() []ActivityKind {
	return []ActivityKind{
		ActivityKindFile, ActivityKindCommand, ActivityKindTest, ActivityKindTool, ActivityKindApproval,
	}
}

// Valid reports whether k is an activity kind.
func (k ActivityKind) Valid() bool { return slices.Contains(ActivityKindValues(), k) }

// ActivityState is how an entry of a card's Activity tab ended (docs/backend-inventory.md 4.4,
// N14). It is the same list as the state a stored event carries (internal/history.State).
type ActivityState string

const (
	// ActivityStateOK is work that finished.
	ActivityStateOK ActivityState = "ok"
	// ActivityStateRunning is work that is still going.
	ActivityStateRunning ActivityState = "running"
	// ActivityStateFailed is work that ended badly.
	ActivityStateFailed ActivityState = "failed"
	// ActivityStateWaiting is work blocked on a person, such as a permission request.
	ActivityStateWaiting ActivityState = "waiting"
)

// ActivityStateValues lists every activity state.
func ActivityStateValues() []ActivityState {
	return []ActivityState{
		ActivityStateOK, ActivityStateRunning, ActivityStateFailed, ActivityStateWaiting,
	}
}

// Valid reports whether s is an activity state.
func (s ActivityState) Valid() bool { return slices.Contains(ActivityStateValues(), s) }

// ChatMessageKind is what one item of a card's chat is (docs/backend-inventory.md 4.3, N13). The
// words are the ones the app's chat already draws, so one mapper turns a wire message into a
// screen block.
type ChatMessageKind string

const (
	// ChatMessageKindUser is a message a person sent.
	ChatMessageKindUser ChatMessageKind = "user"
	// ChatMessageKindAgent is a piece of the agent's answer, as it was streamed and stored.
	ChatMessageKindAgent ChatMessageKind = "agent"
	// ChatMessageKindTool is a tool call with its state, and a detail that opens on demand.
	ChatMessageKindTool ChatMessageKind = "tool"
	// ChatMessageKindSystem is a note the daemon wrote about the session itself.
	ChatMessageKindSystem ChatMessageKind = "system"
	// ChatMessageKindDiff is the summary of the files one turn changed.
	ChatMessageKindDiff ChatMessageKind = "diff"
	// ChatMessageKindPlan is a plan the agent wrote in plan-first mode. It is defined now and
	// nothing writes it yet: the plan side of the chat is Phase 5 (B5.2).
	ChatMessageKindPlan ChatMessageKind = "plan"
	// ChatMessageKindApproval is a permission the agent asked for. It is defined now and nothing
	// writes it yet: approvals are Phase 3 (B3.4).
	ChatMessageKindApproval ChatMessageKind = "approval"
	// ChatMessageKindCard is a card an agent or the Orchestrator made or named.
	ChatMessageKindCard ChatMessageKind = "card"
)

// ChatMessageKindValues lists every chat message kind, in the order of the constants above.
func ChatMessageKindValues() []ChatMessageKind {
	return []ChatMessageKind{
		ChatMessageKindUser, ChatMessageKindAgent, ChatMessageKindTool, ChatMessageKindSystem,
		ChatMessageKindDiff, ChatMessageKindPlan, ChatMessageKindApproval, ChatMessageKindCard,
	}
}

// Valid reports whether k is a chat message kind.
func (k ChatMessageKind) Valid() bool { return slices.Contains(ChatMessageKindValues(), k) }

// ChatPlanState is where a plan block stands (docs/backend-inventory.md 4.3). Like the plan kind
// itself, it is defined now and written in Phase 5.
type ChatPlanState string

const (
	// ChatPlanStateWaiting is a plan the agent waits for an answer on.
	ChatPlanStateWaiting ChatPlanState = "waiting"
	// ChatPlanStateApproved is a plan a person approved.
	ChatPlanStateApproved ChatPlanState = "approved"
	// ChatPlanStateRejected is a plan a person rejected.
	ChatPlanStateRejected ChatPlanState = "rejected"
	// ChatPlanStateEdited is a plan a person changed.
	ChatPlanStateEdited ChatPlanState = "edited"
)

// ChatPlanStateValues lists every plan state, in the order of the constants above.
func ChatPlanStateValues() []ChatPlanState {
	return []ChatPlanState{
		ChatPlanStateWaiting, ChatPlanStateApproved, ChatPlanStateRejected, ChatPlanStateEdited,
	}
}

// Valid reports whether s is a plan state.
func (s ChatPlanState) Valid() bool { return slices.Contains(ChatPlanStateValues(), s) }

// ChatApprovalState is where an approval block stands (docs/backend-inventory.md 4.3). Like the
// approval kind itself, it is defined now and written in Phase 3.
type ChatApprovalState string

const (
	// ChatApprovalStateWaiting is an approval nobody has answered yet.
	ChatApprovalStateWaiting ChatApprovalState = "waiting"
	// ChatApprovalStateApproved is an approval a person allowed.
	ChatApprovalStateApproved ChatApprovalState = "approved"
	// ChatApprovalStateDenied is an approval a person refused.
	ChatApprovalStateDenied ChatApprovalState = "denied"
)

// ChatApprovalStateValues lists every approval state, in the order of the constants above.
func ChatApprovalStateValues() []ChatApprovalState {
	return []ChatApprovalState{
		ChatApprovalStateWaiting, ChatApprovalStateApproved, ChatApprovalStateDenied,
	}
}

// Valid reports whether s is an approval state.
func (s ChatApprovalState) Valid() bool { return slices.Contains(ChatApprovalStateValues(), s) }

// ChatTargetKind is who a project chat talks to (docs/architecture.md 16.2, inventory N13). The
// app's own chat picks a target in one list and writes it as one string: the Orchestrator, a role
// name, or a card key such as `web#118`. The wire splits that into the kind below and the id, so a
// client never has to guess which of the three a string means.
type ChatTargetKind string

const (
	// ChatTargetKindOrchestrator is the project's Orchestrator. Its target id is empty.
	ChatTargetKindOrchestrator ChatTargetKind = "orchestrator"
	// ChatTargetKindRole is a role of the project, by its name, such as "Worker".
	ChatTargetKindRole ChatTargetKind = "role"
	// ChatTargetKindCard is one card's own agent, by the card's opaque id.
	ChatTargetKindCard ChatTargetKind = "card"
)

// ChatTargetKindValues lists every chat target kind, in the order of the constants above.
func ChatTargetKindValues() []ChatTargetKind {
	return []ChatTargetKind{ChatTargetKindOrchestrator, ChatTargetKindRole, ChatTargetKindCard}
}

// Valid reports whether k is a chat target kind.
func (k ChatTargetKind) Valid() bool { return slices.Contains(ChatTargetKindValues(), k) }

// CIState is the state of a CI run.
type CIState string

const (
	// CIStateQueued is a run that has not started.
	CIStateQueued CIState = "queued"
	// CIStateRunning is a run in progress.
	CIStateRunning CIState = "running"
	// CIStatePassed is a run that succeeded.
	CIStatePassed CIState = "passed"
	// CIStateFailed is a run that failed.
	CIStateFailed CIState = "failed"
	// CIStateCancelled is a run that was cancelled.
	CIStateCancelled CIState = "cancelled"
)

// CIStateValues lists every CI state.
func CIStateValues() []CIState {
	return []CIState{CIStateQueued, CIStateRunning, CIStatePassed, CIStateFailed, CIStateCancelled}
}

// Valid reports whether s is a CI state.
func (s CIState) Valid() bool { return slices.Contains(CIStateValues(), s) }

// CardViewMode is how a card's agent is shown.
type CardViewMode string

const (
	// CardViewModeChat shows the conversation.
	CardViewModeChat CardViewMode = "chat"
	// CardViewModeTerminal shows the agent's own terminal.
	CardViewModeTerminal CardViewMode = "terminal"
)

// CardViewModeValues lists every card view mode.
func CardViewModeValues() []CardViewMode {
	return []CardViewMode{CardViewModeChat, CardViewModeTerminal}
}

// Valid reports whether m is a card view mode.
func (m CardViewMode) Valid() bool { return slices.Contains(CardViewModeValues(), m) }

// NeedsReasonKind says why a card is waiting on a person. The words people read belong to the app;
// these are the wire values (architecture.md section 6, "Needs you").
type NeedsReasonKind string

const (
	// NeedsReasonKindPlanReady is a card whose plan is ready to review.
	NeedsReasonKindPlanReady NeedsReasonKind = "plan-ready"
	// NeedsReasonKindApprovalNeeded is a card whose agent asked for permission.
	NeedsReasonKindApprovalNeeded NeedsReasonKind = "approval-needed"
	// NeedsReasonKindStuck is a card whose agent is stuck.
	NeedsReasonKindStuck NeedsReasonKind = "stuck"
	// NeedsReasonKindLimit is a card that hit a cost, time, or round limit.
	NeedsReasonKindLimit NeedsReasonKind = "limit"
	// NeedsReasonKindCIFailed is a card whose CI failed.
	NeedsReasonKindCIFailed NeedsReasonKind = "ci-failed"
	// NeedsReasonKindConflict is a card whose merge has a conflict.
	NeedsReasonKindConflict NeedsReasonKind = "conflict"
	// NeedsReasonKindQuestion is a card whose agent asked a question.
	NeedsReasonKindQuestion NeedsReasonKind = "question"
)

// NeedsReasonKindValues lists every reason a card can wait on a person.
func NeedsReasonKindValues() []NeedsReasonKind {
	return []NeedsReasonKind{
		NeedsReasonKindPlanReady, NeedsReasonKindApprovalNeeded, NeedsReasonKindStuck,
		NeedsReasonKindLimit, NeedsReasonKindCIFailed, NeedsReasonKindConflict, NeedsReasonKindQuestion,
	}
}

// Valid reports whether k is a needs-you reason kind.
func (k NeedsReasonKind) Valid() bool { return slices.Contains(NeedsReasonKindValues(), k) }

// LabelColor is the fixed set of colors a label may have (decision D3). The values are token
// names, not color values: the app turns them into the token of the same name, so a label cannot
// introduce a color the design does not have.
type LabelColor string

const (
	// LabelColorSlate is the neutral color.
	LabelColorSlate LabelColor = "slate"
	// LabelColorBlue is blue.
	LabelColorBlue LabelColor = "blue"
	// LabelColorGreen is green.
	LabelColorGreen LabelColor = "green"
	// LabelColorAmber is amber.
	LabelColorAmber LabelColor = "amber"
	// LabelColorRed is red.
	LabelColorRed LabelColor = "red"
	// LabelColorPurple is purple.
	LabelColorPurple LabelColor = "purple"
)

// LabelColorValues lists every color a label may have.
func LabelColorValues() []LabelColor {
	return []LabelColor{
		LabelColorSlate, LabelColorBlue, LabelColorGreen,
		LabelColorAmber, LabelColorRed, LabelColorPurple,
	}
}

// Valid reports whether c is a label color.
func (c LabelColor) Valid() bool { return slices.Contains(LabelColorValues(), c) }

// MoveRefusalReason is why a manual move was refused (architecture.md section 6.1). The sentence a
// person reads comes with it; the reason is stable so a client can act on it.
type MoveRefusalReason string

const (
	// MoveRefusalReasonFromDone is a move of a card that is already done.
	MoveRefusalReasonFromDone MoveRefusalReason = "move_from_done"
	// MoveRefusalReasonToDone is a move to done by hand.
	MoveRefusalReasonToDone MoveRefusalReason = "move_to_done"
	// MoveRefusalReasonToNeeds is a move to needs by hand.
	MoveRefusalReasonToNeeds MoveRefusalReason = "move_to_needs"
	// MoveRefusalReasonNeedsPullRequest is a move to review with no pull request, or from backlog
	// or planning.
	MoveRefusalReasonNeedsPullRequest MoveRefusalReason = "move_needs_pull_request"
	// MoveRefusalReasonNeedsReview is a move to ready from anywhere but review.
	MoveRefusalReasonNeedsReview MoveRefusalReason = "move_needs_review"
	// MoveRefusalReasonChecksNotPassed is a move to ready whose checks have not passed.
	MoveRefusalReasonChecksNotPassed MoveRefusalReason = "move_checks_not_passed"
	// MoveRefusalReasonCardMerging is a move of a card that is being merged.
	MoveRefusalReasonCardMerging MoveRefusalReason = "move_card_merging"
)

// MoveRefusalReasonValues lists every reason a move can be refused, in the order
// docs/architecture.md section 6.1 checks them.
func MoveRefusalReasonValues() []MoveRefusalReason {
	return []MoveRefusalReason{
		MoveRefusalReasonFromDone, MoveRefusalReasonToDone, MoveRefusalReasonToNeeds,
		MoveRefusalReasonNeedsPullRequest, MoveRefusalReasonNeedsReview,
		MoveRefusalReasonChecksNotPassed, MoveRefusalReasonCardMerging,
	}
}

// Valid reports whether r is a move refusal reason.
func (r MoveRefusalReason) Valid() bool { return slices.Contains(MoveRefusalReasonValues(), r) }

// HoldRefusalReason is why a pause or a sleep of a card was refused (architecture.md section 5.1,
// the four controls a person presses). The sentence a person reads comes with it; the reason is
// stable so a client can act on it, the way MoveRefusalReason is for a refused drag.
type HoldRefusalReason string

const (
	// HoldRefusalReasonPauseNotWorking is a pause of a card that is not working.
	HoldRefusalReasonPauseNotWorking HoldRefusalReason = "pause_not_working"
	// HoldRefusalReasonSleepWorking is a sleep of a working card that nobody paused.
	HoldRefusalReasonSleepWorking HoldRefusalReason = "sleep_working"
	// HoldRefusalReasonSleepNeedsYou is a sleep of a card that is waiting on a person.
	HoldRefusalReasonSleepNeedsYou HoldRefusalReason = "sleep_needs_you"
	// HoldRefusalReasonSleepNoSession is a sleep of a card with no awake session to put to sleep.
	HoldRefusalReasonSleepNoSession HoldRefusalReason = "sleep_no_session"
	// HoldRefusalReasonSleepHoldingMessages is a sleep of a paused card whose pause is still
	// holding a message. Sleeping it would stop the process the message is waiting for.
	HoldRefusalReasonSleepHoldingMessages HoldRefusalReason = "sleep_holding_messages"
)

// HoldRefusalReasonValues lists every reason a pause or a sleep can be refused, in the order
// architecture.md section 5.1 checks them.
func HoldRefusalReasonValues() []HoldRefusalReason {
	return []HoldRefusalReason{
		HoldRefusalReasonPauseNotWorking,
		HoldRefusalReasonSleepWorking,
		HoldRefusalReasonSleepNeedsYou,
		HoldRefusalReasonSleepNoSession,
		HoldRefusalReasonSleepHoldingMessages,
	}
}

// Valid reports whether r is a hold refusal reason.
func (r HoldRefusalReason) Valid() bool { return slices.Contains(HoldRefusalReasonValues(), r) }

// ChatRefusalReason is why a message to a project chat was refused (docs/backend-checklist.md
// B2.10). The sentence a person reads comes with it; the reason is stable so a client can act on
// it, the way HoldRefusalReason is for a refused pause.
type ChatRefusalReason string

const (
	// ChatRefusalReasonArchived is a message to a chat that is archived. Restoring the chat lets it
	// talk again.
	ChatRefusalReasonArchived ChatRefusalReason = "chat_archived"
	// ChatRefusalReasonCannotResume is a message to a chat whose earlier conversation the agent can
	// no longer pick up. Marshal never starts a new conversation in its place without saying so
	// (docs/architecture.md section 5.3).
	ChatRefusalReasonCannotResume ChatRefusalReason = "chat_cannot_resume"
)

// ChatRefusalReasonValues lists every reason a message to a chat can be refused.
func ChatRefusalReasonValues() []ChatRefusalReason {
	return []ChatRefusalReason{ChatRefusalReasonArchived, ChatRefusalReasonCannotResume}
}

// Valid reports whether r is a chat refusal reason.
func (r ChatRefusalReason) Valid() bool { return slices.Contains(ChatRefusalReasonValues(), r) }
