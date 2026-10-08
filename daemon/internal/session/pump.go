package session

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	// Closed last of the three, so a waiter that sees it released knows finishPump's cleanup - and the
	// giving up of the card's internal server that happens there - is done (see waitPump).
	defer close(ls.done)
	defer m.finishPump(ls)
	for ev := range ls.events {
		m.handleEvent(ls, ev)
	}
}

// waitPump waits for a session's pump goroutine to finish. A view switch and a handoff both stop one
// process and start the next for the same card in a single step, and both re-attach the card's
// internal MCP server as they do it. The server is one per card, so the ending session's own cleanup
// would otherwise take it away from the session that replaced it - leaving the new agent with a tool
// command that answers nothing - whenever the pump happened to run after the new attachment. Waiting
// makes the order certain: the old session has let go before the card is given to the new one.
//
// It answers false when the wait ran out, which the caller logs and carries on from: the restart is
// still the right thing to do, and a pump that has not finished by then is a session whose agent is
// ignoring its stop.
func (m *Manager) waitPump(ls *liveSession) bool {
	timer := time.NewTimer(pumpDrainTimeout)
	defer timer.Stop()
	select {
	case <-ls.done:
		return true
	case <-timer.C:
		return false
	case <-m.ctx.Done():
		// The daemon is shutting down: the new session will not be started either, so there is
		// nothing left to order against.
		return false
	}
}

// handleEvent logs and rings every event, publishes the ones that carry content, and reacts to the
// ones that change state.
func (m *Manager) handleEvent(ls *liveSession, ev agents.AgentEvent) {
	m.logEvent(ls, ev)
	// The stuck detector watches the same events the person's chat does, in the same order, so a
	// loop it catches is one the chat shows too (B5.3).
	m.feedStuckDetector(ls, ev)
	switch e := ev.(type) {
	case agents.MessageChunk, agents.ThoughtChunk, agents.PlanUpdate:
		m.publishOutput(ls, e)
	case agents.ToolCall, agents.ToolCallUpdate:
		m.publishToolCall(ls, e)
	case agents.TurnEnded:
		m.recordUsage(ls, e)
		m.onTurnEnded(ls)
	case agents.TerminalOutput:
		m.publishTerminalOutput(ls, e)
	case agents.PermissionRequested:
		// The agent is blocked until a person answers, so this is where an approval is recorded,
		// held, and announced (B3.4).
		m.onPermissionRequested(ls, e)
	case agents.Failed:
		// A failure that kills the process is followed by its Exited, which finishPump reacts to.
		m.log.Warn("an agent session failed", ls.noun()+"_id", ls.key(), "message", e.Message)
		// A card's agent that is signed out can do nothing, and its process stays alive, so the end
		// of its turn ends it (see onTurnEnded).
		ls.signedOut = e.SignedOut && !ls.isChat()
		ls.failureCause = failureCause(e.Detail)
	}
	// agents.Exited needs no case here; finishPump reacts to it once the loop ends, which covers
	// every way the loop can end.
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
	// A failure that left the process running was said in the chat; it is not why a later exit happened.
	ls.failureCause = ""
	if ls.wasStopRequested() {
		return
	}
	if ls.signedOut {
		// Ending the idle process is what lets the person resume with a new one, which reads the
		// login they have just made. finishPump then moves the card and says why.
		m.endSignedOut(ls)
		return
	}
	if err := m.setSessionState(m.ctx, ls, protocol.SessionStateAwake); err != nil {
		m.log.Error("could not record that a turn ended", ls.noun()+"_id", ls.key(), "error", err)
	}
	// The turn's bookkeeping is settled before "awake" is announced, so a client that hears the
	// session is awake sees a session that really is free. Announcing first would leave a window in
	// which the card reads as awake but a switch is still told a turn is running; the commit scan
	// below makes that window wide enough to matter.
	if m.scanTurnForSecrets(ls) {
		// The agent's commit holds something that looks like a credential, so the card is stopped:
		// it has been moved to Needs you with the reason and audited. The session is freed without
		// delivering the next queued message, so the agent does not build on top of the commit; the
		// message waits in the queue for the person to act.
		ls.release()
		m.publishState(ls, protocol.SessionStateAwake, "")
		return
	}
	if m.checkAfterTurn(ls) {
		// A loop the stuck detector caught, or a ceiling of the card's role that the turn just
		// went past (B5.3): the card has been moved to Needs you with the reason and audited. The
		// session is freed the way the secret scan frees it, so whatever was queued waits for the
		// person rather than being fed into a card that is waiting on them.
		ls.release()
		m.publishState(ls, protocol.SessionStateAwake, "")
		return
	}
	if !ls.isChat() && m.cardPaused(m.ctx, ls.cardID) {
		// The turn is over even though the queue is not empty, so the session is free: the next
		// message starts its turn the moment the card is resumed. Without this the session would
		// stay marked busy for a turn that already ended, and the held message could never go.
		ls.release()
		m.publishState(ls, protocol.SessionStateAwake, "")
		return
	}
	text, ok := ls.next()
	m.publishState(ls, protocol.SessionStateAwake, "")
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
	// A message that waited for the turn before it starts its own turn with the card's settings as
	// they are now, the same way a message sent into an idle session does (B3.6).
	m.applyCardSettings(ctx, ls)
	m.markTurnStarting(ctx, ls)
	if err := m.sendTurn(ctx, ls, text); err != nil {
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
	// What the session was given at its start - the internal MCP server - is given up here, which
	// is the one place every ending passes through: a stop, a sleep, a view switch, a crash, and the
	// daemon closing.
	defer m.detach(ls)
	if ls.term != nil {
		// The terminal's writer stops with its session.
		defer ls.term.close()
	}
	defer func() {
		if err := ls.diskLog.close(); err != nil {
			m.log.Error("could not close a session log", ls.noun()+"_id", ls.key(), "error", err)
		}
	}()
	// Any request this session was still waiting on is answered as the daemon, so no approval is
	// left waiting for an agent that no longer exists (B3.4).
	m.withdrawApprovals(m.ctx, ls)
	if ls.wasStopRequested() {
		return
	}
	if err := m.setSessionState(m.ctx, ls, protocol.SessionStateStopped); err != nil {
		m.log.Error("could not record that a session exited", ls.noun()+"_id", ls.key(), "error", err)
	}
	if !ls.isChat() {
		reason := protocol.NeedsReason{Kind: protocol.NeedsReasonKindStuck, Text: m.stoppedReason(ls)}
		if _, err := m.projects.SetNeeds(m.ctx, ls.cardID, reason); err != nil {
			m.log.Error("could not move a card to needs you after its agent exited", "card_id", ls.cardID, "error", err)
		}
	}
	note := "The agent stopped unexpectedly."
	if ls.signedOut {
		note = signedOutNote
	}
	m.publishState(ls, protocol.SessionStateStopped, note)
	m.log.Warn("an agent session exited unexpectedly", ls.noun()+"_id", ls.key(), "signed_out", ls.signedOut)
}

// endSignedOut ends the process of a session whose agent is signed out. It runs on its own
// goroutine because stopping waits on the process, and this one is the reader of its events.
func (m *Manager) endSignedOut(ls *liveSession) {
	go func() {
		if err := ls.agent.Stop(m.ctx, ls.handle); err != nil {
			m.log.Error("could not end the session of an agent that is signed out", "card_id", ls.cardID, "error", err)
		}
	}()
}

// signedOutNote is what the card's chat says when its agent is signed out.
const signedOutNote = "Claude is signed out, so the agent could not work. Sign in, then resume it."

// stoppedReason is what a card says when its agent stopped without being asked to: which card, why
// as far as the agent said, and what to do. It names the card because the same sentence is sent to
// a chat, where nothing around it says which project it is about.
func (m *Manager) stoppedReason(ls *liveSession) string {
	label := "this card"
	if card, err := m.projects.Card(m.ctx, ls.cardID); err == nil && card.Key != "" {
		label = card.Key
	}
	if ls.signedOut {
		return fmt.Sprintf("Claude is signed out on this Mac, so the agent for %s could not do any work. "+
			"Open a terminal, run claude, and sign in with /login. Then press Resume agent.", label)
	}
	cause := ""
	if ls.failureCause != "" {
		cause = ": " + ls.failureCause
	}
	return fmt.Sprintf("The agent for %s stopped unexpectedly%s. "+
		"Press Resume agent to start it again where it left off.", label, cause)
}

// failureCauseMax is how much of an agent's last words a card repeats.
const failureCauseMax = 160

// failureCause is the last thing the agent printed as it failed, cut short, for a card to say why.
// "exit status 1" alone is not a cause worth repeating, so it is left out.
func failureCause(detail string) string {
	lines := strings.Split(strings.TrimSpace(detail), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	if last == "" || strings.HasPrefix(last, "exit status") {
		return ""
	}
	if r := []rune(last); len(r) > failureCauseMax {
		last = string(r[:failureCauseMax]) + "..."
	}
	return strings.TrimRight(last, ". ")
}
