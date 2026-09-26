package session

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// RestoreAll is called once by cmd/marshald after the store and bus are ready. In ResumeModeAuto
// it resumes every session that was starting, awake, working, or left half-woken when the daemon
// last stopped, right away. A session that was asleep stays asleep: sleep is a person's decision,
// and only a person resumes it (Wake, or Start). In ResumeModeManual it only logs how many are
// waiting; a person resumes one later with Resume. It never blocks past one resumeTimeout per row:
// the caller runs it in its own goroutine so a slow or stuck agent program never holds up the
// health endpoint.
func (m *Manager) RestoreAll(ctx context.Context) error {
	rows, err := m.store.Queries().ListResumableSessions(ctx)
	if err != nil {
		return fmt.Errorf("list the sessions to restore: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}
	if m.cfg.ResumeMode == ResumeModeManual {
		m.log.Info("sessions are waiting to be resumed", "count", len(rows))
		return nil
	}
	for _, row := range rows {
		if err := m.restoreRow(ctx, row); err != nil {
			m.log.Error("could not resume a session on restart", "card_id", row.CardID, "error", err)
		}
	}
	return nil
}

// restoreRow resumes one row for RestoreAll, reserving its card id first so it cannot race a
// concurrent Start or an explicit Resume of the same card.
func (m *Manager) restoreRow(ctx context.Context, row db.Session) error {
	if err := m.reserve(row.CardID); err != nil {
		return err
	}
	defer m.release(row.CardID)
	_, err := m.resumeRow(ctx, row)
	return err
}

// Resume resumes one card's session explicitly, for the "resume" route that recovers a session a
// restart left waiting, and for RestoreAll's own auto-mode loop. A session that has stopped is
// refused here: nothing is waiting to be recovered, and Start is the route that continues a
// stopped session.
func (m *Manager) Resume(ctx context.Context, cardID string) error {
	if err := m.reserve(cardID); err != nil {
		return err
	}
	defer m.release(cardID)
	row, err := m.store.Queries().GetSessionByCard(ctx, cardID)
	if err != nil {
		if store.IsNotFound(err) {
			return protocol.NotFound("session").With("cardId", cardID)
		}
		return fmt.Errorf("look up the session of card %s: %w", cardID, err)
	}
	if row.State == string(protocol.SessionStateStopped) {
		return protocol.Refused(messageSessionStopped).With("cardId", cardID)
	}
	_, err = m.resumeRow(ctx, row)
	return err
}

// resumeRow does the work of resuming one session row: the caller must already hold its
// reservation (see reserve). On success the card moves to working. On failure, unless ctx itself
// is what ended (the daemon shutting down while a resume was in flight, not a real resume
// failure), the card moves to needs you and the row is marked stopped, matching
// docs/architecture.md 5.3: "If resume fails, the card moves to Needs you."
func (m *Manager) resumeRow(ctx context.Context, row db.Session) (protocol.Card, error) {
	card, err := m.projects.Card(ctx, row.CardID)
	if err != nil {
		return protocol.Card{}, err
	}
	path, _, err := m.projects.Worktree(ctx, row.CardID)
	if err != nil {
		return protocol.Card{}, err
	}
	if path == "" {
		return m.failResume(ctx, row, errors.New("this card has no recorded worktree to resume in"))
	}
	// The session resumes in the view it was left in: a card that was in the terminal view when its
	// process ended, by a restart, a sleep, or a person's press, comes back in the terminal.
	view := protocol.CardViewMode(row.ViewMode)
	agent, err := m.agentFor(protocol.AgentKind(row.AgentKind), view)
	if err != nil {
		return m.failResume(ctx, row, err)
	}
	rctx, cancel := context.WithTimeout(ctx, resumeTimeout)
	defer cancel()
	handle, err := agent.Resume(rctx, row.AgentSessionID, resumeSpec(card, path))
	if err != nil {
		if ctx.Err() != nil {
			// The caller's own context ended (the daemon is shutting down, most likely), not the
			// resume itself: leave the row exactly as it is, so the next start still resumes it,
			// instead of treating a shutdown race as a failed resume.
			return protocol.Card{}, fmt.Errorf("resume the session of card %s: %w", row.CardID, err)
		}
		return m.failResume(ctx, row, err)
	}
	return m.registerResumedSession(ctx, card, row, startedAgent{agent: agent, handle: handle, view: view})
}

// resumeSpec is how a card's agent is asked to resume its session: in the card's worktree, with the
// settings the card has now.
func resumeSpec(card protocol.Card, path string) agents.StartSpec {
	return agents.StartSpec{
		Cwd: path, Model: card.Model, Thinking: thinkingOrEmpty(card.Thinking), PermissionMode: string(card.PermissionMode),
		Instructions: "", Label: card.ID,
	}
}

// failResume records that a resume really failed: the row is marked stopped (nothing is left
// running for it), the card moves to needs you, and the state change is published. A person
// restarts the card fresh later, which is a feature this task does not build (see the report).
// The bookkeeping writes use a context that survives the caller's own ending, since a failure is
// still owed a record even when it was noticed while shutting down.
func (m *Manager) failResume(ctx context.Context, row db.Session, cause error) (protocol.Card, error) {
	wctx := context.WithoutCancel(ctx)
	now := m.cfg.Now()
	if err := m.store.Write(wctx, func(q *db.Queries) error {
		_, err := q.UpdateSessionRuntime(wctx, db.UpdateSessionRuntimeParams{
			State: string(protocol.SessionStateStopped), AgentSessionID: row.AgentSessionID,
			LastActiveAt: row.LastActiveAt, UpdatedAt: now.UnixMilli(), ID: row.ID,
		})
		return err
	}); err != nil {
		m.log.Error("could not record that a session could not resume", "card_id", row.CardID, "error", err)
	}
	card, err := m.projects.SetState(wctx, row.CardID, protocol.CardStateNeeds)
	if err != nil {
		m.log.Error("could not move a card to needs you after a failed resume", "card_id", row.CardID, "error", err)
	}
	// Starting a fresh session for a card that already had one is not built yet, so the sentence
	// does not offer it.
	reason := "Marshal could not pick this session back up. The card now needs you."
	m.publishSessionState(row.CardID, row.ID, protocol.SessionStateStopped, reason)
	m.log.Warn("a session could not be resumed", "card_id", row.CardID, "error", cause)
	return card, protocol.Refused(reason).With("cardId", row.CardID).WithCause(cause)
}

// registerResumedSession records that a resume succeeded, moves the card to working, and
// registers the live session.
func (m *Manager) registerResumedSession(ctx context.Context, card protocol.Card, row db.Session, sa startedAgent) (protocol.Card, error) {
	now := m.cfg.Now()
	if err := m.store.Write(ctx, func(q *db.Queries) error {
		_, err := q.UpdateSessionRuntime(ctx, db.UpdateSessionRuntimeParams{
			State: string(protocol.SessionStateAwake), AgentSessionID: sa.handle.ID,
			LastActiveAt: now.UnixMilli(), UpdatedAt: now.UnixMilli(), ID: row.ID,
		})
		return err
	}); err != nil {
		return protocol.Card{}, fmt.Errorf("record that the session of card %s resumed: %w", card.ID, err)
	}
	updated, err := m.projects.SetState(ctx, card.ID, protocol.CardStateWorking)
	if err != nil {
		return protocol.Card{}, err
	}
	return m.finishRegistering(updated, row.ID, sa)
}
