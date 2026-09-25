package protocol_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func TestTimestampEncodesAsUTCMilliseconds(t *testing.T) {
	plusThree := time.FixedZone("plus3", 3*60*60)
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{"utc with milliseconds", time.Date(2026, time.September, 25, 10, 15, 30, 123_000_000, time.UTC), `"2026-09-25T10:15:30.123Z"`},
		{"whole seconds still show three digits", time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC), `"2026-09-25T10:00:00.000Z"`},
		{"finer than a millisecond is cut, not rounded", time.Date(2026, time.September, 25, 10, 0, 0, 999_999_999, time.UTC), `"2026-09-25T10:00:00.999Z"`},
		{"another zone is converted to UTC", time.Date(2026, time.September, 25, 13, 15, 30, 5_000_000, plusThree), `"2026-09-25T10:15:30.005Z"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(protocol.NewTimestamp(tc.in))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestZeroTimestampDoesNotEncode(t *testing.T) {
	if _, err := json.Marshal(protocol.Timestamp{}); err == nil {
		t.Error("a zero timestamp encoded, want an error")
	}
	// A time that may be absent is a pointer, and it encodes as null.
	var absent *protocol.Timestamp
	got, err := json.Marshal(struct {
		Due *protocol.Timestamp `json:"due"`
	}{Due: absent})
	if err != nil || string(got) != `{"due":null}` {
		t.Errorf("got %s, %v; want {\"due\":null}", got, err)
	}
}

func TestTimestampDecodes(t *testing.T) {
	want := time.Date(2026, time.September, 25, 10, 15, 30, 123_000_000, time.UTC)
	tests := []struct {
		name string
		in   string
	}{
		{"the wire form", `"2026-09-25T10:15:30.123Z"`},
		{"finer than a millisecond is cut", `"2026-09-25T10:15:30.123456Z"`},
		{"an offset is converted to UTC", `"2026-09-25T13:15:30.123+03:00"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got protocol.Timestamp
			if err := json.Unmarshal([]byte(tc.in), &got); err != nil {
				t.Fatal(err)
			}
			if !got.Time().Equal(want) || got.Time().Location() != time.UTC {
				t.Errorf("got %v, want %v in UTC", got.Time(), want)
			}
		})
	}
}

func TestTimestampRejectsBadInput(t *testing.T) {
	for _, in := range []string{`""`, `"2026-09-25"`, `"yesterday"`, `12345`, `"0001-01-01T00:00:00Z"`} {
		var got protocol.Timestamp
		if err := json.Unmarshal([]byte(in), &got); err == nil {
			t.Errorf("%s decoded to %v, want an error", in, got.Time())
		}
	}
}

func TestTimestampNullLeavesTheValue(t *testing.T) {
	keep := protocol.NewTimestamp(time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC))
	got := keep
	if err := json.Unmarshal([]byte("null"), &got); err != nil {
		t.Fatal(err)
	}
	if got != keep {
		t.Errorf("null changed the value to %v", got.Time())
	}
}

func TestTimestampRoundTrips(t *testing.T) {
	in := protocol.NewTimestamp(time.Date(2026, time.December, 31, 23, 59, 59, 999_000_000, time.UTC))
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out protocol.Timestamp
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if !out.Time().Equal(in.Time()) {
		t.Errorf("round trip gave %v, want %v", out.Time(), in.Time())
	}
}
