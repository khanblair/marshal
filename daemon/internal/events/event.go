package events

import (
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Event is one thing that happened, as the bus hands it to subscribers. Keep it small: pass ids
// and let the reader look up the rest, never copy logs or diffs into Data. Data is shared by
// every subscriber and by the replay ring, so nobody may change it after publishing.
type Event struct {
	// Seq is the number the bus gave the event. It starts at 1 in each epoch and grows by one for
	// every event on any topic, so a subscriber that follows some topics sees gaps.
	Seq uint64
	// Topic is what the event is about, for example "project:api". Subscribers filter on it.
	Topic string
	// Type says what happened and how to read Data, for example "card.moved".
	Type string
	// At is when the bus accepted the event, by the bus clock.
	At time.Time
	// Critical events are never dropped for a slow subscriber. Use it for approvals and state
	// changes that a client cannot recover from a later event.
	Critical bool
	// Data is the payload. The API layer encodes it.
	Data any
}

// Replay says whether the bus can replay what a client missed.
type Replay string

const (
	// ReplayOK means every event after the client's position is still in the ring. The list may be
	// empty.
	ReplayOK Replay = "ok"
	// ReplayTooOld means the client's position is older than the ring reaches.
	ReplayTooOld Replay = "too-old"
	// ReplayOtherEpoch means the client's epoch is not this bus's epoch. That includes a client
	// that has no epoch yet.
	ReplayOtherEpoch Replay = "other-epoch"
	// ReplayAhead means the client's position is newer than any event this bus has sent, which a
	// client of this epoch cannot have.
	ReplayAhead Replay = "ahead"
)

// ResyncReason maps the result to the reason of the Resync frame the API sends. The second
// result is false for ReplayOK, where no frame is needed.
func (r Replay) ResyncReason() (protocol.ResyncReason, bool) {
	switch r {
	case ReplayTooOld:
		return protocol.ResyncReasonTooFarBehind, true
	case ReplayOtherEpoch:
		return protocol.ResyncReasonEpochChanged, true
	case ReplayAhead:
		return protocol.ResyncReasonUnknownPosition, true
	case ReplayOK:
		return "", false
	}
	return "", false
}

// CloseReason says why a subscription's channel was closed.
type CloseReason int

const (
	// StillOpen means the subscription is not closed.
	StillOpen CloseReason = iota
	// ClosedByCaller means Close was called on the subscription.
	ClosedByCaller
	// ClosedByBus means the bus was closed.
	ClosedByBus
	// ClosedForResync means the subscriber fell so far behind that even critical events
	// overflowed. The consumer must reload from the store, then subscribe again.
	ClosedForResync
)
