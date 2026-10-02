package mcpattach

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/integrator"
	"github.com/khanblair/marshal/daemon/internal/mcpserver"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
)

// ChatDeps is what a ChatAttacher adds to the Attacher it is built on.
type ChatDeps struct {
	// Harness reads a chat's permission rules as they are now, by chat id. It is the session
	// manager's HarnessConfigForChat.
	Harness func(chatID string) (harness.Config, bool)
	// MergeTools answers the Integrator chat's three merge tools. It is read for every call, so the
	// implementation may be set after the chat's server is built; nil answers that the tools are not
	// available.
	MergeTools func() integrator.MergeTools
}

// ChatAttacher is the session manager's ChatAttacher (internal/session/chat_spec.go): it gives a
// project chat the internal MCP server of its kind and the instructions it starts from, the way the
// Attacher does for a card. It is built on an Attacher so that the two share the host, the services
// the tools call, and the command the agent runs.
//
// A chat's server is hosted under the chat's id, and the agent reaches it through the same `mcp`
// command a card's does: the command's --card flag carries the id, and the host does not care which
// kind of id it is, because card and chat ids never collide.
type ChatAttacher struct {
	base *Attacher
	deps ChatDeps
}

// NewChatAttacher builds the chat attacher on an attacher.
func NewChatAttacher(base *Attacher, deps ChatDeps) (*ChatAttacher, error) {
	switch {
	case base == nil:
		return nil, errors.New("mcpattach: the attacher a chat attacher is built on is required")
	case deps.Harness == nil:
		return nil, errors.New("mcpattach: the chat permission rules are required")
	}
	return &ChatAttacher{base: base, deps: deps}, nil
}

// AttachChat builds this chat's server, hosts it, and answers the command the agent runs and the
// instructions a fresh session starts from. Like a card's, a daemon that cannot give a server gives
// the instructions alone.
func (c *ChatAttacher) AttachChat(ctx context.Context, chat protocol.Chat) (session.Attachment, error) {
	kind := chatKindOf(chat)
	deps := c.base.deps
	if deps.Command == "" || deps.Address == "" {
		return session.Attachment{Instructions: c.instructions(ctx, chat, kind, nil)}, nil
	}
	server, err := mcpserver.New(c.serverDeps(chat), mcpserver.Identity{
		ChatID: chat.ID, ChatKind: kind, ProjectID: chat.ProjectID, Role: chatRole(chat, kind),
	})
	if err != nil {
		return session.Attachment{}, err
	}
	secret, err := deps.Host.Add(chat.ID, server)
	if err != nil {
		return session.Attachment{}, err
	}
	return session.Attachment{
		Servers: []agents.MCPServer{{
			Name: serverName, Command: deps.Command,
			Args: []string{"mcp", "--address", deps.Address, "--card", chat.ID},
			Env:  []string{TokenEnv + "=" + secret},
		}},
		Instructions: c.instructions(ctx, chat, kind, server.ToolNames()),
	}, nil
}

// DetachChat takes the chat's server away, when the chat is deleted. It does nothing for a chat that
// was never attached.
func (c *ChatAttacher) DetachChat(_ context.Context, chatID string) {
	c.base.deps.Host.Remove(chatID)
}

// serverDeps is what one chat's server is built from.
func (c *ChatAttacher) serverDeps(chat protocol.Chat) mcpserver.Deps {
	deps := c.base.deps
	chatID := chat.ID
	return mcpserver.Deps{
		Cards: deps.Cards, Notes: deps.Notes, Claims: deps.Claims, Agents: deps.Agents,
		Codebase: deps.Codebase, MergeTools: c.deps.MergeTools,
		Harness: func() (harness.Config, bool) { return c.deps.Harness(chatID) },
		Now:     deps.Now, Logger: deps.Logger,
	}
}

// chatKindOf reads which tools a chat's server serves.
func chatKindOf(chat protocol.Chat) mcpserver.ChatKind {
	switch {
	case session.IsIntegratorChat(chat):
		return mcpserver.ChatKindIntegrator
	case session.IsOrchestratorChat(chat):
		return mcpserver.ChatKindOrchestrator
	}
	return mcpserver.ChatKindOther
}

// chatRole is the role a chat runs as, whose instructions it starts from: the Integrator chat's and
// the Orchestrator chat's own, or the role a chat was made to talk to. A chat that talks to a card
// runs as no role.
func chatRole(chat protocol.Chat, kind mcpserver.ChatKind) string {
	switch kind {
	case mcpserver.ChatKindIntegrator:
		return "Integrator"
	case mcpserver.ChatKindOrchestrator:
		return "Orchestrator"
	}
	if chat.Target.Kind == protocol.ChatTargetKindRole {
		return strings.TrimSpace(chat.Target.ID)
	}
	return ""
}

// instructions builds what a fresh chat session starts from: the role's instructions, then a short
// account of where this chat runs and what it may do, then the list of tools it has. Every part is
// best-effort, as a card's context is: a role that cannot be read is left out.
func (c *ChatAttacher) instructions(ctx context.Context, chat protocol.Chat, kind mcpserver.ChatKind, tools []string) string {
	var parts []string
	if instr := c.roleInstructions(ctx, chat, kind); instr != "" {
		parts = append(parts, instr)
	}
	parts = append(parts, chatPreface(kind))
	if len(tools) > 0 {
		parts = append(parts, fmt.Sprintf("You have Marshal's own tools: %s. Every call is checked against this "+
			"chat's permission rules, and a call that is not allowed comes back explaining why.",
			strings.Join(tools, ", ")))
	}
	return strings.Join(parts, "\n\n")
}

// roleInstructions is the chat's role's own system prompt, or empty when there is no role or it
// cannot be read.
func (c *ChatAttacher) roleInstructions(ctx context.Context, chat protocol.Chat, kind mcpserver.ChatKind) string {
	name := chatRole(chat, kind)
	if c.base.deps.Roles == nil || name == "" {
		return ""
	}
	role, err := c.base.deps.Roles.Role(ctx, name, chat.ProjectID)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(role.Spec.Instr)
}

// chatPreface says where a chat runs and what it may do, in the words its agent reads.
func chatPreface(kind mcpserver.ChatKind) string {
	switch kind {
	case mcpserver.ChatKindIntegrator:
		return "You are the Integrator for this project, in the Integrator chat. You work in your own " +
			"workspace on the integration branch, never in the owner's folder. A merge task arrives as a " +
			"message that names a task id: call merge_context with it, resolve the conflicts here, stage " +
			"them with git add, and finish with merge_report. Use ask_owner only when you cannot decide, " +
			"and still finish with merge_report. You may commit and merge in this workspace, unstage with " +
			"git restore --staged, and take one side of a conflict with git checkout --ours or --theirs. " +
			"You may not push, pull, rebase, reset, switch branches, or delete or move a branch, tag, or worktree."
	case mcpserver.ChatKindOrchestrator:
		return "You are in this project's Orchestrator chat. You can read the project and its board, but " +
			"you cannot change its files or run commands: plan the work with the owner, and when the plan " +
			"is agreed, create the cards with create_card. A card you create goes to the backlog and is not " +
			"started; the owner starts it. Name the cards a card waits for in dependsOn."
	}
	return "You are in a chat with the owner of this project. You can see the project's board with " +
		"board_status."
}
