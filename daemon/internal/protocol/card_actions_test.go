package protocol_test

import (
	"encoding/json"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

func TestSendMessageRequestGolden(t *testing.T) {
	testutil.Golden(t, "send-message-request", protocol.SendMessageRequest{
		Text: "Please also add a test for the empty case.",
	})
}

// The text is sent as it is: an empty message is refused by the daemon, not dropped from the JSON,
// so the refusal can say what is missing.
func TestSendMessageRequestAlwaysHasItsText(t *testing.T) {
	got, err := json.Marshal(protocol.SendMessageRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"text":""}`; string(got) != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
}

func TestMaxMessageCharsIsTheDocumentedLimit(t *testing.T) {
	if protocol.MaxMessageChars != 100000 {
		t.Errorf("MaxMessageChars = %d, want 100000", protocol.MaxMessageChars)
	}
}
