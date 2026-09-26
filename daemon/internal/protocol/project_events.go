package protocol

// The payloads (Event.Data) of the project and card events. Each carries the new state of what
// changed, not a difference, so a client that applies one twice, or after a replay, ends in the
// same place (docs/architecture.md section 11.5).
//
// Where they are sent: the project events go to the "home" topic, because the sidebar on every
// screen lists all projects. The card events go to the "project:<id>" topic of the card's project.
// A card that changes state into or out of "needs" also changes its project's badge, so the
// project's own "project.updated" follows on the home topic.

// ProjectEventData is the payload of project.created and project.updated.
type ProjectEventData struct {
	// Project is the project as it is now, with its badges.
	Project Project `json:"project"`
}

// ProjectRemovedEventData is the payload of project.removed.
type ProjectRemovedEventData struct {
	// ProjectID is the id of the project that is gone.
	ProjectID string `json:"projectId"`
}

// CardEventData is the payload of card.created and card.updated.
type CardEventData struct {
	// Card is the card as it is now.
	Card Card `json:"card"`
}

// CardMovedEventData is the payload of card.moved.
type CardMovedEventData struct {
	// Card is the card as it is now, so Card.State is where it moved to.
	Card Card `json:"card"`
	// From is the state the card was in before.
	From CardState `json:"from"`
}

// CardDeletedEventData is the payload of card.deleted. It is critical: a client that missed it
// would keep drawing a card that is gone.
type CardDeletedEventData struct {
	// CardID is the opaque id of the card that is gone, which is what routes use.
	CardID string `json:"cardId"`
	// Key is the card's key, `<projectId>#<number>`, which is how the app names a card in its own
	// store. It is here because a client cannot turn the opaque id back into the key by itself.
	Key string `json:"key"`
	// ProjectID is the project it belonged to, so a client knows which board to redraw.
	ProjectID string `json:"projectId"`
}

// LabelUpdatedEventData is the payload of label.updated. A label that was deleted is not in the
// snapshot the event carries, so a client replaces its list with this one.
type LabelUpdatedEventData struct {
	// ProjectID is the project whose labels changed.
	ProjectID string `json:"projectId"`
	// Labels are the project's labels as they are now, by name.
	Labels []Label `json:"labels"`
}
