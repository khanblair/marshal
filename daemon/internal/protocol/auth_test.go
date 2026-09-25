package protocol_test

import (
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

func TestWhoAmIGolden(t *testing.T) {
	testutil.Golden(t, "whoami", protocol.WhoAmI{
		DeviceID:   "01M3C107JD041061050R3GG28A",
		DeviceKind: protocol.DeviceKindCLI,
		UserID:     "01M3C107JE0R3GG28A04106105",
		Mode:       "normal",
		ServerTime: protocol.NewTimestamp(time.Date(2026, time.September, 25, 10, 15, 30, 123_000_000, time.UTC)),
	})
}

func TestDeviceKindsMatchTheDevicesTable(t *testing.T) {
	// The same five words as the CHECK on devices.kind in the store's first migration.
	want := []protocol.DeviceKind{"web", "desktop", "mobile", "cli", "dev"}
	got := protocol.DeviceKindValues()
	if len(got) != len(want) {
		t.Fatalf("device kinds = %v, want %v", got, want)
	}
	for i, kind := range want {
		if got[i] != kind || !kind.Valid() {
			t.Errorf("device kind %d = %q, want %q", i, got[i], kind)
		}
	}
	if protocol.DeviceKind("phone").Valid() {
		t.Error("DeviceKind accepts a word the devices table refuses")
	}
}
