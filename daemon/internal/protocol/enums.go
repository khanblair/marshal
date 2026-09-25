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
