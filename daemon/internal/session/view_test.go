package session_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
)

// The chat view and the terminal view of a card (checklist item B2.7, docs/architecture.md 4.3):
// switching stops the process and resumes the same session id in the other mode. The terminal is a
// fake that records what it is asked to do, so every rule is checked without a process; the real
// pseudo-terminal is proved in the api package and in agents/pty.

// terminalRegistry is the terminals of a test: the fake for the default card agent.
func terminalRegistry(t *testing.T, term agents.Agent) *agents.Registry {
	t.Helper()
	reg := agents.NewRegistry()
	if err := reg.Register(protocol.AgentKindClaude, func() (agents.Agent, error) { return term, nil }); err != nil {
		t.Fatalf("register the fake terminal: %v", err)
	}
	return reg
}

// viewEnv is an env whose manager can also run a card's agent in a terminal, through a fake.
func viewEnv(t *testing.T) (*env, *fakeTerminal) {
	t.Helper()
	term := newFakeTerminal()
	e := newEnv(t, func(c *session.Config) { c.Terminals = terminalRegistry(t, term) })
	t.Cleanup(func() { _ = e.mgr.Close() })
	return e, term
}

// startedCard starts a card in the chat view and waits for its session to be awake.
func startedCard(t *testing.T, e *env, title string) protocol.Card {
	t.Helper()
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, title)
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	e.untilState(t, card.ID, protocol.SessionStateAwake)
	return card
}

// inTerminal switches a started card to the terminal view and waits for the card.updated that says
// so.
func inTerminal(t *testing.T, e *env, card protocol.Card) protocol.CardView {
	t.Helper()
	view, err := e.mgr.SwitchView(context.Background(), card.ID, protocol.CardViewModeTerminal)
	if err != nil {
		t.Fatalf("SwitchView(terminal): %v", err)
	}
	return view
}

// wantReason fails the test unless err is a refusal with this stable reason and sentence.
func wantReason(t *testing.T, err error, reason, message string) {
	t.Helper()
	refusal := wantCode(t, err, protocol.ErrorCodeRefused)
	if refusal.Details["reason"] != reason || refusal.Message != message {
		t.Errorf("refused with reason %q and %q, want %q and %q", refusal.Details["reason"], refusal.Message, reason, message)
	}
}

// storedView reads a card's session row: its state, its view, and its agent session id.
func (e *env) storedView(t *testing.T, cardID string) (state, view, agentSessionID string) {
	t.Helper()
	row, err := e.store.Queries().GetSessionByCard(context.Background(), cardID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	return row.State, row.ViewMode, row.AgentSessionID
}

// untilOutput reads events until a terminal output event of the card arrives, and returns it.
func (e *env) untilOutput(t *testing.T, cardID string) events.Event {
	t.Helper()
	for {
		ev := e.untilType(t, protocol.EventTypeSessionTerminalOutput)
		if data, ok := ev.Data.(protocol.TerminalOutputEventData); ok && data.CardID == cardID {
			return ev
		}
	}
}

// eventually polls a condition, which is how a test waits for a goroutine of the manager.
func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(eventTimeout)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("%s did not happen before the deadline", what)
}

// screenOf reads a card's terminal screen and decodes it.
func screenOf(t *testing.T, e *env, cardID string) (data []byte, screen protocol.TerminalScreen) {
	t.Helper()
	screen, err := e.mgr.TerminalScreen(cardID)
	if err != nil {
		t.Fatalf("TerminalScreen: %v", err)
	}
	data, err = base64.StdEncoding.DecodeString(screen.Data)
	if err != nil {
		t.Fatalf("the screen is not base64: %v", err)
	}
	return data, screen
}

// The switch stops the chat process and resumes the same agent session id in the terminal, and back,
// and it finishes quickly. Nothing else about the card changes: it stays where it was on the board.
func TestSwitchingViewsResumesTheSameSessionEachWay(t *testing.T) {
	e, term := viewEnv(t)
	card := startedCard(t, e, "Switch me")
	_, view, before := e.storedView(t, card.ID)
	if view != "chat" || before == "" {
		t.Fatalf("a started card's session is in the %q view on agent session %q", view, before)
	}

	begun := time.Now()
	answer, err := e.mgr.SwitchView(context.Background(), card.ID, protocol.CardViewModeTerminal)
	if err != nil {
		t.Fatalf("SwitchView(terminal): %v", err)
	}
	if took := time.Since(begun); took > 3*time.Second {
		t.Errorf("the switch took %v, want under the 3 seconds architecture.md 4.3 promises", took)
	}
	if answer.CardID != card.ID || answer.Mode != protocol.CardViewModeTerminal || answer.Session != protocol.SessionStateAwake ||
		answer.ServerTime.Time().IsZero() {
		t.Errorf("the answer = %+v", answer)
	}
	if ids := term.resumedIDs(); len(ids) != 1 || ids[0] != before {
		t.Errorf("the terminal resumed %v, want the chat's own session id %q", ids, before)
	}
	spec := term.startSpecs()[0]
	worktree, _, err := e.proj.Worktree(context.Background(), card.ID)
	if err != nil || spec.Cwd != worktree || spec.Label != card.ID {
		t.Errorf("the terminal was started in %q as %q (%v), want the card's worktree %q", spec.Cwd, spec.Label, err, worktree)
	}
	state, view, after := e.storedView(t, card.ID)
	if state != "awake" || view != "terminal" || after != before {
		t.Errorf("the row = %s, %s, %q, want awake in the terminal on the same id %q", state, view, after, before)
	}
	got := e.untilCard(t, card.ID, func(c protocol.Card) bool { return c.ViewMode == protocol.CardViewModeTerminal })
	if got.Session == nil || *got.Session != protocol.SessionStateAwake || got.State != protocol.CardStateWorking {
		t.Errorf("the card after the switch = state %s, session %v: the switch must not move the card", got.State, got.Session)
	}
	if starts := fakeStarts(e.agent); starts != 1 {
		t.Errorf("the chat agent started %d sessions, want the one it began with", starts)
	}

	// Back to the chat: the chat agent resumes the same id, and the conversation goes on.
	back, err := e.mgr.SwitchView(context.Background(), card.ID, protocol.CardViewModeChat)
	if err != nil || back.Mode != protocol.CardViewModeChat || back.Session != protocol.SessionStateAwake {
		t.Fatalf("SwitchView(chat) = %+v, %v", back, err)
	}
	if specs := e.agent.startSpecs(); len(specs) != 2 {
		t.Errorf("the chat agent was asked for %d processes, want the start and one resume", len(specs))
	}
	if starts := fakeStarts(e.agent); starts != 1 {
		t.Errorf("going back started a new chat session: %d sessions", starts)
	}
	if state, view, id := e.storedView(t, card.ID); state != "awake" || view != "chat" || id != before {
		t.Errorf("the row = %s, %s, %q, want awake in the chat on %q", state, view, id, before)
	}
	e.untilCard(t, card.ID, func(c protocol.Card) bool { return c.ViewMode == protocol.CardViewModeChat && c.Session != nil })
	if err := e.mgr.Send(context.Background(), card.ID, "still there?"); err != nil {
		t.Fatalf("Send after the way back: %v", err)
	}
	waitForHistory(t, e, card.ID, 2)
	answered := oneKind(t, e.historyOf(t, card.ID), "agent")
	if !strings.Contains(answered.Summary, "still there?") {
		t.Errorf("the answer = %q", answered.Summary)
	}
}

// Asking for the view the card is already in is not an error, and starts nothing.
func TestSwitchingToTheViewTheCardIsInChangesNothing(t *testing.T) {
	e, term := viewEnv(t)
	card := startedCard(t, e, "Already there")
	answer, err := e.mgr.SwitchView(context.Background(), card.ID, protocol.CardViewModeChat)
	if err != nil || answer.Mode != protocol.CardViewModeChat || answer.Session != protocol.SessionStateAwake {
		t.Fatalf("SwitchView(chat) in the chat = %+v, %v", answer, err)
	}
	inTerminal(t, e, card)
	answer, err = e.mgr.SwitchView(context.Background(), card.ID, protocol.CardViewModeTerminal)
	if err != nil || answer.Mode != protocol.CardViewModeTerminal {
		t.Fatalf("SwitchView(terminal) in the terminal = %+v, %v", answer, err)
	}
	if ids := term.resumedIDs(); len(ids) != 1 {
		t.Errorf("the terminal was resumed %d times, want 1", len(ids))
	}
	if specs := e.agent.startSpecs(); len(specs) != 1 {
		t.Errorf("the chat agent was asked for %d processes, want only the start", len(specs))
	}
}

// Every refusal leaves the card and its session exactly as they were, and names the reason.
func TestASwitchThatIsRefusedChangesNothing(t *testing.T) {
	t.Run("a card that does not exist", func(t *testing.T) {
		e, _ := viewEnv(t)
		_, err := e.mgr.SwitchView(context.Background(), "01M3C107JB041061050R3GG28A", protocol.CardViewModeTerminal)
		_ = wantCode(t, err, protocol.ErrorCodeNotFound)
	})
	t.Run("a card that never started", func(t *testing.T) {
		e, term := viewEnv(t)
		card := e.card(t, e.project(t, "small-repo").ID, "Never started")
		_, err := e.mgr.SwitchView(context.Background(), card.ID, protocol.CardViewModeTerminal)
		wantReason(t, err, "view_no_agent", "This card has no agent running. Start the card first.")
		if len(term.resumedIDs()) != 0 {
			t.Error("a terminal was started for a card that never started")
		}
	})
	t.Run("a card that was stopped", func(t *testing.T) {
		e, _ := viewEnv(t)
		card := startedCard(t, e, "Stopped")
		if err := e.mgr.Stop(context.Background(), card.ID); err != nil {
			t.Fatalf("Stop: %v", err)
		}
		_, err := e.mgr.SwitchView(context.Background(), card.ID, protocol.CardViewModeTerminal)
		wantReason(t, err, "view_no_agent", "This card has no agent running. Start the card first.")
	})
	t.Run("a card that is asleep", func(t *testing.T) {
		e, _ := viewEnv(t)
		card := startedCard(t, e, "Asleep")
		if _, err := e.mgr.Pause(context.Background(), card.ID); err != nil {
			t.Fatalf("Pause: %v", err)
		}
		if err := e.mgr.Sleep(context.Background(), card.ID); err != nil {
			t.Fatalf("Sleep: %v", err)
		}
		_, err := e.mgr.SwitchView(context.Background(), card.ID, protocol.CardViewModeTerminal)
		wantReason(t, err, "view_no_agent", "This card has no agent running. Start the card first.")
	})
}

// A turn that is running is not cut off. The process keeps running, the card is untouched, and the
// switch works once the turn is over.
func TestASwitchWaitsForARunningTurn(t *testing.T) {
	e, term := viewEnv(t)
	e.agent.hold = make(chan struct{})
	t.Cleanup(func() { close(e.agent.hold) })
	card := startedCard(t, e, "Mid turn")
	if err := e.mgr.Send(context.Background(), card.ID, "think about it"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	e.untilState(t, card.ID, protocol.SessionStateWorking)

	_, err := e.mgr.SwitchView(context.Background(), card.ID, protocol.CardViewModeTerminal)
	wantReason(t, err, "view_turn_running", "The agent is in the middle of a turn. Wait for it to finish, then switch views.")
	if len(term.resumedIDs()) != 0 {
		t.Error("a terminal was started while a turn was running")
	}
	if _, view, _ := e.storedView(t, card.ID); view != "chat" {
		t.Errorf("the refused switch left the card in the %q view", view)
	}

	e.agent.hold <- struct{}{} // let the turn end
	e.untilState(t, card.ID, protocol.SessionStateAwake)
	if _, err := e.mgr.SwitchView(context.Background(), card.ID, protocol.CardViewModeTerminal); err != nil {
		t.Fatalf("SwitchView after the turn: %v", err)
	}
}

// A paused card that is holding a message cannot switch: the message waits in memory with the process.
func TestASwitchIsRefusedWhileAPauseHoldsAMessage(t *testing.T) {
	e, term := viewEnv(t)
	e.agent.hold = make(chan struct{})
	t.Cleanup(func() { close(e.agent.hold) })
	card := startedCard(t, e, "Holding")
	if err := e.mgr.Send(context.Background(), card.ID, "first"); err != nil {
		t.Fatalf("Send(first): %v", err)
	}
	e.untilState(t, card.ID, protocol.SessionStateWorking)
	if _, err := e.mgr.Pause(context.Background(), card.ID); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "second"); err != nil {
		t.Fatalf("Send(second): %v", err)
	}
	e.agent.hold <- struct{}{} // the first turn ends, and the pause keeps the second message waiting
	e.untilState(t, card.ID, protocol.SessionStateAwake)

	_, err := e.mgr.SwitchView(context.Background(), card.ID, protocol.CardViewModeTerminal)
	wantReason(t, err, "view_holding_messages", "This card has a message waiting for you to resume it. Resume the card first.")
	if len(term.resumedIDs()) != 0 {
		t.Error("a terminal was started while a message was waiting")
	}
}

// An agent with no terminal mode is refused before anything is stopped: the chat process is still
// running and the card can keep talking.
func TestAnAgentWithNoTerminalIsRefusedBeforeAnythingIsStopped(t *testing.T) {
	cases := map[string]func(*session.Config){
		"no terminals at all": func(c *session.Config) { c.Terminals = nil },
		"no terminal for this kind": func(c *session.Config) {
			reg := agents.NewRegistry()
			_ = reg.Register(protocol.AgentKindGemini, func() (agents.Agent, error) { return newFakeTerminal(), nil })
			c.Terminals = reg
		},
		"a terminal that cannot resume": func(c *session.Config) {
			term := newFakeTerminal()
			term.caps.Resume = false
			c.Terminals = terminalRegistry(t, term)
		},
		"an agent that is not a terminal": func(c *session.Config) {
			c.Terminals = terminalRegistry(t, newFakeAgent(agents.Capabilities{Resume: true}))
		},
	}
	for name, configure := range cases {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, configure)
			t.Cleanup(func() { _ = e.mgr.Close() })
			card := startedCard(t, e, "No terminal")
			_, err := e.mgr.SwitchView(context.Background(), card.ID, protocol.CardViewModeTerminal)
			wantReason(t, err, "view_no_terminal", "This agent has no terminal view. Stay in the chat view.")
			if specs := e.agent.startSpecs(); len(specs) != 1 {
				t.Errorf("the chat agent was asked for %d processes, want only the start: the switch must not stop it", len(specs))
			}
			if err := e.mgr.Send(context.Background(), card.ID, "still here"); err != nil {
				t.Errorf("Send after a refused switch: %v", err)
			}
		})
	}
}

// A new process that cannot pick the session up follows section 5.3: the card needs the person, the
// session stops, and Start brings it back in the chat view, on the same conversation.
func TestASwitchThatCannotResumeMovesTheCardToNeedsYou(t *testing.T) {
	e, term := viewEnv(t)
	card := startedCard(t, e, "Cannot resume")
	_, _, before := e.storedView(t, card.ID)
	term.setResumeErr(agents.ErrCannotResume)

	_, err := e.mgr.SwitchView(context.Background(), card.ID, protocol.CardViewModeTerminal)
	wantReason(t, err, "view_cannot_resume", "Marshal could not pick this session back up. The card now needs you.")
	stopped, _ := e.untilState(t, card.ID, protocol.SessionStateStopped)
	if stopped.Reason != "Marshal could not pick this session back up. The card now needs you." {
		t.Errorf("the stopped event says %q", stopped.Reason)
	}
	got := e.untilCard(t, card.ID, func(c protocol.Card) bool { return c.State == protocol.CardStateNeeds })
	if got.ViewMode != protocol.CardViewModeChat || got.Session == nil || *got.Session != protocol.SessionStateStopped {
		t.Errorf("the card = view %s, session %v, want the chat view and a stopped session", got.ViewMode, got.Session)
	}
	if state, view, id := e.storedView(t, card.ID); state != "stopped" || view != "chat" || id != before {
		t.Errorf("the row = %s, %s, %q, want stopped in the chat view with the id kept", state, view, id)
	}

	// Start continues the same conversation in the chat view, as it does for any stopped session.
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if starts := fakeStarts(e.agent); starts != 1 {
		t.Errorf("Start began a new conversation: %d sessions", starts)
	}
}

// The way back is the same rule the other way round.
func TestASwitchBackThatCannotResumeMovesTheCardToNeedsYou(t *testing.T) {
	e, _ := viewEnv(t)
	card := startedCard(t, e, "Cannot resume in the chat")
	inTerminal(t, e, card)
	e.agent.mu.Lock()
	e.agent.resumeErr = agents.ErrCannotResume
	e.agent.mu.Unlock()

	_, err := e.mgr.SwitchView(context.Background(), card.ID, protocol.CardViewModeChat)
	wantReason(t, err, "view_cannot_resume", "Marshal could not pick this session back up. The card now needs you.")
	if state, view, _ := e.storedView(t, card.ID); state != "stopped" || view != "chat" {
		t.Errorf("the row = %s, %s, want stopped in the chat view", state, view)
	}
	if _, err := e.mgr.TerminalScreen(card.ID); err == nil {
		t.Error("the terminal is still there after the switch failed")
	}
}

// What is typed reaches the process in order, the size reaches it, and the screen is what it printed.
func TestTerminalInputResizeAndScreen(t *testing.T) {
	e, term := viewEnv(t)
	card := startedCard(t, e, "Type here")
	inTerminal(t, e, card)
	_, before := screenOf(t, e, card.ID)
	if before.ThroughSeq != 0 || before.Data != "" || before.Cols != 120 || before.Rows != 32 {
		t.Errorf("the screen before anything was printed = %+v", before)
	}

	for _, typed := range []string{"ls", " -la", "\r"} {
		if err := e.mgr.TerminalInput(card.ID, []byte(typed)); err != nil {
			t.Fatalf("TerminalInput(%q): %v", typed, err)
		}
	}
	eventually(t, "the input reaching the terminal", func() bool { return len(term.typed()) == 3 })
	if got := string(bytes.Join(term.typed(), nil)); got != "ls -la\r" {
		t.Errorf("the terminal was typed %q, want the keys in order", got)
	}

	if err := e.mgr.TerminalResize(context.Background(), card.ID, 100, 40); err != nil {
		t.Fatalf("TerminalResize: %v", err)
	}
	if sizes := term.resizes(); len(sizes) != 1 || sizes[0] != [2]int{100, 40} {
		t.Errorf("the terminal was resized to %v, want 100x40", sizes)
	}

	if err := term.print(session1(t, e, card), []byte("hello ")); err != nil {
		t.Fatal(err)
	}
	if err := term.print(session1(t, e, card), []byte("world\r\n")); err != nil {
		t.Fatal(err)
	}
	first := e.untilOutput(t, card.ID)
	second := e.untilOutput(t, card.ID)
	if data, ok := first.Data.(protocol.TerminalOutputEventData); !ok || data.Data != base64.StdEncoding.EncodeToString([]byte("hello ")) {
		t.Errorf("the first output event carries %+v", first.Data)
	}
	if first.Critical || second.Critical {
		t.Error("terminal output must be an ordinary event, so a slow reader drops it and reloads")
	}
	if second.Topic != string(protocol.CardTopic(card.ID)) || second.Seq <= first.Seq {
		t.Errorf("the events are on %q with numbers %d and %d", second.Topic, first.Seq, second.Seq)
	}
	data, screen := screenOf(t, e, card.ID)
	if string(data) != "hello world\r\n" || screen.ThroughSeq != second.Seq || screen.Cols != 100 || screen.Rows != 40 || screen.CardID != card.ID {
		t.Errorf("the screen = %q through %d at %dx%d, want the two pieces through the second event (%d) at 100x40",
			data, screen.ThroughSeq, screen.Cols, screen.Rows, second.Seq)
	}
}

// lockedWriter lets a logger that several goroutines write to be read by a test.
type lockedWriter struct {
	mu  *sync.Mutex
	buf *bytes.Buffer
}

func (w lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

// newBufferLogger makes a logger that writes to a buffer under a lock.
func newBufferLogger(mu *sync.Mutex, buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(lockedWriter{mu: mu, buf: buf}, nil))
}

// session1 is the agent session id the terminal was resumed with.
func session1(t *testing.T, e *env, card protocol.Card) string {
	t.Helper()
	_, _, id := e.storedView(t, card.ID)
	return id
}

// Output is live-only: it is not in the replay ring, so it cannot push the events that matter out of
// it, and it is not stored as history.
func TestTerminalOutputIsNeitherReplayedNorStoredAsHistory(t *testing.T) {
	e, term := viewEnv(t)
	card := startedCard(t, e, "Quiet history")
	inTerminal(t, e, card)
	id := session1(t, e, card)
	before := len(e.historyOf(t, card.ID))
	for range 50 {
		if err := term.print(id, []byte("noise")); err != nil {
			t.Fatal(err)
		}
	}
	for range 50 {
		e.untilOutput(t, card.ID)
	}
	replayed, replay := e.bus.Since("test-epoch", 0)
	if replay != events.ReplayOK {
		t.Fatalf("Replay = %s: the output must not have pushed anything out of the ring", replay)
	}
	for _, ev := range replayed {
		if ev.Type == string(protocol.EventTypeSessionTerminalOutput) {
			t.Fatalf("event %d, a terminal output, is in the replay ring", ev.Seq)
		}
	}
	if after := len(e.historyOf(t, card.ID)); after != before {
		t.Errorf("the card's history grew from %d to %d events: a terminal's bytes are not history", before, after)
	}
}

// The screen keeps the latest 256 KiB and no more, and it stays complete through the last event.
func TestTheScreenKeepsTheLatestOutputAndNoMore(t *testing.T) {
	e, term := viewEnv(t)
	card := startedCard(t, e, "Prints a lot")
	inTerminal(t, e, card)
	id := session1(t, e, card)
	var printed []byte
	for i := range 300 {
		chunk := bytes.Repeat([]byte{byte('a' + i%26)}, 1024)
		printed = append(printed, chunk...)
		if err := term.print(id, chunk); err != nil {
			t.Fatal(err)
		}
	}
	var last events.Event
	for range 300 {
		last = e.untilOutput(t, card.ID)
	}
	data, screen := screenOf(t, e, card.ID)
	if want := printed[len(printed)-256<<10:]; !bytes.Equal(data, want) || screen.ThroughSeq != last.Seq {
		t.Errorf("the screen holds %d bytes through %d, want the last %d bytes through %d", len(data), screen.ThroughSeq, len(want), last.Seq)
	}
}

// A terminal that floods the bus for a subscriber that never reads does not slow the terminal, the
// bus, or anyone else: the subscriber loses its oldest ordinary events and is told to reload.
func TestASlowSubscriberDoesNotStallATerminal(t *testing.T) {
	e, term := viewEnv(t)
	card := startedCard(t, e, "Floods the bus")
	inTerminal(t, e, card)
	id := session1(t, e, card)
	slow := e.bus.Subscribe(events.Topics(string(protocol.CardTopic(card.ID)))) // Never reads.
	t.Cleanup(slow.Close)
	const pieces, size = 2000, 100
	for range pieces {
		if err := term.print(id, bytes.Repeat([]byte("x"), size)); err != nil {
			t.Fatal(err)
		}
	}
	eventually(t, "the pump publishing everything", func() bool {
		screen, err := e.mgr.TerminalScreen(card.ID)
		if err != nil {
			return false
		}
		data, _ := base64.StdEncoding.DecodeString(screen.Data)
		return len(data) == pieces*size
	})
	if !slow.Lagged() {
		t.Error("a subscriber that lost events was not told it lagged")
	}
	// A subscriber that reads still gets the next output promptly.
	reader := e.bus.Subscribe(events.Topics(string(protocol.CardTopic(card.ID))))
	t.Cleanup(reader.Close)
	if err := term.print(id, []byte("after")); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-reader.C():
		if data, ok := ev.Data.(protocol.TerminalOutputEventData); !ok || data.Data != base64.StdEncoding.EncodeToString([]byte("after")) {
			t.Errorf("the reader got %+v, want the output that came after the flood", ev.Data)
		}
	case <-time.After(eventTimeout):
		t.Error("no output arrived for a subscriber that reads")
	}
}

// Every terminal call is refused, with the stable reason, for a card that has no terminal running.
func TestTerminalCallsAreRefusedForACardWithNoTerminal(t *testing.T) {
	e, _ := viewEnv(t)
	card := startedCard(t, e, "In the chat")
	sentence := "This card has no terminal running. Switch it to terminal view first."
	check := func(name string, cardID string) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			wantReason(t, e.mgr.TerminalInput(cardID, []byte("x")), "terminal_not_active", sentence)
			wantReason(t, e.mgr.TerminalResize(context.Background(), cardID, 80, 24), "terminal_not_active", sentence)
			_, err := e.mgr.TerminalScreen(cardID)
			wantReason(t, err, "terminal_not_active", sentence)
		})
	}
	check("a card in the chat view", card.ID)
	check("a card that does not exist", "01M3C107JB041061050R3GG28A")
	inTerminal(t, e, card)
	if err := e.mgr.Stop(context.Background(), card.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	check("a card that was stopped", card.ID)
}

// A program that stops reading its input fills a small queue, and then the input is refused
// instead of held up: the stream that carries the keys must never wait for the program.
func TestInputForAProgramThatStoppedReadingIsRefused(t *testing.T) {
	e, term := viewEnv(t)
	card := startedCard(t, e, "Stuck program")
	inTerminal(t, e, card)
	hold := make(chan struct{})
	term.setWriteHold(hold)
	accepted := 0
	var refusal error
	begun := time.Now()
	for range 200 {
		if err := e.mgr.TerminalInput(card.ID, []byte("k")); err != nil {
			refusal = err
			break
		}
		accepted++
	}
	if took := time.Since(begun); took > time.Second {
		t.Errorf("filling the queue took %v: input must never wait for the program", took)
	}
	wantReason(t, refusal, "terminal_busy", "The terminal is not keeping up with what you type. Wait a moment, then type again.")
	if accepted < 64 || accepted > 66 {
		t.Errorf("%d keys were accepted, want the 64 the queue holds and at most the two the writer had taken", accepted)
	}
	close(hold)
	eventually(t, "the queue draining", func() bool { return len(term.typed()) == accepted })
	if err := e.mgr.TerminalInput(card.ID, []byte("k")); err != nil {
		t.Errorf("input after the program read again: %v", err)
	}
}

// A write that fails does not stop the terminal, and what was typed is never logged.
func TestAFailedWriteIsLoggedWithoutWhatWasTyped(t *testing.T) {
	var logs bytes.Buffer
	var mu sync.Mutex
	term := newFakeTerminal()
	e := newEnv(t, func(c *session.Config) {
		c.Terminals = terminalRegistry(t, term)
		c.Logger = newBufferLogger(&mu, &logs)
	})
	t.Cleanup(func() { _ = e.mgr.Close() })
	card := startedCard(t, e, "Cannot write")
	inTerminal(t, e, card)
	term.setErrs(errors.New("broken pipe"), nil, nil)
	if err := e.mgr.TerminalInput(card.ID, []byte("hunter2-secret")); err != nil {
		t.Fatalf("TerminalInput: %v", err)
	}
	eventually(t, "the failed write being logged", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return strings.Contains(logs.String(), "could not write to a terminal")
	})
	mu.Lock()
	defer mu.Unlock()
	if strings.Contains(logs.String(), "hunter2") {
		t.Error("what was typed reached the log")
	}
}

// A resize or a size that fails because the program is already gone is the same refusal as no
// terminal. Any other failure is the daemon's own.
func TestTerminalCallsThatFailInsideTheProgram(t *testing.T) {
	e, term := viewEnv(t)
	card := startedCard(t, e, "Failing calls")
	inTerminal(t, e, card)
	term.setErrs(nil, agents.ErrStopped, agents.ErrUnknownSession)
	wantReason(t, e.mgr.TerminalResize(context.Background(), card.ID, 80, 24), "terminal_not_active",
		"This card has no terminal running. Switch it to terminal view first.")
	_, err := e.mgr.TerminalScreen(card.ID)
	wantReason(t, err, "terminal_not_active", "This card has no terminal running. Switch it to terminal view first.")
	term.setErrs(nil, errors.New("ioctl failed"), nil)
	err = e.mgr.TerminalResize(context.Background(), card.ID, 80, 24)
	var refusal *protocol.Error
	if err == nil || errors.As(err, &refusal) {
		t.Errorf("a resize that failed for another reason = %v, want the daemon's own error", err)
	}
}

// A program that exits by itself is an unexpected exit, like a chat agent's: the card needs the
// person and the session stops, and it is back in the chat view, so the next start resumes the chat.
func TestATerminalProgramThatExitsMovesTheCardToNeedsYou(t *testing.T) {
	e, term := viewEnv(t)
	card := startedCard(t, e, "Exits")
	inTerminal(t, e, card)
	e.untilCard(t, card.ID, func(c protocol.Card) bool { return c.ViewMode == protocol.CardViewModeTerminal })
	term.crash(session1(t, e, card))

	stopped, _ := e.untilState(t, card.ID, protocol.SessionStateStopped)
	if stopped.Reason != "The agent stopped unexpectedly." {
		t.Errorf("the stopped event says %q", stopped.Reason)
	}
	got := e.untilCard(t, card.ID, func(c protocol.Card) bool { return c.State == protocol.CardStateNeeds })
	if got.ViewMode != protocol.CardViewModeChat {
		t.Errorf("a card whose terminal exited is in the %q view, want chat", got.ViewMode)
	}
	if state, view, _ := e.storedView(t, card.ID); state != "stopped" || view != "chat" {
		t.Errorf("the row = %s, %s, want stopped in the chat view", state, view)
	}
	wantReason(t, e.mgr.TerminalInput(card.ID, []byte("x")), "terminal_not_active",
		"This card has no terminal running. Switch it to terminal view first.")
}

// Stopping a card in the terminal view ends the terminal and puts the card back in the chat view.
func TestStoppingACardInTheTerminalPutsItBackInTheChat(t *testing.T) {
	e, _ := viewEnv(t)
	card := startedCard(t, e, "Stop in the terminal")
	inTerminal(t, e, card)
	if err := e.mgr.Stop(context.Background(), card.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	got := e.untilCard(t, card.ID, func(c protocol.Card) bool { return c.Session != nil && *c.Session == protocol.SessionStateStopped })
	if got.ViewMode != protocol.CardViewModeChat {
		t.Errorf("the card after Stop is in the %q view, want chat", got.ViewMode)
	}
	if _, view, _ := e.storedView(t, card.ID); view != "chat" {
		t.Errorf("the row is in the %q view after Stop", view)
	}
}

// A chat message for a card in the terminal view is refused: the terminal is where a person types.
func TestAMessageForACardInTheTerminalIsRefused(t *testing.T) {
	e, term := viewEnv(t)
	card := startedCard(t, e, "Types in the terminal")
	inTerminal(t, e, card)
	err := e.mgr.Send(context.Background(), card.ID, "hello")
	wantReason(t, err, "view_terminal_active",
		"This card is in terminal view. Type in the terminal, or switch to chat view to send a message.")
	if len(term.typed()) != 0 {
		t.Error("a chat message was typed into the terminal")
	}
	if got := e.historyOf(t, card.ID); len(got) != 0 {
		t.Errorf("the refused message was stored: %+v", got)
	}
}
