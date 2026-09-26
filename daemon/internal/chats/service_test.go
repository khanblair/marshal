package chats_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/chats"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// testTime is the clock every test works from, so a stored time is exact.
var testTime = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

// published is one event the service sent, kept so a test can check what a change announced.
type published struct {
	topic string
	typ   string
	data  any
}

// recorder is the chats.Events the service is built with: it keeps what was published instead of
// sending it anywhere.
type recorder struct {
	mu     sync.Mutex
	events []published
}

func (r *recorder) Publish(topic, eventType string, data any, _ bool) uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, published{topic: topic, typ: eventType, data: data})
	return uint64(len(r.events))
}

func (r *recorder) types() []string {
	out := make([]string, len(r.events))
	for i, event := range r.events {
		out[i] = event.typ
	}
	return out
}

// sessionsStub is the chats.Sessions the service is built with: it records the calls a chat that
// is archived or deleted makes, and the messages sent to a chat, so a test can see that the live
// half was asked for.
type sessionsStub struct {
	mu      sync.Mutex
	stopped []string
	logs    []string
	sent    []sentMessage
	order   []string
	stopErr error
	logsErr error
	sendErr error
	// sendStarted and sendGate, when set, make SendChat announce that it has begun and then wait
	// to be released, so a test can hold a message inside the session manager.
	sendStarted chan struct{}
	sendGate    chan struct{}
}

// sentMessage is one message the service handed to the session manager, with the chat it read.
type sentMessage struct {
	chat protocol.Chat
	text string
}

func (s *sessionsStub) SendChat(_ context.Context, chat protocol.Chat, text string) error {
	if s.sendStarted != nil {
		close(s.sendStarted)
	}
	if s.sendGate != nil {
		<-s.sendGate
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sendErr != nil {
		return s.sendErr
	}
	s.sent = append(s.sent, sentMessage{chat: chat, text: text})
	s.order = append(s.order, "send")
	return nil
}

func (s *sessionsStub) StopChatSession(_ context.Context, chatID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopErr != nil {
		return s.stopErr
	}
	s.stopped = append(s.stopped, chatID)
	s.order = append(s.order, "stop")
	return nil
}

func (s *sessionsStub) RemoveChatLogs(_ context.Context, chatID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.logsErr != nil {
		return s.logsErr
	}
	s.logs = append(s.logs, chatID)
	s.order = append(s.order, "logs")
	return nil
}

// env is the service on a real database with one project, which the chats' foreign keys need.
type env struct {
	store    *store.Store
	service  *chats.Service
	events   *recorder
	sessions *sessionsStub
}

// newEnv opens a store, writes a project, and builds the service over them. A variant with no
// session manager is newBareEnv.
func newEnv(t *testing.T) *env {
	t.Helper()
	return newEnvWith(t, &sessionsStub{})
}

// newBareEnv is newEnv without a session manager, for the tests that are only about the stored
// chats.
func newBareEnv(t *testing.T) *env {
	t.Helper()
	return newEnvWith(t, nil)
}

func newEnvWith(t *testing.T, sessions *sessionsStub) *env {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "marshal.db"))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Write(ctx, func(q *db.Queries) error {
		return q.CreateProject(ctx, db.CreateProjectParams{
			ID: "api", Name: "api", RepoPath: "/tmp/api", DefaultBranch: "main",
			PackagesJSON: "[]", CreatedAt: testTime.UnixMilli(), UpdatedAt: testTime.UnixMilli(),
		})
	}); err != nil {
		t.Fatalf("write the project: %v", err)
	}
	events := &recorder{}
	var manager chats.Sessions
	if sessions != nil {
		manager = sessions
	}
	svc, err := chats.New(
		chats.Deps{Store: st, Bus: events, Sessions: manager},
		chats.WithClock(func() time.Time { return testTime }),
	)
	if err != nil {
		t.Fatalf("make the service: %v", err)
	}
	return &env{store: st, service: svc, events: events, sessions: sessions}
}

// newChat makes a chat through the service with the request given, failing the test when it is
// refused.
func (e *env) newChat(t *testing.T, in protocol.CreateChatRequest) protocol.Chat {
	t.Helper()
	chat, err := e.service.Create(context.Background(), "api", in)
	if err != nil {
		t.Fatalf("create a chat: %v", err)
	}
	return chat
}

const sessionStateStarting = protocol.SessionStateStarting

func TestCreateWritesTheChatAndItsSession(t *testing.T) {
	e := newEnv(t)
	chat := e.newChat(t, protocol.CreateChatRequest{})

	if chat.Title != chats.NewChatTitle {
		t.Errorf("title = %q, want %q", chat.Title, chats.NewChatTitle)
	}
	if chat.Target.Kind != protocol.ChatTargetKindOrchestrator || chat.Target.ID != "" {
		t.Errorf("target = %+v, want the Orchestrator", chat.Target)
	}
	if chat.AgentKind != protocol.AgentKindClaude {
		t.Errorf("agent = %q, want claude", chat.AgentKind)
	}
	if chat.PermissionMode != protocol.PermissionModeAutoEdits {
		t.Errorf("permission mode = %q, want auto-edits", chat.PermissionMode)
	}
	if chat.ArchivedAt != nil {
		t.Errorf("archivedAt = %v, want null", chat.ArchivedAt)
	}
	if !chat.CreatedAt.Time().Equal(testTime) || !chat.LastActiveAt.Time().Equal(testTime) {
		t.Errorf("times = %s / %s, want %s", chat.CreatedAt.Time(), chat.LastActiveAt.Time(), testTime)
	}
	if !protocol.ValidID(chat.ID) {
		t.Errorf("chat id %q is not an opaque id", chat.ID)
	}

	// The chat's own session exists from the moment the chat does, in `starting`, with the chat id
	// rather than a card id.
	session := e.sessionOf(t, chat.ID)
	if session.State != string(sessionStateStarting) {
		t.Errorf("session state = %q, want %q", session.State, sessionStateStarting)
	}
	if session.CardID != "" {
		t.Errorf("session card id = %q, want empty: a chat's session has no card", session.CardID)
	}
	if session.AgentKind != string(protocol.AgentKindClaude) {
		t.Errorf("session agent = %q, want claude", session.AgentKind)
	}

	if got := e.events.types(); len(got) != 1 || got[0] != string(protocol.EventTypeChatCreated) {
		t.Fatalf("events = %v, want one chat.created", got)
	}
	event := e.events.events[0]
	if event.topic != string(protocol.ProjectTopic("api")) {
		t.Errorf("topic = %q, want project:api", event.topic)
	}
	if data, ok := event.data.(protocol.ChatEventData); !ok || data.Chat.ID != chat.ID {
		t.Errorf("payload = %#v, want the chat as it is now", event.data)
	}
}

// sessionOf reads the stored session row of a chat, failing the test when there is none.
func (e *env) sessionOf(t *testing.T, chatID string) db.Session {
	t.Helper()
	var row db.Session
	err := e.store.Read(context.Background(), func(q *db.Queries) error {
		var err error
		row, err = q.GetSessionByChat(context.Background(), &chatID)
		return err
	})
	if err != nil {
		t.Fatalf("read the session of chat %s: %v", chatID, err)
	}
	return row
}

func TestCreateTakesTheTargetAndTheSettingsItIsGiven(t *testing.T) {
	e := newEnv(t)
	thinking := protocol.ThinkingModeHigh
	chat := e.newChat(t, protocol.CreateChatRequest{
		Title:          "Rate limits per key",
		Target:         &protocol.ChatTarget{Kind: protocol.ChatTargetKindRole, ID: "Worker"},
		AgentKind:      protocol.AgentKindGemini,
		Model:          "gemini-2.5-pro",
		Thinking:       &thinking,
		PermissionMode: protocol.PermissionModeAsk,
	})
	if chat.Title != "Rate limits per key" || chat.Target.ID != "Worker" {
		t.Errorf("chat = %+v", chat)
	}
	if chat.AgentKind != protocol.AgentKindGemini || chat.Model != "gemini-2.5-pro" {
		t.Errorf("agent = %q / %q, want gemini / gemini-2.5-pro", chat.AgentKind, chat.Model)
	}
	if chat.Thinking == nil || *chat.Thinking != protocol.ThinkingModeHigh {
		t.Errorf("thinking = %v, want high", chat.Thinking)
	}
	if chat.PermissionMode != protocol.PermissionModeAsk {
		t.Errorf("permission mode = %q, want ask", chat.PermissionMode)
	}
	session := e.sessionOf(t, chat.ID)
	if session.Thinking != "high" || session.PermissionMode != "ask" {
		t.Errorf("session = %q / %q, want high / ask", session.Thinking, session.PermissionMode)
	}
}

func TestCreateRefusesWhatItCannotUse(t *testing.T) {
	e := newEnv(t)
	blank := "   "
	cases := []struct {
		name string
		in   protocol.CreateChatRequest
	}{
		{"a name of only spaces", protocol.CreateChatRequest{Title: blank}},
		{"a target kind that is not one", protocol.CreateChatRequest{
			Target: &protocol.ChatTarget{Kind: "chat"},
		}},
		{"a role with no name", protocol.CreateChatRequest{
			Target: &protocol.ChatTarget{Kind: protocol.ChatTargetKindRole},
		}},
		{"a card target that is not an id", protocol.CreateChatRequest{
			Target: &protocol.ChatTarget{Kind: protocol.ChatTargetKindCard, ID: "api#41"},
		}},
		{"an orchestrator with an id", protocol.CreateChatRequest{
			Target: &protocol.ChatTarget{Kind: protocol.ChatTargetKindOrchestrator, ID: "Worker"},
		}},
		{"an agent Marshal does not know", protocol.CreateChatRequest{AgentKind: "gpt"}},
		{"a permission mode that is not one", protocol.CreateChatRequest{PermissionMode: "yolo"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.service.Create(context.Background(), "api", tc.in)
			wantCode(t, err, protocol.ErrorCodeInvalidArgument)
		})
	}
	if len(e.events.events) != 0 {
		t.Errorf("a refused create published %v", e.events.types())
	}
}

func TestCreateRefusesAProjectThatIsNotThere(t *testing.T) {
	e := newEnv(t)
	_, err := e.service.Create(context.Background(), "gone", protocol.CreateChatRequest{})
	wantCode(t, err, protocol.ErrorCodeNotFound)
}

func TestListAnswersOneHalfOfTheListAtATime(t *testing.T) {
	e := newEnv(t)
	first := e.newChat(t, protocol.CreateChatRequest{Title: "First"})
	second := e.newChat(t, protocol.CreateChatRequest{Title: "Second"})
	if _, err := e.service.Archive(context.Background(), second.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}

	live, err := e.service.List(context.Background(), "api", false)
	if err != nil {
		t.Fatalf("list the live chats: %v", err)
	}
	if live.ProjectID != "api" || len(live.Chats) != 1 || live.Chats[0].ID != first.ID {
		t.Errorf("live = %+v, want only the first chat", live)
	}
	if !live.ServerTime.Time().Equal(testTime) {
		t.Errorf("serverTime = %s, want %s", live.ServerTime.Time(), testTime)
	}
	archived, err := e.service.List(context.Background(), "api", true)
	if err != nil {
		t.Fatalf("list the archived chats: %v", err)
	}
	if len(archived.Chats) != 1 || archived.Chats[0].ID != second.ID {
		t.Errorf("archived = %+v, want only the second chat", archived)
	}
	if archived.Chats[0].ArchivedAt == nil {
		t.Error("an archived chat has no archivedAt")
	}
}

func TestListOfAProjectWithNoChatsIsEmptyNotNil(t *testing.T) {
	e := newEnv(t)
	snapshot, err := e.service.List(context.Background(), "api", false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if snapshot.Chats == nil {
		t.Error("chats is nil; an empty list must be sent, never null")
	}
	if len(snapshot.Chats) != 0 {
		t.Errorf("chats = %v, want none", snapshot.Chats)
	}
}

func TestListRefusesAProjectThatIsNotThere(t *testing.T) {
	e := newEnv(t)
	_, err := e.service.List(context.Background(), "gone", false)
	wantCode(t, err, protocol.ErrorCodeNotFound)
}

func TestUpdateRenamesAChat(t *testing.T) {
	e := newEnv(t)
	chat := e.newChat(t, protocol.CreateChatRequest{})
	name := "Per key limits"
	renamed, err := e.service.Update(context.Background(), chat.ID, protocol.UpdateChatRequest{Title: &name})
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if renamed.Title != name {
		t.Errorf("title = %q, want %q", renamed.Title, name)
	}
	if got := e.events.types(); got[len(got)-1] != string(protocol.EventTypeChatUpdated) {
		t.Errorf("events = %v, want chat.updated last", got)
	}
	// The store really has it, so a second read agrees.
	again, err := e.service.Chat(context.Background(), chat.ID)
	if err != nil {
		t.Fatalf("read the chat: %v", err)
	}
	if again.Title != name {
		t.Errorf("stored title = %q, want %q", again.Title, name)
	}
}

func TestUpdateOfABlankNameIsRefusedAndTheOldOneKept(t *testing.T) {
	e := newEnv(t)
	chat := e.newChat(t, protocol.CreateChatRequest{Title: "Keep me"})
	blank := "   "
	_, err := e.service.Update(context.Background(), chat.ID, protocol.UpdateChatRequest{Title: &blank})
	wantCode(t, err, protocol.ErrorCodeInvalidArgument)

	again, err := e.service.Chat(context.Background(), chat.ID)
	if err != nil {
		t.Fatalf("read the chat: %v", err)
	}
	if again.Title != "Keep me" {
		t.Errorf("title = %q, want the old name kept", again.Title)
	}
}

func TestUpdateWithNoTitleLeavesTheChatAlone(t *testing.T) {
	e := newEnv(t)
	chat := e.newChat(t, protocol.CreateChatRequest{Title: "As it was"})
	before := len(e.events.events)
	same, err := e.service.Update(context.Background(), chat.ID, protocol.UpdateChatRequest{})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if same.Title != "As it was" {
		t.Errorf("title = %q, want it left alone", same.Title)
	}
	if len(e.events.events) != before {
		t.Error("an update with nothing to change published an event")
	}
}

func TestArchiveAndRestoreAnnounceAndStopTheSession(t *testing.T) {
	e := newEnv(t)
	chat := e.newChat(t, protocol.CreateChatRequest{})

	archived, err := e.service.Archive(context.Background(), chat.ID)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if archived.ArchivedAt == nil {
		t.Fatal("archivedAt is null after archiving")
	}
	if len(e.sessions.stopped) != 1 || e.sessions.stopped[0] != chat.ID {
		t.Errorf("stopped = %v, want the chat's session", e.sessions.stopped)
	}
	if got := e.events.types(); got[len(got)-1] != string(protocol.EventTypeChatArchived) {
		t.Errorf("events = %v, want chat.archived last", got)
	}

	// Archiving twice keeps the moment it was archived, so it does not move in the archived list.
	again, err := e.service.Archive(context.Background(), chat.ID)
	if err != nil {
		t.Fatalf("archive twice: %v", err)
	}
	if !again.ArchivedAt.Time().Equal(archived.ArchivedAt.Time()) {
		t.Errorf("archivedAt moved to %s, want %s", again.ArchivedAt.Time(), archived.ArchivedAt.Time())
	}

	restored, err := e.service.Restore(context.Background(), chat.ID)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.ArchivedAt != nil {
		t.Errorf("archivedAt = %s, want null after restoring", restored.ArchivedAt.Time())
	}
	if got := e.events.types(); got[len(got)-1] != string(protocol.EventTypeChatArchived) {
		t.Errorf("events = %v, want chat.archived for the restore too", got)
	}
	live, err := e.service.List(context.Background(), "api", false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(live.Chats) != 1 {
		t.Errorf("live chats = %v, want the restored one", live.Chats)
	}
}

func TestRemoveStopsTheSessionAndDeletesTheChat(t *testing.T) {
	e := newEnv(t)
	chat := e.newChat(t, protocol.CreateChatRequest{})
	if err := e.service.Remove(context.Background(), chat.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(e.sessions.stopped) != 1 || len(e.sessions.logs) != 1 {
		t.Errorf("stopped = %v, logs = %v, want both asked for once", e.sessions.stopped, e.sessions.logs)
	}
	last := e.events.events[len(e.events.events)-1]
	if last.typ != string(protocol.EventTypeChatDeleted) {
		t.Errorf("last event = %q, want chat.deleted", last.typ)
	}
	if data, ok := last.data.(protocol.ChatDeletedEventData); !ok || data.ChatID != chat.ID || data.ProjectID != "api" {
		t.Errorf("payload = %#v, want the chat id and its project", last.data)
	}
	_, err := e.service.Chat(context.Background(), chat.ID)
	wantCode(t, err, protocol.ErrorCodeNotFound)
	// The session row goes with the chat, through the cascade.
	if _, err := e.store.Queries().GetSessionByChat(context.Background(), &chat.ID); !store.IsNotFound(err) {
		t.Errorf("the chat's session survived the delete: %v", err)
	}
}

func TestRemoveOfAChatThatIsNotThereIsNotFound(t *testing.T) {
	e := newEnv(t)
	err := e.service.Remove(context.Background(), "01M3C107JB041061050R3GG28Z")
	wantCode(t, err, protocol.ErrorCodeNotFound)
}

func TestAChangeWithoutASessionManagerStillWorks(t *testing.T) {
	e := newBareEnv(t)
	chat := e.newChat(t, protocol.CreateChatRequest{})
	if _, err := e.service.Archive(context.Background(), chat.ID); err != nil {
		t.Fatalf("archive without a session manager: %v", err)
	}
	if err := e.service.Remove(context.Background(), chat.ID); err != nil {
		t.Fatalf("remove without a session manager: %v", err)
	}
}

func TestRemoveKeepsTheChatWhenTheSessionWillNotStop(t *testing.T) {
	e := newEnvWith(t, &sessionsStub{stopErr: errors.New("the agent refused")})
	chat := e.newChat(t, protocol.CreateChatRequest{})
	err := e.service.Remove(context.Background(), chat.ID)
	if err == nil {
		t.Fatal("remove succeeded although the session would not stop")
	}
	if _, readErr := e.service.Chat(context.Background(), chat.ID); readErr != nil {
		t.Errorf("the chat was deleted anyway: %v", readErr)
	}
}

func TestRemoveKeepsTheChatWhenItsLogsCannotBeRemoved(t *testing.T) {
	e := newEnvWith(t, &sessionsStub{logsErr: errors.New("the folder is busy")})
	chat := e.newChat(t, protocol.CreateChatRequest{})
	err := e.service.Remove(context.Background(), chat.ID)
	if err == nil {
		t.Fatal("remove succeeded although the logs could not be removed")
	}
	if _, readErr := e.service.Chat(context.Background(), chat.ID); readErr != nil {
		t.Errorf("the chat was deleted anyway: %v", readErr)
	}
}

// wantCode fails the test unless err is the daemon's answer with this code.
func wantCode(t *testing.T, err error, code protocol.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("no error; want %s", code)
	}
	var answer *protocol.Error
	if !errors.As(err, &answer) {
		t.Fatalf("error = %v, want a protocol error with code %s", err, code)
	}
	if answer.Code != code {
		t.Errorf("code = %s, want %s", answer.Code, code)
	}
}
