package session_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The harness wired to a card's live session (B5.3, build-plan 5.3 and 5.4): a card is stopped for
// the role it runs under, and for a loop its agent cannot get out of. The decisions themselves are
// tested where they are pure (internal/harness); these tests are the wiring - that a turn's end
// reads the card's role, counts its turns, and moves the card when there is a reason to.

// fakeRoles is a RoleLimitsReader a test sets the ceilings of. It answers by role name, and reports
// a role it does not know as not found, which is how the roles module answers for a deleted role.
type fakeRoles struct {
	byName map[string]harness.Limits
	err    error
}

func (f *fakeRoles) LimitsFor(_ context.Context, _, roleName string) (harness.Limits, bool, error) {
	if f.err != nil {
		return harness.Limits{}, false, f.err
	}
	limits, ok := f.byName[roleName]
	return limits, ok, nil
}

// cardWithRole makes a card that names a role, and starts its agent: the harness only ever reads a
// role at a turn's end, and a turn only happens for a started card.
func cardWithRole(t *testing.T, e *env, projectID, title, role string) protocol.Card {
	t.Helper()
	c, err := e.proj.CreateCard(context.Background(), projectID, protocol.CreateCardRequest{Title: title, Role: role})
	if err != nil {
		t.Fatalf("create the card %q under role %q: %v", title, role, err)
	}
	return startCard(t, e, c)
}

// startCard starts a card's agent and waits until it is awake, so a message can be sent to it.
func startCard(t *testing.T, e *env, card protocol.Card) protocol.Card {
	t.Helper()
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start the card %s: %v", card.ID, err)
	}
	e.untilState(t, card.ID, protocol.SessionStateAwake)
	return card
}

// needsCard waits for a card to be stopped and returns it as it was read.
func needsCard(t *testing.T, e *env, cardID string) protocol.Card {
	t.Helper()
	var card protocol.Card
	waitFor(t, "the card to be moved to needs you", func() bool {
		c, err := e.proj.Card(context.Background(), cardID)
		if err != nil {
			return false
		}
		card = c
		return c.State == protocol.CardStateNeeds
	})
	return card
}

// countAction is how many audit rows one action has.
func countAction(t *testing.T, e *env, action string) int {
	t.Helper()
	count := 0
	for _, row := range e.auditRows(t) {
		if row.Action == action {
			count++
		}
	}
	return count
}

// A role's round ceiling stops the card on the turn past the number it allows: a ceiling of one
// allows one turn, so the card is stopped only after the second.
func TestARolesRoundCeilingStopsTheCardOnTheTurnPastIt(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	e.mgr.SetRoleLimits(&fakeRoles{byName: map[string]harness.Limits{"Worker": {Rounds: 1}}})
	project := e.project(t, "small-repo")
	card := cardWithRole(t, e, project.ID, "Cap the work", "Worker")

	if err := e.mgr.Send(context.Background(), card.ID, "the first turn"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitForHistory(t, e, card.ID, 2)
	if got, err := e.proj.Card(context.Background(), card.ID); err != nil {
		t.Fatalf("Card: %v", err)
	} else if got.State == protocol.CardStateNeeds {
		t.Fatalf("the card was stopped on the turn its role allowed: %+v", got.NeedsReason)
	}

	if err := e.mgr.Send(context.Background(), card.ID, "the second turn"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	stopped := needsCard(t, e, card.ID)
	if stopped.NeedsReason == nil {
		t.Fatal("the stopped card has no reason for a person to read")
	}
	if stopped.NeedsReason.Kind != protocol.NeedsReasonKindLimit {
		t.Errorf("reason kind = %q, want %q", stopped.NeedsReason.Kind, protocol.NeedsReasonKindLimit)
	}
	if !strings.Contains(stopped.NeedsReason.Text, "2 turns") {
		t.Errorf("reason text = %q, want it to name the turns the card took", stopped.NeedsReason.Text)
	}

	// The stop is recorded for the audit log, and the card's own history carries the reason, so the
	// person opening the card can see why it stopped without reading the audit log.
	if rows := countAction(t, e, audit.ActionLimitReached); rows != 1 {
		t.Errorf("the audit log has %d limit rows, want one", rows)
	}
	notes := systemNotes(t, e, card.ID)
	if len(notes) != 1 || !strings.Contains(notes[0].Summary, "past the role's limit") {
		t.Errorf("the card's own notes = %+v, want the reason the harness stopped it", summaries(notes))
	}
}

// A card with no role is not limited at all, however many turns it takes: nobody set a ceiling for
// it, so none is applied.
func TestACardWithNoRoleIsNotLimited(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	// The reader has an answer for every name, so a card that is still not limited can only be one
	// that names no role.
	e.mgr.SetRoleLimits(&fakeRoles{byName: map[string]harness.Limits{"": {Rounds: 1}, "Worker": {Rounds: 1}}})
	project := e.project(t, "small-repo")
	card := startCard(t, e, e.card(t, project.ID, "No role at all"))

	for i := 0; i < 3; i++ {
		if err := e.mgr.Send(context.Background(), card.ID, "another turn"); err != nil {
			t.Fatalf("Send: %v", err)
		}
		waitForHistory(t, e, card.ID, (i+1)*2)
	}
	if got, err := e.proj.Card(context.Background(), card.ID); err != nil {
		t.Fatalf("Card: %v", err)
	} else if got.State == protocol.CardStateNeeds {
		t.Errorf("a card with no role was stopped: %+v", got.NeedsReason)
	}
}

// A card naming a role that is not there runs unlimited: a role somebody deleted must not stop a
// card for a ceiling nobody set.
func TestACardNamingARoleThatIsNotThereIsNotLimited(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	e.mgr.SetRoleLimits(&fakeRoles{byName: map[string]harness.Limits{"Worker": {Rounds: 1}}})
	project := e.project(t, "small-repo")
	card := cardWithRole(t, e, project.ID, "A role nobody has", "Ghost")

	for i := 0; i < 3; i++ {
		if err := e.mgr.Send(context.Background(), card.ID, "another turn"); err != nil {
			t.Fatalf("Send: %v", err)
		}
		waitForHistory(t, e, card.ID, (i+1)*2)
	}
	if got, err := e.proj.Card(context.Background(), card.ID); err != nil {
		t.Fatalf("Card: %v", err)
	} else if got.State == protocol.CardStateNeeds {
		t.Errorf("a card naming a role that is not there was stopped: %+v", got.NeedsReason)
	}
}

// A daemon with no roles module wired in limits nothing, even for a card that names a role: the
// reader is what makes a ceiling real.
func TestADaemonWithNoRolesModuleLimitsNothing(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := cardWithRole(t, e, project.ID, "A role, but no reader", "Worker")

	for i := 0; i < 3; i++ {
		if err := e.mgr.Send(context.Background(), card.ID, "another turn"); err != nil {
			t.Fatalf("Send: %v", err)
		}
		waitForHistory(t, e, card.ID, (i+1)*2)
	}
	if got, err := e.proj.Card(context.Background(), card.ID); err != nil {
		t.Fatalf("Card: %v", err)
	} else if got.State == protocol.CardStateNeeds {
		t.Errorf("a card was stopped with no reader wired in: %+v", got.NeedsReason)
	}
}

// A reader that cannot answer does not stop the card: an unreadable role is a daemon that cannot say
// a ceiling was passed, and it does not guess that one was.
func TestAReaderThatFailsDoesNotStopTheCard(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	e.mgr.SetRoleLimits(&fakeRoles{err: errors.New("the store is unreachable")})
	project := e.project(t, "small-repo")
	card := cardWithRole(t, e, project.ID, "A reader that cannot answer", "Worker")

	if err := e.mgr.Send(context.Background(), card.ID, "one turn"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitForHistory(t, e, card.ID, 2)
	if got, err := e.proj.Card(context.Background(), card.ID); err != nil {
		t.Fatalf("Card: %v", err)
	} else if got.State == protocol.CardStateNeeds {
		t.Errorf("a card was stopped by a reader that failed: %+v", got.NeedsReason)
	}
}

// The same failing tool call three times in one turn stops the card for being stuck, and the reason
// is the detector's own sentence, naming what repeated and the file it happened in.
func TestTheSameFailureThreeTimesStopsTheCard(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := cardWithRole(t, e, project.ID, "A loop to catch", "Worker")

	const failure = "TS2322: Type 'string' is not assignable to type 'number'."
	e.agent.setTurnScript(
		agents.ToolCall{ID: "c1", Title: "Edit columns.tsx", Kind: "edit", Path: "columns.tsx"},
		agents.ToolCallUpdate{ID: "c1", Status: agents.StatusFailed, Content: failure},
		agents.ToolCallUpdate{ID: "c1", Status: agents.StatusFailed, Content: failure},
		agents.ToolCallUpdate{ID: "c1", Status: agents.StatusFailed, Content: failure},
	)

	if err := e.mgr.Send(context.Background(), card.ID, "try the fix"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	stopped := needsCard(t, e, card.ID)
	if stopped.NeedsReason == nil || stopped.NeedsReason.Kind != protocol.NeedsReasonKindStuck {
		t.Fatalf("reason = %+v, want the card stopped for being stuck", stopped.NeedsReason)
	}
	if !strings.Contains(stopped.NeedsReason.Text, "3 times in a row") {
		t.Errorf("reason text = %q, want the detector's own sentence", stopped.NeedsReason.Text)
	}
	if !strings.Contains(stopped.NeedsReason.Text, "columns.tsx") {
		t.Errorf("reason text = %q, want it to name the file the loop happened in", stopped.NeedsReason.Text)
	}
	if rows := countAction(t, e, audit.ActionStuckPaused); rows != 1 {
		t.Errorf("the audit log has %d stuck rows, want one", rows)
	}
}

// Two failures that are not the same are not a loop, so the card keeps working: a card fighting two
// separate problems is not a card going round in circles.
func TestTwoDifferentFailuresDoNotStopTheCard(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := cardWithRole(t, e, project.ID, "Two separate problems", "Worker")

	e.agent.setTurnScript(
		agents.ToolCall{ID: "c1", Title: "Edit a.ts", Kind: "edit", Path: "a.ts"},
		agents.ToolCallUpdate{ID: "c1", Status: agents.StatusFailed, Content: "the first thing broke"},
		agents.ToolCallUpdate{ID: "c1", Status: agents.StatusFailed, Content: "the second thing broke"},
		agents.ToolCallUpdate{ID: "c1", Status: agents.StatusFailed, Content: "the first thing broke"},
		agents.ToolCallUpdate{ID: "c1", Status: agents.StatusFailed, Content: "the second thing broke"},
	)

	if err := e.mgr.Send(context.Background(), card.ID, "work through it"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitForHistory(t, e, card.ID, 2)
	if got, err := e.proj.Card(context.Background(), card.ID); err != nil {
		t.Fatalf("Card: %v", err)
	} else if got.State == protocol.CardStateNeeds {
		t.Errorf("a card alternating between two failures was stopped: %+v", got.NeedsReason)
	}
}

// The same file edited again and again with no failure is a loop too, and the sentence names the
// file: an agent rewriting the same file forever is stuck even when every edit succeeds.
func TestTheSameEditAgainAndAgainStopsTheCard(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := cardWithRole(t, e, project.ID, "Edit loop", "Worker")

	e.agent.setTurnScript(
		agents.ToolCall{ID: "c1", Title: "Edit src/columns.tsx", Kind: "edit", Path: "src/columns.tsx", Status: agents.StatusCompleted},
		agents.ToolCall{ID: "c2", Title: "Edit src/columns.tsx", Kind: "edit", Path: "src/columns.tsx", Status: agents.StatusCompleted},
		agents.ToolCall{ID: "c3", Title: "Edit src/columns.tsx", Kind: "edit", Path: "src/columns.tsx", Status: agents.StatusCompleted},
	)

	if err := e.mgr.Send(context.Background(), card.ID, "make it work"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	stopped := needsCard(t, e, card.ID)
	if stopped.NeedsReason == nil || stopped.NeedsReason.Kind != protocol.NeedsReasonKindStuck {
		t.Fatalf("reason = %+v, want the card stopped for an edit loop", stopped.NeedsReason)
	}
	if !strings.Contains(stopped.NeedsReason.Text, "src/columns.tsx") {
		t.Errorf("reason text = %q, want it to name the file", stopped.NeedsReason.Text)
	}
}

// A chat has no ceilings and no loop to catch: wiring it through the same session path must not stop
// it, and the manager's own history of the exchange must be the whole story.
func TestAChatIsNotStoppedByTheHarness(t *testing.T) {
	e := newChatEnv(t)
	e.mgr.SetRoleLimits(&fakeRoles{byName: map[string]harness.Limits{"": {Rounds: 1}, "Worker": {Rounds: 1}}})
	project := e.project(t, "small-repo")
	chat := e.newChat(t, project.ID, protocol.CreateChatRequest{Title: "Just talking"})

	e.agent.setTurnScript(
		agents.ToolCall{ID: "c1", Title: "Edit a.ts", Kind: "edit", Path: "a.ts"},
		agents.ToolCallUpdate{ID: "c1", Status: agents.StatusFailed, Content: "the same thing broke"},
		agents.ToolCallUpdate{ID: "c1", Status: agents.StatusFailed, Content: "the same thing broke"},
		agents.ToolCallUpdate{ID: "c1", Status: agents.StatusFailed, Content: "the same thing broke"},
	)

	for i := 0; i < 2; i++ {
		if err := e.chats.Send(context.Background(), chat.ID, "hello"); err != nil {
			t.Fatalf("SendChat: %v", err)
		}
		e.waitForChatHistory(t, chat.ID, (i+1)*2)
	}
	// A chat is not a card, so there is no card to be stopped; what matters is that the turn ran,
	// the agent was kept, and no card in the project was moved on the chat's behalf.
	cards, err := e.proj.Cards(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("Cards: %v", err)
	}
	for _, c := range cards {
		if c.State == protocol.CardStateNeeds {
			t.Errorf("a chat moved card %s to needs you: %+v", c.ID, c.NeedsReason)
		}
	}
	if notes := systemNotes(t, e.env, chat.ID); len(notes) != 0 {
		t.Errorf("a chat's own history has harness notes on it: %+v", summaries(notes))
	}
}

// A role whose ceilings are all zero is a role that limits nothing: the harness reads it, finds no
// ceiling, and leaves the card alone however many turns it takes.
func TestARoleWithNoCeilingsLimitsNothing(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	e.mgr.SetRoleLimits(&fakeRoles{byName: map[string]harness.Limits{"Worker": {}}})
	project := e.project(t, "small-repo")
	card := cardWithRole(t, e, project.ID, "A role with no ceilings", "Worker")

	for i := 0; i < 3; i++ {
		if err := e.mgr.Send(context.Background(), card.ID, "another turn"); err != nil {
			t.Fatalf("Send: %v", err)
		}
		waitForHistory(t, e, card.ID, (i+1)*2)
	}
	if got, err := e.proj.Card(context.Background(), card.ID); err != nil {
		t.Fatalf("Card: %v", err)
	} else if got.State == protocol.CardStateNeeds {
		t.Errorf("a card under a role with no ceilings was stopped: %+v", got.NeedsReason)
	}
	if notes := systemNotes(t, e, card.ID); len(notes) != 0 {
		t.Errorf("a card that nothing stopped has harness notes on it: %+v", summaries(notes))
	}
}
