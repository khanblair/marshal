package session

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A card's settings, and what happens to a session that is already running when one of them
// changes (docs/backend-checklist.md B3.6, inventory N8, build-plan 3.12). PATCH /v1/cards/{id}
// has always written card.Model, card.Thinking, and the rest into the database; this file is what
// makes a running session hear about it.

// applyCardSettings gives a running session the card's settings as they are now, so a change a
// person makes while the session is running is in force for the turn that is about to start.
//
// The card is the only source of truth. The session is told what the card says, through the
// agent's own live controls when it has them (agents.SettingsApplier, which the ACP adapters
// implement), and otherwise it keeps what it started with: an agent that only takes these settings
// on its command line has nowhere to put a change, so for that agent the change takes effect the
// next time the session starts or resumes, which reads the card again (startAgent, resumeSpec).
//
// The manager reads the card on every turn. That is one row, and the common answer is "nothing
// changed", which stops here before the agent is asked anything.
//
// A setting the agent does not offer at all is recorded as given, so the same refusal is not
// repeated on every turn; a failure that could be passing is not, so the next turn tries again.
// Neither stops the turn: a person's message is not dropped because a setting could not be
// applied, and the session's own log says what happened.
func (m *Manager) applyCardSettings(ctx context.Context, ls *liveSession) {
	if ls.isChat() {
		return
	}
	card, err := m.projects.Card(ctx, ls.cardID)
	if err != nil {
		m.log.Warn("could not read a card's settings for its next turn", "card_id", ls.cardID, "error", err)
		return
	}
	want := sessionSettings{
		model: card.Model, thinking: thinkingOrEmpty(card.Thinking),
		permissionMode: string(card.PermissionMode),
	}
	if ls.settingsNow() == want {
		return
	}
	applier, ok := ls.agent.(agents.SettingsApplier)
	if !ok {
		// The agent has no live control: the session keeps what it started with, and the change
		// takes effect the next time it starts or resumes. Nothing to record, either, so that a
		// later start of the same card still reads the card's own answer.
		return
	}
	applied, err := applier.ApplySettings(ctx, ls.handle, agents.SessionSettings{
		Model: want.model, Thinking: want.thinking, PermissionMode: want.permissionMode,
	})
	if err != nil {
		m.log.Warn("could not give a running session the card's settings",
			"card_id", ls.cardID, "error", err)
		if errors.Is(err, agents.ErrUnsupportedSetting) {
			ls.setSettings(want)
		}
		return
	}
	ls.setSettings(want)
	m.log.Info("gave a running session the card's settings", "card_id", ls.cardID,
		"model", want.model, "thinking", want.thinking, "permission_mode", want.permissionMode,
		"applied", fmt.Sprintf("model=%t thinking=%t permission_mode=%t",
			applied.Model, applied.Thinking, applied.PermissionMode))
}

// Setting change notes. The sentences are the app's, moved with the call that writes them
// (apps/web/src/mock/actions/cards.ts, setSetting), because the person reads them in the card's
// chat whatever the view.
const (
	// settingNoteFormat is what the chat says after one setting changed. It is the mock's own
	// sentence, and it is the daemon's promise: the change is in force from the next turn.
	settingNoteFormat = "%s set to %s. It takes effect on the next turn."
	// settingClearedFormat is what the chat says when a setting was emptied, which a card's settings
	// row allows for the model, the thinking mode, and the role. The mock has no sentence for it,
	// because its own settings control always carries a value.
	settingClearedFormat = "%s cleared. It takes effect on the next turn."
)

// settingCall is one setting of a card's settings row: what the app calls it, and how to read it
// from a card. The order of settingCalls is the order the notes are written in, and the order the
// row shows them in.
type settingCall struct {
	label string
	value func(protocol.Card) string
}

// settingCalls lists the settings a note is written for: the five of a card's settings row
// (docs/ui-rules.md 3.4). Bypass is deliberately not one of them: it is granted through its own
// call, which writes its own note (bypass.go).
func settingCalls() []settingCall {
	return []settingCall{
		{"Agent", func(c protocol.Card) string { return string(c.Agent) }},
		{"Role", func(c protocol.Card) string { return c.Role }},
		{"Model", func(c protocol.Card) string { return c.Model }},
		{"Thinking mode", func(c protocol.Card) string { return thinkingOrEmpty(c.Thinking) }},
		{"Permission mode", func(c protocol.Card) string { return string(c.PermissionMode) }},
	}
}

// NoteSettingsChanged writes the system message and the activity row into a card's own history for
// every setting that changed between before and after (inventory N8: the daemon writes both). The
// caller hands over the card as it was and as it is now, and nothing is written when none of the
// settings changed.
//
// One record carries both views, which is this daemon's convention for a fact the chat and the
// activity list both show: a kind of KindSystem with a state is a system message in the chat
// (history.ChatMessageOf) and a tool-kind row in the activity list (history.ActivityItemOf). An
// approval and a tool call work the same way. The mock writes the two separately because its chat
// and its activity list are separate stores.
//
// A card that has never started has no session row and therefore no chat to write into: the note
// appears from its first start. The same rule the bypass note follows.
func (m *Manager) NoteSettingsChanged(ctx context.Context, before, after protocol.Card) {
	if before.ID == "" || before.ID != after.ID {
		return
	}
	if m.cfg.History == nil {
		return
	}
	records := settingRecords(before, after)
	if len(records) == 0 {
		return
	}
	sessionID, ok := m.sessionRowIDOf(ctx, after.ID)
	if !ok {
		return
	}
	if err := m.cfg.History.Append(m.ctx, after.ID, sessionID, records); err != nil {
		m.log.Error("could not store a card's setting change", "card_id", after.ID, "error", err)
	}
}

// settingRecords is one record per setting that changed, in the order of a card's settings row.
func settingRecords(before, after protocol.Card) []history.Record {
	var records []history.Record
	for _, call := range settingCalls() {
		was, now := call.value(before), call.value(after)
		if was == now {
			continue
		}
		records = append(records, history.Record{
			Kind: history.KindSystem, State: history.StateOK, Summary: settingSentence(call.label, now),
		})
	}
	return records
}

// settingSentence is the one line the chat and the activity list show for one changed setting.
func settingSentence(label, value string) string {
	if value == "" {
		return fmt.Sprintf(settingClearedFormat, label)
	}
	return fmt.Sprintf(settingNoteFormat, label, value)
}
