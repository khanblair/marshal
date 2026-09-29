package protocol_test

import (
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The card panel's wire shapes (B10.2, B10.5, B10.6). The Go test writes each golden file and the
// web client's tests read the same one.

func TestCardCheckListGolden(t *testing.T) {
	testutil.Golden(t, "card-checks", protocol.CardCheckList{
		Checks: []protocol.CardCheck{
			{ID: "01M3CHECK0000000000000000A", Name: "Tests pass", Kind: "command", Command: "go test ./...",
				Status: protocol.CheckStatusPassed, RunRef: "run-1790000000000"},
			{ID: "01M3CHECK0000000000000000B", Name: "Reviewer approval", Kind: "review", Command: "",
				Status: protocol.CheckStatusPending, RunRef: ""},
		},
		ServerTime: protocol.NewTimestamp(scheduleNow),
	})
}

func TestChecklistListGolden(t *testing.T) {
	done := protocol.NewTimestamp(scheduleNow)
	testutil.Golden(t, "checklists", protocol.ChecklistList{
		Checklists: []protocol.Checklist{{
			ID: "01M3LIST00000000000000000A", Name: "Done when", Required: true, PeopleOnly: false, HideChecked: false,
			Items: []protocol.ChecklistItem{
				{ID: "01M3ITEM00000000000000000A", Text: "Tests are green", Done: true, DoneByKind: "agent", DoneByID: "", DoneAt: &done},
				{ID: "01M3ITEM00000000000000000B", Text: "Docs updated", Done: false, DoneByKind: "", DoneByID: "", DoneAt: nil},
			},
		}},
		ServerTime: protocol.NewTimestamp(scheduleNow),
	})
}

func TestCommentListGolden(t *testing.T) {
	read := protocol.NewTimestamp(scheduleNow)
	testutil.Golden(t, "comments", protocol.CommentList{
		Comments: []protocol.Comment{{
			ID: "01M3COMM00000000000000000A", AuthorKind: protocol.AuthorKindPerson, AuthorID: "01M3USER00000000000000000A",
			Body: "@agent please read the spec", AgentReadAt: &read, CreatedAt: protocol.NewTimestamp(scheduleNow),
			Attachments: []protocol.Attachment{
				{ID: "01M3ATTA00000000000000000A", Kind: protocol.AttachmentKindFile, Name: "spec.md", SizeBytes: 2048},
				{ID: "01M3ATTA00000000000000000B", Kind: protocol.AttachmentKindLink, Name: "example.com/spec", URL: "https://example.com/spec"},
			},
		}},
		ServerTime: protocol.NewTimestamp(scheduleNow),
	})
}

func TestCardMembersGolden(t *testing.T) {
	testutil.Golden(t, "card-members", protocol.CardMembers{
		UserIDs: []string{"01M3USER00000000000000000A"}, ServerTime: protocol.NewTimestamp(scheduleNow),
	})
}

func TestPostCommentRequestGolden(t *testing.T) {
	testutil.Golden(t, "post-comment-request", protocol.PostCommentRequest{
		Body: "Is this the right file?",
		Attachments: []protocol.NewAttachment{
			{Kind: protocol.AttachmentKindFile, Name: "notes.txt", MimeType: "text/plain", URL: "", Data: "aGVsbG8="},
		},
	})
}

func TestChecklistRequestsGolden(t *testing.T) {
	yes := true
	name := "Release"
	testutil.Golden(t, "update-checklist-request", protocol.UpdateChecklistRequest{Name: &name, Required: &yes})
	testutil.Golden(t, "tick-checklist-item-request", protocol.TickChecklistItemRequest{Done: true})
	testutil.Golden(t, "add-card-check-request", protocol.AddCardCheckRequest{Name: "Lint clean", Command: "go vet ./..."})
}
