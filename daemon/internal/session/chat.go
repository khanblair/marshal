package session

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// A project chat's own session (docs/architecture.md 16.2, backlog B2.10, slice C's message half).
//
// The chats module makes the chat and its session row together, in state `starting`, with no agent
// behind it. Nothing runs until the person says something: the first message starts the agent, in
// the project's own repository folder and with the settings the chat was made with, and every later
// message goes into the same process. A chat that is archived is put to sleep, and one that nobody
// has spoken to since the daemon started is not running, so the next message resumes it through the
// agent's saved session id. That is the same sleep, wake, and resume the cards have, with two
// differences: a chat has no worktree (an agent that talks to the Orchestrator has no branch to
// work on), and it has no card to move to "needs you" when its agent goes away.

// messageChatCannotResume is what a person reads when the agent cannot pick a chat's earlier
// conversation back up. Marshal does not start a new conversation in its place: the person would
// not know that the agent had forgotten everything (docs/architecture.md section 5.3).
const messageChatCannotResume = "Marshal could not pick this chat's conversation back up. Start a new chat to keep going."

// SendChat puts the person's message into a chat's session, starting the session on demand. The
// chat is given whole because the manager never reads the chats table (docs/architecture.md section
// 3): the chats module reads it, checks that the chat is there and not archived, and passes what
// the session starts with.
//
// A chat that is running takes the message at once, or queues it while a turn is running. A chat
// that is not running is started, or resumed through its saved session id when it has one, and the
// message follows. Messages to one chat are taken one after another, so two sent together start one
// process. The answer arrives on the chat's topic.
func (m *Manager) SendChat(ctx context.Context, chat protocol.Chat, text string) error {
	unlock := m.chatLocks.Lock(chat.ID)
	defer unlock()
	ls := m.liveOf(chat.ID)
	if ls == nil {
		if err := m.checkOpen(); err != nil {
			return err
		}
		var err error
		if ls, err = m.bringUpChat(ctx, chat); err != nil {
			return err
		}
	}
	return m.deliver(ctx, ls, text)
}

// checkOpen refuses while the daemon is shutting down, so a message that arrives then does not start
// a process only for goLive to end it. A start that is already in flight when the manager closes is
// still caught by goLive.
func (m *Manager) checkOpen() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return protocol.Unavailable("Marshal is shutting down.")
	}
	return nil
}

// chatSession reads the session row of a chat. The chat's id is a pointer in the generated query
// because the column is nullable: it is NULL on a card's session.
func (m *Manager) chatSession(ctx context.Context, chatID string) (db.Session, error) {
	return m.store.Queries().GetSessionByChat(ctx, &chatID)
}

// bringUpChat makes a chat's session live: a fresh one for a chat that has never talked, and the
// saved one, resumed, for a chat that has.
func (m *Manager) bringUpChat(ctx context.Context, chat protocol.Chat) (*liveSession, error) {
	row, err := m.chatSession(ctx, chat.ID)
	if err != nil {
		return nil, fmt.Errorf("read the session of chat %s: %w", chat.ID, err)
	}
	project, err := m.projects.Get(ctx, chat.ProjectID)
	if err != nil {
		return nil, err
	}
	spec := agents.StartSpec{
		Cwd: project.Path, Model: chat.Model, Thinking: thinkingOrEmpty(chat.Thinking),
		PermissionMode: string(chat.PermissionMode), Label: chat.ID,
	}
	if row.AgentSessionID == "" {
		return m.startChat(ctx, chat, row, spec)
	}
	return m.resumeChat(ctx, chat, row, spec)
}

// startChat starts the agent of a chat that has never talked, and records the session it made on
// the chat's row.
func (m *Manager) startChat(ctx context.Context, chat protocol.Chat, row db.Session, spec agents.StartSpec) (*liveSession, error) {
	sa, err := m.launch(ctx, chatOwner(chat), chat.AgentKind, spec)
	if err != nil {
		return nil, err
	}
	return m.registerChat(ctx, chat, row, sa)
}

// resumeChat resumes a chat's session through its saved id. A resume that cannot happen leaves the
// row stopped and answers with the sentence of messageChatCannotResume, unless it was the request's
// own context that ended (the person went away, or the daemon is stopping): that is not a failed
// resume, so the row stays as it was and the next message resumes it.
func (m *Manager) resumeChat(ctx context.Context, chat protocol.Chat, row db.Session, spec agents.StartSpec) (*liveSession, error) {
	o := chatOwner(chat)
	agent, err := m.newAgent(o, protocol.AgentKind(row.AgentKind))
	if err != nil {
		return nil, err
	}
	if protocol.SessionState(row.State) == protocol.SessionStateAsleep {
		// A message woke a sleeping chat: say so, as a card's wake does, before the resume runs.
		if err := m.setRowState(ctx, row, protocol.SessionStateWaking); err != nil {
			return nil, err
		}
		m.publishChatState(chat.ID, row.ID, protocol.SessionStateWaking, "")
	}
	rctx, cancel := context.WithTimeout(ctx, resumeTimeout)
	defer cancel()
	handle, err := agent.Resume(rctx, row.AgentSessionID, spec)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("resume the session of chat %s: %w", chat.ID, err)
		}
		return nil, m.failChatResume(ctx, chat, row, err)
	}
	return m.registerChat(ctx, chat, row, startedAgent{agent: agent, handle: handle})
}

// failChatResume records that a chat's session could not be resumed: the row reads stopped and the
// chat's topic hears it. The writes use a context that survives the caller's own ending, since a
// failure is still owed a record.
func (m *Manager) failChatResume(ctx context.Context, chat protocol.Chat, row db.Session, cause error) error {
	wctx := context.WithoutCancel(ctx)
	if err := m.setRowState(wctx, row, protocol.SessionStateStopped); err != nil {
		m.log.Error("could not record that a chat's session could not resume", "chat_id", chat.ID, "error", err)
	}
	m.publishChatState(chat.ID, row.ID, protocol.SessionStateStopped, messageChatCannotResume)
	m.log.Warn("a chat's session could not be resumed", "chat_id", chat.ID, "error", cause)
	return chatOwner(chat).about(protocol.Refused(messageChatCannotResume)).
		With("reason", string(protocol.ChatRefusalReasonCannotResume)).WithCause(cause)
}

// registerChat records the agent session a chat now has on its row, and makes the session live.
func (m *Manager) registerChat(ctx context.Context, chat protocol.Chat, row db.Session, sa startedAgent) (*liveSession, error) {
	now := m.cfg.Now()
	err := m.store.Write(ctx, func(q *db.Queries) error {
		_, err := q.UpdateSessionRuntime(ctx, db.UpdateSessionRuntimeParams{
			State: string(protocol.SessionStateAwake), AgentSessionID: sa.handle.ID,
			LastActiveAt: now.UnixMilli(), UpdatedAt: now.UnixMilli(), ID: row.ID,
		})
		return err
	})
	if err != nil {
		m.stopUnregistered(sa)
		return nil, fmt.Errorf("record the session of chat %s: %w", chat.ID, err)
	}
	ls, err := m.register(chatOwner(chat), row.ID, sa)
	if err != nil {
		// The agent was ended by whoever refused it (the daemon is shutting down), so the row must
		// not go on saying that a process runs. It keeps the agent's session id, and the chat's next
		// message resumes it.
		row.AgentSessionID = sa.handle.ID
		if writeErr := m.setRowState(context.WithoutCancel(ctx), row, protocol.SessionStateAsleep); writeErr != nil {
			m.log.Error("could not record that a chat's session never went live", "chat_id", chat.ID, "error", writeErr)
		}
		return nil, err
	}
	return ls, nil
}

// stopUnregistered ends an agent that started but could not be made a live session, so no process
// is left behind. A live session does this for itself; this is for the moment before it is one.
func (m *Manager) stopUnregistered(sa startedAgent) {
	ctx, cancel := context.WithTimeout(context.Background(), closeStopTimeout)
	defer cancel()
	if err := sa.agent.Stop(ctx, sa.handle); err != nil {
		m.log.Error("could not stop an agent that never became a session", "error", err)
	}
}

// StopChatSession puts a chat's session to sleep: the agent process ends, and the row keeps the
// agent's session id and reads asleep, so the chat's next message resumes the same conversation.
// The chats module calls it when a chat is archived, and before it deletes one. A chat with no
// process running has nothing to end, and one that never talked has no conversation to keep, so
// neither is an error.
func (m *Manager) StopChatSession(ctx context.Context, chatID string) error {
	unlock := m.chatLocks.Lock(chatID)
	defer unlock()
	if ls := m.liveOf(chatID); ls != nil {
		if err := m.endChatProcess(ctx, ls); err != nil {
			return err
		}
	}
	row, err := m.chatSession(ctx, chatID)
	if err != nil {
		if store.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("read the session of chat %s: %w", chatID, err)
	}
	if !canBePutToSleep(row) {
		return nil
	}
	if err := m.setRowState(ctx, row, protocol.SessionStateAsleep); err != nil {
		return err
	}
	m.publishChatState(chatID, row.ID, protocol.SessionStateAsleep, "")
	m.log.Info("put a chat's session to sleep", "chat_id", chatID, "session_id", row.ID)
	return nil
}

// canBePutToSleep says whether a chat's session row should be written as asleep when the chat is put
// away: it has a conversation to keep (an agent session id), and is not already asleep or stopped.
// A row that never started stays as it is.
func canBePutToSleep(row db.Session) bool {
	if row.AgentSessionID == "" {
		return false
	}
	switch protocol.SessionState(row.State) {
	case protocol.SessionStateAsleep, protocol.SessionStateStopped:
		return false
	}
	return true
}

// endChatProcess stops the agent process of a live chat session, without ending the conversation:
// the session is forgotten, and its exit is marked as expected so the pump does not treat it as a
// crash (see finishPump).
func (m *Manager) endChatProcess(ctx context.Context, ls *liveSession) error {
	ls.setStopRequested()
	if err := ls.agent.Stop(ctx, ls.handle); err != nil {
		// The process is still there, so the chat did not go to sleep after all.
		ls.clearStopRequested()
		return fmt.Errorf("stop the session of chat %s: %w", ls.chatID, err)
	}
	m.forget(ls.key(), ls)
	return nil
}

// RemoveChatLogs deletes the on-disk log folder of a chat's session, when the chat is deleted. The
// session manager owns where those folders live, so the chats module asks it rather than building
// the path itself. A chat that never had a session has nothing to remove, and a missing folder is
// not an error.
func (m *Manager) RemoveChatLogs(ctx context.Context, chatID string) error {
	row, err := m.chatSession(ctx, chatID)
	if err != nil {
		if store.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("read the session of chat %s: %w", chatID, err)
	}
	if err := os.RemoveAll(sessionLogDir(m.cfg.DataDir, row.ID)); err != nil {
		return fmt.Errorf("remove the log folder of chat %s: %w", chatID, err)
	}
	m.log.Info("removed a chat's session logs", "chat_id", chatID, "session_id", row.ID)
	return nil
}

// stopProjectChats puts every live chat session of a project to sleep, when the project is removed.
func (m *Manager) stopProjectChats(ctx context.Context, projectID string) error {
	var errs []error
	for _, chatID := range m.liveChatsOf(projectID) {
		if err := m.StopChatSession(ctx, chatID); err != nil {
			errs = append(errs, fmt.Errorf("stop chat %s: %w", chatID, err))
		}
	}
	return errors.Join(errs...)
}
