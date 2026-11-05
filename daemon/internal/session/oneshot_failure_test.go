package session_test

import (
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

func TestAskRejectsInterruptedPartialAnswers(t *testing.T) {
	for _, ending := range []agents.AgentEvent{
		agents.TurnEnded{Reason: agents.TurnError},
		agents.TurnEnded{Reason: agents.TurnCancelled},
		agents.Exited{Code: 1},
	} {
		e := newEnv(t)
		e.agent.setTurnScript(agents.MessageChunk{Text: "I will review."}, ending)
		answer, err := e.mgr.Ask(t.Context(), "claude", agentsSpec(t), "Review this")
		if err == nil || answer != "" {
			t.Errorf("ending %T returned partial answer %q, %v", ending, answer, err)
		}
		_ = e.mgr.Close()
	}
}
