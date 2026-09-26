package protocol

// Labels are a managed list per project (decision D3). A label has a name and one color from the
// fixed set in enums.go. Phase 2 stores them and puts them on cards; the screens keep showing the
// names exactly as they do today, because the design has no label picker, no label management,
// and no label colors yet (Q20, backend-checklist.md section 5.4).

// Label is one label of a project.
type Label struct {
	// ID is the label's opaque id.
	ID string `json:"id"`
	// ProjectID is the project the label belongs to.
	ProjectID string `json:"projectId"`
	// Name is the label's text, such as "backend". It is unique inside its project.
	Name string `json:"name"`
	// Color is one of the fixed label colors.
	Color LabelColor `json:"color"`
	// CreatedAt is when the label was made.
	CreatedAt Timestamp `json:"createdAt"`
}

// LabelSnapshot is the answer to GET /v1/projects/{id}/labels.
type LabelSnapshot struct {
	// ProjectID is the project the labels belong to.
	ProjectID string `json:"projectId"`
	// Labels are the project's labels, by name.
	Labels []Label `json:"labels"`
	// ServerTime is the daemon's time when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}

// CreateLabelRequest is the body of POST /v1/projects/{id}/labels.
type CreateLabelRequest struct {
	// Name is the label's text. It cannot be empty and must be unique in the project.
	Name string `json:"name"`
	// Color is one of the fixed label colors. Empty means slate.
	Color LabelColor `json:"color,omitempty"`
}

// UpdateLabelRequest is the body of PATCH /v1/labels/{id}. A field that is not set is left alone.
type UpdateLabelRequest struct {
	// Name renames the label. Null leaves it.
	Name *string `json:"name,omitempty"`
	// Color changes the color. Null leaves it.
	Color *LabelColor `json:"color,omitempty"`
}
