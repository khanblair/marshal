package protocol_test

import (
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

func TestSaveResendRequestGolden(t *testing.T) {
	testutil.Golden(t, "save-resend-request", protocol.SaveResendRequest{
		Key: "", From: "Marshal <marshal@kolaborate.co>",
	})
}
