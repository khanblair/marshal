package protocol_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

func TestBypassRequestGolden(t *testing.T) {
	testutil.Golden(t, "bypass-request", protocol.BypassRequest{Acknowledged: true})
}

// TestBypassRequestAlwaysSaysWhetherItWasAcknowledged keeps the one field a decoded request can be
// wrong about out of the omissible set: a client that forgets to send it must be refused, and the
// daemon can only refuse what it can see.
func TestBypassRequestAlwaysSaysWhetherItWasAcknowledged(t *testing.T) {
	for _, in := range []protocol.BypassRequest{{}, {Acknowledged: true}} {
		got, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(got), `"acknowledged":`) {
			t.Errorf("json.Marshal(%+v) = %s, and it must always carry the acknowledged field", in, got)
		}
	}
}

func TestBypassRefusalReasonsAreTheOnesTheAppReads(t *testing.T) {
	want := []string{"unacknowledged", "locked"}
	got := names(protocol.BypassRefusalReasonValues())
	if !equalStrings(got, want) {
		t.Errorf("BypassRefusalReasonValues() = %v, want %v", got, want)
	}
	if !protocol.BypassRefusalReasonLocked.Valid() || protocol.BypassRefusalReason("nope").Valid() {
		t.Error("BypassRefusalReason.Valid accepts the wrong words")
	}
}
