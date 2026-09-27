package protocol_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The wire shape of a notice and of the sleep settings (B5.6, inventory N5). A notice is what the
// shell's notices panel draws and what GET /v1/notices answers with; the sleep kind is the one
// Phase 5 produces.

// sampleNotice is a sleep notice naming two idle cards of one project, with the moment they sleep.
func sampleNotice() protocol.Notice {
	deadline := protocol.NewTimestamp(providersNow.Add(2 * time.Minute))
	return protocol.Notice{
		ID:        "sleep:web-dashboard",
		Kind:      protocol.NoticeKindSleep,
		Cards:     []string{"01JD7Q4M2X8K9V0P5T3RB6NHC3", "01JD7Q4M2X8K9V0P5T3RB6NHC4"},
		Deadline:  &deadline,
		ProjectID: "web-dashboard",
		CreatedAt: protocol.NewTimestamp(providersNow),
	}
}

func TestNoticeGolden(t *testing.T) {
	testutil.Golden(t, "notice", sampleNotice())
}

func TestNoticeListGolden(t *testing.T) {
	testutil.Golden(t, "notice-list", protocol.NewNoticeList([]protocol.Notice{sampleNotice()}))
}

// A fresh install has no notices, and the answer is [] rather than null, so a client never has to
// handle both an empty list and a missing one.
func TestNoticeListNeverEncodesNull(t *testing.T) {
	list, err := json.Marshal(protocol.NewNoticeList(nil))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(list); !strings.Contains(got, `"notices":[]`) {
		t.Errorf("an empty notice list encoded as %s, want []", got)
	}
}

// An informational notice carries its own two sentences and names no card.
func TestAnInformationalNoticeCarriesItsText(t *testing.T) {
	notice := protocol.Notice{
		ID: "ci", Kind: protocol.NoticeKindCI, Text: "The checks on main failed",
		Sub: "Open the run to see the step", CreatedAt: protocol.NewTimestamp(providersNow),
	}
	body, err := json.Marshal(notice)
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	for _, want := range []string{`"text":"The checks on main failed"`, `"sub":"Open the run to see the step"`} {
		if !strings.Contains(got, want) {
			t.Errorf("an informational notice encoded as %s, want %s", got, want)
		}
	}
	if strings.Contains(got, `"cards"`) || strings.Contains(got, `"deadline"`) {
		t.Errorf("an informational notice carried a sleep notice's own fields: %s", got)
	}
}

func TestNoticeActionRequestGolden(t *testing.T) {
	testutil.Golden(t, "notice-action-request", protocol.NoticeActionRequest{
		Action: protocol.NoticeActionKeepAwake, CardID: "01JD7Q4M2X8K9V0P5T3RB6NHC3",
	})
}

// A whole-notice action names no card, so its body carries only the action.
func TestAWholeNoticeActionNamesNoCard(t *testing.T) {
	body, err := json.Marshal(protocol.NoticeActionRequest{Action: protocol.NoticeActionSleepAll})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); got != `{"action":"sleep-all"}` {
		t.Errorf("a whole-notice action encoded as %s", got)
	}
}

func TestNoticeActionResultGolden(t *testing.T) {
	testutil.Golden(t, "notice-action-result", protocol.NoticeActionResult{Cards: 2})
}

func TestSleepSettingsGolden(t *testing.T) {
	testutil.Golden(t, "sleep-settings", protocol.DefaultSleepSettings())
}

// The four action names and the two restore answers are stable strings, because the app sends one
// and reads the other.
func TestTheNoticeActionsAndRestoreAnswersAreStable(t *testing.T) {
	actions := map[string]string{
		protocol.NoticeActionKeepAwake: "keep-awake",
		protocol.NoticeActionSleepNow:  "sleep-now",
		protocol.NoticeActionKeepAll:   "keep-all",
		protocol.NoticeActionSleepAll:  "sleep-all",
	}
	for got, want := range actions {
		if got != want {
			t.Errorf("notice action = %q, want %q", got, want)
		}
	}
	if protocol.SleepRestoreAuto != "auto" || protocol.SleepRestoreManual != "manual" {
		t.Errorf("restore answers = %q/%q, want auto/manual",
			protocol.SleepRestoreAuto, protocol.SleepRestoreManual)
	}
	if protocol.SleepChannelInApp != "in-app" {
		t.Errorf("in-app channel = %q", protocol.SleepChannelInApp)
	}
}
