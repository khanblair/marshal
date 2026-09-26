package session_test

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
)

// The terminal's restart rules (docs/architecture.md 5.3): a daemon that stopped while a card was
// in the terminal view resumes the same session in the terminal, a card whose terminal cannot be
// resumed is moved to Needs you, and the terminal's output is written to the session log. The rest
// of the view rules are in view_test.go, and the helpers both files use are there too.

// A sleeping card in the terminal view wakes into the terminal, and a message does not wake it just
// to be refused.
func TestASleepingTerminalWakesIntoTheTerminal(t *testing.T) {
	e, term := viewEnv(t)
	card := startedCard(t, e, "Sleeps in the terminal")
	inTerminal(t, e, card)
	if _, err := e.mgr.Pause(context.Background(), card.ID); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if err := e.mgr.Sleep(context.Background(), card.ID); err != nil {
		t.Fatalf("Sleep: %v", err)
	}
	if state, view, _ := e.storedView(t, card.ID); state != "asleep" || view != "terminal" {
		t.Fatalf("the row = %s, %s, want asleep in the terminal view", state, view)
	}
	wantReason(t, e.mgr.Send(context.Background(), card.ID, "wake up"), "view_terminal_active",
		"This card is in terminal view. Type in the terminal, or switch to chat view to send a message.")
	if got := len(term.resumedIDs()); got != 1 {
		t.Errorf("a message woke the terminal: it was resumed %d times, want only the switch", got)
	}

	if err := e.mgr.Wake(context.Background(), card.ID); err != nil {
		t.Fatalf("Wake: %v", err)
	}
	if got := len(term.resumedIDs()); got != 2 {
		t.Errorf("the terminal was resumed %d times, want the switch and the wake", got)
	}
	if _, screen := screenOf(t, e, card.ID); screen.CardID != card.ID {
		t.Errorf("the woken card has no terminal: %+v", screen)
	}
	if state, view, _ := e.storedView(t, card.ID); state != "awake" || view != "terminal" {
		t.Errorf("the row after the wake = %s, %s", state, view)
	}
}

// A daemon that stopped while a card was in the terminal view resumes it in the terminal, on the
// same session id, and the card says so.
func TestATerminalSurvivesARestart(t *testing.T) {
	e, term := viewEnv(t)
	card := startedCard(t, e, "Restarts in the terminal")
	inTerminal(t, e, card)
	_, _, id := e.storedView(t, card.ID)
	if err := e.mgr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if state, view, _ := e.storedView(t, card.ID); state != "awake" || view != "terminal" {
		t.Fatalf("the row after the daemon stopped = %s, %s, want it left as it was", state, view)
	}
	stored, err := e.proj.Card(context.Background(), card.ID)
	if err != nil || stored.ViewMode != protocol.CardViewModeTerminal {
		t.Fatalf("the card before the restore = view %q (%v), want terminal: the board must know at once", stored.ViewMode, err)
	}

	fresh := e.reopen(t, func(c *session.Config) { c.Terminals = terminalRegistry(t, term) })
	t.Cleanup(func() { _ = fresh.Close() })
	if err := fresh.RestoreAll(context.Background()); err != nil {
		t.Fatalf("RestoreAll: %v", err)
	}
	if ids := term.resumedIDs(); len(ids) != 2 || ids[1] != id {
		t.Errorf("the terminal was resumed with %v, want the same id %q again", ids, id)
	}
	if err := fresh.TerminalInput(card.ID, []byte("still here")); err != nil {
		t.Errorf("TerminalInput after the restore: %v", err)
	}
	if specs := e.agent.startSpecs(); len(specs) != 1 {
		t.Errorf("the chat agent was asked for %d processes, want only the first start: the restore must use the terminal", len(specs))
	}
	if state, view, _ := e.storedView(t, card.ID); state != "awake" || view != "terminal" {
		t.Errorf("the row after the restore = %s, %s", state, view)
	}
}

// A restart that finds a terminal session but no terminal to resume it in fails the resume by the
// rule of section 5.3, and the card is back in the chat view.
func TestATerminalSessionThatCannotBeResumedAfterARestartNeedsYou(t *testing.T) {
	e, _ := viewEnv(t)
	card := startedCard(t, e, "No terminal after the restart")
	inTerminal(t, e, card)
	if err := e.mgr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	fresh := e.reopen(t) // No terminals configured.
	t.Cleanup(func() { _ = fresh.Close() })
	if err := fresh.RestoreAll(context.Background()); err != nil {
		t.Fatalf("RestoreAll: %v", err)
	}
	if state, view, _ := e.storedView(t, card.ID); state != "stopped" || view != "chat" {
		t.Errorf("the row = %s, %s, want stopped in the chat view", state, view)
	}
	moved, err := e.proj.Card(context.Background(), card.ID)
	if err != nil || moved.State != protocol.CardStateNeeds {
		t.Errorf("the card = %v (%v), want it in needs", moved.State, err)
	}
}

// An agent registered for the terminal view that is not a terminal cannot run one. The restore says
// so, and no process is left running for it.
func TestARestoredTerminalThatIsNotATerminalIsEnded(t *testing.T) {
	e, _ := viewEnv(t)
	card := startedCard(t, e, "Misregistered")
	inTerminal(t, e, card)
	if err := e.mgr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	notATerminal := terminalRegistry(t, e.agent)
	fresh := e.reopen(t, func(c *session.Config) { c.Terminals = notATerminal })
	t.Cleanup(func() { _ = fresh.Close() })
	if err := fresh.RestoreAll(context.Background()); err != nil {
		t.Fatalf("RestoreAll: %v", err)
	}
	wantReason(t, fresh.TerminalInput(card.ID, []byte("x")), "terminal_not_active",
		"This card has no terminal running. Switch it to terminal view first.")
}

// The terminal's output is written to the session's log as base64, and Close ends the terminal's
// writer, so nothing is left running.
func TestTerminalOutputIsWrittenToTheSessionLog(t *testing.T) {
	e, term := viewEnv(t)
	card := startedCard(t, e, "Logged")
	inTerminal(t, e, card)
	id := session1(t, e, card)
	if err := term.print(id, []byte("logged output")); err != nil {
		t.Fatal(err)
	}
	e.untilOutput(t, card.ID)
	row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.mgr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	logged, err := os.ReadFile(filepath.Join(e.dataDir, "logs", "sessions", row.ID, "0001.jsonl"))
	if err != nil {
		t.Fatalf("read the session log: %v", err)
	}
	want := `"kind":"terminal_output"`
	encoded := `"data":"` + base64.StdEncoding.EncodeToString([]byte("logged output")) + `"`
	if !strings.Contains(string(logged), want) || !strings.Contains(string(logged), encoded) {
		t.Errorf("the log does not hold the output as base64:\n%s", logged)
	}
}

// While a switch is under way, a start is refused with its own sentence, and a message, a stop, or a
// sleep waits for the switch and then acts on the session it made.
func TestWhileASwitchIsUnderWayOtherCallsWaitOrAreRefused(t *testing.T) {
	e, term := viewEnv(t)
	card := startedCard(t, e, "Busy switching")
	term.resumeHold = make(chan struct{})
	term.resumeEntered = make(chan struct{}, 1)

	switched := make(chan error, 1)
	go func() {
		_, err := e.mgr.SwitchView(context.Background(), card.ID, protocol.CardViewModeTerminal)
		switched <- err
	}()
	<-term.resumeEntered // The chat process is stopped and the terminal is not up yet.

	_, err := e.mgr.Start(context.Background(), card.ID)
	wantReason(t, err, "view_switching", "This card is switching views. Try again in a moment.")

	sent := make(chan error, 1)
	go func() { sent <- e.mgr.Send(context.Background(), card.ID, "hello?") }()
	select {
	case err := <-sent:
		t.Fatalf("a message did not wait for the switch: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	stopped := make(chan error, 1)
	go func() { stopped <- e.mgr.Stop(context.Background(), card.ID) }()
	select {
	case err := <-stopped:
		t.Fatalf("a stop did not wait for the switch: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(term.resumeHold)
	if err := <-switched; err != nil {
		t.Fatalf("SwitchView: %v", err)
	}
	wantReason(t, <-sent, "view_terminal_active",
		"This card is in terminal view. Type in the terminal, or switch to chat view to send a message.")
	if err := <-stopped; err != nil {
		t.Fatalf("Stop after the switch: %v", err)
	}
	if state, view, _ := e.storedView(t, card.ID); state != "stopped" || view != "chat" {
		t.Errorf("the row = %s, %s: the stop must have ended the session the switch made", state, view)
	}
}
