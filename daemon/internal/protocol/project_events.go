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
