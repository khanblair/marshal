package history_test

import (
	"context"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// A chat's history is kept in the same table as a card's (migration 0009). Each owner numbers its
// own events from 1, and neither can read the other's.

// addChat writes a chat of the env's project and its session row, and returns their ids.
func addChat(t *testing.T, st *store.Store, suffix string) (chatID, sessionID string) {
	t.Helper()
	ctx := context.Background()
	now := testTime.UnixMilli()
	chatID, sessionID = "chat"+suffix, "chatsession"+suffix
	err := st.Write(ctx, func(q *db.Queries) error {
		if err := q.CreateChat(ctx, db.CreateChatParams{
			ID: chatID, ProjectID: "p1", Title: "New chat", TargetKind: "orchestrator", AgentKind: "claude",
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
		t.Fatalf("write the chat and its session: %v", err)
	}
	return chatID, sessionID
}

func appendsToChat(t *testing.T, h *history.Store, chatID, sessionID string, summaries ...string) {
	t.Helper()
	records := make([]history.Record, len(summaries))
	for i, summary := range summaries {
		records[i] = history.Record{Kind: history.KindUser, Summary: summary}
	}
	if err := h.AppendChat(context.Background(), chatID, sessionID, records); err != nil {
		t.Fatalf("AppendChat(%q): %v", summaries, err)
	}
}

func TestAChatsEventsAreNumberedFromOneBesideACards(t *testing.T) {
	e := newEnv(t)
	chatID, sessionID := addChat(t, e.store, "1")
	otherChat, otherSession := addChat(t, e.store, "2")
	appends(t, e.history, e.cardID, e.sessionID, "card one", "card two", "card three")
	appendsToChat(t, e.history, chatID, sessionID, "chat one", "chat two")
	appendsToChat(t, e.history, otherChat, otherSession, "other chat")
	appendsToChat(t, e.history, chatID, sessionID, "chat three")

	page, err := e.history.PageChat(context.Background(), chatID, 0, 10)
	if err != nil {
		t.Fatalf("PageChat: %v", err)
	}
	want := []string{"chat three", "chat two", "chat one"}
	if len(page.Events) != len(want) || page.More {
		t.Fatalf("the chat's page = %+v, want %d events and no more", page.Events, len(want))
	}
	for i, ev := range page.Events {
		if ev.Summary != want[i] || ev.Seq != int64(len(want)-i) {
			t.Errorf("event %d = %q at seq %d, want %q at seq %d", i, ev.Summary, ev.Seq, want[i], len(want)-i)
		}
		if ev.ChatID != chatID || ev.CardID != "" || ev.SessionID != sessionID {
			t.Errorf("event %q belongs to card %q, chat %q, session %q", ev.Summary, ev.CardID, ev.ChatID, ev.SessionID)
		}
	}
	// The other chat and the card each have a history of their own, numbered from one.
	other, err := e.history.PageChat(context.Background(), otherChat, 0, 10)
	if err != nil || len(other.Events) != 1 || other.Events[0].Seq != 1 {
		t.Errorf("the other chat's page = %+v, %v, want its one event at seq 1", other.Events, err)
	}
	card, err := e.history.Page(context.Background(), e.cardID, 0, 10)
	if err != nil || len(card.Events) != 3 || card.Events[0].Seq != 3 {
		t.Errorf("the card's page = %+v, %v, want its three events, newest at seq 3", card.Events, err)
	}
}

func TestAChatsHistoryPagesByCursor(t *testing.T) {
	e := newEnv(t)
	chatID, sessionID := addChat(t, e.store, "1")
	appendsToChat(t, e.history, chatID, sessionID, "a", "b", "c", "d", "e")

	first, err := e.history.PageChat(context.Background(), chatID, 0, 2)
	if err != nil || !first.More || len(first.Events) != 2 || first.Events[0].Summary != "e" {
		t.Fatalf("the first page = %+v, %v, want e and d with more to come", first, err)
	}
	second, err := e.history.PageChat(context.Background(), chatID, first.Cursor, 2)
	if err != nil || !second.More || second.Events[0].Summary != "c" || second.Events[1].Summary != "b" {
		t.Fatalf("the second page = %+v, %v, want c and b with more to come", second, err)
	}
	last, err := e.history.PageChat(context.Background(), chatID, second.Cursor, 2)
	if err != nil || last.More || len(last.Events) != 1 || last.Events[0].Summary != "a" {
		t.Fatalf("the last page = %+v, %v, want a alone", last, err)
	}
}

func TestOneChatEventIsReadByItsOwnersIDOnly(t *testing.T) {
	e := newEnv(t)
	chatID, sessionID := addChat(t, e.store, "1")
	otherChat, _ := addChat(t, e.store, "2")
	appendsToChat(t, e.history, chatID, sessionID, "hello")
	page, err := e.history.PageChat(context.Background(), chatID, 0, 10)
	if err != nil || len(page.Events) != 1 {
		t.Fatalf("PageChat = %+v, %v", page, err)
	}
	id := page.Events[0].ID

	got, err := e.history.ChatEvent(context.Background(), chatID, id)
	if err != nil || got.Summary != "hello" || got.ChatID != chatID {
		t.Errorf("ChatEvent = %+v, %v, want the event", got, err)
	}
	if _, err := e.history.ChatEvent(context.Background(), otherChat, id); !store.IsNotFound(err) {
		t.Errorf("another chat read the event: %v, want not found", err)
	}
	if _, err := e.history.Event(context.Background(), e.cardID, id); !store.IsNotFound(err) {
		t.Errorf("a card read a chat's event: %v, want not found", err)
	}
}

func TestChatHistoryRefusesWhatItCannotStore(t *testing.T) {
	e := newEnv(t)
	chatID, sessionID := addChat(t, e.store, "1")
	ctx := context.Background()
	record := []history.Record{{Kind: history.KindUser, Summary: "hi"}}

	if err := e.history.AppendChat(ctx, "", sessionID, record); err == nil {
		t.Error("AppendChat without a chat succeeded")
	}
	if err := e.history.AppendChat(ctx, chatID, "", record); err == nil {
		t.Error("AppendChat without a session succeeded")
	}
	if err := e.history.AppendChat(ctx, chatID, sessionID, nil); err != nil {
		t.Errorf("AppendChat with nothing to store = %v, want nil", err)
	}
	if err := e.history.AppendChat(ctx, chatID, sessionID, []history.Record{{Kind: "nonsense"}}); err == nil {
		t.Error("AppendChat with a kind nobody stores succeeded")
	}
	// A chat that is not there cannot have history: the foreign key says so.
	if err := e.history.AppendChat(ctx, "nochat", sessionID, record); err == nil {
		t.Error("AppendChat for a chat that does not exist succeeded")
	}
	if _, err := e.history.PageChat(ctx, "", 0, 10); err == nil {
		t.Error("PageChat without a chat succeeded")
	}
	if _, err := e.history.ChatEvent(ctx, chatID, ""); err == nil {
		t.Error("ChatEvent without an id succeeded")
	}
}

// Deleting a chat takes its history with it, and leaves a card's alone.
func TestDeletingAChatDeletesItsHistory(t *testing.T) {
	e := newEnv(t)
	chatID, sessionID := addChat(t, e.store, "1")
	appends(t, e.history, e.cardID, e.sessionID, "kept")
	appendsToChat(t, e.history, chatID, sessionID, "gone")

	if err := e.store.Write(context.Background(), func(q *db.Queries) error {
		_, err := q.DeleteChat(context.Background(), chatID)
		return err
	}); err != nil {
		t.Fatalf("DeleteChat: %v", err)
	}
	page, err := e.history.PageChat(context.Background(), chatID, 0, 10)
	if err != nil || len(page.Events) != 0 {
		t.Errorf("the deleted chat's history = %+v, %v, want none", page.Events, err)
	}
	card, err := e.history.Page(context.Background(), e.cardID, 0, 10)
	if err != nil || len(card.Events) != 1 {
		t.Errorf("the card's history = %+v, %v, want its one event kept", card.Events, err)
	}
}
