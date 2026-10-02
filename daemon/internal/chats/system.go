package chats

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// A system chat is one Marshal keeps for a project itself, such as the pinned Integrator chat. It is
// made once per project and kind, listed first, and cannot be renamed, archived, or deleted. Its
// session is the long-lived agent behind the feature, so it is never made from a person's request.

const (
	// integratorTitle is the pinned Integrator chat's name.
	integratorTitle = "Integrator"
	// integratorRole is the role the Integrator chat talks to, and runs as.
	integratorRole = "Integrator"
	// integratorPermission is the mode the Integrator chat is made with. What it may run is decided
	// by its own rules at session start (internal/session/chat_spec.go), not by this word alone.
	integratorPermission = protocol.PermissionModeAutoEdits
)

// Roles is what the service reads a role's model from. internal/roles' Service satisfies it.
type Roles interface {
	Role(ctx context.Context, name, projectID string) (protocol.Role, error)
}

// EnsureSystemChat answers a project's system chat of one kind, making it when it is not there. It
// is safe to call as often as is convenient: the chat is made once, and chat.created is published
// only by the call that made it. Today the only kind is protocol.ChatSystemIntegrator.
func (s *Service) EnsureSystemChat(ctx context.Context, projectID, kind string) (protocol.Chat, error) {
	if kind != protocol.ChatSystemIntegrator {
		return protocol.Chat{}, protocol.InvalidArgument("Marshal has no system chat of that kind.")
	}
	if err := s.projectExists(ctx, projectID); err != nil {
		return protocol.Chat{}, err
	}
	defer s.locks.Lock(projectID + "/system/" + kind)()
	model := s.roleModel(ctx, projectID, integratorRole)
	made := false
	row, err := s.writeChat(ctx, func(q *db.Queries) (db.Chat, error) {
		have, err := q.GetSystemChat(ctx, db.GetSystemChatParams{ProjectID: projectID, System: kind})
		if err == nil {
			return have, nil
		}
		if !store.IsNotFound(err) {
			return db.Chat{}, err
		}
		made = true
		return s.insertChat(ctx, q, projectID, madeChat{
			title: integratorTitle, system: kind,
			target: protocol.ChatTarget{Kind: protocol.ChatTargetKindRole, ID: integratorRole},
			settings: chatSettings{
				agent: protocol.AgentKindClaude, model: model, permission: integratorPermission,
			},
		})
	})
	if err != nil {
		return protocol.Chat{}, fmt.Errorf("make the %s chat of project %s: %w", kind, projectID, err)
	}
	chat := chatOf(row)
	if made {
		s.publish(protocol.ProjectTopic(projectID), protocol.EventTypeChatCreated, protocol.ChatEventData{Chat: chat})
		s.log.Info("made a system chat", "chat_id", chat.ID, "project_id", projectID, "system", kind)
	}
	return chat, nil
}

// EnsureAllSystemChats makes every project's system chats, for the projects that were made before
// there were any. The daemon calls it once at start-up; a project made later gets its chats from its
// own creation. One project failing does not stop the others, and the errors are joined.
func (s *Service) EnsureAllSystemChats(ctx context.Context) error {
	var rows []db.Project
	err := s.store.Read(ctx, func(q *db.Queries) error {
		var err error
		rows, err = q.ListProjects(ctx)
		return err
	})
	if err != nil {
		return fmt.Errorf("list the projects to give system chats: %w", err)
	}
	var errs []error
	for _, row := range rows {
		if _, err := s.EnsureSystemChat(ctx, row.ID, protocol.ChatSystemIntegrator); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// SetRoles gives the service the roles reader after it is built, for a daemon that builds the roles
// module later. It is safe to call while the service is in use.
func (s *Service) SetRoles(roles Roles) {
	s.rolesMu.Lock()
	s.roles = roles
	s.rolesMu.Unlock()
}

// roleModel reads the model a role runs with, or "" when there is no roles reader or the role cannot
// be read: a chat with no model starts on the agent's own default rather than not at all.
func (s *Service) roleModel(ctx context.Context, projectID, name string) string {
	s.rolesMu.RLock()
	roles := s.roles
	s.rolesMu.RUnlock()
	if roles == nil {
		return ""
	}
	role, err := roles.Role(ctx, name, projectID)
	if err != nil {
		s.log.Warn("could not read a role's model for a system chat", "role", name, "error", err)
		return ""
	}
	return strings.TrimSpace(role.Spec.Model)
}

// ResetSession answers the Integrator-style "start over" for one chat: its live session ends and its
// saved agent conversation is forgotten, so the next message starts a fresh agent. The chat's
// history stays. It takes the chat's lock, so it cannot cross a message that is being sent.
func (s *Service) ResetSession(ctx context.Context, chatID string) error {
	defer s.locks.Lock(chatID)()
	if _, err := s.row(ctx, chatID); err != nil {
		return err
	}
	if s.sessions == nil {
		return nil
	}
	if err := s.sessions.ResetChatSession(ctx, chatID); err != nil {
		return fmt.Errorf("reset the session of chat %s: %w", chatID, err)
	}
	return nil
}

// refuseSystem refuses a change that a system chat does not allow, in a sentence a person can read.
// It answers nil for a chat a person made.
func refuseSystem(row db.Chat, verb string) error {
	if row.System == "" {
		return nil
	}
	return protocol.Refused(fmt.Sprintf(
		"The %s chat is pinned to this project, so it can't be %s.", row.Title, verb)).
		With("chatId", row.ID).With("system", row.System)
}

// IntegratorChat is the pinned Integrator chat as the merge resolver drives it
// (integrator.ChatSession). It is a type of its own because the service's own Send takes a chat id
// and this one takes a project id.
type IntegratorChat struct {
	svc *Service
}

// NewIntegratorChat wraps the service as the Integrator chat's live session.
func NewIntegratorChat(svc *Service) *IntegratorChat { return &IntegratorChat{svc: svc} }

// Send puts a message into the project's Integrator chat, making the chat first when it is missing.
func (c *IntegratorChat) Send(ctx context.Context, projectID, text string) error {
	chat, err := c.svc.EnsureSystemChat(ctx, projectID, protocol.ChatSystemIntegrator)
	if err != nil {
		return err
	}
	return c.svc.Send(ctx, chat.ID, text)
}

// Reset ends the Integrator's session so that the next Send starts a fresh one. The chat's history
// stays, so the owner can still read what was said.
func (c *IntegratorChat) Reset(ctx context.Context, projectID string) error {
	chat, err := c.svc.EnsureSystemChat(ctx, projectID, protocol.ChatSystemIntegrator)
	if err != nil {
		return err
	}
	return c.svc.ResetSession(ctx, chat.ID)
}
