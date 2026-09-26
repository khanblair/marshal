package session

import (
	"context"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// pump reads one live session's events until its channel closes, logging and publishing each one,
// and reacting to the ones that change state. It runs under the Manager's own long-lived context,
// never the context of whatever request started the session, because it outlives every such
// request. Cleanup (closing the log, removing the session from the map, and, unless the exit was
// expected, moving the card to needs you) happens once after the loop ends, not only when the
// last event happens to be Exited, so a channel that closes without one is still cleaned up. A
// chat's session is pumped the same way, and has no card to move.
func (m *Manager) pump(ls *liveSession) {
	defer m.pumpWG.Done()
	defer m.finishPump(ls)
	for ev := range ls.events {
		m.handleEvent(ls, ev)
	}
}

// handleEvent logs and rings every event, publishes the ones that carry content, and reacts to the
// ones that change state.
func (m *Manager) handleEvent(ls *liveSession, ev agents.AgentEvent) {
	m.logEvent(ls, ev)
	switch e := ev.(type) {
	case agents.MessageChunk, agents.ThoughtChunk, agents.PlanUpdate:
		m.publishOutput(ls, e)
	case agents.ToolCall, agents.ToolCallUpdate:
		m.publishToolCall(ls, e)
	case agents.TurnEnded:
		m.onTurnEnded(ls)
	case agents.TerminalOutput:
		m.publishTerminalOutput(ls, e)
	case agents.Failed:
		// Still wait for the Exited that follows (the agents.Failed doc comment says one always
		// does, whether the process survives or not); nothing else to do here.
		m.log.Warn("an agent session failed", ls.noun()+"_id", ls.key(), "message", e.Message)
	}
	// agents.PermissionRequested is logged and ringed above but not published: approvals are
	// Phase 3 (B3.4), which does not exist yet (see the report). agents.Exited needs no case here;
	// finishPump reacts to it once the loop ends, which covers every way the loop can end.
}

// logEvent writes an event to the session's on-disk log, adds it to its in-memory ring, and stores
// the history the API layer pages back. The log write and the history write are independent: a
// history row that cannot be written is logged by record and does not stop the pump, and the event
// is still in the log file.
func (m *Manager) logEvent(ls *liveSession, ev agents.AgentEvent) {
	now := m.cfg.Now().UTC()
	line, err := marshalLogLine(now, ev)
	if err != nil {
		m.log.Error("could not encode a session log line", ls.noun()+"_id", ls.key(), "error", err)
		return
	}
	if err := ls.diskLog.write(line, isStateChanging(ev)); err != nil {
		m.log.Error("could not write to a session log", ls.noun()+"_id", ls.key(), "error", err)
	}
	if _, isTerminal := ev.(agents.TerminalOutput); isTerminal {
		// A terminal keeps its own screen, and its bytes are not history: they would fill the ring
		// with data that is far bigger than the line that was counted for it.
		return
	}
	ls.ring.add(LogEntry{At: now, Kind: eventKind(ev), Event: ev}, len(line))
	m.recordEvent(ls, ev)
}

// publishOutput publishes session.output for a message chunk, a thought chunk, or a plan update.
func (m *Manager) publishOutput(ls *liveSession, ev agents.AgentEvent) {
	data := protocol.SessionOutputEventData{CardID: ls.cardID, ChatID: ls.chatID}
	switch e := ev.(type) {
	case agents.MessageChunk:
		data.Kind = "message"
		data.Text, _ = agents.Truncate(e.Text, agents.MaxContentBytes)
	case agents.ThoughtChunk:
		data.Kind = "thought"
		data.Text, _ = agents.Truncate(e.Text, agents.MaxContentBytes)
	case agents.PlanUpdate:
		data.Kind = "plan"
		data.Plan = wirePlanSteps(e.Steps)
	default:
		return
	}
	m.bus.Publish(string(ls.topic()), string(protocol.EventTypeSessionOutput), data, false)
}

// publishToolCall publishes session.tool_call for a tool call starting or an update to one.
func (m *Manager) publishToolCall(ls *liveSession, ev agents.AgentEvent) {
	data := protocol.SessionToolCallEventData{CardID: ls.cardID, ChatID: ls.chatID}
	switch e := ev.(type) {
	case agents.ToolCall:
		data.Kind, data.ToolCall = "tool_call", wireToolCall(e)
	case agents.ToolCallUpdate:
		data.Kind, data.ToolCall = "tool_call_update", wireToolCallUpdate(e)
	default:
		return
	}
	m.bus.Publish(string(ls.topic()), string(protocol.EventTypeSessionToolCall), data, false)
}

// onTurnEnded reacts to a turn ending: the session goes back to awake, and the next queued
// message, if any, is delivered. A TurnEnded that arrives after Stop (or Close) already asked this
// session to end is not this function's to react to: Stop owns the session's final state (it
// already wrote "stopped" before asking the agent to end), and Close deliberately writes nothing,
// so resurrecting the row to "awake" here, or delivering a queued message to a session that is on
// its way out, would both be wrong. This can happen because a graceful stop can end an in-flight
// turn with TurnEnded(cancelled) before the process actually exits (docs/architecture.md 4.1's
// Interrupt rule), and the pump has no way to tell that turn end apart from an ordinary one except
// by asking whether a stop was requested.
//
// A pause holds the card between turns: the turn that just ended does not start the next message,
// which waits until the card is resumed (see deliverHeld). A chat cannot be paused.
func (m *Manager) onTurnEnded(ls *liveSession) {
	if ls.wasStopRequested() {
		return
	}
	if err := m.setSessionState(m.ctx, ls, protocol.SessionStateAwake); err != nil {
		m.log.Error("could not record that a turn ended", ls.noun()+"_id", ls.key(), "error", err)
	}
	m.publishState(ls, protocol.SessionStateAwake, "")
	if !ls.isChat() && m.cardPaused(m.ctx, ls.cardID) {
		// The turn is over even though the queue is not empty, so the session is free: the next
		// message starts its turn the moment the card is resumed. Without this the session would
		// stay marked busy for a turn that already ended, and the held message could never go.
		ls.release()
		return
	}
	text, ok := ls.next()
	if !ok {
		return
	}
	m.deliverQueued(m.ctx, ls, text)
}

// deliverQueued starts one turn with a message that was waiting: one the pump is releasing after a
// turn ended, or one a pause was holding until the card was resumed. The turn is marked before the
// send for the same reason Send itself marks first (see send.go), and a send that fails releases
// the turn so the session is not left believing one is running.
func (m *Manager) deliverQueued(ctx context.Context, ls *liveSession, text string) {
	m.markTurnStarting(ctx, ls)
	if err := ls.agent.Send(ctx, ls.handle, agents.UserMessage{Text: text}); err != nil {
		m.log.Error("could not deliver a queued message", ls.noun()+"_id", ls.key(), "error", err)
		ls.release()
		m.revertTurnStarting(ctx, ls)
		return
	}
	// A queued message is history at the moment it actually reaches the agent, which is here, not
	// when the person pressed send (see Send).
	m.recordUserMessage(ls, text)
	m.markCardWorking(ctx, ls)
}

// finishPump runs once, after a session's event channel closes, however that happened. An exit
// that Stop, Sleep, or Close asked for is left as those methods already recorded it (Stop marks
// the row stopped, Sleep marks it asleep, and Close deliberately does not, so the row still reads
// as needing a resume next start). Anything else is an unexpected exit: the row is marked stopped
// and the card moves to needs you, mirroring a failed resume (docs/architecture.md 5.3). A chat
// has no card, so its unexpected exit only marks the row stopped and says so on the chat's topic;
// the chat's next message tries to pick the conversation back up.
func (m *Manager) finishPump(ls *liveSession) {
	defer m.forget(ls.key(), ls)
	if ls.term != nil {
		// The terminal's writer stops with its session.
		defer ls.term.close()
	}
	defer func() {
		if err := ls.diskLog.close(); err != nil {
			m.log.Error("could not close a session log", ls.noun()+"_id", ls.key(), "error", err)
		}
	}()
	if ls.wasStopRequested() {
		return
	}
	if err := m.setSessionState(m.ctx, ls, protocol.SessionStateStopped); err != nil {
		m.log.Error("could not record that a session exited", ls.noun()+"_id", ls.key(), "error", err)
	}
	if !ls.isChat() {
		if _, err := m.projects.SetState(m.ctx, ls.cardID, protocol.CardStateNeeds); err != nil {
			m.log.Error("could not move a card to needs you after its agent exited", "card_id", ls.cardID, "error", err)
		}
	}
	reason := "The agent stopped unexpectedly."
	m.publishState(ls, protocol.SessionStateStopped, reason)
	m.log.Warn("an agent session exited unexpectedly", ls.noun()+"_id", ls.key())
}
