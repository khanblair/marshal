package session_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func TestCompletedDistinctEditsDoNotPauseTheCard(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := cardWithRole(t, e, project.ID, "Make progress", "Worker")
	var script []agents.AgentEvent
	for i := range 5 {
		id := fmt.Sprint(i)
		script = append(script,
			agents.ToolCall{ID: id, Kind: "edit", Path: "main.go", EditFingerprint: id, Status: agents.StatusPending},
			agents.ToolCallUpdate{ID: id, Status: agents.StatusCompleted},
		)
	}
	e.agent.setTurnScript(script...)
	if err := e.mgr.Send(context.Background(), card.ID, "implement the feature"); err != nil {
		t.Fatal(err)
	}
	e.untilState(t, card.ID, protocol.SessionStateAwake)
	current, err := e.proj.Card(t.Context(), card.ID)
	if err != nil || current.NeedsReason != nil {
		t.Fatalf("progress paused the card: %+v, %v", current.NeedsReason, err)
	}
}

func TestFailedEditAttemptsStillExposeARepeatedError(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := cardWithRole(t, e, project.ID, "Repeated failure", "Worker")
	var script []agents.AgentEvent
	for i := range 3 {
		id := fmt.Sprint(i)
		script = append(script,
			agents.ToolCall{ID: id, Kind: "edit", Path: "main.go", EditFingerprint: id, Status: agents.StatusPending},
			agents.ToolCallUpdate{ID: id, Status: agents.StatusFailed, Content: "the same replacement failed"},
		)
	}
	e.agent.setTurnScript(script...)
	if err := e.mgr.Send(context.Background(), card.ID, "fix the file"); err != nil {
		t.Fatal(err)
	}
	current := needsCard(t, e, card.ID)
	if current.NeedsReason == nil || current.NeedsReason.Kind != protocol.NeedsReasonKindStuck || !strings.Contains(current.NeedsReason.Text, "error") {
		t.Fatalf("the repeated error was hidden by edit attempts: %+v", current.NeedsReason)
	}
}
