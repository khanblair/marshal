package history_test

import (
	"context"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/providers"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

func bindBuiltin(t *testing.T, e *env, sessionID string) string {
	t.Helper()
	id := "native-" + sessionID
	err := e.store.Write(t.Context(), func(q *db.Queries) error {
		_, err := q.UpdateSessionHandoff(t.Context(), db.UpdateSessionHandoffParams{
			ID: sessionID, AgentKind: "builtin", AgentSessionID: id, State: "stopped",
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestConversationReplaysEveryPageAndKeepsOwnersSeparate(t *testing.T) {
	e := newEnv(t)
	id := bindBuiltin(t, e, e.sessionID)
	records := []history.Record{{Kind: history.KindUser, Summary: "Remember the first marker."}}
	for range history.MaxPageSize + 10 {
		records = append(records, history.Record{Kind: history.KindAgent, Summary: "x"})
	}
	tool, err := history.RecordsOf(agents.ToolCallUpdate{ID: "read-1", Title: "Read file", Status: agents.StatusCompleted, Content: "recorded file content"})
	if err != nil {
		t.Fatal(err)
	}
	records = append(records, tool...)
	records = append(records, history.Record{Kind: history.KindThought, Summary: "private thinking"})
	if err := e.history.Append(t.Context(), e.cardID, e.sessionID, records); err != nil {
		t.Fatal(err)
	}
	otherCard, otherSession := addCard(t, e.store, "other")
	appends(t, e.history, otherCard, otherSession, "unrelated owner")
	messages, err := e.history.Conversation(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Role != providers.RoleUser || messages[0].Parts[0].Text != "Remember the first marker." {
		t.Fatalf("earlier user context missing: %+v", messages)
	}
	answer := messages[1].Parts[0].Text
	if !strings.HasPrefix(answer, strings.Repeat("x", history.MaxPageSize+10)) || !strings.Contains(answer, "recorded file content") {
		t.Fatalf("missing chunks or tool output: %q", answer)
	}
	if strings.Contains(answer, "private thinking") || strings.Contains(answer, "unrelated owner") {
		t.Fatal("unrelated context was replayed")
	}
}

func TestChatConversationAndResumeReadFailures(t *testing.T) {
	e := newEnv(t)
	chatID, sessionID := addChat(t, e.store, "one")
	id := bindBuiltin(t, e, sessionID)
	if err := e.history.AppendChat(t.Context(), chatID, sessionID, []history.Record{{Kind: history.KindUser, Summary: "chat marker"}}); err != nil {
		t.Fatal(err)
	}
	messages, err := e.history.Conversation(t.Context(), id)
	if err != nil || len(messages) != 1 || messages[0].Parts[0].Text != "chat marker" {
		t.Fatalf("chat replay = %+v, %v", messages, err)
	}
	for _, unknown := range []string{"", "unknown", "native-claude-id"} {
		if _, err := e.history.Conversation(t.Context(), unknown); err == nil {
			t.Fatalf("unknown ID accepted: %s", unknown)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := e.history.Conversation(ctx, id); err == nil {
		t.Fatal("cancelled reader succeeded")
	}
	limited, err := history.New(e.store, history.WithConversationBytes(4))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := limited.Conversation(t.Context(), id); err == nil {
		t.Fatal("oversized history silently truncated")
	}
}
