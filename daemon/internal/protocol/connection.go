package protocol

// The wire shape of a connection test (docs/architecture.md section 18). Every adapter that can be
// tested - a model provider today, an integration, an MCP server, or a remote in later phases -
// answers with this one shape, so the screen that shows a test's result is written once.

import "time"

// CheckState is how one check of a connection test came out.
type CheckState string

const (
	// CheckStatePassed is a check that found what it wanted.
	CheckStatePassed CheckState = "passed"
	// CheckStateFailed is a check that found something wrong that a person must fix.
	CheckStateFailed CheckState = "failed"
	// CheckStateWarning is a check that found something worth noting that is not a failure, such
	// as a provider whose rate-limit headers Marshal cannot read.
	CheckStateWarning CheckState = "warning"
)

// CheckStateValues lists every check state, best first.
func CheckStateValues() []CheckState {
	return []CheckState{CheckStatePassed, CheckStateFailed, CheckStateWarning}
}

// Valid reports whether s is a check state.
func (s CheckState) Valid() bool {
	for _, v := range CheckStateValues() {
		if v == s {
			return true
		}
	}
	return false
}

// TestCheck is one thing a connection test looked at. Message is one plain sentence for the person;
// Fix is one plain sentence saying what to do when the check did not pass, and is empty when there
// is nothing to do.
type TestCheck struct {
	// Name is the short label of the check, such as "Key" or "Model".
	Name string `json:"name"`
	// State is how it came out.
	State CheckState `json:"state"`
	// Message is one plain sentence saying what was found.
	Message string `json:"message"`
	// Fix says what to do about a check that did not pass. Empty when there is nothing to do.
	Fix string `json:"fix,omitempty"`
}

// TestResult is what one connection test found. It is the answer to a test call and the thing a
// connection's last result is stored as (docs/architecture.md section 18: results are saved in
// integrations.last_test_result_json).
type TestResult struct {
	// ConnectionID is the id of the connection that was tested: a provider id today, such as
	// "anthropic".
	ConnectionID string `json:"connectionId"`
	// Checks is what the test looked at, in the order the screen shows them.
	Checks []TestCheck `json:"checks"`
	// OK is true when no check failed. A warning does not make a result not OK.
	OK bool `json:"ok"`
	// RanAt is when the test ran.
	RanAt Timestamp `json:"ranAt"`
}

// NewTestResult builds a result from its checks and the time it ran, and decides OK: no check
// failed. A nil check list becomes an empty one, so the JSON has [] and never null.
func NewTestResult(connectionID string, checks []TestCheck, ranAt time.Time) TestResult {
	out := make([]TestCheck, len(checks))
	copy(out, checks)
	ok := true
	for _, check := range out {
		if check.State == CheckStateFailed {
			ok = false
			break
		}
	}
	return TestResult{ConnectionID: connectionID, Checks: out, OK: ok, RanAt: NewTimestamp(ranAt)}
}

// FirstFailed returns the first check that failed, and whether there was one. It is how a caller
// turns a result into the one sentence a provider row shows when its key is refused.
func (r TestResult) FirstFailed() (TestCheck, bool) {
	for _, check := range r.Checks {
		if check.State == CheckStateFailed {
			return check, true
		}
	}
	return TestCheck{}, false
}
