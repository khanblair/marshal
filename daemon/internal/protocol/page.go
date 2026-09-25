package protocol

import "time"

// Page is one page of a list. Ask for the next page by sending NextCursor back as the `cursor`
// parameter. An empty NextCursor means this was the last page. Cursors are opaque to clients:
// they must be passed back exactly as received and never built or read. Like every response that
// shows state, a page carries the daemon's time.
type Page[T any] struct {
	// Items are the entries of this page, in the order the endpoint documents.
	Items []T `json:"items"`
	// NextCursor is the cursor for the next page, or empty at the end.
	NextCursor string `json:"nextCursor"`
	// ServerTime is the daemon's time when the page was made.
	ServerTime Timestamp `json:"serverTime"`
}

// NewPage makes a page. It turns a nil list into an empty one so the JSON is [] and never null,
// and it stamps the page with the daemon's time.
func NewPage[T any](items []T, nextCursor string, now time.Time) Page[T] {
	if items == nil {
		items = []T{}
	}
	return Page[T]{Items: items, NextCursor: nextCursor, ServerTime: NewTimestamp(now)}
}
