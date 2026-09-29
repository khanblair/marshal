package protocol

// The wire shape of what a card's panel holds beyond the card itself (B10.2, B10.5, B10.6):
// acceptance checks, named checklists, comments with attachments, and the people on the card. Each
// list is read whole with one call, and every change answers the whole list again, so a screen
// redraws from one answer.

// CheckStatus is where an acceptance check stands.
type CheckStatus string

const (
	// CheckStatusPending means the check has not been run since it was added.
	CheckStatusPending CheckStatus = "pending"
	// CheckStatusPassed means the last run passed.
	CheckStatusPassed CheckStatus = "passed"
	// CheckStatusFailed means the last run failed.
	CheckStatusFailed CheckStatus = "failed"
)

// CheckStatusValues lists every check status.
func CheckStatusValues() []CheckStatus {
	return []CheckStatus{CheckStatusPending, CheckStatusPassed, CheckStatusFailed}
}

// CardCheck is one acceptance check on a card: a command Marshal runs in the card's worktree, or a
// review the card must earn. A card cannot finish until every check has passed.
type CardCheck struct {
	// ID is the check's opaque id.
	ID string `json:"id"`
	// Name is what the check is called, such as "Tests pass".
	Name string `json:"name"`
	// Kind is "command" for a check Marshal runs, or "review" for the reviewer's approval.
	Kind string `json:"kind"`
	// Command is the shell command a command check runs. Empty for a review check.
	Command string `json:"command"`
	// Status is the answer of the last run.
	Status CheckStatus `json:"status"`
	// RunRef names the run or commit the status came from, so a person can tell whether it is still
	// the answer. Empty until the check has run.
	RunRef string `json:"runRef"`
}

// CardCheckList is the answer to every card-checks call.
type CardCheckList struct {
	// Checks are the card's checks in the order they are drawn. Never null.
	Checks []CardCheck `json:"checks"`
	// ServerTime is the daemon's time when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}

// AddCardCheckRequest is the body of POST /v1/cards/{id}/checks.
type AddCardCheckRequest struct {
	// Name is what the check is called.
	Name string `json:"name"`
	// Command is the shell command to run in the card's worktree.
	Command string `json:"command"`
}

// ChecklistItem is one line of a checklist.
type ChecklistItem struct {
	// ID is the item's opaque id.
	ID string `json:"id"`
	// Text is what the line says.
	Text string `json:"text"`
	// Done says the line is ticked.
	Done bool `json:"done"`
	// DoneByKind is "person" or "agent" for a ticked line, and empty for an open one.
	DoneByKind string `json:"doneByKind"`
	// DoneByID is the user id of the person who ticked it, or empty for the agent or an open line.
	DoneByID string `json:"doneById"`
	// DoneAt is when it was last ticked or reopened. Null if it never was.
	DoneAt *Timestamp `json:"doneAt" tstype:"Timestamp | null"`
}

// Checklist is a named list on a card.
type Checklist struct {
	// ID is the checklist's opaque id.
	ID string `json:"id"`
	// Name is the list's title.
	Name string `json:"name"`
	// Required means the card cannot go to Ready to merge while a line is open.
	Required bool `json:"required"`
	// PeopleOnly means only a person may tick a line; the agent is refused.
	PeopleOnly bool `json:"peopleOnly"`
	// HideChecked keeps ticked lines out of sight.
	HideChecked bool `json:"hideChecked"`
	// Items are the lines in the order they are drawn. Never null.
	Items []ChecklistItem `json:"items"`
}

// ChecklistList is the answer to every checklist call.
type ChecklistList struct {
	// Checklists are the card's lists in the order they are drawn. Never null.
	Checklists []Checklist `json:"checklists"`
	// ServerTime is the daemon's time when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}

// CreateChecklistRequest is the body of POST /v1/cards/{id}/checklists.
type CreateChecklistRequest struct {
	// Name is the list's title. Empty means "Checklist".
	Name string `json:"name"`
}

// UpdateChecklistRequest is the body of PATCH /v1/cards/{id}/checklists/{list}. A field left out is
// left as it was.
type UpdateChecklistRequest struct {
	// Name renames the list.
	Name *string `json:"name,omitempty"`
	// Required turns the merge gate on or off.
	Required *bool `json:"required,omitempty"`
	// PeopleOnly turns the agent's right to tick on or off.
	PeopleOnly *bool `json:"peopleOnly,omitempty"`
	// HideChecked turns hiding of ticked lines on or off.
	HideChecked *bool `json:"hideChecked,omitempty"`
}

// AddChecklistItemRequest is the body of POST /v1/cards/{id}/checklists/{list}/items.
type AddChecklistItemRequest struct {
	// Text is what the line says.
	Text string `json:"text"`
}

// TickChecklistItemRequest is the body of PUT /v1/cards/{id}/checklists/{list}/items/{item}.
type TickChecklistItemRequest struct {
	// Done is true to tick the line and false to reopen it.
	Done bool `json:"done"`
}

// AttachmentKind says what an attachment is.
type AttachmentKind string

const (
	// AttachmentKindImage is a picture.
	AttachmentKindImage AttachmentKind = "image"
	// AttachmentKindFile is any other file.
	AttachmentKindFile AttachmentKind = "file"
	// AttachmentKindLink is a web address.
	AttachmentKindLink AttachmentKind = "link"
)

// AttachmentKindValues lists every attachment kind.
func AttachmentKindValues() []AttachmentKind {
	return []AttachmentKind{AttachmentKindImage, AttachmentKindFile, AttachmentKindLink}
}

// MaxAttachmentBytes is the most one attached file may hold.
const MaxAttachmentBytes = 5 << 20

// MaxCommentFileBytes is the most the files of one comment may hold together.
const MaxCommentFileBytes = 6 << 20

// MaxCommentChars is the most a comment's text may hold.
const MaxCommentChars = 10000

// Attachment is a file, image, or link on a comment. A file is data the agent reads and is never run.
type Attachment struct {
	// ID is the attachment's opaque id.
	ID string `json:"id"`
	// Kind says whether it is an image, a file, or a link.
	Kind AttachmentKind `json:"kind"`
	// Name is the file name, or the link without its scheme.
	Name string `json:"name"`
	// SizeBytes is the file's size. Zero for a link.
	SizeBytes int64 `json:"sizeBytes"`
	// URL is the link's address. Empty for a file.
	URL string `json:"url"`
}

// AuthorKind says who wrote a comment.
type AuthorKind string

const (
	// AuthorKindPerson is a person.
	AuthorKindPerson AuthorKind = "person"
	// AuthorKindAgent is the card's agent.
	AuthorKindAgent AuthorKind = "agent"
)

// AuthorKindValues lists every comment author kind.
func AuthorKindValues() []AuthorKind { return []AuthorKind{AuthorKindPerson, AuthorKindAgent} }

// Comment is one comment on a card.
type Comment struct {
	// ID is the comment's opaque id.
	ID string `json:"id"`
	// AuthorKind says whether a person or the card's agent wrote it.
	AuthorKind AuthorKind `json:"authorKind"`
	// AuthorID is the user id of the person. Empty for the agent.
	AuthorID string `json:"authorId"`
	// Body is the text.
	Body string `json:"body"`
	// Attachments are its files, images, and links. Never null.
	Attachments []Attachment `json:"attachments"`
	// AgentReadAt is when the card's agent read it. Null until it did.
	AgentReadAt *Timestamp `json:"agentReadAt" tstype:"Timestamp | null"`
	// CreatedAt is when it was posted.
	CreatedAt Timestamp `json:"createdAt"`
}

// CommentList is the answer to every comment call.
type CommentList struct {
	// Comments are the card's comments, oldest first. Never null.
	Comments []Comment `json:"comments"`
	// ServerTime is the daemon's time when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}

// NewAttachment is one attachment in a new comment. A link has a URL. A file has a name and its
// bytes, base64 encoded, at most MaxAttachmentBytes.
type NewAttachment struct {
	// Kind is "image", "file", or "link".
	Kind AttachmentKind `json:"kind"`
	// Name is the file name. Ignored for a link.
	Name string `json:"name"`
	// MimeType is the file's type, as the browser named it.
	MimeType string `json:"mimeType"`
	// URL is the address of a link.
	URL string `json:"url"`
	// Data is the file's bytes, base64 encoded.
	Data string `json:"data"`
}

// PostCommentRequest is the body of POST /v1/cards/{id}/comments.
type PostCommentRequest struct {
	// Body is the text. It may be empty when there is an attachment.
	Body string `json:"body"`
	// Attachments are the files, images, and links to keep with it. A link written in the text is
	// added by the daemon.
	Attachments []NewAttachment `json:"attachments"`
}

// CardMembers is the answer to every member call: the people on a card, in the order they were added.
type CardMembers struct {
	// UserIDs are the people's user ids. Never null.
	UserIDs []string `json:"userIds"`
	// ServerTime is the daemon's time when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}
