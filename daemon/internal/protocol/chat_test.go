package protocol_test

import (
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The golden samples of a project chat (docs/backend-checklist.md B2.10, docs/backend-inventory.md
// N13 and the `chats` row of 4.3). Every target kind, every agent setting, an archived chat and a
// live one all appear in them, so a wire shape that changes shows up here, and the app's own mapper
// test reads the same files.

// chatTime is the moment the chat samples are stamped with.
func chatTime() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) }

// sampleChat is a live chat that talks to the Orchestrator and has never been renamed.
func sampleChat() protocol.Chat {
	at := chatTime()
	return protocol.Chat{
		ID: "01M3C107JB041061050R3GG281", ProjectID: "web-dashboard",
		Title:     "JWKS caching question",
		Target:    protocol.ChatTarget{Kind: protocol.ChatTargetKindOrchestrator},
		AgentKind: protocol.AgentKindClaude, Model: "claude-opus-4-1",
		PermissionMode: protocol.PermissionModeAsk,
		LastActiveAt:   protocol.NewTimestamp(at.Add(-2 * time.Hour)),
		CreatedAt:      protocol.NewTimestamp(at.Add(-26 * time.Hour)),
	}
}

// archivedChat talks to one card's own agent and was put away.
func archivedChat() protocol.Chat {
	thinking := protocol.ThinkingModeHigh
	archived := protocol.NewTimestamp(chatTime().Add(-30 * time.Minute))
	return protocol.Chat{
		ID: "01M3C107JB041061050R3GG282", ProjectID: "web-dashboard",
		Title:     "Rate limits per key",
		Target:    protocol.ChatTarget{Kind: protocol.ChatTargetKindCard, ID: "01M3C107JB041061050R3GG28A"},
		AgentKind: protocol.AgentKindClaude, Model: "claude-sonnet-4-1",
		Thinking:       &thinking,
		PermissionMode: protocol.PermissionModeAutoEdits,
		ArchivedAt:     &archived,
		LastActiveAt:   protocol.NewTimestamp(chatTime().Add(-3 * time.Hour)),
		CreatedAt:      protocol.NewTimestamp(chatTime().Add(-50 * time.Hour)),
	}
}

// chatList is one project's chats, live ones and archived ones together, in the order the daemon
// answers with (most recently active first within each half).
func chatList() protocol.ChatListSnapshot {
	return protocol.ChatListSnapshot{
		ProjectID:  "web-dashboard",
		Chats:      []protocol.Chat{sampleChat(), archivedChat()},
		ServerTime: protocol.NewTimestamp(chatTime()),
	}
}

// roleChat is a chat that talks to one role, which is what the app's target list offers beside the
// Orchestrator and a card.
func roleChat() protocol.Chat {
	chat := sampleChat()
	chat.ID = "01M3C107JB041061050R3GG283"
	chat.Title = "Plan the export work"
	chat.Target = protocol.ChatTarget{Kind: protocol.ChatTargetKindRole, ID: "Worker"}
	chat.LastActiveAt = protocol.NewTimestamp(chatTime().Add(-time.Minute))
	return chat
}

// chatEvents is one batch with one event of each type the chats module sends, on the topic each
// goes to: the list of a project is the project's topic, and every one carries the chat as it is
// now (or its id, for a chat that is gone).
func chatEvents(t *testing.T) protocol.EventBatch {
	t.Helper()
	event := func(seq uint64, typ protocol.EventType, data any) protocol.Event {
		return protocol.Event{
			Seq: seq, Topic: protocol.ProjectTopic("web-dashboard"), Type: typ,
			At:   protocol.NewTimestamp(chatTime().Add(time.Duration(seq) * 20 * time.Millisecond)),
			Data: encodeData(t, data),
		}
	}
	return protocol.EventBatch{
		Epoch: "01M3C0ZZZZ000000000000000A",
		Events: []protocol.Event{
			event(1, protocol.EventTypeChatCreated, protocol.ChatEventData{Chat: sampleChat()}),
			event(2, protocol.EventTypeChatUpdated, protocol.ChatEventData{Chat: roleChat()}),
			event(3, protocol.EventTypeChatArchived, protocol.ChatArchivedEventData{Chat: archivedChat()}),
			event(4, protocol.EventTypeChatDeleted, protocol.ChatDeletedEventData{
				ChatID: sampleChat().ID, ProjectID: "web-dashboard",
			}),
		},
	}
}

func TestChatGolden(t *testing.T) {
	testutil.Golden(t, "chat-list", chatList())
	testutil.Golden(t, "chat", roleChat())
	testutil.Golden(t, "create-chat-request", protocol.CreateChatRequest{
		Title:  "Plan the export work",
		Target: &protocol.ChatTarget{Kind: protocol.ChatTargetKindRole, ID: "Worker"},
	})
	testutil.Golden(t, "create-chat-request-orchestrator", protocol.CreateChatRequest{})
	renamed := "Rate limits per key"
	testutil.Golden(t, "update-chat-request", protocol.UpdateChatRequest{Title: &renamed})
	testutil.Golden(t, "chat-events", chatEvents(t))
}

// The target kinds are the three the app's new-chat list offers: the Orchestrator, a role, and a
// card's own agent. A change here is a change on both sides.
func TestChatTargetKindsCarryTheWordsTheScreensUse(t *testing.T) {
	want := []string{"orchestrator", "role", "card"}
	if got := names(protocol.ChatTargetKindValues()); !equalStrings(got, want) {
		t.Errorf("ChatTargetKind = %v, want %v", got, want)
	}
	for _, kind := range protocol.ChatTargetKindValues() {
		if !kind.Valid() {
			t.Errorf("%q is in ChatTargetKindValues but is not valid", kind)
		}
	}
	if protocol.ChatTargetKind("chat").Valid() {
		t.Error("an unknown chat target kind is valid")
	}
}
