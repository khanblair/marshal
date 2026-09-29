package protocol_test

import (
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The wire shape of the alert settings (B9.4): where each alert goes, and which channels can be
// used. The Go test writes the golden file and the web client's tests read the same one.

func TestAlertSettingsGolden(t *testing.T) {
	testutil.Golden(t, "alert-settings", protocol.AlertSettings{
		Routes: []protocol.AlertRoute{
			{Event: "approval.requested", Label: "Needs your approval", Channels: []string{"telegram", "discord"}},
			{Event: "agent.stuck", Label: "An agent is stuck or needs you", Channels: []string{"telegram"}},
			{Event: "ci.failed", Label: "CI failed", Channels: []string{}},
		},
		Channels: []protocol.AlertChannel{
			{ID: "telegram", Name: "Telegram", Connected: true},
			{ID: "discord", Name: "Discord", Connected: false},
			{ID: "ntfy", Name: "ntfy", Connected: true},
		},
		ServerTime: protocol.NewTimestamp(scheduleNow),
	})
}

func TestSaveAlertSettingsRequestGolden(t *testing.T) {
	testutil.Golden(t, "save-alert-settings-request", protocol.SaveAlertSettingsRequest{
		Routes: []protocol.AlertRouteChoice{{Event: "ci.failed", Channels: []string{"ntfy"}}},
	})
}

func TestSaveNtfyRequestGolden(t *testing.T) {
	testutil.Golden(t, "save-ntfy-request", protocol.SaveNtfyRequest{
		Server: "https://ntfy.example.com", Topic: "marshal-7f3a9c", Token: "",
	})
}
