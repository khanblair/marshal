package protocol_test

import (
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

func TestHealthGolden(t *testing.T) {
	testutil.Golden(t, "health", protocol.Health{
		Status:     "ok",
		Version:    "0.0.0",
		Mode:       "dev",
		ServerTime: time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC),
	})
}
