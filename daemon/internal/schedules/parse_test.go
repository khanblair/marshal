package schedules_test

import (
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/schedules"
)

func TestParseCron(t *testing.T) {
	tests := []struct {
		when     string
		timeStr  string
		days     []int
		expected string
	}{
		{"Every weekday at 09:00", "09:00", []int{1, 2, 3, 4, 5}, "0 9 * * 1,2,3,4,5"},
		{"Daily at 17:30", "17:30", nil, "30 17 * * *"},
		{"Every 15 minutes", "", nil, "*/15 * * * *"},
		{"Every 2 hours", "", nil, "0 */2 * * *"},
		{"Custom weekday", "08:15", nil, "15 8 * * 1-5"},
	}

	for _, tt := range tests {
		got, err := schedules.ParseCron(tt.when, tt.timeStr, tt.days)
		if err != nil {
			t.Errorf("ParseCron(%q, %q, %v) unexpected error: %v", tt.when, tt.timeStr, tt.days, err)
		}
		if got != tt.expected {
			t.Errorf("ParseCron(%q, %q, %v) = %q, want %q", tt.when, tt.timeStr, tt.days, got, tt.expected)
		}
	}
}

func TestParseOneTime(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		when     string
		timeStr  string
		expected string
	}{
		{"On 1 October at 14:00", "14:00", "0 14 1 10 *"},
		{"On October 1 at 14:00", "14:00", "0 14 1 10 *"},
		{"On 25 December", "09:00", "0 9 25 12 *"},
	}
	for _, tt := range tests {
		got := schedules.ParseOneTime(tt.when, tt.timeStr, now)
		if got != tt.expected {
			t.Errorf("ParseOneTime(%q, %q) = %q, want %q", tt.when, tt.timeStr, got, tt.expected)
		}
	}
}

// TestParseOneTimeFallsBackWhenNoDateIsFound proves an unreadable sentence still answers a spec -
// tomorrow, at whatever clock time it found - rather than losing what was typed.
func TestParseOneTimeFallsBackWhenNoDateIsFound(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := schedules.ParseOneTime("Next Tuesday sometime", "14:00", now)
	want := "0 14 2 1 *"
	if got != want {
		t.Errorf("ParseOneTime with no date = %q, want %q (tomorrow)", got, want)
	}
}
