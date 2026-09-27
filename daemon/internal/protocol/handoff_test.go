package protocol_test

import (
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// A handoff carries the agent to continue on and the summary of where the work got to
// (docs/marshal-product-scope.md section 10.5).
func TestHandoffRequestGolden(t *testing.T) {
	testutil.Golden(t, "handoff-request", protocol.HandoffRequest{
		To: protocol.AgentKindGemini,
		Summary: "Goal: add a health check endpoint.\n" +
			"Done: the handler and its test.\n" +
			"Left: register the route and document it.\n" +
			"Decision: the version comes from the build, not a constant.",
	})
}

// A handoff with no summary leaves the field out, which is what a client sends when it hands a card
// to another agent without one: the new agent then starts with the card's own context alone.
func TestHandoffRequestWithoutASummaryGolden(t *testing.T) {
	testutil.Golden(t, "handoff-request-empty", protocol.HandoffRequest{To: protocol.AgentKindCodex})
}

// Every reason the daemon can refuse a handoff for is a reason a client can name, and nothing else
// is.
func TestHandoffRefusalReasonsAreTheOnesAClientMayName(t *testing.T) {
	for _, reason := range protocol.HandoffRefusalReasonValues() {
		if !reason.Valid() {
			t.Errorf("%q is listed as a reason but Valid says it is not one", reason)
		}
	}
	if protocol.HandoffRefusalReason("handoff_whenever").Valid() {
		t.Error("a made-up reason is reported as a handoff refusal reason")
	}
	if protocol.HandoffRefusalReason("").Valid() {
		t.Error("the empty reason is reported as a handoff refusal reason")
	}
}
