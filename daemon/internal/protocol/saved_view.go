package protocol

// Saved views (docs/backend-checklist.md B2.5, inventory N20): a project's named sets of filter
// chips and a swimlane, which the board's view chips apply in one click. They belong to the
// project, not to a person, like its labels. Every change publishes saved_view.updated on the
// project's topic with the project's views as they now are.

// SavedView is one saved view of a project.
type SavedView struct {
	// ID is the view's opaque id.
	ID string `json:"id"`
	// ProjectID is the project the view belongs to.
	ProjectID string `json:"projectId"`
	// Name is the text on the view's chip, such as "Needs me". It is unique inside its project.
	Name string `json:"name"`
	// Filters are the filter chips the view applies. Never null.
	Filters []Filter `json:"filters"`
	// Swimlane is what the view groups the board's rows by.
	Swimlane Swimlane `json:"swimlane"`
	// CreatedAt is when the view was first saved.
	CreatedAt Timestamp `json:"createdAt"`
	// UpdatedAt is when the view was last saved or changed. The list is in this order, so a view
	// saved again moves to the end, as it does on the screen.
	UpdatedAt Timestamp `json:"updatedAt"`
}

// SavedViewListSnapshot is the answer to GET /v1/projects/{id}/saved-views.
type SavedViewListSnapshot struct {
	// ProjectID is the project the views belong to.
	ProjectID string `json:"projectId"`
	// Views are the project's saved views, the one saved longest ago first. Never null.
	Views []SavedView `json:"views"`
	// ServerTime is the daemon's time when the list was made.
	ServerTime Timestamp `json:"serverTime"`
}

// CreateSavedViewRequest is the body of POST /v1/projects/{id}/saved-views. Saving under a name the
// project already uses replaces that view, as the board's "Save view" does, and keeps its id.
type CreateSavedViewRequest struct {
	// Name is the text on the view's chip. It cannot be empty.
	Name string `json:"name"`
	// Filters are the filter chips to keep. Absent means none.
	Filters []Filter `json:"filters,omitempty"`
	// Swimlane is the grouping to keep. Empty means none.
	Swimlane Swimlane `json:"swimlane,omitempty"`
}

// UpdateSavedViewRequest is the body of PATCH /v1/saved-views/{id}. A field that is not set is left
// alone.
type UpdateSavedViewRequest struct {
	// Name renames the view. It must stay unique in the project.
	Name *string `json:"name,omitempty"`
	// Filters replaces the view's filter chips.
	Filters *[]Filter `json:"filters,omitempty"`
	// Swimlane changes the view's grouping.
	Swimlane *Swimlane `json:"swimlane,omitempty"`
}

// SavedViewUpdatedEventData is the payload of saved_view.updated, on the project's topic. A view
// that was deleted is not in the list it carries, so a client replaces its list with this one.
type SavedViewUpdatedEventData struct {
	// ProjectID is the project whose saved views changed.
	ProjectID string `json:"projectId"`
	// Views are the project's saved views as they are now.
	Views []SavedView `json:"views"`
}
