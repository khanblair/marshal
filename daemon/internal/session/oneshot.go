package session

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Ask puts one question to a fresh agent and answers what it said. The agent is started with the spec,
// sent the prompt, read until its turn ends, and stopped. It is how the Reviewer reads a card's
// changes: nothing is kept, no card or chat is involved, and nothing the agent says reaches a board.
//
// An agent that fails, or ends without saying anything, is an error that says why in the agent's own
// words, so a signed-out Claude reads as that and not as an empty answer.
func (m *Manager) Ask(ctx context.Context, kind protocol.AgentKind, spec agents.StartSpec, prompt string) (string, error) {
	if m.cfg.DirectWorkspace {
		spec.PermissionMode = string(protocol.PermissionModePlan)
		spec.DisallowedTools = append(spec.DisallowedTools, "Edit", "Write", "MultiEdit", "NotebookEdit", "Bash")
	}
	agent, err := m.registry.New(kind)
	if err != nil {
		return "", fmt.Errorf("make an agent to ask: %w", err)
	}
	handle, err := agent.Start(ctx, spec)
	if err != nil {
		return "", fmt.Errorf("start an agent to ask: %w", err)
	}
	defer func() {
		if err := agent.Stop(context.WithoutCancel(ctx), handle); err != nil {
			m.log.Warn("could not stop an agent that answered a question", "error", err)
		}
	}()
	events := agent.Events(handle)
	if err := agent.Send(ctx, handle, agents.UserMessage{Text: prompt}); err != nil {
		return "", fmt.Errorf("ask the agent: %w", err)
	}
	var said strings.Builder
	failure := ""
	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("the agent did not answer in time: %w", ctx.Err())
		case ev, open := <-events:
			if !open {
				return answerOf(&said, failure)
			}
			switch e := ev.(type) {
			case agents.MessageChunk:
				said.WriteString(e.Text)
			case agents.Failed:
				failure = e.Message
			case agents.TurnEnded, agents.Exited:
				return answerOf(&said, failure)
			}
		}
	}
}

// answerOf is what an agent said, or the reason it said nothing.
func answerOf(said *strings.Builder, failure string) (string, error) {
	text := strings.TrimSpace(said.String())
	switch {
	case failure != "" && text == "":
		return "", errors.New(failure)
	case text == "":
		return "", errors.New("the agent ended without an answer")
	}
	return text, nil
}
