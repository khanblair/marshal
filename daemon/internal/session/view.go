package session

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The chat view and the terminal view of a card (docs/architecture.md 4.3, checklist item B2.7, and
// the proposed answer to open question Q19).
//
// A CLI's structured mode and its interactive terminal mode are different ways of running it, and one
// process cannot serve both. Switching stops the current process and resumes the same agent session
// id in the other mode, so the conversation and its context carry over. The mode is stored with the
// session row, so a restart resumes the session in the mode it was left in, and the wire card says
// which one it is in.
//
// Rules, decided here and written into architecture.md 4.3:
//
//   - A card needs an agent running in this daemon. A card that never started, or whose session
//     slept, stopped, or has not been resumed since a restart, has nothing to switch.
//   - A turn that is running is not cut off. The agent's process would end in the middle of it, so
//     the switch is refused until the turn ends. Nothing is interrupted for the person: there is no
//     interrupt call, and stopping the card is the way to end a turn early. Leaving the terminal
//     view has no such rule, because a terminal has no turns.
//   - A paused card that is holding a message cannot switch, for the reason it cannot sleep: the
//     waiting messages live in memory with the process.
//   - An agent with no terminal mode that Marshal can resume a session in is refused before anything
//     is stopped, and nothing changes.
//   - A switch whose new process cannot pick the session up follows section 5.3: the old process has
//     already stopped, so the card moves to Needs you, its session stops, and the person starts it
//     again in the chat view. Marshal never starts a fresh conversation in its place.
//   - The terminal view is never restored by a restart of the chat: a session that stops, however it
//     stops, is back in the chat view (queries/sessions.sql, UpdateSessionRuntime).
//   - What is typed in the terminal is not part of the card's chat history, which is made of the
//     agent's structured events. The agent's own session file keeps it, so the conversation is whole
//     when the agent reads it back, but the chat view does not show what happened in the terminal.

// The sentences a person reads when a switch is refused, next to the stable reason of each.
const (
	messageViewNoAgent          = "This card has no agent running. Start the card first."
	messageViewTurnRunning      = "The agent is in the middle of a turn. Wait for it to finish, then switch views."
	messageViewHoldingMessages  = "This card has a message waiting for you to resume it. Resume the card first."
	messageViewNoTerminal       = "This agent has no terminal view. Stay in the chat view."
	messageViewSwitching        = "This card is switching views. Try again in a moment."
	messageViewTerminalActive   = "This card is in terminal view. Type in the terminal, or switch to chat view to send a message."
	messageViewCannotResumeNote = "Marshal could not pick this session back up. The card now needs you."
)

// viewRefusalMessage is the sentence for a reason.
func viewRefusalMessage(reason protocol.ViewRefusalReason) string {
	switch reason {
	case protocol.ViewRefusalReasonNoAgent:
		return messageViewNoAgent
	case protocol.ViewRefusalReasonTurnRunning:
		return messageViewTurnRunning
	case protocol.ViewRefusalReasonHoldingMessages:
		return messageViewHoldingMessages
	case protocol.ViewRefusalReasonNoTerminal:
		return messageViewNoTerminal
	case protocol.ViewRefusalReasonSwitching:
		return messageViewSwitching
	case protocol.ViewRefusalReasonTerminalActive:
		return messageViewTerminalActive
	case protocol.ViewRefusalReasonCannotResume:
		return messageViewCannotResumeNote
	}
	return ""
}

// refusedView builds the refusal for a reason, with its sentence and the stable reason in details.
func refusedView(reason protocol.ViewRefusalReason) *protocol.Error {
	return protocol.Refused(viewRefusalMessage(reason)).With("reason", string(reason))
}

// viewMode is the view the live session runs in.
func (ls *liveSession) viewMode() protocol.CardViewMode {
	if ls.term != nil {
		return protocol.CardViewModeTerminal
	}
	return protocol.CardViewModeChat
}

// SwitchView stops the process of a card's agent and resumes the same session in the other view,
// and answers with the view the card is in. Asking for the view the card is already in is not an
// error and changes nothing. The rules are at the top of this file. It can take as long as
// resuming a card, because a new process starts.
//
// Once the old process is stopped the switch finishes even if the request that asked for it goes
// away, so a client that gave up cannot leave the card between two processes.
func (m *Manager) SwitchView(ctx context.Context, cardID string, mode protocol.CardViewMode) (protocol.CardView, error) {
	unlock := m.viewLocks.Lock(cardID)
	defer unlock()
	card, err := m.projects.Card(ctx, cardID)
	if err != nil {
		return protocol.CardView{}, err
	}
	old := m.liveOf(cardID)
	if old == nil {
		return protocol.CardView{}, refusedView(protocol.ViewRefusalReasonNoAgent).With("cardId", cardID)
	}
	if old.viewMode() == mode {
		return m.currentView(ctx, cardID, mode)
	}
	plan, err := m.planSwitch(ctx, old, mode)
	if err != nil {
		return protocol.CardView{}, err
	}
	if err := m.beginSwitch(cardID); err != nil {
		return protocol.CardView{}, err
	}
	defer m.endSwitch(cardID)
	return m.swap(context.WithoutCancel(ctx), card, old, plan)
}

// currentView answers for a card that is already in the view that was asked for.
func (m *Manager) currentView(ctx context.Context, cardID string, mode protocol.CardViewMode) (protocol.CardView, error) {
	row, err := m.store.Queries().GetSessionByCard(ctx, cardID)
	if err != nil {
		return protocol.CardView{}, fmt.Errorf("read the session of card %s: %w", cardID, err)
	}
	return m.viewAnswer(cardID, mode, protocol.SessionState(row.State)), nil
}

// viewAnswer builds the answer of a view switch.
func (m *Manager) viewAnswer(cardID string, mode protocol.CardViewMode, state protocol.SessionState) protocol.CardView {
	return protocol.CardView{
		CardID: cardID, Mode: mode, Session: state, ServerTime: protocol.NewTimestamp(m.cfg.Now()),
	}
}

// beginSwitch claims a card for a view switch, refusing while the card is being started, resumed,
// woken, or switched already, and while the manager is shutting down. Call endSwitch when done.
func (m *Manager) beginSwitch(cardID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return protocol.Unavailable("Marshal is shutting down.")
	}
	_, switching := m.switching[cardID]
	_, pending := m.pending[cardID]
	if switching || pending {
		return refusedView(protocol.ViewRefusalReasonSwitching).With("cardId", cardID)
	}
	m.switching[cardID] = struct{}{}
	return nil
}

func (m *Manager) endSwitch(cardID string) {
	m.mu.Lock()
	delete(m.switching, cardID)
	m.mu.Unlock()
}

// switchPlan is everything a switch needs, found and checked before anything is stopped, so a
// refusal changes nothing.
type switchPlan struct {
	// mode is the view to switch to.
	mode protocol.CardViewMode
	// agent is the agent that runs in that view.
	agent agents.Agent
	// row is the session row, with the agent session id to resume.
	row db.Session
	// path is the card's worktree.
	path string
}

// planSwitch checks that a live session can switch to a view and finds what the switch needs. Every
// refusal is made here, before the process is touched.
func (m *Manager) planSwitch(ctx context.Context, ls *liveSession, mode protocol.CardViewMode) (switchPlan, error) {
	cardID := ls.cardID
	if ls.isBusy() {
		return switchPlan{}, refusedView(protocol.ViewRefusalReasonTurnRunning).With("cardId", cardID)
	}
	if ls.waiting() > 0 {
		return switchPlan{}, refusedView(protocol.ViewRefusalReasonHoldingMessages).With("cardId", cardID)
	}
	row, err := m.store.Queries().GetSessionByCard(ctx, cardID)
	if err != nil {
		return switchPlan{}, fmt.Errorf("read the session of card %s: %w", cardID, err)
	}
	path, _, err := m.projects.Worktree(ctx, cardID)
	if err != nil {
		return switchPlan{}, err
	}
	if path == "" || row.AgentSessionID == "" {
		return switchPlan{}, refusedView(protocol.ViewRefusalReasonNoAgent).With("cardId", cardID)
	}
	agent, err := m.agentForSwitch(protocol.AgentKind(row.AgentKind), mode)
	if err != nil {
		return switchPlan{}, m.noTerminalOr(err, cardID)
	}
	return switchPlan{mode: mode, agent: agent, row: row, path: path}, nil
}

// agentForSwitch makes the agent for the view to switch to. An agent for the terminal view must be
// a terminal and must be able to resume a session, or the conversation would not carry over. That is
// checked here, before anything is stopped, and not left to fail once the chat process is gone.
func (m *Manager) agentForSwitch(kind protocol.AgentKind, mode protocol.CardViewMode) (agents.Agent, error) {
	agent, err := m.agentFor(kind, mode)
	if err != nil {
		return nil, err
	}
	if mode != protocol.CardViewModeTerminal {
		return agent, nil
	}
	if _, isTerminal := agent.(agents.Terminal); !isTerminal || !agent.Capabilities().Resume {
		return nil, fmt.Errorf("%w: the %s terminal cannot resume a session", agents.ErrUnknownKind, kind)
	}
	return agent, nil
}

// noTerminalOr turns "there is no terminal for this agent" into its refusal, and leaves any other
// failure to make an agent as it is.
func (m *Manager) noTerminalOr(err error, cardID string) error {
	if errors.Is(err, agents.ErrUnknownKind) {
		m.log.Info("refused to switch a card's view: its agent has no terminal view", "card_id", cardID, "error", err)
		return refusedView(protocol.ViewRefusalReasonNoTerminal).With("cardId", cardID)
	}
	return fmt.Errorf("make the agent to switch card %s to: %w", cardID, err)
}

// agentFor makes the agent that runs a kind in a view: the registry's for the chat view, and the
// terminals' for the terminal view. A kind that has no terminal is agents.ErrUnknownKind.
func (m *Manager) agentFor(kind protocol.AgentKind, view protocol.CardViewMode) (agents.Agent, error) {
	if view != protocol.CardViewModeTerminal {
		return m.registry.New(kind)
	}
	if m.cfg.Terminals == nil {
		return nil, fmt.Errorf("%w: %q has no terminal", agents.ErrUnknownKind, kind)
	}
	return m.cfg.Terminals.New(kind)
}

// swap does the switch: stop the old process, resume the same session in the new mode, store the
// mode, and make the new session live. It is called once the plan has passed every check. The
// context outlives the request, see SwitchView.
func (m *Manager) swap(ctx context.Context, card protocol.Card, old *liveSession, plan switchPlan) (protocol.CardView, error) {
	stopCtx, cancelStop := context.WithTimeout(ctx, closeStopTimeout)
	defer cancelStop()
	old.setStopRequested()
	if err := old.agent.Stop(stopCtx, old.handle); err != nil {
		// The process is still there, so nothing changed after all.
		old.clearStopRequested()
		return protocol.CardView{}, fmt.Errorf("stop the process of card %s to switch its view: %w", card.ID, err)
	}
	m.forget(card.ID, old)

	resumeCtx, cancelResume := context.WithTimeout(ctx, resumeTimeout)
	defer cancelResume()
	handle, err := plan.agent.Resume(resumeCtx, plan.row.AgentSessionID, resumeSpec(card, plan.path))
	if err != nil {
		return protocol.CardView{}, m.failSwitch(ctx, plan.row, err)
	}
	started := startedAgent{agent: plan.agent, handle: handle, view: plan.mode}
	if err := m.storeSwitch(ctx, plan.row, started); err != nil {
		m.stopUnregistered(started)
		return protocol.CardView{}, err
	}
	if _, err := m.register(cardOwner(card), plan.row.ID, started); err != nil {
		return protocol.CardView{}, err
	}
	m.log.Info("switched a card's view", "card_id", card.ID, "session_id", plan.row.ID, "view", plan.mode)
	return m.viewAnswer(card.ID, plan.mode, protocol.SessionStateAwake), nil
}

// failSwitch records that the new process could not pick the session up, by the rule of section 5.3
// (the session stops and the card needs the person), and answers with that sentence and the reason
// of a switch.
func (m *Manager) failSwitch(ctx context.Context, row db.Session, cause error) error {
	_, err := m.failResume(ctx, row, cause)
	var refusal *protocol.Error
	if errors.As(err, &refusal) {
		return refusal.With("reason", string(protocol.ViewRefusalReasonCannotResume))
	}
	return err
}

// storeSwitch writes what a successful switch changed in the session row: the session is awake, and
// runs in the new view. It is one write, so a restart never sees one without the other.
func (m *Manager) storeSwitch(ctx context.Context, row db.Session, sa startedAgent) error {
	now := m.cfg.Now().UnixMilli()
	err := m.store.Write(ctx, func(q *db.Queries) error {
		if _, err := q.UpdateSessionRuntime(ctx, db.UpdateSessionRuntimeParams{
			State: string(protocol.SessionStateAwake), AgentSessionID: sa.handle.ID,
			LastActiveAt: now, UpdatedAt: now, ID: row.ID,
		}); err != nil {
			return err
		}
		_, err := q.UpdateSessionView(ctx, db.UpdateSessionViewParams{ViewMode: string(sa.view), UpdatedAt: now, ID: row.ID})
		return err
	})
	if err != nil {
		return fmt.Errorf("record that the session of card %s switched to the %s view: %w", row.CardID, sa.view, err)
	}
	return nil
}

// terminalActiveRefusal is the refusal for a chat message to a card that is in the terminal view.
func terminalActiveRefusal(cardID string) *protocol.Error {
	return refusedView(protocol.ViewRefusalReasonTerminalActive).With("cardId", cardID)
}

// sessionIsInTheTerminal says whether a session row reads as being in the terminal view.
func sessionIsInTheTerminal(row db.Session) bool {
	return protocol.CardViewMode(row.ViewMode) == protocol.CardViewModeTerminal
}
