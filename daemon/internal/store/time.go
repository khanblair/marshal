package store

import (
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Millis converts a time to the Unix milliseconds the database stores.
func Millis(t time.Time) int64 { return t.UnixMilli() }

// Timestamp converts stored Unix milliseconds to the wire type.
func Timestamp(millis int64) protocol.Timestamp {
	return protocol.NewTimestamp(time.UnixMilli(millis).UTC())
}

// OptionalTimestamp converts a stored time that may be missing. A missing time stays nil, which
// is null on the wire.
func OptionalTimestamp(millis *int64) *protocol.Timestamp {
	if millis == nil {
		return nil
	}
	ts := Timestamp(*millis)
	return &ts
}
