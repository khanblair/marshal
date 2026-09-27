package session_test

import (
	"context"
	"errors"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A card's settings that change while the session is running (docs/backend-checklist.md B3.6,
// inventory N8). The manager hands the change to the running agent before the next turn, and the
// card's own history says what happened.

func TestASettingChangedWhileTheSessionRunsReachesTheNextTurn(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	ctx := context.Background()
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Switch the model")
	if _, err := e.mgr.Start(ctx, card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	if got := e.agent.appliedSettings(); len(got) != 0 {
		t.Fatalf("the session was handed %+v before anything changed", got)
	}

	model := "opus"
	if _, err := e.proj.UpdateCard(ctx, card.ID, protocol.UpdateCardRequest{Model: &model}); err != nil {
		t.Fatalf("change the card's model: %v", err)
	}
	if err := e.mgr.Send(ctx, card.ID, "carry on"); err != nil {
		t.Fatalf("send: %v", err)
	}
	got := e.agent.appliedSettings()
	if len(got) != 1 || got[0].Model != "opus" {
		t.Fatalf("the session was handed %+v, want the card's new model", got)
	}
}

func TestASettingTheSessionAlreadyHasIsNotHandedOverAgain(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	ctx := context.Background()
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Leave the model alone")
	if _, err := e.mgr.Start(ctx, card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	for _, text := range []string{"first", "second"} {
		if err := e.mgr.Send(ctx, card.ID, text); err != nil {
			t.Fatalf("send %q: %v", text, err)
		}
	}
	waitForHistory(t, e, card.ID, 4)
	if calls := e.agent.settingsCalls(); calls != 0 {
		t.Errorf("the agent was handed settings %d times, want none: the card never changed", calls)
	}
}

func TestASettingTheAgentCannotTakeIsNotOfferedOnEveryTurn(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	e.agent.setApplyErr(agents.ErrUnsupportedSetting)
	ctx := context.Background()
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "A model the agent does not offer")
	if _, err := e.mgr.Start(ctx, card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	model := "gpt-9"
	if _, err := e.proj.UpdateCard(ctx, card.ID, protocol.UpdateCardRequest{Model: &model}); err != nil {
		t.Fatalf("change the card's model: %v", err)
	}
	for _, text := range []string{"first", "second"} {
		if err := e.mgr.Send(ctx, card.ID, text); err != nil {
			t.Fatalf("send %q: %v", text, err)
		}
	}
	waitForHistory(t, e, card.ID, 4)
	if calls := e.agent.settingsCalls(); calls != 1 {
		t.Errorf("the agent was handed settings %d times, want once: a value it does not offer must not be retried", calls)
	}
}

func TestASettingChangeThatCouldBePassingIsTriedAgain(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	e.agent.setApplyErr(errors.New("the agent's connection dropped"))
	ctx := context.Background()
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "A setting that failed for now")
	if _, err := e.mgr.Start(ctx, card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	model := "opus"
	if _, err := e.proj.UpdateCard(ctx, card.ID, protocol.UpdateCardRequest{Model: &model}); err != nil {
		t.Fatalf("change the card's model: %v", err)
	}
	for _, text := range []string{"first", "second"} {
		if err := e.mgr.Send(ctx, card.ID, text); err != nil {
			t.Fatalf("send %q: %v", text, err)
		}
	}
	waitForHistory(t, e, card.ID, 4)
	if calls := e.agent.settingsCalls(); calls != 2 {
		t.Errorf("the agent was handed settings %d times, want both turns: a failure that could be passing is retried", calls)
	}
}

func TestASettingChangeIsSaidInTheCardsHistory(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	ctx := context.Background()
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Say what changed")
	if _, err := e.mgr.Start(ctx, card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	before, err := e.proj.Card(ctx, card.ID)
	if err != nil {
		t.Fatalf("read the card: %v", err)
	}
	model, thinking := "opus", protocol.ThinkingModeHigh
	after, err := e.proj.UpdateCard(ctx, card.ID, protocol.UpdateCardRequest{Model: &model, Thinking: &thinking})
	if err != nil {
		t.Fatalf("change two settings: %v", err)
	}
	e.mgr.NoteSettingsChanged(ctx, before, after)

	notes := systemNotes(t, e, card.ID)
	// The stored notes come back newest first, so the thinking note is the second one written and
	// the model note the first: this is also the order a card's settings row is read in.
	want := []string{
		"Thinking mode set to high. It takes effect on the next turn.",
		"Model set to opus. It takes effect on the next turn.",
	}
	if len(notes) != len(want) {
		t.Fatalf("the card's system notes = %+v, want one per changed setting", notes)
	}
	for i, note := range notes {
		if note.Summary != want[i] {
			t.Errorf("note %d = %q, want %q", i, note.Summary, want[i])
		}
		// The same record is what the chat draws and what the activity list draws: a system
		// message, and a tool-kind row that ended ok.
		message, err := history.ChatMessageOf(note)
		if err != nil {
			t.Fatalf("turn note %d into a chat message: %v", i, err)
		}
		if message.Kind != protocol.ChatMessageKindSystem || message.Text != want[i] {
			t.Errorf("note %d as a chat message = %+v, want a system message", i, message)
		}
		item, ok, err := history.ActivityItemOf(note)
		if err != nil || !ok {
			t.Fatalf("turn note %d into an activity item: ok=%t err=%v", i, ok, err)
		}
		if item.Kind != protocol.ActivityKindTool || item.State != protocol.ActivityStateOK || item.Text != want[i] {
			t.Errorf("note %d as an activity item = %+v, want a tool row that ended ok", i, item)
		}
	}
}

func TestASettingChangeIsSaidInTheChatOnlyForWhatChanged(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	ctx := context.Background()
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Only what changed")
	if _, err := e.mgr.Start(ctx, card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	card, err := e.proj.Card(ctx, card.ID)
	if err != nil {
		t.Fatalf("read the card: %v", err)
	}
	// The same card twice: nothing changed, so nothing is said.
	e.mgr.NoteSettingsChanged(ctx, card, card)
	if notes := systemNotes(t, e, card.ID); len(notes) != 0 {
		t.Errorf("the card's system notes = %+v, want none", notes)
	}
	// A change with no session row is a card with no chat: nothing is written, and the card's next
	// start is where its history begins.
	fresh := e.card(t, project.ID, "Never started")
	before, err := e.proj.Card(ctx, fresh.ID)
	if err != nil {
		t.Fatalf("read the fresh card: %v", err)
	}
	model := "opus"
	after, err := e.proj.UpdateCard(ctx, fresh.ID, protocol.UpdateCardRequest{Model: &model})
	if err != nil {
		t.Fatalf("change the fresh card's model: %v", err)
	}
	e.mgr.NoteSettingsChanged(ctx, before, after)
	if events := e.historyOf(t, fresh.ID); len(events) != 0 {
		t.Errorf("the card's history = %+v, want nothing for a card with no session", events)
	}
}

func TestAClearedSettingIsSaidToo(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	ctx := context.Background()
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Clear the model")
	if _, err := e.mgr.Start(ctx, card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	model := "opus"
	before, err := e.proj.UpdateCard(ctx, card.ID, protocol.UpdateCardRequest{Model: &model})
	if err != nil {
		t.Fatalf("set the model: %v", err)
	}
	empty := ""
	after, err := e.proj.UpdateCard(ctx, card.ID, protocol.UpdateCardRequest{Model: &empty})
	if err != nil {
		t.Fatalf("clear the model: %v", err)
	}
	e.mgr.NoteSettingsChanged(ctx, before, after)
	notes := systemNotes(t, e, card.ID)
	if len(notes) != 1 || notes[0].Summary != "Model cleared. It takes effect on the next turn." {
		t.Errorf("the card's system notes = %+v, want the cleared sentence", notes)
	}
}
