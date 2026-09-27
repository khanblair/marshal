package protocol

import "time"

// What came of a screenshot that was asked for (docs/architecture.md section 11.2 and N10,
// docs/library-docs.md's note on the browser, B6.6 and B6.7). A screenshot check is never silently
// passed: an answer says either that the shot was taken or that the check was skipped, and a skipped
// check always carries the sentence that says why.

// PreviewShotOutcome says what happened to a screenshot that was asked for.
type PreviewShotOutcome string

const (
	// PreviewShotOutcomeTaken means the screenshot was taken and is on the card.
	PreviewShotOutcomeTaken PreviewShotOutcome = "taken"
	// PreviewShotOutcomeSkipped means the check was skipped, and Notice says why. The one reason
	// today is that neither Chrome nor Edge is installed; Marshal will not report a check as passed
	// when it never ran.
	PreviewShotOutcomeSkipped PreviewShotOutcome = "skipped"
)

// PreviewShotOutcomeValues lists both outcomes, taken first.
func PreviewShotOutcomeValues() []PreviewShotOutcome {
	return []PreviewShotOutcome{PreviewShotOutcomeTaken, PreviewShotOutcomeSkipped}
}

// Valid reports whether o is a shot outcome.
func (o PreviewShotOutcome) Valid() bool {
	for _, v := range PreviewShotOutcomeValues() {
		if v == o {
			return true
		}
	}
	return false
}

// PreviewShotRequest is the body of POST /v1/cards/{id}/preview/shots. The card comes from the
// path, and the kind says which half of the before/after pair to take.
type PreviewShotRequest struct {
	// Kind says which half of the pair to capture: before the card's change, or after it.
	Kind PreviewShotKind `json:"kind"`
}

// PreviewShotResult is the answer to POST /v1/cards/{id}/preview/shots: the card's preview as it now
// is, and what came of the screenshot. Notice is always a sentence a person can read - what was
// captured, or why the check was skipped - so a client never has to guess from an empty shot list
// whether the check passed.
type PreviewShotResult struct {
	// Preview is the card's preview, with the new shot in it when one was taken.
	Preview Preview `json:"preview"`
	// Outcome says whether the shot was taken or the check was skipped.
	Outcome PreviewShotOutcome `json:"outcome"`
	// Notice is one plain sentence saying what was captured or why the check was skipped. Never
	// empty.
	Notice string `json:"notice"`
	// ServerTime is the daemon's time when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}

// NewPreviewShotResult makes a shot answer stamped with the daemon's time, over a preview whose shot
// list is never null.
func NewPreviewShotResult(preview Preview, outcome PreviewShotOutcome, notice string, now time.Time) PreviewShotResult {
	shots := make([]PreviewShot, len(preview.Shots))
	copy(shots, preview.Shots)
	preview.Shots = shots
	return PreviewShotResult{
		Preview:    preview,
		Outcome:    outcome,
		Notice:     notice,
		ServerTime: NewTimestamp(now),
	}
}
