package protocol

// The merge flow a person watches: where finished cards are headed, what the Integrator is doing, and
// what it has delivered. One read model serves the Integration view and the board header.

// MergePhase is where a card in the merge queue is.
type MergePhase string

const (
	// MergePhaseQueued means the card waits for the Integrator to take it.
	MergePhaseQueued MergePhase = "queued"
	// MergePhaseResolving means the Integrator is resolving conflicts.
	MergePhaseResolving MergePhase = "resolving"
	// MergePhaseTesting means the merged result is being tested.
	MergePhaseTesting MergePhase = "testing"
	// MergePhaseLanding means the tested result is being delivered to the branch and the folder.
	MergePhaseLanding MergePhase = "landing"
	// MergePhaseStopped means the merge stopped and needs the owner; the card is in Needs you and
	// can be retried.
	MergePhaseStopped MergePhase = "stopped"
)

// MergePhaseValues lists every merge phase, in the order a merge passes through them, then stopped.
func MergePhaseValues() []MergePhase {
	return []MergePhase{
		MergePhaseQueued, MergePhaseResolving, MergePhaseTesting, MergePhaseLanding, MergePhaseStopped,
	}
}

// IntegratorState is what a project's Integrator is doing.
type IntegratorState string

const (
	// IntegratorStateIdle means nothing is waiting to be merged.
	IntegratorStateIdle IntegratorState = "idle"
	// IntegratorStateMerging means a card is being merged.
	IntegratorStateMerging IntegratorState = "merging"
	// IntegratorStateWaiting means the Integrator stopped and needs the owner.
	IntegratorStateWaiting IntegratorState = "waiting"
	// IntegratorStatePaused means the owner paused merging.
	IntegratorStatePaused IntegratorState = "paused"
)

// IntegratorStateValues lists every Integrator state.
func IntegratorStateValues() []IntegratorState {
	return []IntegratorState{
		IntegratorStateIdle, IntegratorStateMerging, IntegratorStateWaiting, IntegratorStatePaused,
	}
}

// IntegrationBranchName is the one branch the Integrator works on, in its own workspace.
const IntegrationBranchName = "integrator"

// IntegrationQueueItem is one card waiting for, or in, a merge.
type IntegrationQueueItem struct {
	// CardID is the card's opaque id.
	CardID string `json:"cardId"`
	// Key is the card's project-and-number key, such as "web#12".
	Key string `json:"key"`
	// Title is the card's title.
	Title string `json:"title"`
	// Phase is where the card is in the merge.
	Phase MergePhase `json:"phase"`
	// Position counts from 1; the card being merged is 1.
	Position int `json:"position"`
}

// IntegrationHistoryItem is one card the Integrator delivered.
type IntegrationHistoryItem struct {
	CardID string `json:"cardId"`
	Key    string `json:"key"`
	Title  string `json:"title"`
	// MergedAt is when the work landed.
	MergedAt Timestamp `json:"mergedAt"`
	// Commit is the commit that landed.
	Commit string `json:"commit"`
	// Resolved counts the conflicts the Integrator resolved, 0 for a clean merge.
	Resolved int `json:"resolved"`
	// Summary is the Integrator's one-paragraph report.
	Summary string `json:"summary"`
	// CanUndo is true while the branch tip is still this merge and the folder is clean.
	CanUndo bool `json:"canUndo"`
}

// IntegrationState is the answer to GET /v1/projects/{id}/integration.
type IntegrationState struct {
	ProjectID string `json:"projectId"`
	// Target is the integration branch finished cards land on, such as "development".
	Target string `json:"target"`
	// IntegratorBranch is the Integrator's own branch, always IntegrationBranchName.
	IntegratorBranch string `json:"integratorBranch"`
	// AheadBy counts the commits the Integrator's branch has that Target does not yet.
	AheadBy int `json:"aheadBy"`
	// State is what the Integrator is doing.
	State IntegratorState `json:"state"`
	// CurrentCardID is the card being merged, empty when none is.
	CurrentCardID string `json:"currentCardId,omitempty"`
	// Queue lists the cards waiting or being merged, in order. Never null.
	Queue []IntegrationQueueItem `json:"queue"`
	// History lists the most recent deliveries, newest first. Never null.
	History []IntegrationHistoryItem `json:"history"`
	// Message is one plain sentence when State is waiting, such as why it stopped.
	Message string `json:"message,omitempty"`
	// ServerTime is the daemon's time when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}

// MergeProgressEvent is the data of a merge.progress event: a card's merge moved to a new phase. It
// is sent on the project's topic and the card's topic.
type MergeProgressEvent struct {
	ProjectID string `json:"projectId"`
	CardID    string `json:"cardId"`
	// Phase is the new phase, empty when the merge ended.
	Phase MergePhase `json:"phase,omitempty"`
	// Note is one plain sentence for the card's activity line.
	Note string `json:"note,omitempty"`
}

// OpenWorktreeRequest is the body of POST /v1/cards/{id}/worktree/open.
type OpenWorktreeRequest struct {
	// With is "finder" to reveal the folder, or "editor" to open it in the machine's editor.
	With string `json:"with"`
}
