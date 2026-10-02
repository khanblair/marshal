package session

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// What a chat's agent is started with, by the kind of chat it is (docs/architecture.md 16.2). Until
// now every chat's agent ran as a plain agent in the owner's folder. A chat now carries its role and
// its tools, and the kinds differ in where they run and what they may do:
//
//   - The Integrator chat (the system chat) runs in the Integrator's own workspace, never the
//     owner's folder, with the merge tools and a guard on Git commands that move refs.
//   - The Orchestrator chat runs in the owner's folder but cannot change it: it reads, and plans
//     through the board tools.
//   - Every other chat is as it was: the owner's folder, with the mode it was made with, and a
//     read-only view of the board.
//
// Each kind also starts with its role's instructions, which is what a card's session is given.

// ChatWorkspace makes the Integrator's own workspace, a folder outside the owner's repository on the
// integration branch. It is satisfied by internal/integrator's Workspace, and named here so this
// package does not import that one.
type ChatWorkspace interface {
	// Ensure makes the workspace when it is missing and answers its folder.
	Ensure(ctx context.Context, projectID string) (string, error)
}

// ChatAttacher gives a chat's session the internal MCP server of its kind and the instructions it
// starts with, as Attacher does for a card. Nothing is attached for a chat when none is set.
type ChatAttacher interface {
	// AttachChat builds what this chat's session is given. It is called each time the chat's agent is
	// started or resumed; the instructions it answers are sent to a fresh session only.
	AttachChat(ctx context.Context, chat protocol.Chat) (Attachment, error)
	// DetachChat gives up what AttachChat built, when the chat is deleted. A chat's server stays
	// hosted while the chat sleeps, so waking it never races the old session's own ending.
	DetachChat(ctx context.Context, chatID string)
}

// chatKit is what the manager keeps for chats beyond their rows: the Integrator's workspace maker,
// the chat attacher, and the folder each live Integrator chat runs in.
type chatKit struct {
	mu        sync.RWMutex
	workspace ChatWorkspace
	attacher  ChatAttacher
	// guards remembers where each Integrator chat's agent runs, so a permission request is held to
	// that folder. It is written when the chat's agent starts.
	guards map[string]chatGuard
}

// chatGuard is the folder a chat's agent is held to, and the project's default branch it must not
// name where it could move.
type chatGuard struct {
	worktree string
	main     string
}

// SetChatWorkspace gives the manager the Integrator's workspace maker. It is set after the manager
// is built, because that module is built after it, and is safe to call while sessions are running.
func (m *Manager) SetChatWorkspace(w ChatWorkspace) {
	m.chatKit.mu.Lock()
	m.chatKit.workspace = w
	m.chatKit.mu.Unlock()
}

// SetChatAttacher gives the manager the module that attaches the internal server and the starting
// instructions to a chat's session, for the same reason and in the same way as SetAttacher.
func (m *Manager) SetChatAttacher(a ChatAttacher) {
	m.chatKit.mu.Lock()
	m.chatKit.attacher = a
	m.chatKit.mu.Unlock()
}

// chatKind is what a chat is for, which decides where it runs and what it may do.
type chatKind int

const (
	chatKindOther chatKind = iota
	chatKindOrchestrator
	chatKindIntegrator
)

// orchestratorRole is the role the Orchestrator chat talks to.
const orchestratorRole = "Orchestrator"

// kindOfChat reads a chat's kind from the three fields that name it. The Integrator chat is the
// system chat of that name; a chat whose target is the Orchestrator, or the role of that name, is an
// Orchestrator chat; everything else is another chat.
func kindOfChat(system, targetKind, targetID string) chatKind {
	switch {
	case system == protocol.ChatSystemIntegrator:
		return chatKindIntegrator
	case system != "":
		return chatKindOther
	case protocol.ChatTargetKind(targetKind) == protocol.ChatTargetKindOrchestrator:
		return chatKindOrchestrator
	case protocol.ChatTargetKind(targetKind) == protocol.ChatTargetKindRole &&
		strings.EqualFold(strings.TrimSpace(targetID), orchestratorRole):
		return chatKindOrchestrator
	}
	return chatKindOther
}

// kindOf is kindOfChat for a chat as clients see it.
func kindOf(chat protocol.Chat) chatKind {
	return kindOfChat(chat.System, string(chat.Target.Kind), chat.Target.ID)
}

// IsIntegratorChat says whether a chat is the pinned Integrator chat.
func IsIntegratorChat(chat protocol.Chat) bool { return kindOf(chat) == chatKindIntegrator }

// IsOrchestratorChat says whether a chat is an Orchestrator chat.
func IsOrchestratorChat(chat protocol.Chat) bool { return kindOf(chat) == chatKindOrchestrator }

// chatSpec is what a chat's agent is started or resumed with. fresh is true for a session that has
// never talked, which is the only one that is sent its instructions. The Integrator's workspace is
// made here, and a chat that cannot have it is refused: it never runs in the owner's folder instead.
func (m *Manager) chatSpec(ctx context.Context, chat protocol.Chat, project protocol.Project, fresh bool) (agents.StartSpec, error) {
	kind := kindOf(chat)
	cwd := project.Path
	if kind == chatKindIntegrator {
		path, err := m.integratorWorkspace(ctx, chat, project)
		if err != nil {
			return agents.StartSpec{}, err
		}
		cwd = path
	}
	spec := agents.StartSpec{
		Cwd: cwd, Model: chat.Model, Thinking: thinkingOrEmpty(chat.Thinking),
		PermissionMode: chatLaunchMode(chat, kind), Label: chat.ID,
	}
	attached := m.attachChat(ctx, chat)
	spec.MCPServers = attached.Servers
	if fresh {
		spec.Instructions = attached.Instructions
	}
	spec.AllowedTools, spec.DisallowedTools = chatToolRules(kind, len(attached.Servers) > 0)
	return spec, nil
}

// integratorWorkspace makes the Integrator's workspace and remembers it as the folder the chat's
// agent is held to. A workspace that cannot be made, or that is the owner's own folder, refuses the
// start with a plain sentence.
func (m *Manager) integratorWorkspace(ctx context.Context, chat protocol.Chat, project protocol.Project) (string, error) {
	m.chatKit.mu.RLock()
	workspace := m.chatKit.workspace
	m.chatKit.mu.RUnlock()
	if workspace == nil {
		return "", chatOwner(chat).about(protocol.Unavailable(
			"Marshal has not set up the Integrator's workspace yet, so the Integrator chat cannot start. " +
				"Try again in a moment."))
	}
	path, err := workspace.Ensure(ctx, project.ID)
	if err != nil {
		return "", chatOwner(chat).about(protocol.Unavailable(
			"Marshal could not prepare the Integrator's workspace. Try again in a moment.")).WithCause(err)
	}
	if path == "" || filepath.Clean(path) == filepath.Clean(project.Path) {
		return "", chatOwner(chat).about(protocol.Unavailable(
			"The Integrator's workspace is the project's own folder, so the Integrator chat will not start there."))
	}
	m.chatKit.mu.Lock()
	if m.chatKit.guards == nil {
		m.chatKit.guards = make(map[string]chatGuard)
	}
	m.chatKit.guards[chat.ID] = chatGuard{worktree: filepath.Clean(path), main: project.DefaultBranch}
	m.chatKit.mu.Unlock()
	return path, nil
}

// chatGuardOf reads the folder a live Integrator chat's agent was started in.
func (m *Manager) chatGuardOf(chatID string) (chatGuard, bool) {
	m.chatKit.mu.RLock()
	defer m.chatKit.mu.RUnlock()
	guard, ok := m.chatKit.guards[chatID]
	return guard, ok
}

// attachChat builds what a chat's session is given, and answers an empty Attachment, never an error,
// when nothing is set up or the module fails: a chat without its tools still talks. The failure is
// logged, because an Integrator without its merge tools should be visible.
func (m *Manager) attachChat(ctx context.Context, chat protocol.Chat) Attachment {
	m.chatKit.mu.RLock()
	module := m.chatKit.attacher
	m.chatKit.mu.RUnlock()
	if module == nil {
		return Attachment{}
	}
	got, err := module.AttachChat(ctx, chat)
	if err != nil {
		m.log.Error("could not attach the internal server and instructions to a chat's session",
			"chat_id", chat.ID, "error", err)
		return Attachment{}
	}
	return got
}

// detachChat gives up what a chat was given, when the chat is deleted.
func (m *Manager) detachChat(chatID string) {
	m.chatKit.mu.Lock()
	module := m.chatKit.attacher
	delete(m.chatKit.guards, chatID)
	m.chatKit.mu.Unlock()
	if module != nil {
		module.DetachChat(context.WithoutCancel(m.ctx), chatID)
	}
}

// chatLaunchMode is the permission mode a chat's agent is started with. The Orchestrator's is "ask",
// not "plan": the agent's own plan mode is for writing a plan to be approved and holds back every
// action, create_card included, while "ask" lets it read and leaves each edit to a person - and
// chatDecisionMode refuses those edits for the Orchestrator outright. Every other chat is started
// with the mode it was made with.
func chatLaunchMode(chat protocol.Chat, kind chatKind) string {
	if kind == chatKindOrchestrator {
		return string(protocol.PermissionModeAsk)
	}
	return string(chat.PermissionMode)
}
