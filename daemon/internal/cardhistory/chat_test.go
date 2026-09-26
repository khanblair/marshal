package cardhistory_test

import (
	"context"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// A project chat's messages are the same stored events under the same wire types as a card's, so
// the service pages and shows them the same way.

const (
	testChatID    = "chat-1"
	testChatSess  = "chat-session-1"
	testOtherChat = "chat-2"
)

// addChat writes a chat of the env's project and its session row.
func (e *env) addChat(t *testing.T, chatID, sessionID string) {
	t.Helper()
	ctx := context.Background()
	now := testTime.UnixMilli()
	err := e.store.Write(ctx, func(q *db.Queries) error {
		if err := q.CreateChat(ctx, db.CreateChatParams{
			ID: chatID, ProjectID: "api", Title: "New chat", TargetKind: "orchestrator", AgentKind: "claude",
			PermissionMode: "auto-edits", LastActiveAt: now, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
		return q.CreateChatSession(ctx, db.CreateChatSessionParams{
			ID: sessionID, ChatID: &chatID, AgentKind: "claude", State: "awake",
			LastActiveAt: now, CreatedAt: now, UpdatedAt: now,
		})
	})
	if err != nil {
		t.Fatalf("write the chat: %v", err)
	}
}

func (e *env) appendChat(t *testing.T, chatID, sessionID string, records ...history.Record) {
	t.Helper()
	if err := e.history.AppendChat(context.Background(), chatID, sessionID, records); err != nil {
		t.Fatalf("append to the chat: %v", err)
	}
}

func TestChatMessagesPageNewestFirstAndStaySeparateFromACards(t *testing.T) {
	e := newEnv(t)
	e.addChat(t, testChatID, testChatSess)
	e.append(t, chatRecords(2)...)
	e.appendChat(t, testChatID, testChatSess,
		history.Record{Kind: history.KindUser, Summary: "What is blocked?"},
		history.Record{Kind: history.KindAgent, Summary: "Nothing is blocked right now."},
		history.Record{Kind: history.KindUser, Summary: "Thanks"})

	first, err := e.service.ChatMessages(context.Background(), testChatID, 0, 2)
	if err != nil {
		t.Fatalf("ChatMessages: %v", err)
	}
	if len(first.Items) != 2 || !first.More || first.Items[0].Text != "Thanks" ||
		first.Items[1].Text != "Nothing is blocked right now." {
		t.Fatalf("the first page = %+v, want the newest two messages with more to come", first)
	}
	if first.Items[0].Kind != protocol.ChatMessageKindUser || first.Items[1].Kind != protocol.ChatMessageKindAgent {
		t.Errorf("the message kinds = %s, %s", first.Items[0].Kind, first.Items[1].Kind)
	}
	last, err := e.service.ChatMessages(context.Background(), testChatID, first.Cursor, 2)
	if err != nil || last.More || len(last.Items) != 1 || last.Items[0].Text != "What is blocked?" {
		t.Fatalf("the last page = %+v, %v, want the first message alone", last, err)
	}
	// The card's two chunks are its own, and are not in the chat.
	if all, _ := e.service.ChatMessages(context.Background(), testChatID, 0, 50); len(all.Items) != 3 {
		t.Errorf("the chat has %d messages, want 3", len(all.Items))
	}
}

func TestChatMessagesRefuseAnUnknownChat(t *testing.T) {
	e := newEnv(t)
	_, err := e.service.ChatMessages(context.Background(), "no-such-chat", 0, 10)
	assertCode(t, err, protocol.ErrorCodeNotFound)
	// A card's id is not a chat's.
	_, err = e.service.ChatMessages(context.Background(), e.cardID, 0, 10)
	assertCode(t, err, protocol.ErrorCodeNotFound)
	_, err = e.service.ChatMessageDetail(context.Background(), "no-such-chat", "x")
	assertCode(t, err, protocol.ErrorCodeNotFound)
}

func TestAChatToolCallOpensItsDetailOnDemand(t *testing.T) {
	e := newEnv(t)
	e.addChat(t, testChatID, testChatSess)
	e.addChat(t, testOtherChat, "chat-session-2")
	e.appendChat(t, testChatID, testChatSess,
		history.Record{Kind: history.KindToolCall, Summary: "Run the tests", Detail: toolDetail("t1", "execute", "go test ./...", "ok")},
		history.Record{Kind: history.KindUser, Summary: "a plain message"})
	page, err := e.service.ChatMessages(context.Background(), testChatID, 0, 10)
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("ChatMessages = %+v, %v", page, err)
	}
	var toolID, plainID string
	for _, item := range page.Items {
		if item.Kind == protocol.ChatMessageKindTool {
			toolID = item.ID
		} else {
			plainID = item.ID
		}
	}

	detail, err := e.service.ChatMessageDetail(context.Background(), testChatID, toolID)
	if err != nil || detail.Tool == nil || detail.Tool.Command != "go test ./..." || detail.Tool.Content != "ok" {
		t.Fatalf("ChatMessageDetail = %+v, %v, want the tool's command and output", detail, err)
	}
	plain, err := e.service.ChatMessageDetail(context.Background(), testChatID, plainID)
	if err != nil || plain.Tool != nil || plain.Message.Text != "a plain message" {
		t.Errorf("the detail of a plain message = %+v, %v, want the message alone", plain, err)
	}
	// Another chat cannot read this chat's message, and an empty id is not a message.
	for name, args := range map[string][2]string{
		"another chat": {testOtherChat, toolID}, "an unknown id": {testChatID, "nope"}, "an empty id": {testChatID, ""},
	} {
		_, err := e.service.ChatMessageDetail(context.Background(), args[0], args[1])
		t.Run(name, func(t *testing.T) { assertCode(t, err, protocol.ErrorCodeNotFound) })
	}
}
