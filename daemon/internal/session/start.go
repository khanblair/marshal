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

// Start starts a card's session: it makes the worktree, starts the agent process, and persists the
// session row. A card that already has a live session in this process is refused. A card whose
// sessions row exists but is not live (a previous crash left it behind, since a card has at most
// one session row for its whole life) is resumed instead of refused, because a second insert would
// violate the row's unique card id.
func (m *Manager) Start(ctx context.Context, cardID string) (protocol.Card, error) {
	if err := m.reserve(cardID); err != nil {
		return protocol.Card{}, err
	}
	defer m.release(cardID)

	row, err := m.store.Queries().GetSessionByCard(ctx, cardID)
	switch {
	case err == nil && row.State == string(protocol.SessionStateStopped):
		return protocol.Card{}, protocol.Refused(
			"This card already had a session, and it has stopped. Starting it again is not supported yet.").
			With("cardId", cardID)
	case err == nil:
		return m.resumeRow(ctx, row)
	case !store.IsNotFound(err):
		return protocol.Card{}, fmt.Errorf("look for an existing session of card %s: %w", cardID, err)
	}
	return m.startFresh(ctx, cardID)
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
	spec := gitx.WorktreeSpec{Path: path, Branch: branch, Base: project.DefaultBranch}
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
	agent, err := m.registry.New(card.Agent)
	if err != nil {
		if errors.Is(err, agents.ErrUnknownKind) {
			return startedAgent{},
				protocol.Unsupported("Marshal does not have that agent ready to run yet.").With("agent", string(card.Agent))
		}
		return startedAgent{}, fmt.Errorf("make an agent for card %s: %w", card.ID, err)
	}
	spec := agents.StartSpec{
		Cwd: path, Model: card.Model, Thinking: thinkingOrEmpty(card.Thinking), PermissionMode: string(card.PermissionMode),
		// Instructions (role instructions, project memory, board awareness: architecture section 7)
		// are always empty in Phase 1: the roles module (Phase 5) and the memory module (Phase 7)
		// that would fill them do not exist yet.
		Instructions: "", Label: card.ID,
	}
	handle, err := agent.Start(ctx, spec)
	if err != nil {
		return startedAgent{}, startFailure(card.ID, err)
	}
	return startedAgent{agent: agent, handle: handle}, nil
}

// registerNewSession inserts a session row for a freshly started agent, moves the card to
// working, and registers the live session.
func (m *Manager) registerNewSession(ctx context.Context, card protocol.Card, sa startedAgent) (protocol.Card, error) {
	now := m.cfg.Now()
	rowID, err := protocol.NewID(now, rand.Reader)
	if err != nil {
		return protocol.Card{}, fmt.Errorf("make a session id: %w", err)
	}
	params := db.CreateSessionParams{
		ID: rowID, CardID: card.ID, AgentKind: string(card.Agent), AgentSessionID: sa.handle.ID,
		State: string(protocol.SessionStateAwake), Model: sa.handle.Model, Thinking: sa.handle.Thinking,
		PermissionMode: sa.handle.PermissionMode, LastActiveAt: now.UnixMilli(), CreatedAt: now.UnixMilli(), UpdatedAt: now.UnixMilli(),
	}
	if err := m.store.Write(ctx, func(q *db.Queries) error { return q.CreateSession(ctx, params) }); err != nil {
		return protocol.Card{}, fmt.Errorf("save the session of card %s: %w", card.ID, err)
	}
	updated, err := m.projects.SetState(ctx, card.ID, protocol.CardStateWorking)
	if err != nil {
		return protocol.Card{}, err
	}
	return m.finishRegistering(updated, rowID, sa)
}

// finishRegistering builds a live session, registers it, publishes that it is awake, and starts
// its pump. Shared by a fresh start and a resume.
func (m *Manager) finishRegistering(card protocol.Card, rowID string, sa startedAgent) (protocol.Card, error) {
	ls, err := m.newLiveSession(card, rowID, sa)
	if err != nil {
		return protocol.Card{}, err
	}
	if err := m.goLive(ls); err != nil {
		return protocol.Card{}, err
	}
	m.publishState(ls, protocol.SessionStateAwake, "")
	return card, nil
}

// messageAgentWontStart is what a person is told when an agent program could not be started. The
// adapters have no shared error for "the program is missing or broken", so any start failure that
// is not one the API layer recognizes reads this way, and the real cause goes to the log.
const messageAgentWontStart = "Marshal could not start the agent for this card. " +
	"Check that the agent is installed and that you are signed in to it, then try again."

// startFailure keeps the errors that the API layer already turns into their own sentence (a sign-in
// that is needed, a timeout) and gives every other failure to start one plain sentence.
func startFailure(cardID string, err error) error {
	var signIn *agents.AuthRequiredError
	if errors.As(err, &signIn) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return fmt.Errorf("start the agent for card %s: %w", cardID, err)
	}
	return protocol.Unavailable(messageAgentWontStart).With("cardId", cardID).WithCause(err)
}
