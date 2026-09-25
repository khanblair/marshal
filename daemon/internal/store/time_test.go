package store

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTimestampConversions(t *testing.T) {
	moment := time.Date(2026, 9, 25, 10, 15, 30, 123_456_789, time.FixedZone("EAT", 3*60*60))
	millis := Millis(moment)
	if want := int64(1_790_320_530_123); millis != want {
		t.Fatalf("Millis = %d, want %d", millis, want)
	}
	data, err := json.Marshal(Timestamp(millis))
	if err != nil {
		t.Fatal(err)
	}
	if want := `"2026-09-25T07:15:30.123Z"`; string(data) != want {
		t.Errorf("wire form = %s, want %s", data, want)
	}
	if got := Timestamp(millis).Time().Location(); got != time.UTC {
		t.Errorf("location = %v, want UTC", got)
	}
}

func TestOptionalTimestamp(t *testing.T) {
	if got := OptionalTimestamp(nil); got != nil {
		t.Errorf("OptionalTimestamp(nil) = %v, want nil", got)
	}
	millis := int64(1_000)
	got := OptionalTimestamp(&millis)
	if got == nil || got.Time().UnixMilli() != millis {
		t.Errorf("OptionalTimestamp = %v, want %d ms", got, millis)
	}
}
