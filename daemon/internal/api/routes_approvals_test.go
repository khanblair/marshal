package api_test

import (
	"crypto/rand"
	"net/http"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The approval route: POST /v1/approvals/{id} answers a waiting approval (B3.4). The answer the
// route gives for a request nobody can be waiting on is the point here; the flow that actually
// creates one is covered where the sessions live (internal/session's approval tests).
func TestDecideApprovalRoute(t *testing.T) {
	st := newStack(t)
	id, err := protocol.NewID(time.Now(), rand.Reader)
	if err != nil {
		t.Fatalf("make an id: %v", err)
	}

	// A decision that is not one of the two is refused before the request is looked up.
	got := st.do(http.MethodPost, "/v1/approvals/"+id, map[string]any{"decision": "maybe"})
	if got.Status != http.StatusBadRequest {
		t.Errorf("a bad decision answered %d, want 400: %s", got.Status, got.Body)
	}

	// An approval nobody is waiting on is not found.
	got = st.do(http.MethodPost, "/v1/approvals/"+id, protocol.DecideApprovalRequest{Decision: protocol.ApprovalDecisionApproved})
	if got.Status != http.StatusNotFound {
		t.Errorf("an unknown approval answered %d, want 404: %s", got.Status, got.Body)
	}

	// An id that cannot be one is not found without asking anything.
	got = st.do(http.MethodPost, "/v1/approvals/not-an-id", protocol.DecideApprovalRequest{Decision: protocol.ApprovalDecisionDenied})
	if got.Status != http.StatusNotFound {
		t.Errorf("a malformed id answered %d, want 404: %s", got.Status, got.Body)
	}
}
