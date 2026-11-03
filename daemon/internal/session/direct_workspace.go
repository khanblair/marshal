package session

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const directWorkspaceInstructions = "You are the main coding session for this project and work in its original folder. " +
	"You may delegate to native subagents in this same folder; give each editing worker separate file ownership and coordinate shared files. " +
	"Do not create Git worktrees or switch the project branch. Preserve existing user edits and staged files. " +
	"Do not stage or commit unrelated files. Finish by calling finish_card after checking your changes; Marshal checks the files in place."

// canonicalWorkspace treats aliases of the same folder as one editing workspace.
func canonicalWorkspace(path string) (string, error) {
	root, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("read the project's folder: %w", err)
	}
	return filepath.Abs(root)
}

func (m *Manager) cardWorkspaceRoot(ctx context.Context, cardID string) (string, error) {
	if !m.cfg.DirectWorkspace {
		return "", nil
	}
	card, err := m.projects.Card(ctx, cardID)
	if err != nil {
		return "", err
	}
	project, err := m.projects.Get(ctx, card.ProjectID)
	if err != nil {
		return "", err
	}
	return canonicalWorkspace(project.Path)
}

// The caller holds mu. A main coding session can use native subagents within its own
// ownership. Independent cards and chats must wait until that main session ends.
func (m *Manager) acquireEditorLocked(root, key string) error {
	if root == "" {
		return nil
	}
	if m.closed {
		return protocol.Unavailable("Marshal is shutting down.")
	}
	if owner := m.workspaceEditors[root]; owner != "" && owner != key {
		return protocol.Refused("Another coding session owns this project folder. Stop or sleep it before starting a different card or coding chat.").
			With("reason", "project_editor_busy").With("editorId", owner)
	}
	if m.workspaceBlocked[key] || m.workspaceStops[key] > 0 {
		return protocol.Refused("The previous agent could not be stopped. End that process before starting another coding session.").With("reason", "project_editor_stopping")
	}
	if m.workspaceEditors == nil {
		m.workspaceEditors = make(map[string]string)
	}
	m.workspaceEditors[root] = key
	return nil
}

func (m *Manager) releaseEditorLocked(key string) {
	if m.sessions[key] != nil || m.workspaceBlocked[key] || m.completing[key] || m.workspaceChats[key] || m.workspaceStops[key] > 0 {
		return
	}
	if _, pending := m.pending[key]; pending {
		return
	}
	if _, switching := m.switching[key]; switching {
		return
	}
	for root, owner := range m.workspaceEditors {
		if owner == key {
			delete(m.workspaceEditors, root)
		}
	}
}

// Keep ownership while recording the final snapshot, even when Stop closes events early.
func (m *Manager) holdStoppingEditor(ls *liveSession) func() {
	if !m.cfg.DirectWorkspace {
		return func() {}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions[ls.key()] != ls {
		return func() {}
	}
	if m.workspaceStops == nil {
		m.workspaceStops = make(map[string]int)
	}
	m.workspaceStops[ls.key()]++
	return func() {
		m.mu.Lock()
		m.workspaceStops[ls.key()]--
		m.releaseEditorLocked(ls.key())
		m.mu.Unlock()
	}
}

func (m *Manager) reserveChatEditor(chat protocol.Chat, path string) (func(), error) {
	if !m.cfg.DirectWorkspace || IsOrchestratorChat(chat) {
		return func() {}, nil
	}
	root, err := canonicalWorkspace(path)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.acquireEditorLocked(root, chat.ID); err != nil {
		return nil, err
	}
	if m.workspaceChats == nil {
		m.workspaceChats = make(map[string]bool)
	}
	m.workspaceChats[chat.ID] = true
	return func() {
		m.mu.Lock()
		delete(m.workspaceChats, chat.ID)
		m.releaseEditorLocked(chat.ID)
		m.mu.Unlock()
	}, nil
}

func completionRefusal(cardID string) *protocol.Error {
	return protocol.Refused("This card's changes are being checked. Wait for completion before sending another message or changing its agent.").
		With("reason", "card_completing").With("cardId", cardID)
}

func (m *Manager) checkCompletion(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.completing[key] {
		return completionRefusal(key)
	}
	return nil
}

// AcquireCompletion keeps the folder reserved through review, checks, and final shutdown.
// Completion may reuse its own idle worker's lease, but never another card's lease.
func (m *Manager) AcquireCompletion(ctx context.Context, cardID string) (func(), error) {
	if !m.cfg.DirectWorkspace {
		return func() {}, nil
	}
	root, err := m.cardWorkspaceRoot(ctx, cardID)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.completing[cardID] {
		return nil, completionRefusal(cardID)
	}
	if _, pending := m.pending[cardID]; pending {
		return nil, protocol.Refused("This card is still starting. Wait for its agent to be ready.")
	}
	if _, switching := m.switching[cardID]; switching {
		return nil, refusedView(protocol.ViewRefusalReasonSwitching)
	}
	if ls := m.sessions[cardID]; ls != nil && (ls.isBusy() || ls.waiting() > 0 || ls.term != nil) {
		return nil, protocol.Refused("The coding agent is still working. Wait for its turn and queued messages to finish before completing this card.")
	}
	if err := m.acquireEditorLocked(root, cardID); err != nil {
		return nil, err
	}
	if m.completing == nil {
		m.completing = make(map[string]bool)
	}
	m.completing[cardID] = true
	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			delete(m.completing, cardID)
			m.releaseEditorLocked(cardID)
			m.mu.Unlock()
		})
	}, nil
}

func (m *Manager) blockEditor(key string) {
	if !m.cfg.DirectWorkspace || key == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.workspaceBlocked == nil {
		m.workspaceBlocked = make(map[string]bool)
	}
	m.workspaceBlocked[key] = true
}

// A registered process can finish draining after a timeout. Only block its own live
// registration, so a pump that already ended cannot leave a permanent reservation behind.
func (m *Manager) blockLiveEditor(ls *liveSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions[ls.key()] != ls {
		return
	}
	if m.workspaceBlocked == nil {
		m.workspaceBlocked = make(map[string]bool)
	}
	m.workspaceBlocked[ls.key()] = true
}

func (m *Manager) checkDirectResume(ctx context.Context, card protocol.Card, path string) error {
	if !m.cfg.DirectWorkspace {
		return nil
	}
	root, err := m.cardWorkspaceRoot(ctx, card.ID)
	if err != nil {
		return err
	}
	workspace, err := canonicalWorkspace(path)
	if err != nil || workspace != root {
		return protocol.Refused("This card has earlier work in a separate worktree. Recover or finish that work before starting a new card in the project folder. Marshal has kept its files and branch.").
			With("reason", "legacy_workspace_recovery").With("cardId", card.ID).With("path", path)
	}
	if _, err := m.git.Rev(ctx, root, gitx.DirectBaseRef(card.ID)); err != nil {
		return protocol.Refused("This card has no saved starting snapshot for the project folder. Keep its existing work and start a new coding card.").With("reason", "direct_baseline_missing")
	}
	branch, err := m.git.Run(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || branch != card.Branch {
		return protocol.Refused("The project is on a different branch from this card. Return to the card's branch or start a new card for the current branch.").With("reason", "direct_branch_changed")
	}
	previous := gitx.DirectResultRef(card.ID)
	if _, err := m.git.Rev(ctx, root, previous); err != nil {
		previous = gitx.DirectBaseRef(card.ID)
	}
	priorTree, err := m.git.TreeOf(ctx, root, previous)
	if err != nil {
		return err
	}
	current, err := m.git.SnapshotFolder(ctx, root, gitx.DirectResultRef(card.ID)+"-resume-check", "Marshal: check project before resume")
	if err != nil {
		return err
	}
	if current.Tree != priorTree {
		return protocol.Refused("The project files changed since this card last ran. Start a new card for the current files so earlier work is not counted as this card's changes. This card's conversation and snapshots are kept.").With("reason", "direct_workspace_changed")
	}
	return nil
}
