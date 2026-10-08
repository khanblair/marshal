package integrator

import (
	"context"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// This file is the seam between the workstreams that build the Integrator flow. It holds types and
// interfaces only: each is implemented by one workstream and consumed by another, so the four can be
// built at once and fitted together afterwards.
//
//	W1 (branch and delivery)  implements Workspace and Reader, calls ConflictResolver.
//	W2 (the agent)            implements ConflictResolver and MergeTools, calls ChatSession.
//	W3 (chats)                implements ChatSession, calls Workspace and MergeTools.
//	W4 (the Integration view) consumes Reader.

// MergeTaskKind says what the resolver is asked to merge.
type MergeTaskKind string

const (
	// MergeTaskCard merges a card's branch into the integrator branch.
	MergeTaskCard MergeTaskKind = "card"
	// MergeTaskWIP reconciles the owner's uncommitted changes with what was just merged.
	MergeTaskWIP MergeTaskKind = "wip"
)

// CardContext is what the resolver knows about one card whose work is involved.
type CardContext struct {
	CardID  string
	Key     string
	Title   string
	Body    string
	Plan    string
	Handoff string
	Branch  string
	Changed []string
}

// MergeTask is one conflict the Integrator agent is asked to resolve, in the integrator workspace.
type MergeTask struct {
	// ID is unique per task; the agent's merge_report names it.
	ID        string
	ProjectID string
	Kind      MergeTaskKind
	// Worktree is the integrator workspace, where the conflicted merge is waiting.
	Worktree string
	// Target is the integration branch, such as "development".
	Target    string
	Conflicts []string
	// Cards is every card whose work is involved, the one being merged first.
	Cards []CardContext
	// Instruction is anything the owner told the Integrator chat about this merge.
	Instruction string
}

// Verdict is the Integrator agent's answer to a MergeTask.
type Verdict struct {
	// Resolved is true when every conflict is resolved in the worktree and staged.
	Resolved bool
	// Confident is false when the agent wants the owner to look, even though it resolved.
	Confident bool
	// Summary is the report a person reads: what clashed and how it was resolved.
	Summary string
	// Files are the files it changed to resolve.
	Files []string
	// Questions are what it needs the owner to answer, when it cannot decide.
	Questions []string
}

// ConflictResolver resolves a conflicted merge by intent. A nil resolver means a conflict sends the
// card to Needs you, as the queue has always done.
type ConflictResolver interface {
	Resolve(ctx context.Context, task MergeTask) (Verdict, error)
}

// ChatSession is the pinned Integrator chat's live agent session, as the resolver drives it.
type ChatSession interface {
	// Send puts a message into the project's Integrator chat, starting or waking its session.
	Send(ctx context.Context, projectID, text string) error
	// Reset ends the Integrator session and starts it fresh on the next Send, so it cannot drift.
	Reset(ctx context.Context, projectID string) error
}

// MergeTools is what the Integrator agent's internal tools call: merge_context, merge_report, and
// ask_owner. The chats workstream registers the tools; the agent workstream implements them.
type MergeTools interface {
	// Context answers the task the agent was sent, whole.
	Context(ctx context.Context, taskID string) (MergeTask, error)
	// Report records the agent's verdict, releasing the resolver that waits on the task.
	Report(ctx context.Context, taskID string, verdict Verdict) error
	// Ask posts a question to the owner in the Integrator chat and in the card's Needs you.
	Ask(ctx context.Context, taskID, question string) error
}

// Workspace is the Integrator's own workspace: a worktree on the branch named by
// protocol.IntegrationBranchName, outside the owner's folder.
type Workspace interface {
	// Ensure makes the workspace if it is missing and answers its folder.
	Ensure(ctx context.Context, projectID string) (string, error)
}

// Reader is what the Integration view and the merge routes need from the merge queue.
type Reader interface {
	State(ctx context.Context, projectID string) (protocol.IntegrationState, error)
	Pause(ctx context.Context, projectID string) (protocol.IntegrationState, error)
	Resume(ctx context.Context, projectID string) (protocol.IntegrationState, error)
	// Retry runs a card's merge or delivery again, after a stop that needed the owner.
	Retry(ctx context.Context, cardID string) (protocol.Card, error)
	// Undo puts the integration branch back to where it was before the card's merge.
	Undo(ctx context.Context, cardID string) (protocol.Card, error)
	// SendToMerge moves a card of a project with no GitHub origin to Ready to merge, once its work
	// is committed, and lets the queue take it.
	SendToMerge(ctx context.Context, cardID string) (protocol.Card, error)
}
