package protocol_test

import (
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

func TestNetworkStatusGolden(t *testing.T) {
	testutil.Golden(t, "network-status", protocol.NetworkStatus{
		Online: false, Since: protocol.NewTimestamp(time.Date(2026, 11, 1, 9, 30, 0, 0, time.UTC)),
	})
}
