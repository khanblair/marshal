package protocol

import "time"

// Checkpoint is one restore point of a card (docs/architecture.md section 10, docs/backend-checklist
// B5.3, build-plan 5.21): a Git commit Marshal made before an agent turn, before a merge, or when a
// person asked, that the card's worktree can be put back to. A card's checkpoints are listed newest
// first, and only the restore route changes anything: the list is a history of moments, not a thing
// that is edited.
//
// There is no checkpoint document to read: a restore point is the commit and the label a person
// reads beside it. The commit itself is kept on a hidden ref, so it survives the branch moving on.
type Checkpoint struct {
	// ID is the checkpoint's own opaque id. The restore route addresses it, and it is what names
	// the hidden ref the commit is kept on.
	ID string `json:"id"`
	// CardID is the card the restore point belongs to.
	CardID string `json:"cardId"`
	// SHA is the commit the checkpoint points at, full length.
	SHA string `json:"sha"`
	// Label says what the checkpoint was made before ("before turn 3", "before merge"). It is
	// empty when there is nothing to say, and the app then names the moment by its time.
	Label string `json:"label"`
	// CreatedAt is when the checkpoint was made.
	CreatedAt Timestamp `json:"createdAt"`
}

// CheckpointList is the answer to GET /v1/cards/{id}/checkpoints: every restore point of one card,
// newest first.
type CheckpointList struct {
	// CardID is the card the list is about.
	CardID string `json:"cardId"`
	// Checkpoints are the card's restore points, newest first. Never null.
	Checkpoints []Checkpoint `json:"checkpoints"`
	// ServerTime is the daemon's time when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}

// NewCheckpointList makes an answer stamped with the daemon's time. A nil list becomes an empty
// one, so the JSON has [] and never null, and a screen maps over it without a check.
func NewCheckpointList(cardID string, checkpoints []Checkpoint, now time.Time) CheckpointList {
	out := make([]Checkpoint, len(checkpoints))
	copy(out, checkpoints)
	return CheckpointList{CardID: cardID, Checkpoints: out, ServerTime: NewTimestamp(now)}
}

// RestoreCheckpointRequest is the body of POST /v1/cards/{id}/checkpoints/{cp}/restore. An empty
// body is a worktree restore, which is what the plain Restore button asks for.
type RestoreCheckpointRequest struct {
	// Conversation asks for the card's own conversation to be put back to the checkpoint as well as
	// its worktree, which is what the "and the conversation" choice in the restore dialog sends.
	// It is the one field, so an absent body and a body of `{}` mean the same thing: the worktree
	// alone.
	Conversation bool `json:"conversation,omitempty"`
}
