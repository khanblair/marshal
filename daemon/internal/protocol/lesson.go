package protocol

import "time"

// The wire shape of a project's lesson: something an agent learned worth remembering next time,
// written to the vault as its own markdown file the same way a card's note is (see note.go). It is
// a sibling of Note and not a variant of it on the wire, even though internal/memory indexes both in
// the same table (migration 0019's header comment): a screen that draws a card's one note and a
// screen that draws a project's many lessons want different shapes, and giving each its own type is
// what lets a lesson carry a title and a slug without a card note growing fields it never uses.
//
// Unlike a card's note, a lesson has no "nothing saved yet" state: a card always exists to write a
// placeholder note for, but a slug nobody has saved a lesson under names nothing, so a read that
// finds none answers NotFound rather than a lesson with no time on it. That is why UpdatedAt here is
// a plain Timestamp and not a pointer the way Note's is: every Lesson this package hands out is one
// that was actually saved.

// Lesson is one project's lesson as a client or an agent's search_memory sees it.
type Lesson struct {
	// ProjectID is the project the lesson belongs to, and the name of the folder its file sits in
	// under the vault (see Path).
	ProjectID string `json:"projectId"`
	// Slug is the lesson's identity inside its project: the readable, file-name-safe form of its
	// title (memory.noteSlug), and the second half of the unique key migration 0019 enforces.
	Slug string `json:"slug"`
	// Title is the lesson's heading, in full: what the lessons screen lists and what post_note's
	// sibling for lessons, once one exists, is given.
	Title string `json:"title"`
	// Path is where the lesson lives, relative to the vault root, in the shape
	// `<project>/lessons/<slug>.md` - so the lessons screen shows something like
	// `small-repo/lessons/ci-is-flaky-on-windows.md`. Relative for the same reason Note.Path is:
	// the vault lives wherever the person put it.
	Path string `json:"path"`
	// Body is the whole lesson, in markdown.
	Body string `json:"body"`
	// Author is who wrote the lesson: "person" for one written or edited through the lessons
	// screen, "agent" for one an agent leaves after a fix, once an internal MCP tool can write one
	// (lessons.go's package comment says where that stands today).
	Author NoteAuthor `json:"author"`
	// UpdatedAt is when the lesson was last saved. Never the zero time - see the package comment.
	UpdatedAt Timestamp `json:"updatedAt"`
}

// NewLesson makes a lesson, stamping it with the given save time.
func NewLesson(projectID, slug, title, path, body string, author NoteAuthor, updatedAt time.Time) Lesson {
	return Lesson{
		ProjectID: projectID, Slug: slug, Title: title, Path: path, Body: body, Author: author,
		UpdatedAt: NewTimestamp(updatedAt),
	}
}

// SaveLessonRequest is the body of the call that writes a project's lesson (task 7.6, 7.13). Like
// SaveNoteRequest, the body is the whole lesson and not a patch.
type SaveLessonRequest struct {
	// Title is the lesson's heading. Required: a lesson's slug and its place in the vault are made
	// from it, so a lesson cannot be titled after the fact by editing only its body.
	Title string `json:"title"`
	// Body is the lesson in full, in markdown. It may be empty, the same as a card note's may.
	Body string `json:"body"`
}
