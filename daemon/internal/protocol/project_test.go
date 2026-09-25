package protocol_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

const (
	sampleCardID    = "01M3C107JB041061050R3GG28A"
	sampleProjectID = "web-dashboard"
)

var sampleTime = time.Date(2026, time.September, 25, 10, 15, 30, 123_000_000, time.UTC)

func sampleProject() protocol.Project {
	return protocol.Project{
		ID:            sampleProjectID,
		Name:          "web-dashboard",
		Path:          "/home/ada/code/web-dashboard",
		Language:      "TypeScript",
		DefaultBranch: "main",
		DevCommand:    "pnpm dev",
		BypassLocked:  true,
		IsMonorepo:    false,
		Packages:      []string{},
		CreatedAt:     protocol.NewTimestamp(sampleTime),
		Badges:        protocol.ProjectBadges{Needs: 2, Awake: 1},
	}
}

func sampleMonorepo() protocol.Project {
	p := sampleProject()
	p.ID, p.Name, p.Path = "mobile", "mobile-app", "/home/ada/code/mobile-app"
	p.Language, p.IsMonorepo, p.DevCommand, p.BypassLocked = "Monorepo", true, "", false
	p.Packages = []string{"apps/android", "apps/ios", "packages/ui"}
	p.Badges = protocol.ProjectBadges{}
	return p
}

func sampleCard() protocol.Card {
	thinking := protocol.ThinkingModeHigh
	return protocol.Card{
		ID:             sampleCardID,
		ProjectID:      sampleProjectID,
		Number:         12,
		Key:            protocol.CardKey{ProjectID: sampleProjectID, Number: 12}.String(),
		Title:          "Add a health check endpoint",
		Body:           "Serve GET /health with the version.",
		State:          protocol.CardStateBacklog,
		Agent:          protocol.AgentKindClaude,
		Model:          "claude-sonnet-4-5",
		Thinking:       &thinking,
		PermissionMode: protocol.PermissionModeAutoEdits,
		Branch:         "",
		CreatedAt:      protocol.NewTimestamp(sampleTime),
		UpdatedAt:      protocol.NewTimestamp(sampleTime.Add(time.Minute)),
	}
}

func TestProjectGolden(t *testing.T) {
	testutil.Golden(t, "project", sampleProject())
}

func TestProjectListSnapshotGolden(t *testing.T) {
	testutil.Golden(t, "project-list", protocol.ProjectListSnapshot{
		Projects:   []protocol.Project{sampleProject(), sampleMonorepo()},
		ServerTime: protocol.NewTimestamp(sampleTime.Add(time.Hour)),
	})
}

func TestCreateProjectRequestGolden(t *testing.T) {
	testutil.Golden(t, "create-project-request", protocol.CreateProjectRequest{
		Source: protocol.ProjectSourceClone,
		URL:    "https://github.com/acme/web-dashboard.git",
		Dest:   "/home/ada/code/web-dashboard",
		Branch: "develop",
		Name:   "Web dashboard",
	})
}

func TestCreateProjectRequestFromAFolderLeavesTheCloneFieldsOut(t *testing.T) {
	got, err := json.Marshal(protocol.CreateProjectRequest{Source: protocol.ProjectSourceFolder, Path: "/code/api"})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"source":"folder","path":"/code/api"}`; string(got) != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
}

func TestUpdateProjectRequestGolden(t *testing.T) {
	name, dev, branch, locked := "Dashboard", "npm run dev", "develop", false
	testutil.Golden(t, "update-project-request", protocol.UpdateProjectRequest{
		Name: &name, DevCommand: &dev, DefaultBranch: &branch, BypassLocked: &locked,
	})
}

// A field that is left out is not changed, but a field that is set to its zero value is a change:
// an empty dev command clears it, and false turns the lock off.
func TestUpdateProjectRequestSendsOnlyWhatIsSet(t *testing.T) {
	empty, off := "", false
	tests := []struct {
		name string
		in   protocol.UpdateProjectRequest
		want string
	}{
		{"nothing", protocol.UpdateProjectRequest{}, `{}`},
		{"a cleared dev command", protocol.UpdateProjectRequest{DevCommand: &empty}, `{"devCommand":""}`},
		{"the lock turned off", protocol.UpdateProjectRequest{BypassLocked: &off}, `{"bypassLocked":false}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.in)
			if err != nil || string(got) != tc.want {
				t.Errorf("got %s, %v; want %s", got, err, tc.want)
			}
		})
	}
}

func TestRemoveProjectRequestGolden(t *testing.T) {
	testutil.Golden(t, "remove-project-request", protocol.RemoveProjectRequest{KeepBranches: true, KeepMemory: false})
}

func TestCardGolden(t *testing.T) {
	testutil.Golden(t, "card", sampleCard())
}

func TestCardWithoutThinkingSendsNull(t *testing.T) {
	card := sampleCard()
	card.Thinking = nil
	got, err := json.Marshal(card)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(got, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["thinking"]) != "null" {
		t.Errorf("thinking = %s, want null", fields["thinking"])
	}
}

func TestBoardSnapshotGolden(t *testing.T) {
	second := sampleCard()
	second.ID, second.Number = "01M3C107JC0R3GG28A04106105", 13
	second.Key = protocol.CardKey{ProjectID: sampleProjectID, Number: 13}.String()
	second.Title, second.Body = "Fix the typo in the README", ""
	second.State, second.Thinking = protocol.CardStateNeeds, nil
	second.Branch = "marshal/web-dashboard-13-fix-the-typo"
	testutil.Golden(t, "board", protocol.BoardSnapshot{
		ProjectID: sampleProjectID,
		Columns: []protocol.CardState{
			protocol.CardStateBacklog, protocol.CardStatePlanning, protocol.CardStateWorking,
			protocol.CardStateNeeds, protocol.CardStateReview, protocol.CardStateReady, protocol.CardStateDone,
		},
		Cards:      []protocol.Card{sampleCard(), second},
		ServerTime: protocol.NewTimestamp(sampleTime.Add(time.Hour)),
	})
}

func TestCreateCardRequestGolden(t *testing.T) {
	testutil.Golden(t, "create-card-request", protocol.CreateCardRequest{
		Title:          "Add a health check endpoint",
		Body:           "Serve GET /health with the version.",
		Agent:          protocol.AgentKindGemini,
		Model:          "gemini-2.5-pro",
		Thinking:       protocol.ThinkingModeMedium,
		PermissionMode: protocol.PermissionModePlan,
	})
}

func TestCreateCardRequestWithOnlyATitle(t *testing.T) {
	got, err := json.Marshal(protocol.CreateCardRequest{Title: "Fix it"})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"title":"Fix it"}`; string(got) != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
}

func encodeData(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// The batch holds one event of each project and card type that the projects module sends, with
// the topic it goes to.
func TestProjectAndCardEventsGolden(t *testing.T) {
	moved := sampleCard()
	moved.State = protocol.CardStateNeeds
	event := func(seq uint64, topic protocol.Topic, typ protocol.EventType, data any) protocol.Event {
		return protocol.Event{
			Seq: seq, Topic: topic, Type: typ,
			At:   protocol.NewTimestamp(sampleTime.Add(time.Duration(seq) * 20 * time.Millisecond)),
			Data: encodeData(t, data),
		}
	}
	project := protocol.ProjectEventData{Project: sampleProject()}
	testutil.Golden(t, "project-events", protocol.EventBatch{
		Epoch: "01M3C0ZZZZ000000000000000A",
		Events: []protocol.Event{
			event(1, protocol.HomeTopic, protocol.EventTypeProjectCreated, project),
			event(2, protocol.HomeTopic, protocol.EventTypeProjectUpdated, project),
			event(3, protocol.ProjectTopic(sampleProjectID), protocol.EventTypeCardCreated,
				protocol.CardEventData{Card: sampleCard()}),
			event(4, protocol.ProjectTopic(sampleProjectID), protocol.EventTypeCardUpdated,
				protocol.CardEventData{Card: sampleCard()}),
			event(5, protocol.ProjectTopic(sampleProjectID), protocol.EventTypeCardMoved,
				protocol.CardMovedEventData{Card: moved, From: protocol.CardStateBacklog}),
			event(6, protocol.HomeTopic, protocol.EventTypeProjectRemoved,
				protocol.ProjectRemovedEventData{ProjectID: sampleProjectID}),
		},
	})
}

func TestProjectSourceValid(t *testing.T) {
	if !protocol.ProjectSourceFolder.Valid() || !protocol.ProjectSourceClone.Valid() {
		t.Error("ProjectSource.Valid rejects a source the daemon knows")
	}
	for _, bad := range []protocol.ProjectSource{"", "github", "Folder"} {
		if bad.Valid() {
			t.Errorf("ProjectSource(%q).Valid() = true, want false", bad)
		}
	}
}
