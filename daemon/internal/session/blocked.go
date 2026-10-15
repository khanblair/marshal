package session

import (
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// deniedMarker is what Claude Code answers a tool call that needs an approval nobody can give. Marshal
// runs the agent with its permission prompts off, so such a call is refused, and the agent usually
// says so in words and then waits: nothing on the board moves.
const deniedMarker = "permission for this tool use was denied"

// trackDenial remembers when a tool call was refused for lack of a permission, and forgets it when the
// agent goes on to make another call: a refusal the agent worked around is not what stopped it.
func (ls *liveSession) trackDenial(ev agents.AgentEvent) {
	switch e := ev.(type) {
	case agents.ToolCall:
		if ls.toolTitles == nil {
			ls.toolTitles = map[string]string{}
		}
		ls.toolTitles[e.ID] = e.Title
		ls.lastDenied = ""
	case agents.ToolCallUpdate:
		if e.Status != agents.StatusFailed || !strings.Contains(strings.ToLower(e.Content), deniedMarker) {
			return
		}
		title := e.Title
		if title == "" {
			title = ls.toolTitles[e.ID]
		}
		if title == "" {
			title = "a command"
		}
		ls.lastDenied = title
	}
}

// alertIfBlocked moves a card to Needs you when its agent's turn ended on a command that was refused
// for lack of a permission, with what was refused and what to do about it.
func (m *Manager) alertIfBlocked(ls *liveSession) {
	denied := ls.lastDenied
	ls.lastDenied = ""
	if denied == "" || ls.isChat() {
		return
	}
	text := fmt.Sprintf("The agent is blocked: %s was refused, and Claude cannot ask you to approve it here. "+
		"Switch the card to Full auto to let it run, or do that step yourself, then message the agent to go on.", denied)
	if _, err := m.projects.SetNeeds(m.ctx, ls.cardID, protocol.NeedsReason{Kind: protocol.NeedsReasonKindStuck, Text: text}); err != nil {
		m.log.Warn("could not tell a person that a card's agent is blocked", "card_id", ls.cardID, "error", err)
	}
}
