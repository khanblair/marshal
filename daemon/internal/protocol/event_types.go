package protocol

import "slices"

// EventType names what happened, in the form <thing>.<what happened>. The list is from
// docs/architecture.md section 11.2. A type that the daemon does not emit yet is here only so the
// name is reserved. Phase 1 emits the project, card, and session types marked below.

// EventType is the Type of an Event.
type EventType string

const (
	// EventTypeProjectCreated is sent when a project is added.
	EventTypeProjectCreated EventType = "project.created"
	// EventTypeProjectUpdated is sent when a project changes.
	EventTypeProjectUpdated EventType = "project.updated"
	// EventTypeProjectRemoved is sent when a project is removed from Marshal.
	EventTypeProjectRemoved EventType = "project.removed"
	// EventTypeChatCreated is sent when a chat is created.
	EventTypeChatCreated EventType = "chat.created"
	// EventTypeChatUpdated is sent when a chat changes.
	EventTypeChatUpdated EventType = "chat.updated"
	// EventTypeChatArchived is sent when a chat is archived.
	EventTypeChatArchived EventType = "chat.archived"
	// EventTypeChatDeleted is sent when a chat is deleted.
	EventTypeChatDeleted EventType = "chat.deleted"
	// EventTypeActivityCreated is sent when an entry is added to the Home feed.
	EventTypeActivityCreated EventType = "activity.created"
	// EventTypeCardCreated is sent when a card is created.
	EventTypeCardCreated EventType = "card.created"
	// EventTypeCardUpdated is sent when a card changes.
	EventTypeCardUpdated EventType = "card.updated"
	// EventTypeCardMoved is sent when a card changes state or position.
	EventTypeCardMoved EventType = "card.moved"
	// EventTypeCardMembersChanged is sent when the people on a card change.
	EventTypeCardMembersChanged EventType = "card.members_changed"
	// EventTypeChecklistUpdated is sent when a checklist or one of its items changes.
	EventTypeChecklistUpdated EventType = "checklist.updated"
	// EventTypeCommentCreated is sent when a comment is posted.
	EventTypeCommentCreated EventType = "comment.created"
	// EventTypeCommentReadByAgent is sent when an agent reads a comment.
	EventTypeCommentReadByAgent EventType = "comment.read_by_agent"
	// EventTypeSessionStateChanged is sent when a session changes state.
	EventTypeSessionStateChanged EventType = "session.state_changed"
	// EventTypeSessionOutput is sent for output from an agent session.
	EventTypeSessionOutput EventType = "session.output"
	// EventTypeSessionToolCall is sent when an agent calls a tool.
	EventTypeSessionToolCall EventType = "session.tool_call"
	// EventTypeApprovalRequested is sent when an agent asks for permission.
	EventTypeApprovalRequested EventType = "approval.requested"
	// EventTypeApprovalResolved is sent when a permission request is answered.
	EventTypeApprovalResolved EventType = "approval.resolved"
	// EventTypeCIUpdated is sent when a CI run changes.
	EventTypeCIUpdated EventType = "ci.updated"
	// EventTypeQualityChecked is sent when the quality checks finish for a card.
	EventTypeQualityChecked EventType = "quality.checked"
	// EventTypeMergeProgress is sent as a merge moves along.
	EventTypeMergeProgress EventType = "merge.progress"
	// EventTypeNoticeCreated is sent when a notice is added.
	EventTypeNoticeCreated EventType = "notice.created"
	// EventTypeUsageUpdated is sent when token use or cost changes.
	EventTypeUsageUpdated EventType = "usage.updated"
	// EventTypeBudgetWarning is sent when a cost or awake limit is close.
	EventTypeBudgetWarning EventType = "budget.warning"
)

// EventTypeValues lists every event type, in the order of docs/architecture.md section 11.2.
func EventTypeValues() []EventType {
	return []EventType{
		EventTypeProjectCreated, EventTypeProjectUpdated, EventTypeProjectRemoved,
		EventTypeChatCreated, EventTypeChatUpdated, EventTypeChatArchived, EventTypeChatDeleted,
		EventTypeActivityCreated,
		EventTypeCardCreated, EventTypeCardUpdated, EventTypeCardMoved, EventTypeCardMembersChanged,
		EventTypeChecklistUpdated, EventTypeCommentCreated, EventTypeCommentReadByAgent,
		EventTypeSessionStateChanged, EventTypeSessionOutput, EventTypeSessionToolCall,
		EventTypeApprovalRequested, EventTypeApprovalResolved, EventTypeCIUpdated,
		EventTypeQualityChecked, EventTypeMergeProgress, EventTypeNoticeCreated,
		EventTypeUsageUpdated, EventTypeBudgetWarning,
	}
}

// Valid reports whether t is an event type.
func (t EventType) Valid() bool { return slices.Contains(EventTypeValues(), t) }
