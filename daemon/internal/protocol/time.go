package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// timestampLayout is UTC in RFC 3339 with exactly three fractional digits. The trailing Z is
// literal: values are converted to UTC before they are formatted.
const timestampLayout = "2006-01-02T15:04:05.000Z"

// quotedLayoutLength is the room for the layout plus its two quote marks.
const quotedLayoutLength = len(timestampLayout) + len(`""`)

var errUnsetTimestamp = errors.New("protocol: a timestamp is not set")

// Timestamp is a moment on the wire. It is always UTC, written as RFC 3339 with milliseconds,
// for example 2026-09-25T10:15:30.123Z, and it is a plain string in TypeScript. A Timestamp that
// was never set does not encode: a missing time is a bug to fix, not a date in the year 1. Use a
// pointer for a time that may be absent, which encodes as null.
type Timestamp time.Time

// NewTimestamp wraps a time for the wire.
func NewTimestamp(t time.Time) Timestamp {
	return Timestamp(t)
}

// Time returns the moment as a time.Time.
func (t Timestamp) Time() time.Time {
	return time.Time(t)
}

// MarshalJSON writes the UTC millisecond form, cutting anything finer than a millisecond.
func (t Timestamp) MarshalJSON() ([]byte, error) {
	moment := time.Time(t)
	if moment.IsZero() {
		return nil, errUnsetTimestamp
	}
	out := make([]byte, 0, quotedLayoutLength)
	out = append(out, '"')
	out = moment.UTC().AppendFormat(out, timestampLayout)
	return append(out, '"'), nil
}

// UnmarshalJSON reads any RFC 3339 time and keeps it as UTC to the millisecond. A JSON null
// leaves the value unchanged, like the standard library does.
func (t *Timestamp) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("read a timestamp: %w", err)
	}
	moment, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return fmt.Errorf("read a timestamp: %w", err)
	}
	if moment.IsZero() {
		return errUnsetTimestamp
	}
	*t = Timestamp(moment.UTC().Truncate(time.Millisecond))
	return nil
}
