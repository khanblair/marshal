package session_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/chats"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// A project chat's own session (docs/backend-checklist.md B2.10, slice C's message half): it starts
// on its first message, in the project's repository folder, and follows the sleep, wake, and resume
// rules of a card without a card behind it.

// chatEnv is an env with a chats service, which is what makes chats and gives the manager the chat
// it is asked to talk to.
type chatEnv struct {
	*env
	chats *chats.Service
	mgr   *session.Manager
}

// newChatEnv builds the env with a chat service that is handed the env's session manager, as
// cmd/marshald hands the daemon's.
func newChatEnv(t *testing.T, mutate ...func(*session.Config)) *chatEnv {
	t.Helper()
	e := newEnv(t, mutate...)
	t.Cleanup(func() { _ = e.mgr.Close() })
	ce := &chatEnv{env: e, mgr: e.mgr}
	ce.chats = ce.chatsWith(t, e.mgr)
	return ce
}

// chatsWith builds a chats service over the env's store and bus that talks to this manager.
func (e *chatEnv) chatsWith(t *testing.T, mgr *session.Manager) *chats.Service {
	t.Helper()
	svc, err := chats.New(chats.Deps{Store: e.store, Bus: e.bus, Sessions: mgr})
	if err != nil {
		t.Fatalf("make the chats service: %v", err)
	}
	return svc
}

// newChat makes a chat in a project.
func (e *chatEnv) newChat(t *testing.T, projectID string, in protocol.CreateChatRequest) protocol.Chat {
	t.Helper()
	chat, err := e.chats.Create(context.Background(), projectID, in)
	if err != nil {
		t.Fatalf("create a chat: %v", err)
	}
	return chat
}

// row reads a chat's stored session.
func (e *chatEnv) row(t *testing.T, chatID string) db.Session {
	t.Helper()
	row, err := e.store.Queries().GetSessionByChat(context.Background(), &chatID)
	if err != nil {
		t.Fatalf("read the session of chat %s: %v", chatID, err)
	}
	return row
}

// untilChatState reads events until a chat's session reports this state on the chat's topic, and
// returns that payload.
func (e *chatEnv) untilChatState(t *testing.T, chatID string, state protocol.SessionState) protocol.SessionStateChangedEventData {
	t.Helper()
	timeout := time.After(eventTimeout)
	for {
		select {
		case ev, ok := <-e.sub.C():
			if !ok {
				t.Fatalf("the subscription closed while waiting for %s", state)
			}
			data, ok := ev.Data.(protocol.SessionStateChangedEventData)
			if ev.Type != string(protocol.EventTypeSessionStateChanged) || !ok || data.ChatID != chatID || data.State != state {
				continue
			}
			if ev.Topic != string(protocol.ChatTopic(chatID)) || !ev.Critical || data.CardID != "" {
				t.Fatalf("the %s event = %+v, want a critical event on the chat topic with no card id", state, ev)
			}
			return data
		case <-timeout:
			t.Fatalf("no %s event arrived for chat %s", state, chatID)
			return protocol.SessionStateChangedEventData{}
		}
	}
}

// chatHistory reads a chat's stored history, oldest first.
func (e *chatEnv) chatHistory(t *testing.T, chatID string) []history.Event {
	t.Helper()
	h, err := history.New(e.store)
	if err != nil {
		t.Fatalf("make the history store: %v", err)
	}
	page, err := h.PageChat(context.Background(), chatID, 0, 100)
	if err != nil {
		t.Fatalf("read the history of chat %s: %v", chatID, err)
	}
	for i, j := 0, len(page.Events)-1; i < j; i, j = i+1, j-1 {
		page.Events[i], page.Events[j] = page.Events[j], page.Events[i]
	}
	return page.Events
}

// waitForChatHistory waits until a chat has at least count stored events.
func (e *chatEnv) waitForChatHistory(t *testing.T, chatID string, count int) []history.Event {
	t.Helper()
	deadline := time.Now().Add(eventTimeout)
	for time.Now().Before(deadline) {
		if got := e.chatHistory(t, chatID); len(got) >= count {
			return got
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("chat %s stored fewer than %d events before the deadline", chatID, count)
	return nil
}

// summaries lists the summaries of stored events.
func summaries(evs []history.Event) []string {
	out := make([]string, len(evs))
	for i, ev := range evs {
		out[i] = string(ev.Kind) + ": " + ev.Summary
	}
	return out
}

// send sends a message to a chat through the chats service, as the route does.
func (e *chatEnv) send(t *testing.T, chatID, text string) {
	t.Helper()
	if err := e.chats.Send(context.Background(), chatID, text); err != nil {
		t.Fatalf("send %q to chat %s: %v", text, chatID, err)
	}
}

// A chat starts idle: making one starts no process. Its first message starts the agent, in the
// project's own folder and with the settings the chat was made with, and the answer, the tool calls,
// and the state changes all go out on the chat's own topic, each carrying the chat's id.
func TestAChatStartsOnItsFirstMessage(t *testing.T) {
	e := newChatEnv(t)
	project := e.project(t, "small-repo")
	chat := e.newChat(t, project.ID, protocol.CreateChatRequest{
		Model: "claude-sonnet-4-5", PermissionMode: protocol.PermissionModeAsk,
	})
	if got := fakeStarts(e.agent); got != 0 {
		t.Fatalf("making a chat started %d agents, want none", got)
	}
	if row := e.row(t, chat.ID); row.State != string(protocol.SessionStateStarting) || row.AgentSessionID != "" {
		t.Fatalf("the idle chat's session = %+v, want starting with no agent session", row)
	}

	e.send(t, chat.ID, "What is blocked right now?")
	awake := e.untilChatState(t, chat.ID, protocol.SessionStateAwake)
	if awake.SessionID != e.row(t, chat.ID).ID {
		t.Errorf("the state event names the session %q, want the chat's own", awake.SessionID)
	}
	e.untilChatState(t, chat.ID, protocol.SessionStateWorking)

	specs := e.agent.startSpecs()
	if len(specs) != 1 || fakeStarts(e.agent) != 1 {
		t.Fatalf("the agent was started %d times with %d specs, want once", fakeStarts(e.agent), len(specs))
	}
	spec := specs[0]
	if spec.Cwd != project.Path || spec.Model != "claude-sonnet-4-5" || spec.PermissionMode != string(protocol.PermissionModeAsk) ||
		spec.Label != chat.ID {
		t.Errorf("the agent was started with %+v, want the project's folder, the chat's settings, and its id as the label", spec)
	}

	e.untilChatState(t, chat.ID, protocol.SessionStateAwake)
	row := e.row(t, chat.ID)
	if row.State != string(protocol.SessionStateAwake) || row.AgentSessionID == "" || row.CardID != "" {
		t.Errorf("the running chat's session = %+v, want awake with an agent session and no card", row)
	}
}

// What the person said and what the agent answered are stored as the chat's history, in order, and
// the chat's own sequence starts at one whatever the cards have stored.
func TestAChatsMessagesAreRecordedInItsHistory(t *testing.T) {
	e := newChatEnv(t)
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "A card with history of its own")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "card message"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitForHistory(t, e.env, card.ID, 2)
	chat := e.newChat(t, project.ID, protocol.CreateChatRequest{})

	e.send(t, chat.ID, "first question")
	e.waitForChatHistory(t, chat.ID, 2)
	e.send(t, chat.ID, "second question")
	got := e.waitForChatHistory(t, chat.ID, 4)

	want := []string{
		"user: first question",
		"agent: turn 1 remembers 0 earlier turns: first question",
		"user: second question",
		"agent: turn 2 remembers 1 earlier turns: second question",
	}
	if diff := summaries(got); strings.Join(diff, "\n") != strings.Join(want, "\n") {
		t.Errorf("the chat's history =\n%s\nwant\n%s", strings.Join(diff, "\n"), strings.Join(want, "\n"))
	}
	for i, ev := range got {
		if ev.Seq != int64(i+1) || ev.ChatID != chat.ID || ev.CardID != "" || ev.SessionID != e.row(t, chat.ID).ID {
			t.Errorf("event %d = %+v, want seq %d in the chat's own history", i, ev, i+1)
		}
	}
	if cardEvents := e.historyOf(t, card.ID); len(cardEvents) != 2 {
		t.Errorf("the card's history has %d events, want its own 2 and nothing from the chat", len(cardEvents))
	}
}

// A chat's session talks on the chat's topic and never on a card's, and no card moves because a chat
// spoke: the board hears nothing about it.
func TestAChatsEventsCarryItsIDAndMoveNoCard(t *testing.T) {
	e := newChatEnv(t)
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Bystander")
	chat := e.newChat(t, project.ID, protocol.CreateChatRequest{})
	e.send(t, chat.ID, "hello")
	// Wait for the whole turn, then look at everything that was published.
	e.waitForChatHistory(t, chat.ID, 2)
	waitForRowState(t, e, chat.ID, protocol.SessionStateAwake)
	var seen []events.Event
	quiet := time.After(100 * time.Millisecond)
drain:
	for {
		select {
		case ev := <-e.sub.C():
			seen = append(seen, ev)
		case <-quiet:
			break drain
		}
	}
	var output int
	for _, ev := range seen {
		switch ev.Type {
		case string(protocol.EventTypeSessionOutput):
			data := ev.Data.(protocol.SessionOutputEventData)
			if ev.Topic != string(protocol.ChatTopic(chat.ID)) || data.ChatID != chat.ID || data.CardID != "" {
				t.Errorf("a chat's output = %+v on %s, want the chat's id on the chat's topic", data, ev.Topic)
			}
			output++
		case string(protocol.EventTypeSessionStateChanged):
			if ev.Topic != string(protocol.ChatTopic(chat.ID)) {
				t.Errorf("a chat's state change went to %s", ev.Topic)
			}
		case string(protocol.EventTypeCardUpdated), string(protocol.EventTypeCardMoved):
			data, _ := ev.Data.(protocol.CardEventData)
			t.Errorf("a chat's message published %s for card %q", ev.Type, data.Card.ID)
		}
	}
	if output == 0 {
		t.Error("the chat's answer was never published")
	}
	if got, err := e.proj.Card(context.Background(), card.ID); err != nil || got.State != protocol.CardStateBacklog {
		t.Errorf("the other card = %+v, %v, want it left in the backlog", got, err)
	}
}

// Two messages sent together to a chat that is not running start one process, not two.
func TestTwoFirstMessagesStartOneProcess(t *testing.T) {
	e := newChatEnv(t)
	project := e.project(t, "small-repo")
	chat := e.newChat(t, project.ID, protocol.CreateChatRequest{})
	release := make(chan struct{})
	e.agent.startHold = release

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, text := range []string{"one", "two"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- e.chats.Send(context.Background(), chat.ID, text)
		}()
	}
	// The first sender is inside Start, and the second waits behind it.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("a message was refused: %v", err)
		}
	}
	if got := fakeStarts(e.agent); got != 1 {
		t.Errorf("%d agents were started, want 1", got)
	}
	got := e.waitForChatHistory(t, chat.ID, 4)
	if users := kindEvents(t, got, history.KindUser); len(users) != 2 {
		t.Errorf("the chat stored %d messages from the person, want 2: %v", len(users), summaries(got))
	}
}

// Archiving a chat puts its session to sleep and keeps the conversation: the process ends, the row
// reads asleep on the same agent session id, and the next message after a restore wakes it, waking
// then awake, and the agent remembers what was said before.
func TestASleepingChatWakesWithItsConversation(t *testing.T) {
	e := newChatEnv(t)
	project := e.project(t, "small-repo")
	chat := e.newChat(t, project.ID, protocol.CreateChatRequest{})
	e.send(t, chat.ID, "first question")
	e.waitForChatHistory(t, chat.ID, 2)
	before := e.row(t, chat.ID)
	e.untilChatState(t, chat.ID, protocol.SessionStateAwake)

	// The chat must be at rest for the archive to find it awake and not working.
	waitForRowState(t, e, chat.ID, protocol.SessionStateAwake)
	if _, err := e.chats.Archive(context.Background(), chat.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	e.untilChatState(t, chat.ID, protocol.SessionStateAsleep)
	slept := e.row(t, chat.ID)
	if slept.State != string(protocol.SessionStateAsleep) || slept.AgentSessionID != before.AgentSessionID {
		t.Fatalf("the archived chat's session = %+v, want asleep on the same agent session %q", slept, before.AgentSessionID)
	}
	if e.mgr.RecentOutput(chat.ID) != nil {
		t.Error("the archived chat still has a live session")
	}
	err := e.chats.Send(context.Background(), chat.ID, "are you there?")
	if perr := wantCode(t, err, protocol.ErrorCodeRefused); perr.Details["reason"] != string(protocol.ChatRefusalReasonArchived) {
		t.Errorf("a message to the archived chat was refused with %+v", perr.Details)
	}

	if _, err := e.chats.Restore(context.Background(), chat.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if e.row(t, chat.ID).State != string(protocol.SessionStateAsleep) {
		t.Error("restoring the chat woke its session by itself; only a message does that")
	}
	e.send(t, chat.ID, "second question")
	e.untilChatState(t, chat.ID, protocol.SessionStateWaking)
	woken := e.untilChatState(t, chat.ID, protocol.SessionStateAwake)
	if woken.SessionID != before.ID {
		t.Errorf("the woken session = %q, want the chat's own %q", woken.SessionID, before.ID)
	}
	if got := fakeStarts(e.agent); got != 1 {
		t.Errorf("%d agents were started, want the one that slept and woke", got)
	}
	answers := kindEvents(t, e.waitForChatHistory(t, chat.ID, 4), history.KindAgent)
	if len(answers) != 2 || !strings.Contains(answers[1].Summary, "turn 2 remembers 1 earlier turns") {
		t.Errorf("the answers = %v, want the second to remember the first turn", summaries(answers))
	}
}

// waitForRowState waits until a chat's session row reads a state, which is how a test waits for the
// pump to finish a turn.
func waitForRowState(t *testing.T, e *chatEnv, chatID string, state protocol.SessionState) {
	t.Helper()
	deadline := time.Now().Add(eventTimeout)
	for time.Now().Before(deadline) {
		if e.row(t, chatID).State == string(state) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("the session of chat %s never reached %s", chatID, state)
}

// A daemon that stops takes its chats' processes with it, and writes their rows asleep, because
// nothing starts a chat on its own. A new manager over the same store does not start any, and the
// next message resumes the same conversation.
func TestAChatSurvivesARestartAsleep(t *testing.T) {
	e := newChatEnv(t)
	project := e.project(t, "small-repo")
	chat := e.newChat(t, project.ID, protocol.CreateChatRequest{})
	e.send(t, chat.ID, "first question")
	e.waitForChatHistory(t, chat.ID, 2)
	waitForRowState(t, e, chat.ID, protocol.SessionStateAwake)
	before := e.row(t, chat.ID)

	if err := e.mgr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := e.row(t, chat.ID); got.State != string(protocol.SessionStateAsleep) || got.AgentSessionID != before.AgentSessionID {
		t.Fatalf("the chat's session after the daemon stopped = %+v, want asleep on the same agent session", got)
	}

	fresh := e.reopen(t)
	t.Cleanup(func() { _ = fresh.Close() })
	if err := fresh.RestoreAll(context.Background()); err != nil {
		t.Fatalf("RestoreAll: %v", err)
	}
	if got := fakeStarts(e.agent); got != 1 || len(e.agent.startSpecs()) != 1 {
		t.Fatalf("the restore started or resumed a process for a chat: %d starts, %d specs", got, len(e.agent.startSpecs()))
	}
	chatsAfter := e.chatsWith(t, fresh)
	if err := chatsAfter.Send(context.Background(), chat.ID, "second question"); err != nil {
		t.Fatalf("Send after the restart: %v", err)
	}
	answers := kindEvents(t, e.waitForChatHistory(t, chat.ID, 4), history.KindAgent)
	if len(answers) != 2 || !strings.Contains(answers[1].Summary, "turn 2 remembers 1 earlier turns") {
		t.Errorf("the answers after the restart = %v, want the second to remember the first turn", summaries(answers))
	}
	if specs := e.agent.startSpecs(); len(specs) != 2 || specs[1].Cwd != project.Path {
		t.Errorf("the resume was given %+v, want the project's folder again", specs)
	}
}

// A chat whose agent cannot pick the old conversation back up is refused, with its own sentence and
// reason and the row left stopped. Marshal does not start a new conversation in its place. Once the
// agent can resume, the next message does.
func TestAChatThatCannotBeResumedIsRefused(t *testing.T) {
	e := newChatEnv(t)
	project := e.project(t, "small-repo")
	chat := e.newChat(t, project.ID, protocol.CreateChatRequest{})
	e.send(t, chat.ID, "first question")
	e.waitForChatHistory(t, chat.ID, 2)
	waitForRowState(t, e, chat.ID, protocol.SessionStateAwake)
	if err := e.mgr.StopChatSession(context.Background(), chat.ID); err != nil {
		t.Fatalf("StopChatSession: %v", err)
	}

	e.agent.mu.Lock()
	e.agent.resumeErr = fmt.Errorf("%w: gone", agents.ErrCannotResume)
	e.agent.mu.Unlock()
	err := e.chats.Send(context.Background(), chat.ID, "second question")
	perr := wantCode(t, err, protocol.ErrorCodeRefused)
	if perr.Message != "Marshal could not pick this chat's conversation back up. Start a new chat to keep going." ||
		perr.Details["reason"] != string(protocol.ChatRefusalReasonCannotResume) || perr.Details["chatId"] != chat.ID {
		t.Errorf("the refusal = %+v", perr)
	}
	stopped := e.untilChatState(t, chat.ID, protocol.SessionStateStopped)
	if stopped.Reason != perr.Message {
		t.Errorf("the stopped event says %q, want the refusal's sentence", stopped.Reason)
	}
	if row := e.row(t, chat.ID); row.State != string(protocol.SessionStateStopped) || row.AgentSessionID == "" {
		t.Errorf("the session after the failed resume = %+v, want stopped and still holding its agent session id", row)
	}
	if got := fakeStarts(e.agent); got != 1 {
		t.Errorf("a fresh agent was started in place of the lost conversation: %d starts", got)
	}

	e.agent.mu.Lock()
	e.agent.resumeErr = nil
	e.agent.mu.Unlock()
	e.send(t, chat.ID, "second question, again")
	e.untilChatState(t, chat.ID, protocol.SessionStateAwake)
	answers := kindEvents(t, e.waitForChatHistory(t, chat.ID, 4), history.KindAgent)
	if len(answers) != 2 || !strings.Contains(answers[1].Summary, "remembers 1 earlier turns") {
		t.Errorf("the answers = %v, want the resumed session to remember the first turn", summaries(answers))
	}
}

// A chat whose agent cannot start is refused with a sentence that says "chat", and nothing is left
// half made: the session row is as it was, and a later message can start it.
func TestAChatWhoseAgentWillNotStart(t *testing.T) {
	e := newChatEnv(t)
	project := e.project(t, "small-repo")
	chat := e.newChat(t, project.ID, protocol.CreateChatRequest{})
	e.agent.mu.Lock()
	e.agent.startErr = errors.New("the program is missing")
	e.agent.mu.Unlock()

	err := e.chats.Send(context.Background(), chat.ID, "hello")
	perr := wantCode(t, err, protocol.ErrorCodeUnavailable)
	if !strings.Contains(perr.Message, "start the agent for this chat") || perr.Details["chatId"] != chat.ID {
		t.Errorf("the refusal = %+v, want the chat sentence with the chat's id", perr)
	}
	if row := e.row(t, chat.ID); row.State != string(protocol.SessionStateStarting) || row.AgentSessionID != "" {
		t.Errorf("the session after a failed start = %+v, want it untouched", row)
	}
	if hist := e.chatHistory(t, chat.ID); len(hist) != 0 {
		t.Errorf("a message that never reached an agent was stored: %v", summaries(hist))
	}

	e.agent.mu.Lock()
	e.agent.startErr = nil
	e.agent.mu.Unlock()
	e.send(t, chat.ID, "hello again")
	e.untilChatState(t, chat.ID, protocol.SessionStateAwake)
}

// An agent kind that has no adapter yet is a plain refusal, and it does not stop the chat.
func TestAChatWithAnAgentThatIsNotReadyIsRefused(t *testing.T) {
	e := newChatEnv(t)
	project := e.project(t, "small-repo")
	chat := e.newChat(t, project.ID, protocol.CreateChatRequest{AgentKind: protocol.AgentKindCodex})
	err := e.chats.Send(context.Background(), chat.ID, "hello")
	perr := wantCode(t, err, protocol.ErrorCodeUnsupported)
	if perr.Message != "Marshal does not have that agent ready to run yet." {
		t.Errorf("the refusal = %q", perr.Message)
	}
}

// An agent that dies on its own leaves the chat's row stopped and says so on the chat's topic. There
// is no card to move to "needs you", and the next message tries to pick the conversation back up.
func TestAChatWhoseAgentDiesIsMarkedStopped(t *testing.T) {
	e := newChatEnv(t)
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Bystander")
	chat := e.newChat(t, project.ID, protocol.CreateChatRequest{})
	e.send(t, chat.ID, "first question")
	e.waitForChatHistory(t, chat.ID, 2)
	waitForRowState(t, e, chat.ID, protocol.SessionStateAwake)

	e.agent.crash(e.row(t, chat.ID).AgentSessionID, "the process died")
	stopped := e.untilChatState(t, chat.ID, protocol.SessionStateStopped)
	if stopped.Reason != "The agent stopped unexpectedly." {
		t.Errorf("the reason = %q", stopped.Reason)
	}
	if got, _ := e.proj.Card(context.Background(), card.ID); got.State != protocol.CardStateBacklog {
		t.Errorf("a chat's agent dying moved the card %s to %s", card.ID, got.State)
	}
	e.send(t, chat.ID, "are you back?")
	e.untilChatState(t, chat.ID, protocol.SessionStateAwake)
}

// Messages sent while a chat's turn is running are queued and delivered one after another, and the
// queue is bounded: a full queue is refused with a sentence that names the chat.
func TestAChatQueuesMessagesWhileATurnRuns(t *testing.T) {
	e := newChatEnv(t)
	project := e.project(t, "small-repo")
	chat := e.newChat(t, project.ID, protocol.CreateChatRequest{})
	e.agent.hold = make(chan struct{})
	t.Cleanup(func() { close(e.agent.hold) })

	e.send(t, chat.ID, "first")
	var refused error
	for i := 0; i < 60 && refused == nil; i++ {
		refused = e.chats.Send(context.Background(), chat.ID, fmt.Sprintf("queued %d", i))
	}
	perr := wantCode(t, refused, protocol.ErrorCodeRefused)
	if perr.Message != "There are already too many messages waiting for this agent. Wait for it to catch up." ||
		perr.Details["chatId"] != chat.ID || perr.Details["cardId"] != "" {
		t.Errorf("the refusal of a full queue = %+v, want the shared sentence naming the chat", perr)
	}
}

// StopChatSession and RemoveChatLogs are safe to call for a chat that never talked, and for one
// that is not there at all: the chats module calls them for every archive and every delete.
func TestStoppingAChatThatNeverTalkedChangesNothing(t *testing.T) {
	e := newChatEnv(t)
	project := e.project(t, "small-repo")
	chat := e.newChat(t, project.ID, protocol.CreateChatRequest{})
	ctx := context.Background()

	if err := e.mgr.StopChatSession(ctx, chat.ID); err != nil {
		t.Fatalf("StopChatSession: %v", err)
	}
	if row := e.row(t, chat.ID); row.State != string(protocol.SessionStateStarting) {
		t.Errorf("stopping a chat that never talked left its session %s, want starting", row.State)
	}
	if err := e.mgr.RemoveChatLogs(ctx, chat.ID); err != nil {
		t.Errorf("RemoveChatLogs of a chat with no logs = %v", err)
	}
	if err := e.mgr.StopChatSession(ctx, "01M3C107JB041061050R3GG28Z"); err != nil {
		t.Errorf("StopChatSession of a chat that is not there = %v", err)
	}
	if err := e.mgr.RemoveChatLogs(ctx, "01M3C107JB041061050R3GG28Z"); err != nil {
		t.Errorf("RemoveChatLogs of a chat that is not there = %v", err)
	}
}

// Deleting a chat stops its process, deletes the folder its session wrote its log in, and takes its
// history with it.
func TestDeletingAChatStopsItAndRemovesItsLogs(t *testing.T) {
	e := newChatEnv(t)
	project := e.project(t, "small-repo")
	chat := e.newChat(t, project.ID, protocol.CreateChatRequest{})
	e.send(t, chat.ID, "first question")
	e.waitForChatHistory(t, chat.ID, 2)
	waitForRowState(t, e, chat.ID, protocol.SessionStateAwake)
	sessionID := e.row(t, chat.ID).ID
	logs := e.dataDir + "/logs/sessions/" + sessionID
	if _, err := os.Stat(logs); err != nil {
		t.Fatalf("the running chat has no log folder: %v", err)
	}

	if err := e.chats.Remove(context.Background(), chat.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if e.mgr.RecentOutput(chat.ID) != nil {
		t.Error("the deleted chat still has a live session")
	}
	if _, err := os.Stat(logs); !os.IsNotExist(err) {
		t.Errorf("the deleted chat's log folder is still there: %v", err)
	}
	if hist := e.chatHistory(t, chat.ID); len(hist) != 0 {
		t.Errorf("the deleted chat kept %d history events", len(hist))
	}
}

// Removing a project stops its chats' processes as well as its cards', and the awake count is about
// cards only.
func TestARemovedProjectStopsItsChatsToo(t *testing.T) {
	e := newChatEnv(t)
	project := e.project(t, "small-repo")
	other := e.project(t, "small-repo")
	chat := e.newChat(t, project.ID, protocol.CreateChatRequest{})
	otherChat := e.newChat(t, other.ID, protocol.CreateChatRequest{})
	e.send(t, chat.ID, "hello")
	e.send(t, otherChat.ID, "hello")
	waitForRowState(t, e, chat.ID, protocol.SessionStateAwake)
	waitForRowState(t, e, otherChat.ID, protocol.SessionStateAwake)

	if n, err := e.mgr.AwakeCards(context.Background(), project.ID); err != nil || n != 0 {
		t.Errorf("AwakeCards = %d, %v, want 0: a chat is not an awake card", n, err)
	}
	if err := e.mgr.StopProjectSessions(context.Background(), project.ID); err != nil {
		t.Fatalf("StopProjectSessions: %v", err)
	}
	if e.mgr.RecentOutput(chat.ID) != nil {
		t.Error("the removed project's chat is still running")
	}
	if e.mgr.RecentOutput(otherChat.ID) == nil {
		t.Error("another project's chat was stopped with it")
	}
}

// A message that arrives while the daemon is shutting down is refused before any process is started.
// (A start that is already in flight when the manager closes is ended by goLive, which
// TestGoLiveStopsTheAgentWhenTheManagerIsAlreadyClosed covers.)
func TestAChatMessageDuringShutdownStartsNoProcess(t *testing.T) {
	e := newChatEnv(t)
	project := e.project(t, "small-repo")
	chat := e.newChat(t, project.ID, protocol.CreateChatRequest{})
	if err := e.mgr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err := e.chats.Send(context.Background(), chat.ID, "too late")
	_ = wantCode(t, err, protocol.ErrorCodeUnavailable)
	if e.mgr.RecentOutput(chat.ID) != nil || len(e.agent.startSpecs()) != 0 {
		t.Errorf("a chat started while the daemon was shutting down: %d agents started", len(e.agent.startSpecs()))
	}
}

// A tool call of a chat's agent goes out on the chat's topic with the chat's id, and is stored in the
// chat's history the way a card's tool calls are, so the block can be opened later.
func TestAChatsToolCallsGoToItsTopicAndHistory(t *testing.T) {
	e := newChatEnv(t)
	project := e.project(t, "small-repo")
	chat := e.newChat(t, project.ID, protocol.CreateChatRequest{})
	e.send(t, chat.ID, "list the cards")
	e.waitForChatHistory(t, chat.ID, 2)
	waitForRowState(t, e, chat.ID, protocol.SessionStateAwake)
	fake, err := e.agent.find(e.row(t, chat.ID).AgentSessionID)
	if err != nil {
		t.Fatalf("find the fake session: %v", err)
	}
	fake.sink.Emit(agents.ToolCall{ID: "call-1", Title: "List the cards", Kind: "other", Status: agents.StatusInProgress})
	fake.sink.Emit(agents.ToolCallUpdate{ID: "call-1", Status: agents.StatusCompleted, Content: "3 cards"})

	var calls []protocol.SessionToolCallEventData
	timeout := time.After(eventTimeout)
	for len(calls) < 2 {
		select {
		case ev := <-e.sub.C():
			data, ok := ev.Data.(protocol.SessionToolCallEventData)
			if ev.Type != string(protocol.EventTypeSessionToolCall) || !ok {
				continue
			}
			if ev.Topic != string(protocol.ChatTopic(chat.ID)) || data.ChatID != chat.ID || data.CardID != "" {
				t.Errorf("a chat's tool call = %+v on %s, want the chat's id on the chat's topic", data, ev.Topic)
			}
			calls = append(calls, data)
		case <-timeout:
			t.Fatalf("saw %d tool call events, want 2", len(calls))
		}
	}
	if calls[0].Kind != "tool_call" || calls[1].Kind != "tool_call_update" || calls[1].ToolCall.Content != "3 cards" {
		t.Errorf("the tool call events = %+v", calls)
	}
	tools := kindEvents(t, e.waitForChatHistory(t, chat.ID, 4), history.KindToolCall)
	if len(tools) != 1 || tools[0].Summary != "List the cards" || tools[0].ChatID != chat.ID {
		t.Errorf("the stored tool calls = %+v, want the one call in the chat's history", tools)
	}
}

// A session that would not end is not put to sleep: the chat is still running, and says so.
func TestAChatWhoseAgentWillNotStopIsLeftRunning(t *testing.T) {
	e := newChatEnv(t)
	project := e.project(t, "small-repo")
	chat := e.newChat(t, project.ID, protocol.CreateChatRequest{})
	e.send(t, chat.ID, "hello")
	e.waitForChatHistory(t, chat.ID, 2)
	waitForRowState(t, e, chat.ID, protocol.SessionStateAwake)

	e.agent.mu.Lock()
	e.agent.stopErr = errors.New("the agent refused to end")
	e.agent.mu.Unlock()
	if err := e.mgr.StopChatSession(context.Background(), chat.ID); err == nil {
		t.Fatal("StopChatSession succeeded although the agent would not end")
	}
	if err := e.mgr.StopProjectSessions(context.Background(), project.ID); err == nil {
		t.Error("StopProjectSessions succeeded although a chat's agent would not end")
	}
	if e.mgr.RecentOutput(chat.ID) == nil {
		t.Error("the chat's session was forgotten although its process is still there")
	}
	if row := e.row(t, chat.ID); row.State != string(protocol.SessionStateAwake) {
		t.Errorf("the session reads %s, want awake", row.State)
	}
	// The session is still one a later stop can end, and its exit is still not taken for a crash.
	e.agent.mu.Lock()
	e.agent.stopErr = nil
	e.agent.mu.Unlock()
	if err := e.mgr.StopChatSession(context.Background(), chat.ID); err != nil {
		t.Fatalf("StopChatSession once the agent can end: %v", err)
	}
	e.untilChatState(t, chat.ID, protocol.SessionStateAsleep)
}

// A chat whose session already stopped or fell asleep is not written asleep again, and a chat with
// no session row is a plain error rather than a panic.
func TestAChatsSessionRowEdgeCases(t *testing.T) {
	e := newChatEnv(t)
	project := e.project(t, "small-repo")
	chat := e.newChat(t, project.ID, protocol.CreateChatRequest{})
	e.send(t, chat.ID, "hello")
	e.waitForChatHistory(t, chat.ID, 2)
	waitForRowState(t, e, chat.ID, protocol.SessionStateAwake)
	e.agent.crash(e.row(t, chat.ID).AgentSessionID, "the process died")
	e.untilChatState(t, chat.ID, protocol.SessionStateStopped)

	if err := e.mgr.StopChatSession(context.Background(), chat.ID); err != nil {
		t.Fatalf("StopChatSession of a stopped chat: %v", err)
	}
	if row := e.row(t, chat.ID); row.State != string(protocol.SessionStateStopped) {
		t.Errorf("stopping a chat that had stopped rewrote its session as %s", row.State)
	}

	orphan := e.newChat(t, project.ID, protocol.CreateChatRequest{})
	if err := e.store.Write(context.Background(), func(q *db.Queries) error {
		_, err := q.DeleteSession(context.Background(), e.row(t, orphan.ID).ID)
		return err
	}); err != nil {
		t.Fatalf("delete the session row: %v", err)
	}
	if err := e.chats.Send(context.Background(), orphan.ID, "hello"); err == nil {
		t.Error("a chat with no session row took a message")
	}
}
