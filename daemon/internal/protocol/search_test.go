package protocol_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The golden samples of a search (docs/backend-checklist.md B2.11, docs/backend-inventory.md N23)
// and of a project made from the sample repository (B2.12, N24). The search answer carries a hit
// of every kind, from two projects, with a list that was cut; the empty answer shows the lists as
// empty lists, never null. The app's own tests read the same files.

// searchTime is the moment the search samples are stamped with.
func searchTime() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) }

// searchSnapshot is what a search for "token" finds across two projects.
func searchSnapshot() protocol.SearchSnapshot {
	at := searchTime()
	return protocol.SearchSnapshot{
		Query: "token",
		Projects: []protocol.ProjectHit{{
			ProjectID: "token-service", Name: "token-service", Path: "/home/ada/code/token-service",
			Language: "Go",
		}},
		Cards: []protocol.CardHit{
			{
				CardID: "01M3C107JB041061050R3GG28A", Key: "api#41", Number: 41,
				Title: "Refresh the token before it expires", State: protocol.CardStateWorking,
				ProjectID: "api", ProjectName: "api-gateway",
			},
			{
				CardID: "01M3C107JB041061050R3GG28B", Key: "web#7", Number: 7,
				Title: "Show the session timeout", State: protocol.CardStateBacklog,
				ProjectID: "web", ProjectName: "web-dashboard",
			},
		},
		Chats: []protocol.ChatHit{{
			ChatID: "01M3C107JB041061050R3GG281", Title: "Token rotation question",
			ProjectID: "api", ProjectName: "api-gateway",
			LastActiveAt: protocol.NewTimestamp(at.Add(-2 * time.Hour)),
		}},
		Totals:     protocol.SearchTotals{Projects: 1, Cards: 11, Chats: 1},
		ServerTime: protocol.NewTimestamp(at),
	}
}

func TestSearchSnapshotGolden(t *testing.T) {
	testutil.Golden(t, "search", searchSnapshot())
}

// An empty query, or one that matches nothing, is three empty lists: never null, so the palette
// draws its empty state without a guard.
func TestAnEmptySearchSendsEmptyLists(t *testing.T) {
	empty := protocol.SearchSnapshot{
		Projects: []protocol.ProjectHit{}, Cards: []protocol.CardHit{}, Chats: []protocol.ChatHit{},
		ServerTime: protocol.NewTimestamp(searchTime()),
	}
	testutil.Golden(t, "search-empty", empty)
	data, err := json.Marshal(empty)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "null") {
		t.Errorf("an empty search sent a null: %s", data)
	}
}

// The sample needs nothing but its source: no path, no address, no folder to clone into.
func TestCreateProjectRequestForTheSampleGolden(t *testing.T) {
	testutil.Golden(t, "create-project-request-sample", protocol.CreateProjectRequest{
		Source: protocol.ProjectSourceSample,
	})
	if !protocol.ProjectSourceSample.Valid() {
		t.Error("the sample is not a valid project source")
	}
}
