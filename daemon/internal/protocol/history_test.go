package protocol_test

import (
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The golden samples of a card's chat and its activity (docs/backend-inventory.md 4.3 and 4.4).
// Every message kind and every activity kind appears in them, so a wire shape that changes shows
// up here and in the app's own test of the same file.

// historyTime is the moment every sample is stamped with.
func historyTime() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) }

// chatMessages is one page of a card's chat, newest first, with one message of every kind.
func chatMessages() protocol.Page[protocol.ChatMessage] {
	at := historyTime()
	ok := protocol.ActivityStateOK
	items := []protocol.ChatMessage{
		{
			ID: "01M3C107JB041061050R3GG28A", Kind: protocol.ChatMessageKindCard, Seq: 8, At: protocol.NewTimestamp(at),
			Text: "Made a card for the failing check.",
			Card: &protocol.ChatCardRef{Cards: []protocol.CardKey{{ProjectID: "api", Number: 43}}},
		},
		{
			ID: "01M3C107JB041061050R3GG28B", Kind: protocol.ChatMessageKindApproval, Seq: 7, At: protocol.NewTimestamp(at),
			Text: "Asked to run rm -rf build",
			Approval: &protocol.ChatApproval{
				State: protocol.ChatApprovalStateWaiting, Command: "rm -rf build", Reason: "Clean the build folder",
			},
		},
		{
			ID: "01M3C107JB041061050R3GG28C", Kind: protocol.ChatMessageKindPlan, Seq: 6, At: protocol.NewTimestamp(at),
			Text: "Plan with 2 steps",
			Plan: &protocol.ChatPlan{
				State: protocol.ChatPlanStateWaiting,
				Steps: []string{"Read the callers", "Change the signature"},
				Files: []string{"internal/upstream/conn.go"}, Risks: []string{}, Checks: []string{"go test ./..."},
			},
		},
		{
			ID: "01M3C107JB041061050R3GG28D", Kind: protocol.ChatMessageKindDiff, Seq: 5, At: protocol.NewTimestamp(at),
			Text: "3 files changed",
			Diff: &protocol.ChatDiffSummary{Files: 3, Additions: 42, Deletions: 7},
		},
		{
			ID: "01M3C107JB041061050R3GG28E", Kind: protocol.ChatMessageKindSystem, Seq: 4, At: protocol.NewTimestamp(at),
			Text: "Session resumed after Marshal restarted.",
		},
		{
			ID: "01M3C107JB041061050R3GG28F", Kind: protocol.ChatMessageKindTool, Seq: 3, At: protocol.NewTimestamp(at),
			Tool: &protocol.ChatToolCall{
				ID: "call_1", Title: "Edited internal/upstream/conn.go", ToolKind: "edit",
				State: &ok, HasDetail: true,
			},
		},
		{
			ID: "01M3C107JB041061050R3GG28G", Kind: protocol.ChatMessageKindAgent, Seq: 2, At: protocol.NewTimestamp(at),
			Text: "I read the callers before changing the signature.",
		},
		{
			ID: "01M3C107JB041061050R3GG28H", Kind: protocol.ChatMessageKindUser, Seq: 1, At: protocol.NewTimestamp(at),
			Text: "Upgrade the upstream library.",
		},
	}
	return protocol.NewPage(items, "eyJzZXEiOjF9", at)
}

// activityItems is one page of a card's activity, newest first, with one entry of every kind.
func activityItems() protocol.Page[protocol.ActivityItem] {
	at := historyTime()
	items := []protocol.ActivityItem{
		{
			ID: "01M3C107JB041061050R3GG28A", Kind: protocol.ActivityKindFile, Seq: 5, At: protocol.NewTimestamp(at),
			Text: "Edited internal/upstream/conn.go", Result: "", State: protocol.ActivityStateOK,
		},
		{
			ID: "01M3C107JB041061050R3GG28B", Kind: protocol.ActivityKindCommand, Seq: 4, At: protocol.NewTimestamp(at),
			Text: "Bash: go build ./...", Result: "", State: protocol.ActivityStateRunning,
		},
		{
			ID: "01M3C107JB041061050R3GG28C", Kind: protocol.ActivityKindTest, Seq: 3, At: protocol.NewTimestamp(at),
			Text: "Bash: go test ./...", Result: "ok github.com/khanblair/marshal/daemon", State: protocol.ActivityStateOK,
		},
		{
			ID: "01M3C107JB041061050R3GG28D", Kind: protocol.ActivityKindTool, Seq: 2, At: protocol.NewTimestamp(at),
			Text: "Searched the repository for conn.go", Result: "", State: protocol.ActivityStateFailed,
		},
		{
			ID: "01M3C107JB041061050R3GG28E", Kind: protocol.ActivityKindApproval, Seq: 1, At: protocol.NewTimestamp(at),
			Text: "Asked to run go mod tidy", Result: "", State: protocol.ActivityStateWaiting,
		},
	}
	return protocol.NewPage(items, "", at)
}

// toolMessageDetail is the detail a chat block opens: the tool call in full, with its diffs.
func toolMessageDetail() protocol.ChatMessageDetail {
	at := historyTime()
	ok := protocol.ActivityStateOK
	return protocol.ChatMessageDetail{
		Message: protocol.ChatMessage{
			ID: "01M3C107JB041061050R3GG28F", Kind: protocol.ChatMessageKindTool, Seq: 3, At: protocol.NewTimestamp(at),
			Tool: &protocol.ChatToolCall{
				ID: "call_1", Title: "Edited internal/upstream/conn.go", ToolKind: "edit",
				State: &ok, HasDetail: true,
			},
		},
		Tool: &protocol.ChatToolDetail{
			ID: "call_1", Title: "Edited internal/upstream/conn.go", ToolKind: "edit",
			Path: "internal/upstream/conn.go", Command: "",
			Content: "Replaced the deprecated Dial with DialContext.",
			Diffs: []protocol.FileDiff{{
				Path: "internal/upstream/conn.go", OldText: "conn, err := net.Dial(\"tcp\", addr)\n",
				NewText: "conn, err := dialer.DialContext(ctx, \"tcp\", addr)\n",
			}},
			Truncated: true,
		},
		ServerTime: protocol.NewTimestamp(at),
	}
}

func TestChatAndActivityGolden(t *testing.T) {
	testutil.Golden(t, "chat-messages", chatMessages())
	testutil.Golden(t, "activity-items", activityItems())
	testutil.Golden(t, "chat-message-detail", toolMessageDetail())
}

// The message kinds are the ones the app's chat draws: a change here is a change on both sides.
func TestChatMessageKindsCarryTheWordsTheScreensUse(t *testing.T) {
	want := []string{"user", "agent", "tool", "system", "diff", "plan", "approval", "card"}
	if got := names(protocol.ChatMessageKindValues()); !equalStrings(got, want) {
		t.Errorf("ChatMessageKind = %v, want %v", got, want)
	}
	for _, kind := range protocol.ChatMessageKindValues() {
		if !kind.Valid() {
			t.Errorf("%q is in ChatMessageKindValues but is not valid", kind)
		}
	}
	if protocol.ChatMessageKind("nonsense").Valid() {
		t.Error("an unknown chat message kind is valid")
	}
}

// The activity states are the ones inventory 4.4 gives, and a state that is not one of them is
// refused.
func TestActivityStatesCarryTheWordsTheInventoryUses(t *testing.T) {
	want := []string{"ok", "running", "failed", "waiting"}
	if got := names(protocol.ActivityStateValues()); !equalStrings(got, want) {
		t.Errorf("ActivityState = %v, want %v", got, want)
	}
	for _, state := range protocol.ActivityStateValues() {
		if !state.Valid() {
			t.Errorf("%q is in ActivityStateValues but is not valid", state)
		}
	}
	if protocol.ActivityState("fail").Valid() {
		t.Error("the prototype's own word, fail, is a wire value; the wire uses failed")
	}
}

// The plan and approval states are defined now, for the phases that write them.
func TestPlanAndApprovalStatesAreFixedLists(t *testing.T) {
	if got := names(protocol.ChatPlanStateValues()); !equalStrings(got, []string{"waiting", "approved", "rejected", "edited"}) {
		t.Errorf("ChatPlanState = %v", got)
	}
	if got := names(protocol.ChatApprovalStateValues()); !equalStrings(got, []string{"waiting", "approved", "denied"}) {
		t.Errorf("ChatApprovalState = %v", got)
	}
}
