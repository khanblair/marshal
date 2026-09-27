package harness

import (
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The stuck detector (docs/architecture.md section 3, "stuck detection"; docs/backend-checklist.md
// B5.3, build-plan 5.4): it catches a card going round in circles, so a person is asked instead of
// the agent burning the card's own limits on the same failure. It is fed one signal at a time, in
// the order a turn produced them, and reports when the same signal appears a set number of times in
// a row.
//
// It is a pure value: it decides from the signals it has been given and nothing else, so the loop
// it catches can be tested without an agent.

// SignalKind is the kind of thing a detector watches repeat.
type SignalKind string

const (
	// SignalError is a failed tool call. Its Key is the fingerprint of what went wrong, and its
	// Where names the file or command it happened in.
	SignalError SignalKind = "error"
	// SignalEdit is an edit to a file. Its Key is the file's path, and its Where repeats it for
	// the sentence a person reads.
	SignalEdit SignalKind = "edit"
)

// Signal is one thing a turn produced that the detector watches. An empty Key is not a signal and
// is ignored: a failure the agent did not describe well enough to fingerprint is not a loop.
type Signal struct {
	Kind  SignalKind
	Key   string
	Where string
	// Label is the human word for what repeated, used in the sentence ("type error"). Empty means
	// the detector's own word for the kind.
	Label string
}

// StuckThreshold is how many times the same signal in a row means the card is stuck. Three, the
// number the prototype's own stuck message names.
const StuckThreshold = 3

// StuckDetector is a card's loop watcher. It is not safe for use by many goroutines; one card's
// pump feed is one goroutine.
type StuckDetector struct {
	threshold int
	// run is the current run of signals that are all the same. A signal that differs starts a new
	// run, so only a repeat counts and an alternating loop of two different failures is not one.
	run []Signal
}

// NewStuckDetector makes a detector with the usual threshold.
func NewStuckDetector() *StuckDetector { return NewStuckDetectorN(StuckThreshold) }

// NewStuckDetectorN makes a detector that calls the card stuck after threshold repeats in a row. A
// threshold below two catches nothing but noise, so it is raised to two.
func NewStuckDetectorN(threshold int) *StuckDetector {
	if threshold < 2 {
		threshold = 2
	}
	return &StuckDetector{threshold: threshold}
}

// Observe records one signal and reports whether the card is stuck. A signal with no key is ignored
// and leaves the current run alone. The reason's sentence is built once, at the moment the loop
// crosses the threshold, so a card stopped for being stuck is stopped once.
func (d *StuckDetector) Observe(s Signal) (Reason, bool) {
	if strings.TrimSpace(s.Key) == "" {
		return Reason{}, false
	}
	if len(d.run) > 0 && sameSignal(d.run[len(d.run)-1], s) {
		d.run = append(d.run, s)
	} else {
		d.run = []Signal{s}
	}
	if len(d.run) < d.threshold {
		return Reason{}, false
	}
	return d.reason(d.run), true
}

// sameSignal says whether two signals are the same thing repeating: the same kind, and the same key.
func sameSignal(a, b Signal) bool { return a.Kind == b.Kind && a.Key == b.Key }

// reason is the sentence a person reads for a run of repeats. An edit loop names the file that keeps
// being rewritten; an error loop names what kept failing and where, in the words the caller gave it
// when it described the failure.
func (d *StuckDetector) reason(run []Signal) Reason {
	at := run[0]
	var text string
	if at.Kind == SignalEdit {
		text = fmt.Sprintf("The stuck detector paused this card. The same file was edited %d times in a row: %s.",
			len(run), at.Where)
	} else {
		label := strings.TrimSpace(at.Label)
		if label == "" {
			label = "error"
		}
		text = fmt.Sprintf("The stuck detector paused this card. The same %s appeared %d times in a row",
			label, len(run))
		if at.Where != "" {
			text += " in " + at.Where
		}
		text += "."
	}
	return Reason{Kind: protocol.NeedsReasonKindStuck, Text: text}
}
