package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A project chat talks (docs/backend-checklist.md B2.10, slice C's message half): the message goes
// into the chat's own session, which the first message starts, and the answer, its tool calls, and
// its state changes arrive on the topic chat:<id>. The chat's history is paged the way a card's is.
// These tests run the real stub agent behind the real ACP adapter, as the card tests do.

// chatState matches the session.state_changed event that moves a chat's session to a state.
func chatState(chatID string, state protocol.SessionState) func(protocol.Event) bool {
	return func(ev protocol.Event) bool {
		if ev.Type != protocol.EventTypeSessionStateChanged {
			return false
		}
		var data protocol.SessionStateChangedEventData
		return json.Unmarshal(ev.Data, &data) == nil && data.ChatID == chatID && data.State == state
	}
}

// chatTitled matches a chat.updated event that carries this chat with this title.
func chatTitled(chatID, title string) func(protocol.Event) bool {
	return func(ev protocol.Event) bool {
		if ev.Type != protocol.EventTypeChatUpdated {
			return false
		}
		var data protocol.ChatEventData
		return json.Unmarshal(ev.Data, &data) == nil && data.Chat.ID == chatID && data.Chat.Title == title
	}
}

// chatMessages reads a chat's messages through the API.
func (st *stack) chatMessages(chatID, query string) protocol.Page[protocol.ChatMessage] {
	st.t.Helper()
	r := st.do(http.MethodGet, "/v1/chats/"+chatID+"/messages"+query, nil).want(st.t, http.StatusOK)
	return decode[protocol.Page[protocol.ChatMessage]](st.t, r)
}

// waitForMessages polls a chat's messages until it holds at least count.
func (st *stack) waitForMessages(chatID string, count int) protocol.Page[protocol.ChatMessage] {
	st.t.Helper()
	for range 2000 {
		if page := st.chatMessages(chatID, ""); len(page.Items) >= count {
			return page
		}
	}
	st.t.Fatalf("chat %s never held %d messages", chatID, count)
	return protocol.Page[protocol.ChatMessage]{}
}

func TestAChatTalksThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	chat := st.newChat(project.ID, protocol.CreateChatRequest{})
	base := "/v1/chats/" + chat.ID
	stream := st.dial(protocol.ChatTopic(chat.ID), protocol.ProjectTopic(project.ID))

	// A chat nobody has spoken to has an empty history, and no process is running for it.
	empty := st.do(http.MethodGet, base+"/messages", nil).want(t, http.StatusOK)
	sameShape(t, "chat-messages", empty.Body)
	if page := decode[protocol.Page[protocol.ChatMessage]](t, empty); page.Items == nil || len(page.Items) != 0 {
		t.Fatalf("the idle chat's messages = %+v, want an empty list", page.Items)
	}

	sent := st.do(http.MethodPost, base+"/messages", protocol.SendMessageRequest{Text: "What is blocked right now?"}).
		want(t, http.StatusNoContent)
	if len(sent.Body) != 0 {
		t.Errorf("a 204 has no body, got %q", sent.Body)
	}
	seen := stream.until(chatState(chat.ID, protocol.SessionStateWorking))
	seen = append(seen, stream.until(chatState(chat.ID, protocol.SessionStateAwake))...)
	if texts := strings.Join(messageTexts(seen), ""); !strings.Contains(texts, "Turn 1. I remember 0 earlier turns.") {
		t.Errorf("the chat's answer was %q, want the stub agent's first turn", texts)
	}
	for _, ev := range seen {
		switch ev.Type {
		case protocol.EventTypeSessionOutput:
			if data := dataOf[protocol.SessionOutputEventData](t, ev); ev.Topic != protocol.ChatTopic(chat.ID) || data.ChatID != chat.ID || data.CardID != "" {
				t.Errorf("a chat's output = %+v on %s", data, ev.Topic)
			}
		case protocol.EventTypeSessionToolCall:
			if data := dataOf[protocol.SessionToolCallEventData](t, ev); ev.Topic != protocol.ChatTopic(chat.ID) || data.ChatID != chat.ID || data.CardID != "" {
				t.Errorf("a chat's tool call = %+v on %s", data, ev.Topic)
			}
		}
	}
	st.mustNotLog("What is blocked right now?")

	// The first message named the chat, and the project topic heard it, once the session had taken
	// the message.
	named := 0
	for _, ev := range seen {
		if chatTitled(chat.ID, "What is blocked right now")(ev) {
			named++
		}
	}
	if named != 1 {
		t.Errorf("the project topic announced the chat's name %d times, want once: %s", named, describeEvents(seen))
	}
	listed := st.chatListOf(project.ID, false)
	if len(listed.Chats) != 1 || listed.Chats[0].Title != "What is blocked right now" {
		t.Errorf("the chat list = %+v, want the chat named from its first words", listed.Chats)
	}

	// What was said is in the history: the person's words, the tool calls, and the answer.
	page := st.waitForMessages(chat.ID, 3)
	sameShape(t, "chat-messages", mustMarshal(t, page))
	oldest := page.Items[len(page.Items)-1]
	if oldest.Kind != protocol.ChatMessageKindUser || oldest.Text != "What is blocked right now?" {
		t.Errorf("the oldest message = %+v, want the person's words", oldest)
	}
	var tool *protocol.ChatMessage
	for i := range page.Items {
		if page.Items[i].Kind == protocol.ChatMessageKindTool && page.Items[i].Tool != nil && page.Items[i].Tool.HasDetail {
			tool = &page.Items[i]
		}
	}
	if tool == nil {
		t.Fatalf("the chat's messages hold no tool call with a detail: %+v", page.Items)
	}
	detail := decode[protocol.ChatMessageDetail](t,
		st.do(http.MethodGet, base+"/messages/"+tool.ID, nil).want(t, http.StatusOK))
	if detail.Tool == nil || detail.Message.ID != tool.ID {
		t.Errorf("the tool detail = %+v, want the tool call's own", detail)
	}
	st.do(http.MethodGet, base+"/messages/nope", nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	// A message of one chat is not another's.
	other := st.newChat(project.ID, protocol.CreateChatRequest{})
	st.do(http.MethodGet, "/v1/chats/"+other.ID+"/messages/"+tool.ID, nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	if got := st.chatMessages(other.ID, ""); len(got.Items) != 0 {
		t.Errorf("another chat holds %+v", got.Items)
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return data
}

func TestAChatsMessagesPageByCursor(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	chat := st.newChat(project.ID, protocol.CreateChatRequest{})
	session, err := st.store.Queries().GetSessionByChat(context.Background(), &chat.ID)
	if err != nil {
		t.Fatalf("read the chat's session: %v", err)
	}
	err = st.hist.AppendChat(context.Background(), chat.ID, session.ID, []history.Record{
		{Kind: history.KindUser, Summary: "one"}, {Kind: history.KindAgent, Summary: "two"}, {Kind: history.KindAgent, Summary: "three"},
	})
	if err != nil {
		t.Fatalf("store the chat's history: %v", err)
	}

	first := st.chatMessages(chat.ID, "?limit=2")
	if len(first.Items) != 2 || first.NextCursor == "" || first.Items[0].Text != "three" || first.Items[1].Text != "two" {
		t.Fatalf("the first page = %+v, want the newest two and a cursor", first)
	}
	second := st.chatMessages(chat.ID, "?limit=2&cursor="+first.NextCursor)
	if len(second.Items) != 1 || second.Items[0].Text != "one" || second.NextCursor != "" {
		t.Fatalf("the second page = %+v, want the last message and no cursor", second)
	}
	st.do(http.MethodGet, "/v1/chats/"+chat.ID+"/messages?limit=0", nil).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	st.do(http.MethodGet, "/v1/chats/"+chat.ID+"/messages?cursor=%25", nil).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
}

func TestAChatMessageRefusals(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	chat := st.newChat(project.ID, protocol.CreateChatRequest{})
	base := "/v1/chats/" + chat.ID
	missing := "/v1/chats/01M3C107JB041061050R3GG28Z"

	// Text that is empty, only spaces, or too long is refused before anything starts.
	st.do(http.MethodPost, base+"/messages", protocol.SendMessageRequest{Text: "   "}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	st.do(http.MethodPost, base+"/messages", protocol.SendMessageRequest{Text: strings.Repeat("a", protocol.MaxMessageChars+1)}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	st.do(http.MethodPost, base+"/messages", "not json").
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	// A chat that is not there, or an id that cannot be one, is not found, for every chat message route.
	st.do(http.MethodPost, missing+"/messages", protocol.SendMessageRequest{Text: "hi"}).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodGet, missing+"/messages", nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodGet, missing+"/messages/x", nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodPost, "/v1/chats/nope/messages", protocol.SendMessageRequest{Text: "hi"}).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodGet, base+"/messages/"+strings.Repeat("a", 200), nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)

	// None of that started an agent or changed the chat.
	row, err := st.store.Queries().GetSessionByChat(context.Background(), &chat.ID)
	if err != nil || row.State != string(protocol.SessionStateStarting) || row.AgentSessionID != "" {
		t.Errorf("the chat's session after the refusals = %+v, %v, want it untouched", row, err)
	}
	if got := st.chatListOf(project.ID, false); got.Chats[0].Title != chat.Title {
		t.Errorf("a refused message renamed the chat to %q", got.Chats[0].Title)
	}
}

// Archiving a chat puts its session to sleep and refuses its messages with a reason; restoring it
// lets the next message wake the same conversation.
func TestArchivingARunningChatPutsItToSleep(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	chat := st.newChat(project.ID, protocol.CreateChatRequest{})
	base := "/v1/chats/" + chat.ID
	stream := st.dial(protocol.ChatTopic(chat.ID))

	st.do(http.MethodPost, base+"/messages", protocol.SendMessageRequest{Text: "first question"}).want(t, http.StatusNoContent)
	stream.until(chatState(chat.ID, protocol.SessionStateWorking))
	stream.until(chatState(chat.ID, protocol.SessionStateAwake))
	st.do(http.MethodPost, base+"/archive", nil).want(t, http.StatusOK)
	stream.until(chatState(chat.ID, protocol.SessionStateAsleep))

	got := st.do(http.MethodPost, base+"/messages", protocol.SendMessageRequest{Text: "still there?"}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got.Message != "This chat is archived. Restore it to keep talking." ||
		got.Details["reason"] != string(protocol.ChatRefusalReasonArchived) {
		t.Errorf("the refusal = %+v", got)
	}

	st.do(http.MethodPost, base+"/restore", nil).want(t, http.StatusOK)
	st.do(http.MethodPost, base+"/messages", protocol.SendMessageRequest{Text: "second question"}).want(t, http.StatusNoContent)
	stream.until(chatState(chat.ID, protocol.SessionStateWaking))
	seen := stream.until(chatState(chat.ID, protocol.SessionStateAwake))
	seen = append(seen, stream.until(chatState(chat.ID, protocol.SessionStateAwake))...)
	if texts := strings.Join(messageTexts(seen), ""); !strings.Contains(texts, "Turn 2. I remember 1 earlier turns.") {
		t.Errorf("the woken chat answered %q, want the stub's second turn, which remembers the first", texts)
	}
}

// Deleting a running chat stops its process and removes its logs.
func TestDeletingARunningChatStopsItAndRemovesItsLogs(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	chat := st.newChat(project.ID, protocol.CreateChatRequest{})
	base := "/v1/chats/" + chat.ID
	stream := st.dial(protocol.ChatTopic(chat.ID))

	st.do(http.MethodPost, base+"/messages", protocol.SendMessageRequest{Text: "hello"}).want(t, http.StatusNoContent)
	stream.until(chatState(chat.ID, protocol.SessionStateWorking))
	stream.until(chatState(chat.ID, protocol.SessionStateAwake))
	row, err := st.store.Queries().GetSessionByChat(context.Background(), &chat.ID)
	if err != nil {
		t.Fatalf("read the chat's session: %v", err)
	}
	logs := filepath.Join(st.dataDir, "logs", "sessions", row.ID)
	if _, err := os.Stat(logs); err != nil {
		t.Fatalf("the running chat has no log folder: %v", err)
	}

	st.do(http.MethodDelete, base, nil).want(t, http.StatusNoContent)
	if _, err := os.Stat(logs); !os.IsNotExist(err) {
		t.Errorf("the deleted chat's log folder is still there: %v", err)
	}
	if st.mgr.RecentOutput(chat.ID) != nil {
		t.Error("the deleted chat is still running")
	}
	st.do(http.MethodGet, base+"/messages", nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodPost, base+"/messages", protocol.SendMessageRequest{Text: "hello?"}).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}

// A chat survives a daemon restart: the process is gone, the history is not, and the next message
// resumes the same conversation through the agent's saved session.
func TestAChatSurvivesADaemonRestart(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	chat := st.newChat(project.ID, protocol.CreateChatRequest{})
	base := "/v1/chats/" + chat.ID
	stream := st.dial(protocol.ChatTopic(chat.ID))
	st.do(http.MethodPost, base+"/messages", protocol.SendMessageRequest{Text: "first question"}).want(t, http.StatusNoContent)
	stream.until(chatState(chat.ID, protocol.SessionStateWorking))
	stream.until(chatState(chat.ID, protocol.SessionStateAwake))
	st.waitForMessages(chat.ID, 3)

	st.restart()
	if err := st.mgr.RestoreAll(context.Background()); err != nil {
		t.Fatalf("RestoreAll: %v", err)
	}
	if st.mgr.RecentOutput(chat.ID) != nil {
		t.Fatal("the restart started the chat's agent, which only a message should do")
	}
	if got := st.chatMessages(chat.ID, ""); len(got.Items) < 3 {
		t.Fatalf("the chat's history after the restart = %+v, want what was said before", got.Items)
	}

	stream = st.dial(protocol.ChatTopic(chat.ID))
	st.do(http.MethodPost, base+"/messages", protocol.SendMessageRequest{Text: "second question"}).want(t, http.StatusNoContent)
	stream.until(chatState(chat.ID, protocol.SessionStateWorking))
	seen := stream.until(chatState(chat.ID, protocol.SessionStateAwake))
	if texts := strings.Join(messageTexts(seen), ""); !strings.Contains(texts, "Turn 2. I remember 1 earlier turns.") {
		t.Errorf("the resumed chat answered %q, want the stub's second turn", texts)
	}
}

func TestChatMessageRoutesAreNotRegisteredWithoutTheirServices(t *testing.T) {
	// Sending needs the chats service and paging needs the history, and each is registered only with
	// its own. The two share an address, so the one that is missing answers "method not allowed"
	// there, and the detail route, which has an address of its own, is not found.
	id := "01M3C107JB041061050R3GG28Z"
	st := newStack(t, withoutChats())
	st.do(http.MethodPost, "/v1/chats/"+id+"/messages", protocol.SendMessageRequest{Text: "hi"}).want(t, http.StatusMethodNotAllowed)
	noHistory := newStack(t, withoutHistory())
	noHistory.do(http.MethodGet, "/v1/chats/"+id+"/messages", nil).want(t, http.StatusMethodNotAllowed)
	noHistory.do(http.MethodGet, "/v1/chats/"+id+"/messages/x", nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}
