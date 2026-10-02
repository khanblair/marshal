package session

import (
	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/providers"
)

// recordUsage files what a finished turn cost, when the agent said. It runs before the session goes
// back to awake, so the Home numbers are there by the time a client hears the turn is over. A
// failure is logged and nothing else: the cost is a record of the turn, and never a reason to fail it.
func (m *Manager) recordUsage(ls *liveSession, ev agents.TurnEnded) {
	if m.cfg.Usage == nil || ev.Usage == nil {
		return
	}
	model := ev.Usage.Model
	if model == "" {
		model = ls.handle.Model
	}
	err := m.cfg.Usage.Record(m.ctx, providers.UsageRecord{
		CardID: ls.cardID, ProjectID: ls.projectID,
		Provider: ev.Usage.Provider, Model: model,
		InputTokens: ev.Usage.InputTokens, OutputTokens: ev.Usage.OutputTokens,
		CostMicros: ev.Usage.CostMicros, At: m.cfg.Now(),
	})
	if err != nil {
		m.log.Error("could not record what a turn cost", ls.noun()+"_id", ls.key(), "error", err)
	}
}
