package session

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Start starts or resumes a card's session. A card that has never had one gets a worktree, a
// fresh agent process, and a session row. A card whose session is asleep, stopped, or running but
// with no process in this daemon is resumed through its saved agent session id instead, so Stop, a
// move back to Backlog, and Sleep never strand a card (docs/architecture.md 5.1, and the owner's
// decision of 2026-09-26). A card a pause holds is not restarted at all: Start releases the hold,
// which is all a paused card needs while its agent is still running. A card whose agent is already
// running and is not paused is refused.
//
// A resume that cannot happen moves the card to needs you with a plain sentence, exactly as a
// resume after a restart does (see resumeRow).
func (m *Manager) Start(ctx context.Context, cardID string) (protocol.Card, error) {
	card, err := m.projects.Card(ctx, cardID)
	if err != nil {
		return protocol.Card{}, err
	}
	if card.Paused {
		return m.startHeld(ctx, card)
	}
	if err := m.reserve(cardID); err != nil {
		return protocol.Card{}, err
	}
	defer m.release(cardID)
	return m.startOrResume(ctx, cardID)
}

// startOrResume is the shared tail of Start: resume the session row a card already has, whatever
// state that row is in, and start a fresh session only for a card that has never had one. Every
// other insert would break the row's unique card id, and every state it can be in is one a person
// can meaningfully continue. The caller holds the card's reservation.
func (m *Manager) startOrResume(ctx context.Context, cardID string) (protocol.Card, error) {
	row, err := m.store.Queries().GetSessionByCard(ctx, cardID)
	switch {
	case err == nil:
		return m.resumeRow(ctx, row)
	case !store.IsNotFound(err):
		return protocol.Card{}, fmt.Errorf("look for an existing session of card %s: %w", cardID, err)
	}
	return m.startFresh(ctx, cardID)
}

// startHeld is Start's path for a card a pause holds. The pause is cleared first. When the agent
// process never stopped, releasing the hold is all there is to do and the message the pause was
// holding is delivered; when there is no live process (a sleeping or stopped session, or one left
// over from an earlier run), the card's own session is resumed through its saved id.
func (m *Manager) startHeld(ctx context.Context, card protocol.Card) (protocol.Card, error) {
	no := false
	released, err := m.projects.SetHold(ctx, card.ID, &no, nil)
	if err != nil {
		return protocol.Card{}, err
	}
	if m.liveOf(card.ID) != nil {
		m.deliverHeld(card.ID)
		m.log.Info("released a card's pause", "card_id", card.ID)
		return released, nil
	}
	if err := m.reserve(card.ID); err != nil {
		return protocol.Card{}, err
	}
	defer m.release(card.ID)
	return m.startOrResume(ctx, card.ID)
}

// startFresh is Start's path for a card that has never had a session: it builds the worktree,
// starts the agent, and inserts the session row.
func (m *Manager) startFresh(ctx context.Context, cardID string) (protocol.Card, error) {
	card, err := m.projects.Card(ctx, cardID)
	if err != nil {
		return protocol.Card{}, err
	}
	project, err := m.projects.Get(ctx, card.ProjectID)
	if err != nil {
		return protocol.Card{}, err
	}
	path, branch, err := m.makeWorktree(ctx, project, card)
	if err != nil {
		return protocol.Card{}, err
	}
	sa, err := m.startAgent(ctx, card, path)
	if err != nil {
		m.undoWorktree(project, path, branch, cardID)
		return protocol.Card{}, err
	}
	return m.registerNewSession(ctx, card, sa)
}

// makeWorktree creates the card's worktree and branch, sparse in a monorepo, and records them on
// the card.
func (m *Manager) makeWorktree(ctx context.Context, project protocol.Project, card protocol.Card) (path, branch string, err error) {
	branch = gitx.CardBranchName(project.ID, card.Number, card.Title)
	path = filepath.Join(projects.WorktreesDir(m.cfg.DataDir, project.ID), card.ID)
	// A fork starts from the branch of the card it came from, so its work continues that card's
	// work instead of starting again from the project's default branch. An empty answer means the
	// default branch: a card that is not a fork, or a fork whose source is gone.
	base := project.DefaultBranch
	if forkBase := m.projects.ForkBase(ctx, project, card.ID); forkBase != "" {
		base = forkBase
	}
	spec := gitx.WorktreeSpec{Path: path, Branch: branch, Base: base}
	if project.IsMonorepo {
		spec.Sparse = project.Packages
	}
	if err := m.git.AddWorktree(ctx, project.Path, spec); err != nil {
		return "", "", fmt.Errorf("make a worktree for card %s: %w", card.ID, err)
	}
	if _, err := m.projects.SetWorktree(ctx, card.ID, path, branch); err != nil {
		m.undoWorktree(project, path, branch, card.ID)
		return "", "", fmt.Errorf("record the worktree of card %s: %w", card.ID, err)
	}
	return path, branch, nil
}

// undoWorktree removes a worktree that was made but must not be left behind, deletes the branch
// that was made with it, and clears both from the card. The branch is new and has no work on it
// (the agent never started), and leaving it would make the next start of this card fail because
// the branch already exists. Errors are logged: this already runs on another error's path, and
// every caller has its own error to return instead.
func (m *Manager) undoWorktree(project protocol.Project, path, branch, cardID string) {
	root := projects.WorktreesDir(m.cfg.DataDir, project.ID)
	ctx, cancel := context.WithTimeout(context.Background(), closeStopTimeout)
	defer cancel()
	if err := m.git.RemoveWorktree(ctx, project.Path, path, root, true); err != nil {
		m.log.Error("could not remove a worktree after starting the card's session failed",
			"project_id", project.ID, "card_id", cardID, "path", path, "error", err)
	}
	if err := m.git.DeleteBranch(ctx, project.Path, branch, true); err != nil {
		m.log.Error("could not delete a branch after starting the card's session failed",
			"project_id", project.ID, "card_id", cardID, "branch", branch, "error", err)
	}
	if _, err := m.projects.SetWorktree(ctx, cardID, "", ""); err != nil {
		m.log.Error("could not clear a card's worktree after starting its session failed",
			"card_id", cardID, "error", err)
	}
}

// startAgent gets a fresh agent for a card's kind and starts a session in path. A fresh agent is
// made on every call rather than one kept per kind: Phase 1 gives a card at most one session for
// its whole life, so there is nothing to share, and Registry.New already documents "every call
// makes a new one".
func (m *Manager) startAgent(ctx context.Context, card protocol.Card, path string) (startedAgent, error) {
	spec := agents.StartSpec{
		Cwd: path, Model: card.Model, Thinking: thinkingOrEmpty(card.Thinking), PermissionMode: string(card.PermissionMode),
		// Instructions (role instructions, project memory, board awareness: architecture section 7)
		// are always empty in Phase 1: the roles module (Phase 5) and the memory module (Phase 7)
		// that would fill them do not exist yet.
		Instructions: "", Label: card.ID,
	}
	return m.launch(ctx, cardOwner(card), card.Agent, spec)
}

// launch makes an agent of a kind and starts a session with the spec, for a card or for a chat.
func (m *Manager) launch(ctx context.Context, o owner, kind protocol.AgentKind, spec agents.StartSpec) (startedAgent, error) {
	agent, err := m.newAgent(o, kind)
	if err != nil {
		return startedAgent{}, err
	}
	handle, err := agent.Start(ctx, spec)
	if err != nil {
		return startedAgent{}, startFailure(o, err)
	}
	return startedAgent{agent: agent, handle: handle}, nil
}

// newAgent makes the agent for a kind, with a plain sentence for a kind that has no adapter yet.
func (m *Manager) newAgent(o owner, kind protocol.AgentKind) (agents.Agent, error) {
	agent, err := m.registry.New(kind)
	if err != nil {
		if errors.Is(err, agents.ErrUnknownKind) {
			return nil, protocol.Unsupported("Marshal does not have that agent ready to run yet.").With("agent", string(kind))
		}
		return nil, fmt.Errorf("make an agent for %s %s: %w", o.noun(), o.key(), err)
	}
	return agent, nil
}

// registerNewSession inserts a session row for a freshly started agent, moves the card to
// working, and registers the live session.
func (m *Manager) registerNewSession(ctx context.Context, card protocol.Card, sa startedAgent) (protocol.Card, error) {
	now := m.cfg.Now()
	rowID, err := protocol.NewID(now, rand.Reader)
	if err != nil {
		return protocol.Card{}, fmt.Errorf("make a session id: %w", err)
	}
	params := db.CreateCardSessionParams{
		ID: rowID, CardID: card.ID, AgentKind: string(card.Agent), AgentSessionID: sa.handle.ID,
		State: string(protocol.SessionStateAwake), Model: sa.handle.Model, Thinking: sa.handle.Thinking,
		PermissionMode: sa.handle.PermissionMode, LastActiveAt: now.UnixMilli(), CreatedAt: now.UnixMilli(), UpdatedAt: now.UnixMilli(),
	}
	if err := m.store.Write(ctx, func(q *db.Queries) error { return q.CreateCardSession(ctx, params) }); err != nil {
		return protocol.Card{}, fmt.Errorf("save the session of card %s: %w", card.ID, err)
	}
	updated, err := m.projects.SetState(ctx, card.ID, protocol.CardStateWorking)
	if err != nil {
		return protocol.Card{}, err
	}
	return m.finishRegistering(updated, rowID, sa)
}

// finishRegistering builds a card's live session, registers it, publishes that it is awake, and
// starts its pump. Shared by a fresh start and a resume.
func (m *Manager) finishRegistering(card protocol.Card, rowID string, sa startedAgent) (protocol.Card, error) {
	if _, err := m.register(cardOwner(card), rowID, sa); err != nil {
		return protocol.Card{}, err
	}
	return card, nil
}

// register builds a live session for an owner, makes it live, publishes that it is awake, and
// starts its pump. A card's session and a chat's both come through here.
func (m *Manager) register(o owner, rowID string, sa startedAgent) (*liveSession, error) {
	ls, err := m.newLiveSession(o, rowID, sa)
	if err != nil {
		// Nothing owns the agent yet, so it is ended here, or it would run with nobody to stop it.
		m.stopUnregistered(sa)
		return nil, err
	}
	if err := m.goLive(ls); err != nil {
		return nil, err
	}
	m.publishState(ls, protocol.SessionStateAwake, "")
	return ls, nil
}

// messageAgentWontStart is what a person is told when an agent program could not be started. The
// adapters have no shared error for "the program is missing or broken", so any start failure that
// is not one the API layer recognizes reads this way, and the real cause goes to the log.
const (
	messageAgentWontStart = "Marshal could not start the agent for this card. " +
		"Check that the agent is installed and that you are signed in to it, then try again."
	messageChatAgentWontStart = "Marshal could not start the agent for this chat. " +
		"Check that the agent is installed and that you are signed in to it, then try again."
)

// startFailure keeps the errors that the API layer already turns into their own sentence (a sign-in
// that is needed, a timeout) and gives every other failure to start one plain sentence.
func startFailure(o owner, err error) error {
	var signIn *agents.AuthRequiredError
	if errors.As(err, &signIn) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return fmt.Errorf("start the agent for %s %s: %w", o.noun(), o.key(), err)
	}
	message := messageAgentWontStart
	if o.isChat() {
		message = messageChatAgentWontStart
	}
	return o.about(protocol.Unavailable(message)).WithCause(err)
}
