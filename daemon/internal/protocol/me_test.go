package protocol_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The golden samples of the person using Marshal and of a project's saved views
// (docs/backend-checklist.md B2.2, B2.5, and B2.13, inventory N20, N27, and N30). Every field is
// set in at least one of them, and both a null and a value appear for each field that can be null,
// so a wire shape that changes shows up here, and the app's own mapper test reads the same files.

// meTime is the moment the samples are stamped with.
func meTime() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) }

const (
	sampleUserID = "01M3C107JB041061050R3GG2U1"
	sampleViewID = "01M3C107JB041061050R3GG2V1"
)

func sampleProfile() protocol.Profile {
	avatar := "/v1/users/" + sampleUserID + "/avatar?v=1759233600000"
	return protocol.Profile{
		ID: sampleUserID, Name: "Ada Okafor", Email: "ada@example.com", Initials: "AO",
		TimeZone: "Europe/London", AvatarURL: &avatar,
		CreatedAt: protocol.NewTimestamp(meTime().Add(-72 * time.Hour)),
		UpdatedAt: protocol.NewTimestamp(meTime()),
	}
}

func sampleProgress() protocol.Progress {
	skipped := protocol.NewTimestamp(meTime().Add(-time.Hour))
	return protocol.Progress{
		Onboarding: protocol.OnboardingProgress{Status: protocol.ProgressStatusPending, Step: 2},
		Tutorial:   protocol.TutorialProgress{Status: protocol.ProgressStatusSkipped, FinishedAt: &skipped},
	}
}

func samplePreferences() protocol.Preferences {
	view := sampleViewID
	return protocol.Preferences{
		Theme:       protocol.ThemeDark,
		ListColumns: map[string]bool{"pkg": true, "cost": false},
		Sort: protocol.SortPreferences{
			List: &protocol.SortOrder{Key: "id", Direction: protocol.SortDirectionDesc},
		},
		Projects: map[string]protocol.ProjectPreferences{
			"api": {
				LastView: protocol.ProjectViewList,
				Filters:  []protocol.Filter{{Key: protocol.FilterKeyStatus, Value: "needs"}},
				Query:    "cache", Swimlane: protocol.SwimlaneRole,
				CollapsedLanes: []string{"role:Tester"}, ShowAllDone: true, SavedViewID: &view,
			},
			"web": {
				LastView: protocol.ProjectViewBoard, Filters: []protocol.Filter{},
				Swimlane: protocol.SwimlaneNone, CollapsedLanes: []string{},
			},
		},
	}
}

func sampleSavedView() protocol.SavedView {
	return protocol.SavedView{
		ID: sampleViewID, ProjectID: "api", Name: "Needs me",
		Filters:   []protocol.Filter{{Key: protocol.FilterKeyStatus, Value: "needs"}},
		Swimlane:  protocol.SwimlaneNone,
		CreatedAt: protocol.NewTimestamp(meTime().Add(-48 * time.Hour)),
		UpdatedAt: protocol.NewTimestamp(meTime().Add(-time.Hour)),
	}
}

func byAgentView() protocol.SavedView {
	return protocol.SavedView{
		ID: "01M3C107JB041061050R3GG2V2", ProjectID: "api", Name: "Claude Code by role",
		Filters:   []protocol.Filter{{Key: protocol.FilterKeyAgent, Value: "Claude Code"}},
		Swimlane:  protocol.SwimlaneRole,
		CreatedAt: protocol.NewTimestamp(meTime().Add(-30 * time.Minute)),
		UpdatedAt: protocol.NewTimestamp(meTime().Add(-30 * time.Minute)),
	}
}

// meEvents is one batch with the two events this slice sends, each on its own topic.
func meEvents(t *testing.T) protocol.EventBatch {
	t.Helper()
	at := func(seq uint64) protocol.Timestamp {
		return protocol.NewTimestamp(meTime().Add(time.Duration(seq) * 20 * time.Millisecond))
	}
	return protocol.EventBatch{
		Epoch: "01M3C0ZZZZ000000000000000A",
		Events: []protocol.Event{
			{
				Seq: 1, Topic: protocol.MeTopic, Type: protocol.EventTypeMeUpdated, At: at(1),
				Data: encodeData(t, protocol.MeUpdatedEventData{
					Profile: sampleProfile(), Preferences: samplePreferences(), Progress: sampleProgress(),
				}),
			},
			{
				Seq: 2, Topic: protocol.ProjectTopic("api"), Type: protocol.EventTypeSavedViewUpdated, At: at(2),
				Data: encodeData(t, protocol.SavedViewUpdatedEventData{
					ProjectID: "api", Views: []protocol.SavedView{sampleSavedView(), byAgentView()},
				}),
			},
		},
	}
}

func TestMeGolden(t *testing.T) {
	name, email, zone := "Ada Okafor", "", "Africa/Lagos"
	testutil.Golden(t, "profile", sampleProfile())
	testutil.Golden(t, "update-profile-request", protocol.UpdateProfileRequest{Name: &name, Email: &email, TimeZone: &zone})
	testutil.Golden(t, "user-list", protocol.UserListSnapshot{
		Users: []protocol.User{
			{ID: sampleUserID, Name: "Ada Okafor", Initials: "AO", AvatarURL: sampleProfile().AvatarURL},
			{ID: "01M3C107JB041061050R3GG2U2", Name: "Sam Reyes", Initials: "SR"},
		},
		ServerTime: protocol.NewTimestamp(meTime()),
	})
	testutil.Golden(t, "progress", sampleProgress())
	step, done, pending := 3, protocol.ProgressStatusDone, protocol.ProgressStatusPending
	testutil.Golden(t, "update-progress-request", protocol.UpdateProgressRequest{
		Onboarding: &protocol.UpdateOnboardingProgress{Step: &step, Status: &done},
		Tutorial:   &protocol.UpdateTutorialProgress{Status: &pending},
	})
	testutil.Golden(t, "preferences", samplePreferences())
	testutil.Golden(t, "update-preferences-request", updatePreferencesSample())
	testutil.Golden(t, "me-events", meEvents(t))
}

func updatePreferencesSample() protocol.UpdatePreferencesRequest {
	theme, view, swim, query, clear := protocol.ThemeLight, protocol.ProjectViewTimeline, protocol.SwimlanePackage, "", ""
	filters := []protocol.Filter{{Key: protocol.FilterKeyLabel, Value: "ui"}}
	lanes := []string{"package:packages/api-client"}
	all := false
	return protocol.UpdatePreferencesRequest{
		Theme:       &theme,
		ListColumns: map[string]bool{"think": true},
		Sort: &protocol.UpdateSortPreferences{
			Agents: &protocol.SortOrder{Key: "state", Direction: protocol.SortDirectionAsc},
		},
		Projects: map[string]protocol.UpdateProjectPreferences{
			"mobile": {
				LastView: &view, Filters: &filters, Query: &query, Swimlane: &swim,
				CollapsedLanes: &lanes, ShowAllDone: &all, SavedViewID: &clear,
			},
		},
	}
}

func TestSavedViewGolden(t *testing.T) {
	testutil.Golden(t, "saved-view", sampleSavedView())
	testutil.Golden(t, "saved-view-list", protocol.SavedViewListSnapshot{
		ProjectID: "api", Views: []protocol.SavedView{sampleSavedView(), byAgentView()},
		ServerTime: protocol.NewTimestamp(meTime()),
	})
	testutil.Golden(t, "create-saved-view-request", protocol.CreateSavedViewRequest{
		Name: "UI work", Filters: []protocol.Filter{{Key: protocol.FilterKeyLabel, Value: "ui"}},
		Swimlane: protocol.SwimlaneAgent,
	})
	name, swim := "Needs me now", protocol.SwimlaneLabel
	filters := []protocol.Filter{}
	testutil.Golden(t, "update-saved-view-request", protocol.UpdateSavedViewRequest{
		Name: &name, Filters: &filters, Swimlane: &swim,
	})
}

// The words below are the ones the app's screens use (apps/web/src/mock/types.ts: Theme, ViewKey,
// SwimKey, and FilterKey), so the mapper passes them through unchanged. A change here is a change on
// both sides.
func TestPreferenceWordsAreTheScreensWords(t *testing.T) {
	want := map[string][]string{
		"Theme":          {"light", "dark", "system"},
		"ProjectView":    {"chat", "agents", "board", "list", "timeline", "calendar"},
		"Swimlane":       {"none", "role", "agent", "package", "label"},
		"FilterKey":      {"status", "role", "agent", "model", "label", "package"},
		"SortDirection":  {"asc", "desc"},
		"ProgressStatus": {"pending", "done", "skipped"},
	}
	got := allEnums()
	for name, values := range want {
		if !equalStrings(got[name], values) {
			t.Errorf("%s = %v, want %v", name, got[name], values)
		}
	}
	checks := []struct {
		name  string
		valid bool
	}{
		{"light theme", protocol.ThemeLight.Valid()},
		{"a theme that is not one", !protocol.Theme("sepia").Valid()},
		{"the list view", protocol.ProjectViewList.Valid()},
		{"a view that is not one", !protocol.ProjectView("kanban").Valid()},
		{"the label swimlane", protocol.SwimlaneLabel.Valid()},
		{"a swimlane that is not one", !protocol.Swimlane("owner").Valid()},
		{"the model filter", protocol.FilterKeyModel.Valid()},
		{"a filter that is not one", !protocol.FilterKey("cost").Valid()},
		{"descending", protocol.SortDirectionDesc.Valid()},
		{"a direction that is not one", !protocol.SortDirection("-1").Valid()},
		{"skipped", protocol.ProgressStatusSkipped.Valid()},
		{"a status that is not one", !protocol.ProgressStatus("started").Valid()},
	}
	for _, check := range checks {
		if !check.valid {
			t.Errorf("%s: Valid answered wrongly", check.name)
		}
	}
}

// The me topic has no id, like home, and anything after it is junk.
func TestTheMeTopic(t *testing.T) {
	if protocol.MeTopic != "me" {
		t.Fatalf("MeTopic = %q, want me", protocol.MeTopic)
	}
	kind, id, err := protocol.ParseTopic(protocol.MeTopic)
	if err != nil || kind != protocol.TopicKindMe || id != "" {
		t.Errorf("ParseTopic(me) = %q, %q, %v", kind, id, err)
	}
	for _, junk := range []protocol.Topic{"me:", "me:" + sampleUserID, "Me", " me", "me "} {
		if _, _, err := protocol.ParseTopic(junk); err == nil {
			t.Errorf("ParseTopic(%q) accepted junk", junk)
		}
	}
}

func TestCheckFilters(t *testing.T) {
	ok := []protocol.Filter{{Key: protocol.FilterKeyStatus, Value: "needs"}, {Key: protocol.FilterKeyLabel, Value: "ui"}}
	if err := protocol.CheckFilters(ok); err != nil {
		t.Errorf("CheckFilters refused good filters: %v", err)
	}
	if err := protocol.CheckFilters(nil); err != nil {
		t.Errorf("CheckFilters refused no filters: %v", err)
	}
	tooMany := make([]protocol.Filter, 51)
	for i := range tooMany {
		tooMany[i] = protocol.Filter{Key: protocol.FilterKeyRole, Value: "Tester"}
	}
	bad := map[string]struct {
		filters []protocol.Filter
		message string
	}{
		"an unknown key": {
			[]protocol.Filter{{Key: "cost", Value: "1"}},
			"That is not a filter Marshal knows. Filter by status, role, agent, model, label, or package.",
		},
		"an empty value": {
			[]protocol.Filter{{Key: protocol.FilterKeyRole}},
			"A filter needs a value of at most 200 characters.",
		},
		"a long value": {
			[]protocol.Filter{{Key: protocol.FilterKeyRole, Value: strings.Repeat("x", 201)}},
			"A filter needs a value of at most 200 characters.",
		},
		"too many": {tooMany, "A board can have at most 50 filters. Remove some and try again."},
	}
	for name, tc := range bad {
		err := protocol.CheckFilters(tc.filters)
		var perr *protocol.Error
		if !errors.As(err, &perr) || perr.Code != protocol.ErrorCodeInvalidArgument || perr.Message != tc.message {
			t.Errorf("%s: CheckFilters = %v, want invalid_argument %q", name, err, tc.message)
		}
	}
}
