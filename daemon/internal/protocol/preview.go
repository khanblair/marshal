package protocol

import "time"

// The wire shape of a card's live preview (docs/architecture.md section 11.2 and N10, build-plan
// 6.6 and 6.7, B6.6 and B6.7). The three state names are the app's own, taken from
// `apps/web/src/views/card/preview-model.ts`, which draws the tab today from the fake daemon: they
// do not change when the daemon takes the tab over.

// PreviewState is where a card's preview is. The three words are the ones the Preview tab already
// reads, and the tab is not redesigned around them.
type PreviewState string

const (
	// PreviewStateStopped means no dev server is running for the card.
	PreviewStateStopped PreviewState = "stopped"
	// PreviewStateStarting means the dev server has been asked for and is not answering yet.
	PreviewStateStarting PreviewState = "starting"
	// PreviewStateRunning means the dev server answered, so the address is worth showing.
	PreviewStateRunning PreviewState = "running"
)

// PreviewStateValues lists every preview state, in the order the tab moves through them.
func PreviewStateValues() []PreviewState {
	return []PreviewState{PreviewStateStopped, PreviewStateStarting, PreviewStateRunning}
}

// Valid reports whether s is a preview state.
func (s PreviewState) Valid() bool {
	for _, v := range PreviewStateValues() {
		if v == s {
			return true
		}
	}
	return false
}

// PreviewShotKind says which half of the before/after pair a screenshot is: the page as the branch
// left it, or the page with the card's change.
type PreviewShotKind string

const (
	// PreviewShotKindBefore is the screenshot taken before the card's change.
	PreviewShotKindBefore PreviewShotKind = "before"
	// PreviewShotKindAfter is the screenshot taken after the card's change.
	PreviewShotKindAfter PreviewShotKind = "after"
)

// PreviewShotKindValues lists both halves, in the order they are shown.
func PreviewShotKindValues() []PreviewShotKind {
	return []PreviewShotKind{PreviewShotKindBefore, PreviewShotKindAfter}
}

// Valid reports whether k is a shot kind.
func (k PreviewShotKind) Valid() bool {
	for _, v := range PreviewShotKindValues() {
		if v == k {
			return true
		}
	}
	return false
}

// PreviewShot is one screenshot of a card's preview. Marshal keeps the image as a file and serves
// it from the daemon; the wire carries the address, its size, and when it was taken, never the
// bytes.
type PreviewShot struct {
	// Kind says which half of the pair this is.
	Kind PreviewShotKind `json:"kind"`
	// URL is the daemon path of the image, with a version so a new image is a new address and a
	// client never shows a cached one. It needs the token, like every other route.
	URL string `json:"url"`
	// Width and Height are the image's size in pixels, so a screen can lay it out before the
	// image arrives.
	Width  int `json:"width"`
	Height int `json:"height"`
	// TakenAt is when the screenshot was taken.
	TakenAt Timestamp `json:"takenAt"`
}

// Preview is one card's live preview: where it is, the address it answers on when it is running,
// the command that was run, and the screenshots taken of it.
type Preview struct {
	// CardID is the card the preview belongs to.
	CardID string `json:"cardId"`
	// State is where the preview is.
	State PreviewState `json:"state"`
	// URL is the address the app answers on, such as "http://127.0.0.1:5103". Empty until State
	// is running: a preview that is starting has no address worth showing yet.
	URL string `json:"url"`
	// Port is the port the dev server was given. Zero when the card has no preview of its own.
	// Each card gets its own, so two cards preview at once without sharing state.
	Port int `json:"port"`
	// Command is the command that was run to start it, such as "pnpm dev", so a person can see
	// what the tab is doing. Empty when the project has no dev command set.
	Command string `json:"command"`
	// StartedAt is when the preview reached running. Null while it is stopped or starting.
	StartedAt *Timestamp `json:"startedAt" tstype:"Timestamp | null"`
	// Error is one plain sentence saying why the preview could not start, and is empty when
	// nothing went wrong. A preview that failed says so instead of spinning forever.
	Error string `json:"error"`
	// Shots are the before and after screenshots, at most one of each. Never null.
	Shots []PreviewShot `json:"shots"`
}

// PreviewSnapshot is the answer to GET /v1/cards/{id}/preview: the card's preview as it is now.
// Opening the route runs nothing: it reports what is running, so a person looking at the tab does
// not start a dev server on the machine by looking.
type PreviewSnapshot struct {
	// Preview is the card's preview.
	Preview Preview `json:"preview"`
	// ServerTime is the daemon's time when the answer was made, so a client counts from it how
	// long the preview has been running.
	ServerTime Timestamp `json:"serverTime"`
}

// NewPreviewSnapshot makes an answer stamped with the daemon's time. A nil shot list becomes an
// empty one, so the JSON has [] and never null.
func NewPreviewSnapshot(preview Preview, now time.Time) PreviewSnapshot {
	shots := make([]PreviewShot, len(preview.Shots))
	copy(shots, preview.Shots)
	preview.Shots = shots
	return PreviewSnapshot{Preview: preview, ServerTime: NewTimestamp(now)}
}

// PreviewEventData is what a `preview.state_changed` event carries (section 11.2): the card's
// preview as it now is, whole, so a screen applies the event exactly as it applies the snapshot and
// the two can never disagree. It is published on the card's own topic.
type PreviewEventData struct {
	// Preview is the card's preview.
	Preview Preview `json:"preview"`
}
