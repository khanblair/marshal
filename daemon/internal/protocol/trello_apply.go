package protocol

// The Trello apply flow (docs/marshal-product-scope.md section 19.2). Marshal reads a linked
// board and works on its own copy. What it changed since it last read Trello is a list of pending
// changes, and nothing reaches Trello until the person applies some of them.

// TrelloChangeKind says what a pending change would do in Trello.
type TrelloChangeKind string

const (
	// TrelloChangeKindCreateCard makes a Trello card for a Marshal card that has none.
	TrelloChangeKindCreateCard TrelloChangeKind = "createCard"
	// TrelloChangeKindMoveDone moves the Trello card to the board's done list.
	TrelloChangeKindMoveDone TrelloChangeKind = "moveDone"
	// TrelloChangeKindUpdateCard writes a new title or description.
	TrelloChangeKindUpdateCard TrelloChangeKind = "updateCard"
	// TrelloChangeKindAddComment posts a comment as the linked person.
	TrelloChangeKindAddComment TrelloChangeKind = "addComment"
	// TrelloChangeKindSetItem ticks or reopens one checklist item.
	TrelloChangeKindSetItem TrelloChangeKind = "setItem"
	// TrelloChangeKindAddItem adds one item to a checklist that is already on the Trello card.
	TrelloChangeKindAddItem TrelloChangeKind = "addItem"
	// TrelloChangeKindAddChecklist adds a whole checklist, with its items, to the Trello card.
	TrelloChangeKindAddChecklist TrelloChangeKind = "addChecklist"
)

// TrelloChangeKindValues lists every kind of pending change.
func TrelloChangeKindValues() []TrelloChangeKind {
	return []TrelloChangeKind{
		TrelloChangeKindCreateCard, TrelloChangeKindMoveDone, TrelloChangeKindUpdateCard, TrelloChangeKindAddComment,
		TrelloChangeKindSetItem, TrelloChangeKindAddItem, TrelloChangeKindAddChecklist,
	}
}

// TrelloChange is one thing Marshal would do in Trello.
type TrelloChange struct {
	// ID names the change. It stays the same while the change stays the same, and it changes when
	// the change does, so a dismissed change that is edited shows up again.
	ID string `json:"id"`
	// CardID is the Marshal card the change belongs to.
	CardID string `json:"cardId"`
	// CardTitle is that card's title, for the list.
	CardTitle string `json:"cardTitle"`
	// Kind says what the change does.
	Kind TrelloChangeKind `json:"kind"`
	// Summary is one line that says what the change does, such as "Move to Done".
	Summary string `json:"summary"`
	// Detail is the text that would be written, such as a comment's body. Empty when the summary
	// says it all.
	Detail string `json:"detail"`
	// ByKind is "person" or "agent" for who made the change in Marshal, and empty when that is not
	// known. Trello never shows it: everything is written as the linked person.
	ByKind string `json:"byKind"`
	// Conflict says Trello changed the same thing since Marshal last read it. Applying it would
	// overwrite that, so it is only done when the person chooses to.
	Conflict bool `json:"conflict"`
	// ConflictNote says what changed in Trello, such as "Trello moved this card to Review." Empty
	// when Conflict is false.
	ConflictNote string `json:"conflictNote"`
}

// TrelloLinkedCard is a Marshal card that came from, or was sent to, a Trello card.
type TrelloLinkedCard struct {
	// CardID is the Marshal card.
	CardID string `json:"cardId"`
	// URL is the Trello card's address.
	URL string `json:"url"`
	// ListName is the Trello list the card was on when Marshal last read the board.
	ListName string `json:"listName"`
	// FromTrello says the card was first made in Trello. It is false for a Marshal card that was
	// pushed to Trello later.
	FromTrello bool `json:"fromTrello"`
}

// TrelloPending is the answer to the Trello calls of a project.
type TrelloPending struct {
	// Linked says the project is linked to a Trello board. Everything below is empty when it is not.
	Linked bool `json:"linked"`
	// BoardName is the linked board's name.
	BoardName string `json:"boardName"`
	// Changes are the pending changes, card by card, in the order they would be applied. Never null.
	Changes []TrelloChange `json:"changes"`
	// Cards are the project's cards that have a Trello card. Never null.
	Cards []TrelloLinkedCard `json:"cards"`
	// FetchedAt is when Marshal last read the board. Null if it never did.
	FetchedAt *Timestamp `json:"fetchedAt" tstype:"Timestamp | null"`
	// ServerTime is the daemon's time when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}

// ApplyTrelloRequest is the body of POST /v1/projects/{id}/trello/apply.
type ApplyTrelloRequest struct {
	// ChangeIDs are the changes to write to Trello. A change not listed is left pending.
	ChangeIDs []string `json:"changeIds"`
	// OverrideIDs are the listed changes that are in conflict and should be written anyway. A
	// conflicting change that is not named here is skipped.
	OverrideIDs []string `json:"overrideIds"`
}

// TrelloApplySkip is a change that was not written, and why.
type TrelloApplySkip struct {
	// ChangeID is the change.
	ChangeID string `json:"changeId"`
	// Reason says why, in a sentence for the person.
	Reason string `json:"reason"`
}

// ApplyTrelloResult is the answer to an apply.
type ApplyTrelloResult struct {
	// Applied are the changes that were written. Never null.
	Applied []string `json:"applied"`
	// Skipped are the changes that were not written. Never null.
	Skipped []TrelloApplySkip `json:"skipped"`
	// Pending is what is still pending afterwards.
	Pending TrelloPending `json:"pending"`
}

// DismissTrelloRequest is the body of POST /v1/projects/{id}/trello/dismiss.
type DismissTrelloRequest struct {
	// ChangeIDs are the changes to stop showing. They stay in Marshal and are never written.
	ChangeIDs []string `json:"changeIds"`
}

// RefreshTrelloRequest is the body of POST /v1/projects/{id}/trello/refresh.
type RefreshTrelloRequest struct {
	// KeepTrelloCardIDs are Marshal cards whose title, description, and ticks are set back to
	// what Trello holds, which drops their pending changes of those kinds. Empty reads the board
	// and changes nothing Marshal has edited.
	KeepTrelloCardIDs []string `json:"keepTrelloCardIds"`
}

// RefreshTrelloResult is the answer to a refresh.
type RefreshTrelloResult struct {
	// Imported is how many cards were made from Trello cards Marshal had not seen.
	Imported int `json:"imported"`
	// Updated is how many Marshal cards took a change from Trello.
	Updated int `json:"updated"`
	// Pending is what is pending afterwards.
	Pending TrelloPending `json:"pending"`
}
