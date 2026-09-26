package chats_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/chats"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Sending a message to a chat (docs/backend-checklist.md B2.10): the message goes into the chat's
// own session, and the chat is named from its first words until a model provider exists.

// send sends a message to a chat and fails the test when it is refused.
func (e *env) send(t *testing.T, chatID, text string) {
	t.Helper()
	if err := e.service.Send(context.Background(), chatID, text); err != nil {
		t.Fatalf("Send(%q): %v", text, err)
	}
}

// lastChatUpdated returns the chat carried by the newest chat.updated event, and fails the test when
// the newest event is not one.
func (e *env) lastChatUpdated(t *testing.T) protocol.Chat {
	t.Helper()
	last := e.events.events[len(e.events.events)-1]
	if last.typ != string(protocol.EventTypeChatUpdated) || last.topic != string(protocol.ProjectTopic("api")) {
		t.Fatalf("the newest event is %s on %s, want chat.updated on the project topic", last.typ, last.topic)
	}
	data, ok := last.data.(protocol.ChatEventData)
	if !ok {
		t.Fatalf("chat.updated carries %T, want a chat", last.data)
	}
	return data.Chat
}

func TestSendHandsTheMessageAndTheChatToTheSession(t *testing.T) {
	e := newEnv(t)
	chat := e.newChat(t, protocol.CreateChatRequest{
		AgentKind: protocol.AgentKindGemini, Model: "gemini-2.5-pro", PermissionMode: protocol.PermissionModeAsk,
	})
	e.send(t, chat.ID, "What is blocked right now?")

	if len(e.sessions.sent) != 1 {
		t.Fatalf("the session was given %d messages, want 1", len(e.sessions.sent))
	}
	got := e.sessions.sent[0]
	if got.text != "What is blocked right now?" {
		t.Errorf("the session was given %q", got.text)
	}
	// The session starts with what the chat was made with, so the chat goes over whole.
	if got.chat.ID != chat.ID || got.chat.ProjectID != "api" || got.chat.AgentKind != protocol.AgentKindGemini ||
		got.chat.Model != "gemini-2.5-pro" || got.chat.PermissionMode != protocol.PermissionModeAsk {
		t.Errorf("the session was given the chat %+v, want the chat as it was made", got.chat)
	}
}

func TestTheFirstMessageNamesANewChatFromItsFirstWords(t *testing.T) {
	tests := []struct {
		message string
		want    string
	}{
		{"What is blocked?", "What is blocked"},
		{"make cards for the export work please and thanks", "Make cards for the export work"},
		{"  Trim me.  ", "Trim me"},
		{"Really?!", "Really"},
		{"élan vital first", "Élan vital first"},
		{"one\ntwo\tthree   four five six seven", "One two three four five six"},
		{strings.Repeat("x", 150), strings.ToUpper("x") + strings.Repeat("x", 99)},
		// Nothing to name it after, so it keeps the name it has.
		{"?!", chats.NewChatTitle},
	}
	for _, tc := range tests {
		t.Run(tc.message, func(t *testing.T) {
			e := newEnv(t)
			chat := e.newChat(t, protocol.CreateChatRequest{})
			e.send(t, chat.ID, tc.message)
			if got := e.lastChatUpdated(t); got.Title != tc.want {
				t.Errorf("the chat is called %q, want %q", got.Title, tc.want)
			}
			stored, err := e.service.Chat(context.Background(), chat.ID)
			if err != nil || stored.Title != tc.want {
				t.Errorf("the stored chat = %+v, %v, want the title %q", stored, err, tc.want)
			}
		})
	}
}

func TestOnlyTheFirstMessageNamesTheChat(t *testing.T) {
	e := newEnv(t)
	chat := e.newChat(t, protocol.CreateChatRequest{})
	e.send(t, chat.ID, "Plan the export work")
	e.send(t, chat.ID, "And what about the tests?")

	if got := e.lastChatUpdated(t); got.Title != "Plan the export work" {
		t.Errorf("the second message renamed the chat to %q", got.Title)
	}
	if n := len(e.sessions.sent); n != 2 {
		t.Errorf("the session was given %d messages, want 2", n)
	}
}

// A person can rename a chat at any time, so a name they chose before the first message is theirs.
func TestAChatThatWasNamedIsNotRenamedByItsFirstMessage(t *testing.T) {
	e := newEnv(t)
	named := e.newChat(t, protocol.CreateChatRequest{Title: "Release notes"})
	e.send(t, named.ID, "Draft the notes for 1.4")
	if got := e.lastChatUpdated(t); got.Title != "Release notes" {
		t.Errorf("a chat made with a name is now called %q", got.Title)
	}

	renamed := e.newChat(t, protocol.CreateChatRequest{})
	title := "My own name"
	if _, err := e.service.Update(context.Background(), renamed.ID, protocol.UpdateChatRequest{Title: &title}); err != nil {
		t.Fatalf("rename: %v", err)
	}
	e.send(t, renamed.ID, "Something else entirely")
	if got := e.lastChatUpdated(t); got.Title != "My own name" {
		t.Errorf("a chat renamed before its first message is now called %q", got.Title)
	}
}

func TestAMessageMovesTheChatToTheTopOfTheList(t *testing.T) {
	e := newEnv(t)
	chat := e.newChat(t, protocol.CreateChatRequest{})
	// The chat was last active an hour ago, and another chat is newer.
	if err := e.store.Write(context.Background(), func(q *db.Queries) error {
		earlier := testTime.Add(-time.Hour).UnixMilli()
		_, err := q.TouchChat(context.Background(), db.TouchChatParams{LastActiveAt: earlier, UpdatedAt: earlier, ID: chat.ID})
		return err
	}); err != nil {
		t.Fatalf("age the chat: %v", err)
	}
	e.newChat(t, protocol.CreateChatRequest{})

	e.send(t, chat.ID, "Hello")
	if got := e.lastChatUpdated(t); !got.LastActiveAt.Time().Equal(testTime) {
		t.Errorf("lastActiveAt = %s, want %s", got.LastActiveAt.Time(), testTime)
	}
	list, err := e.service.List(context.Background(), "api", false)
	if err != nil || len(list.Chats) != 2 {
		t.Fatalf("List = %+v, %v", list, err)
	}
	// Both are active at the fixed clock's moment now, so the tie is broken by id: the point is only
	// that the touched chat is not behind the hour it was.
	for _, listed := range list.Chats {
		if listed.ID == chat.ID && !listed.LastActiveAt.Time().Equal(testTime) {
			t.Errorf("the listed chat was last active at %s, want %s", listed.LastActiveAt.Time(), testTime)
		}
	}
}

func TestAMessageToAnArchivedChatIsRefusedAndSendsNothing(t *testing.T) {
	e := newEnv(t)
	chat := e.newChat(t, protocol.CreateChatRequest{})
	if _, err := e.service.Archive(context.Background(), chat.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	before := len(e.events.events)

	err := e.service.Send(context.Background(), chat.ID, "Are you there?")
	wantCode(t, err, protocol.ErrorCodeRefused)
	var perr *protocol.Error
	if !errors.As(err, &perr) {
		t.Fatalf("Send = %v, want an answer", err)
	}
	if perr.Message != "This chat is archived. Restore it to keep talking." ||
		perr.Details["reason"] != string(protocol.ChatRefusalReasonArchived) || perr.Details["chatId"] != chat.ID {
		t.Errorf("the refusal = %+v, want the archived sentence with its reason and the chat id", perr)
	}
	if len(e.sessions.sent) != 0 || len(e.events.events) != before {
		t.Errorf("a refused message reached the session (%d) or published (%d events)", len(e.sessions.sent), len(e.events.events)-before)
	}

	// Restoring the chat lets it talk again.
	if _, err := e.service.Restore(context.Background(), chat.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}
	e.send(t, chat.ID, "Are you there?")
}

func TestAMessageToAChatThatIsNotThereIsNotFound(t *testing.T) {
	e := newEnv(t)
	err := e.service.Send(context.Background(), "01M3C107JB041061050R3GG28Z", "Hello")
	wantCode(t, err, protocol.ErrorCodeNotFound)
	if len(e.sessions.sent) != 0 {
		t.Errorf("a message to a chat that is not there reached the session")
	}
}

func TestAMessageWithoutASessionManagerIsRefusedPlainly(t *testing.T) {
	e := newBareEnv(t)
	chat := e.newChat(t, protocol.CreateChatRequest{})
	err := e.service.Send(context.Background(), chat.ID, "Hello")
	wantCode(t, err, protocol.ErrorCodeUnavailable)
	if got, _ := e.service.Chat(context.Background(), chat.ID); got.Title != chats.NewChatTitle {
		t.Errorf("a message nobody took named the chat %q", got.Title)
	}
}

// When the session refuses the message, the chat is left as it was: no name, no move up the list,
// and nothing announced.
func TestAMessageTheSessionRefusesChangesNothing(t *testing.T) {
	refusal := protocol.Refused("Marshal could not pick this chat's conversation back up. Start a new chat to keep going.")
	e := newEnvWith(t, &sessionsStub{sendErr: refusal})
	chat := e.newChat(t, protocol.CreateChatRequest{})
	before := len(e.events.events)

	err := e.service.Send(context.Background(), chat.ID, "Hello there")
	if !errors.Is(err, refusal) {
		t.Fatalf("Send = %v, want the session's own refusal", err)
	}
	if len(e.events.events) != before {
		t.Errorf("a refused message published %v", e.events.types()[before:])
	}
	if got, _ := e.service.Chat(context.Background(), chat.ID); got.Title != chats.NewChatTitle {
		t.Errorf("a refused message named the chat %q", got.Title)
	}
}

// A message and an archive or a delete of the same chat follow one another. Without that, a message
// that had passed the archived check could reach the session manager after the archive or the delete
// had stopped the session, and start an agent for a chat that is put away or gone.
func TestAnArchiveOrADeleteWaitsForAMessageThatIsBeingSent(t *testing.T) {
	tests := []struct {
		name   string
		change func(*env, string) error
		want   []string
	}{
		{"an archive", func(e *env, id string) error {
			_, err := e.service.Archive(context.Background(), id)
			return err
		}, []string{"send", "stop"}},
		{"a delete", func(e *env, id string) error { return e.service.Remove(context.Background(), id) },
			[]string{"send", "stop", "logs"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stub := &sessionsStub{sendStarted: make(chan struct{}), sendGate: make(chan struct{})}
			e := newEnvWith(t, stub)
			chat := e.newChat(t, protocol.CreateChatRequest{})
			sent := make(chan error, 1)
			go func() { sent <- e.service.Send(context.Background(), chat.ID, "hello") }()
			<-stub.sendStarted

			changed := make(chan error, 1)
			go func() { changed <- tc.change(e, chat.ID) }()
			select {
			case err := <-changed:
				t.Fatalf("%s ran while a message was inside the session manager: %v", tc.name, err)
			case <-time.After(100 * time.Millisecond):
			}
			close(stub.sendGate)
			if err := <-sent; err != nil {
				t.Errorf("the message: %v", err)
			}
			if err := <-changed; err != nil {
				t.Errorf("%s: %v", tc.name, err)
			}
			stub.mu.Lock()
			defer stub.mu.Unlock()
			if strings.Join(stub.order, ",") != strings.Join(tc.want, ",") {
				t.Errorf("the session manager was asked %v, want %v: the message first, then the stop", stub.order, tc.want)
			}
		})
	}
}
