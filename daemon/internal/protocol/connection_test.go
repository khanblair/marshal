package protocol_test

import (
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// testResultNow is the fixed time every test-result golden is stamped with, so the file does not
// change from run to run.
var testResultNow = time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)

func TestTestResultGolden(t *testing.T) {
	result := protocol.NewTestResult("anthropic", []protocol.TestCheck{
		{Name: "Key", State: protocol.CheckStatePassed, Message: "The provider accepted the key."},
		{Name: "Model", State: protocol.CheckStatePassed, Message: "Claude Sonnet 4.5 answered."},
	}, testResultNow)
	testutil.Golden(t, "test-result", result)
}

func TestTestResultFailedGolden(t *testing.T) {
	result := protocol.NewTestResult("openai", []protocol.TestCheck{
		{Name: "Key", State: protocol.CheckStateFailed, Message: "The provider refused the key.",
			Fix: "Check the key, then save it again."},
	}, testResultNow)
	testutil.Golden(t, "test-result-failed", result)
}

// The Obsidian vault connection's own test result (section S29b, build-plan task 7.7): a summary
// check first, then the two the vault folder itself answers (daemon/internal/integrations/
// obsidian.go), all passed - the shape a vault that is there and writable produces.
func TestTestResultObsidianGolden(t *testing.T) {
	result := protocol.NewTestResult("obsidian", []protocol.TestCheck{
		{Name: "Summary", State: protocol.CheckStatePassed, Message: "Marshal's vault is ready to open in Obsidian."},
		{Name: "Vault folder", State: protocol.CheckStatePassed, Message: "The vault folder is at /Users/person/Notes/Marshal."},
		{Name: "Vault writable", State: protocol.CheckStatePassed, Message: "The vault folder's permissions let Marshal write in it."},
	}, testResultNow)
	testutil.Golden(t, "test-result-obsidian", result)
}

func TestNewTestResultIsOKWhenNoCheckFailed(t *testing.T) {
	tests := []struct {
		name   string
		checks []protocol.TestCheck
		want   bool
	}{
		{"no checks", nil, true},
		{"all passed", []protocol.TestCheck{{State: protocol.CheckStatePassed}}, true},
		{"a warning", []protocol.TestCheck{{State: protocol.CheckStateWarning}}, true},
		{"a failure", []protocol.TestCheck{{State: protocol.CheckStatePassed}, {State: protocol.CheckStateFailed}}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := protocol.NewTestResult("x", tc.checks, testResultNow)
			if got.OK != tc.want {
				t.Errorf("OK = %v, want %v", got.OK, tc.want)
			}
			if got.ConnectionID != "x" {
				t.Errorf("ConnectionID = %q, want x", got.ConnectionID)
			}
			if got.RanAt.Time() != testResultNow {
				t.Errorf("RanAt = %v, want %v", got.RanAt.Time(), testResultNow)
			}
		})
	}
}

func TestNewTestResultNeverHasANilCheckList(t *testing.T) {
	if got := protocol.NewTestResult("x", nil, testResultNow); got.Checks == nil {
		t.Error("Checks = nil, want an empty list so the JSON has []")
	}
}

func TestFirstFailed(t *testing.T) {
	result := protocol.NewTestResult("x", []protocol.TestCheck{
		{Name: "Key", State: protocol.CheckStatePassed},
		{Name: "Model", State: protocol.CheckStateWarning},
		{Name: "Limits", State: protocol.CheckStateFailed, Message: "no"},
	}, testResultNow)
	check, ok := result.FirstFailed()
	if !ok || check.Name != "Limits" {
		t.Fatalf("FirstFailed = %+v %v, want the Limits check", check, ok)
	}
	if _, ok := protocol.NewTestResult("x", []protocol.TestCheck{{State: protocol.CheckStatePassed}}, testResultNow).FirstFailed(); ok {
		t.Error("FirstFailed found a failure in a result that has none")
	}
}
