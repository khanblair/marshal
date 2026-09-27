package session

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Handing a card's work off to a different agent (docs/marshal-product-scope.md section 10.5,
// build-plan 7.5). The wire types, and why a handoff is not a resume, are in
// protocol/handoff.go.
//
// The rules, decided here:
//
//   - A card's session is one row for the card's whole life, so a handoff does not open a second
//     one: it stops the process the card has, rewrites which agent the row runs, and starts that
//     agent fresh in the same worktree. The card's history belongs to the card, not to the agent, so
//     the conversation the person sees is continuous across the change.
//   - The new agent is a new conversation, and the daemon never replays the stored history to an
//     agent. What makes the handoff "clean" is therefore the summary the request carries: it is
//     folded into the context the new session starts with, exactly the way a fresh card's context
//     reaches its agent, and is read once by the new agent's first turn.
//   - A card needs a session to hand off from. A card that never started has no work to continue and
//     is refused; a card whose session is stopped or asleep is handed off the same as a running one,
//     because the row is what says which agent it runs.
//   - A turn that is running is not cut off, and a paused card holding a message is not either: both
//     are refused, for the reasons a view switch refuses them (view.go).
//   - A handoff to the agent the card already runs is refused: that is a restart, which Stop and
//     Start are for, and the summary would go to the same agent that wrote the work.
//   - An agent kind Marshal has no adapter for, and one that cannot be made into an agent, are
//     refused before anything is stopped, so a refusal changes nothing.
//   - A handoff whose new agent could not be started after the old process had stopped follows
//     section 5.3: the row stops, the card moves to Needs you, and the person starts it again. The
//     row is left on the agent it had, so that restart continues what was there.

// The sentences a person reads when a handoff is refused, next to the stable reason of each.
const (
	messageHandoffNoSession       = "This card has no session to hand off. Start the card first."
	messageHandoffTurnRunning     = "The agent is in the middle of a turn. Wait for it to finish, then hand the card off."
	messageHandoffHoldingMessages = "This card has a message waiting for you to resume it. Resume the card first."
	messageHandoffSameAgent       = "This card already runs this agent. Choose a different one to hand off to."
	messageHandoffUnknownAgent    = "Marshal does not have that agent ready to run yet."
	messageHandoffCannotStart     = "Marshal could not start the new agent. The card now needs you."
)

// handoffRefusalMessage is the sentence for a reason.
func handoffRefusalMessage(reason protocol.HandoffRefusalReason) string {
	switch reason {
	case protocol.HandoffRefusalReasonNoSession:
		return messageHandoffNoSession
	case protocol.HandoffRefusalReasonTurnRunning:
		return messageHandoffTurnRunning
	case protocol.HandoffRefusalReasonHoldingMessages:
		return messageHandoffHoldingMessages
	case protocol.HandoffRefusalReasonSameAgent:
		return messageHandoffSameAgent
	case protocol.HandoffRefusalReasonUnknownAgent:
		return messageHandoffUnknownAgent
	case protocol.HandoffRefusalReasonCannotStart:
		return messageHandoffCannotStart
	}
	return ""
}

// refusedHandoff builds the refusal for a reason, with its sentence and the stable reason in details.
func refusedHandoff(reason protocol.HandoffRefusalReason) *protocol.Error {
	return protocol.Refused(handoffRefusalMessage(reason)).With("reason", string(reason))
}

// handoffHeading introduces a handoff's summary inside the new session's starting context, so the
// new agent reads it as the account of work it did not do, and not as something the person said.
const handoffHeading = "Work on this card is continuing here, on a new agent. A summary of where it got to follows."

// handoffNoteFormat is the one line the card's chat and activity list show after a handoff.
const handoffNoteFormat = "Handed this card off to %s. Its work continues there from a summary of where it got to."

// handoffInstructions joins a handoff's summary to the context a card's session is normally given.
// An empty summary leaves the context exactly as a fresh start would have it.
func handoffInstructions(summary, base string) string {
	summary, base = strings.TrimSpace(summary), strings.TrimSpace(base)
	switch {
	case summary == "":
		return base
	case base == "":
		return handoffHeading + "\n\n" + summary
	}
	return handoffHeading + "\n\n" + summary + "\n\n" + base
}

// Handoff continues a card's work on a different agent, from a clean summary of where it got to, and
// answers with the card. It can take as long as starting a card, because a new process starts. Once
// the old process is stopped the handoff finishes even if the request that asked for it goes away, so
// a client that gave up cannot leave the card between two agents.
func (m *Manager) Handoff(ctx context.Context, cardID string, in protocol.HandoffRequest) (protocol.Card, error) {
	// A handoff is the same shape of work as a view switch - one process stops and another starts for
	// the same card - so it takes the same per-card lock, and a message, a stop, or a sleep that
	// arrives in the middle waits for the handoff and then acts on the session it made.
	unlock := m.viewLocks.Lock(cardID)
	defer unlock()
	card, err := m.projects.Card(ctx, cardID)
	if err != nil {
		return protocol.Card{}, err
	}
	plan, err := m.planHandoff(ctx, card, in)
	if err != nil {
		return protocol.Card{}, err
	}
	// beginSwitch marks the card as one being restarted, the same mark a view switch makes, so a
	// start, a resume, a wake, or a second switch or handoff that arrives now is refused the same way.
	if err := m.beginSwitch(cardID); err != nil {
		return protocol.Card{}, err
	}
	defer m.endSwitch(cardID)
	return m.handOff(context.WithoutCancel(ctx), card, plan)
}

// handoffPlan is everything a handoff needs, found and checked before anything is stopped, so a
// refusal changes nothing.
type handoffPlan struct {
	// to is the agent kind the card continues on.
	to protocol.AgentKind
	// agent is the process to start, made before the old one is stopped.
	agent agents.Agent
	// row is the session row, whose id is kept and whose agent is rewritten.
	row db.Session
	// path is the card's worktree, where the new agent starts.
	path string
	// summary is what the new agent is told about the work so far.
	summary string
}

// planHandoff checks that a card can be handed off and finds what the handoff needs. Every refusal
// is made here, before the process is touched.
func (m *Manager) planHandoff(ctx context.Context, card protocol.Card, in protocol.HandoffRequest) (handoffPlan, error) {
	if !in.To.Valid() {
		return handoffPlan{}, protocol.InvalidArgument("Marshal does not know that agent.").With("agent", string(in.To))
	}
	if n := len([]rune(in.Summary)); n > protocol.MaxHandoffSummaryChars {
		return handoffPlan{}, protocol.InvalidArgument(
			"That summary is too long. Keep it to 20,000 characters.").With("length", fmt.Sprint(n))
	}
	row, err := m.store.Queries().GetSessionByCard(ctx, card.ID)
	if err != nil {
		if store.IsNotFound(err) {
			return handoffPlan{}, refusedHandoff(protocol.HandoffRefusalReasonNoSession).With("cardId", card.ID)
		}
		return handoffPlan{}, fmt.Errorf("read the session of card %s: %w", card.ID, err)
	}
	// The agent the card runs now is what the row says, not what the card's own setting says: the
	// setting is what the next start would use, and the row is what is actually running (or last ran).
	if protocol.AgentKind(row.AgentKind) == in.To {
		return handoffPlan{}, refusedHandoff(protocol.HandoffRefusalReasonSameAgent).
			With("cardId", card.ID).With("agent", string(in.To))
	}
	if ls := m.liveOf(card.ID); ls != nil {
		if ls.isBusy() {
			return handoffPlan{}, refusedHandoff(protocol.HandoffRefusalReasonTurnRunning).With("cardId", card.ID)
		}
		if ls.waiting() > 0 {
			return handoffPlan{}, refusedHandoff(protocol.HandoffRefusalReasonHoldingMessages).With("cardId", card.ID)
		}
	}
	if row.AgentSessionID == "" {
		return handoffPlan{}, refusedHandoff(protocol.HandoffRefusalReasonNoSession).With("cardId", card.ID)
	}
	path, _, err := m.projects.Worktree(ctx, card.ID)
	if err != nil {
		return handoffPlan{}, err
	}
	if path == "" {
		return handoffPlan{}, refusedHandoff(protocol.HandoffRefusalReasonNoSession).With("cardId", card.ID)
	}
	agent, err := m.agentFor(in.To, protocol.CardViewModeChat)
	if err != nil {
		if errors.Is(err, agents.ErrUnknownKind) {
			return handoffPlan{}, refusedHandoff(protocol.HandoffRefusalReasonUnknownAgent).
				With("cardId", card.ID).With("agent", string(in.To))
		}
		return handoffPlan{}, fmt.Errorf("make the agent to hand card %s off to: %w", card.ID, err)
	}
	return handoffPlan{to: in.To, agent: agent, row: row, path: path, summary: in.Summary}, nil
}

// handOff does the handoff: stop the old process, start the new agent in the same worktree with the
// summary, rewrite the session row, follow the card's own agent setting, and make the new session
// live. It is called once the plan has passed every check. The context outlives the request, see
// Handoff.
func (m *Manager) handOff(ctx context.Context, card protocol.Card, plan handoffPlan) (protocol.Card, error) {
	// A card whose session is stopped, asleep, or left over from an earlier run has no process to
	// stop, and there is nothing else to do before the new agent starts.
	if old := m.liveOf(card.ID); old != nil {
		stopCtx, cancelStop := context.WithTimeout(ctx, closeStopTimeout)
		defer cancelStop()
		old.setStopRequested()
		if err := old.agent.Stop(stopCtx, old.handle); err != nil {
			// The process is still there, so nothing changed after all.
			old.clearStopRequested()
			return protocol.Card{}, fmt.Errorf("stop the process of card %s to hand it off: %w", card.ID, err)
		}
		// The old session has let go of the card's internal server before the new one is attached to
		// it, so the two cannot both hold it and the handoff cannot leave the new agent without one.
		if !m.waitPump(old) {
			m.log.Warn("a card's old session did not finish before it was handed off", "card_id", card.ID)
		}
		m.forget(card.ID, old)
	}
	// The new agent starts fresh in the card's worktree, with the summary folded into the context the
	// card's session is given, and the internal server attached again - which, the old session having
	// just let go of it, is a new server with a new secret.
	attached := m.attach(ctx, card)
	spec := agents.StartSpec{
		Cwd: plan.path, Model: card.Model, Thinking: thinkingOrEmpty(card.Thinking),
		PermissionMode: string(card.PermissionMode),
		Instructions:   handoffInstructions(plan.summary, attached.Instructions),
		MCPServers:     attached.Servers, Label: card.ID,
	}
	startCtx, cancelStart := context.WithTimeout(ctx, resumeTimeout)
	defer cancelStart()
	handle, err := plan.agent.Start(startCtx, spec)
	if err != nil {
		// The new agent never started, so no pump will release what its session was given.
		m.detachCard(card.ID)
		return m.failHandoff(ctx, card, plan, err)
	}
	started := startedAgent{agent: plan.agent, handle: handle, view: protocol.CardViewModeChat}
	if err := m.storeHandoff(ctx, plan.row, plan.to, started); err != nil {
		m.stopUnregistered(started)
		m.detachCard(card.ID)
		return protocol.Card{}, err
	}
	// The card's own agent setting follows the session, so the card never shows one agent while its
	// session runs another. A failure here is logged rather than returned: the handoff itself has
	// happened, and the row - which is what a start or resume reads - is already right.
	if updated, err := m.projects.UpdateCard(ctx, card.ID, protocol.UpdateCardRequest{Agent: &plan.to}); err != nil {
		m.log.Warn("could not record a card's new agent after handing it off",
			"card_id", card.ID, "agent", plan.to, "error", err)
	} else {
		card = updated
	}
	working, err := m.projects.SetState(ctx, card.ID, protocol.CardStateWorking)
	if err != nil {
		m.stopUnregistered(started)
		return protocol.Card{}, err
	}
	if _, err := m.register(cardOwner(working), plan.row.ID, started); err != nil {
		m.detachCard(card.ID)
		return protocol.Card{}, err
	}
	m.noteHandoff(working, plan.row.ID, plan.to)
	m.log.Info("handed a card off to another agent", "card_id", card.ID, "session_id", plan.row.ID,
		"from", plan.row.AgentKind, "to", plan.to)
	return working, nil
}

// storeHandoff writes what a successful handoff changed in the session row: which agent the session
// runs, the new agent's own session id, its settings, and the session awake in the chat view. It is
// one write, so a restart never sees one of those without the others.
func (m *Manager) storeHandoff(ctx context.Context, row db.Session, to protocol.AgentKind, sa startedAgent) error {
	now := m.cfg.Now().UnixMilli()
	err := m.store.Write(ctx, func(q *db.Queries) error {
		_, err := q.UpdateSessionHandoff(ctx, db.UpdateSessionHandoffParams{
			AgentKind: string(to), AgentSessionID: sa.handle.ID, State: string(protocol.SessionStateAwake),
			Model: sa.handle.Model, Thinking: sa.handle.Thinking, PermissionMode: sa.handle.PermissionMode,
			LastActiveAt: now, UpdatedAt: now, ID: row.ID,
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("record that the session of card %s was handed off: %w", row.CardID, err)
	}
	return nil
}

// failHandoff records that a handoff really failed, after the old process had stopped, by the rule of
// section 5.3 (the row stops, the card needs the person). The row keeps the agent it had, so starting
// the card again continues what was there rather than the handoff that failed. The bookkeeping writes
// use a context that survives the caller's own ending: a failure is still owed a record even when it
// was noticed while shutting down.
func (m *Manager) failHandoff(ctx context.Context, card protocol.Card, plan handoffPlan, cause error) (protocol.Card, error) {
	wctx := context.WithoutCancel(ctx)
	now := m.cfg.Now()
	if err := m.store.Write(wctx, func(q *db.Queries) error {
		_, err := q.UpdateSessionRuntime(wctx, db.UpdateSessionRuntimeParams{
			State: string(protocol.SessionStateStopped), AgentSessionID: plan.row.AgentSessionID,
			LastActiveAt: plan.row.LastActiveAt, UpdatedAt: now.UnixMilli(), ID: plan.row.ID,
		})
		return err
	}); err != nil {
		m.log.Error("could not record that a card's session stopped when its handoff failed",
			"card_id", card.ID, "error", err)
	}
	moved, err := m.projects.SetState(wctx, card.ID, protocol.CardStateNeeds)
	if err != nil {
		m.log.Error("could not move a card to needs you after a failed handoff", "card_id", card.ID, "error", err)
	}
	m.publishSessionState(card.ID, plan.row.ID, protocol.SessionStateStopped, messageHandoffCannotStart)
	m.log.Warn("a card could not be handed off", "card_id", card.ID, "to", plan.to, "error", cause)
	return moved, refusedHandoff(protocol.HandoffRefusalReasonCannotStart).
		With("cardId", card.ID).With("agent", string(plan.to)).WithCause(cause)
}

// noteHandoff writes the one system message and activity row a handoff adds to a card's own history.
// It is a record of its own kind rather than the "Agent set to X" note a settings change writes,
// because that one promises the change takes effect on the next turn, and a handoff's takes effect
// at once.
func (m *Manager) noteHandoff(card protocol.Card, sessionRowID string, to protocol.AgentKind) {
	if m.cfg.History == nil {
		return
	}
	record := history.Record{
		Kind: history.KindSystem, State: history.StateOK, Summary: fmt.Sprintf(handoffNoteFormat, to),
	}
	if err := m.cfg.History.Append(m.ctx, card.ID, sessionRowID, []history.Record{record}); err != nil {
		m.log.Error("could not store a card's handoff", "card_id", card.ID, "error", err)
	}
}
