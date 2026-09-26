package session

import (
	"context"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/history"
)

// HistoryRecorder stores the typed history and activity of a session, one record per moment the
// session emits, so a card's chat or a project chat can be paged and survives a restart. The
// history module implements it (history.Store). The manager names only what it needs here and never
// reaches into the history tables, the way the projects service names SessionStopper and this
// manager implements that.
type HistoryRecorder interface {
	// Append stores records as the card's next events, in the order given. The recorder gives each
	// one its own id and the card's next sequence number.
	Append(ctx context.Context, cardID, sessionID string, records []history.Record) error
	// AppendChat is Append for a project chat's session: the records are the chat's next events.
	AppendChat(ctx context.Context, chatID, sessionID string, records []history.Record) error
}

// recordEvent stores one agent event in the card's history, if the event is history at all. The
// write runs on the manager's own context, never a request's: the event has already happened and
// does not belong to whatever request happened to be open. A failure is logged and swallowed,
// because the live chat still reaches the client over the bus and a chapter of history that could
// not be written must not stop the pump.
func (m *Manager) recordEvent(ls *liveSession, ev agents.AgentEvent) {
	records, err := history.RecordsOf(ev)
	if err != nil {
		m.log.Error("could not build a session history record", ls.noun()+"_id", ls.key(), "error", err)
		return
	}
	m.appendRecords(ls, records)
}

// recordUserMessage stores a message that was just delivered into the session, so the person sees
// their own words when the card is opened again. It is called only after the agent took the
// message, never for one that was refused.
func (m *Manager) recordUserMessage(ls *liveSession, text string) {
	m.appendRecords(ls, []history.Record{{Kind: history.KindUser, Summary: text}})
}

// appendRecords hands records to the recorder, if one is set. A manager with no recorder keeps no
// history: see Config.History.
func (m *Manager) appendRecords(ls *liveSession, records []history.Record) {
	if m.cfg.History == nil || len(records) == 0 {
		return
	}
	var err error
	if ls.isChat() {
		err = m.cfg.History.AppendChat(m.ctx, ls.chatID, ls.sessionRowID, records)
	} else {
		err = m.cfg.History.Append(m.ctx, ls.cardID, ls.sessionRowID, records)
	}
	if err != nil {
		m.log.Error("could not store a session history record", ls.noun()+"_id", ls.key(), "error", err)
	}
}
