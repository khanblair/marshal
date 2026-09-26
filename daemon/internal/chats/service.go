// Package chats owns a project's chats (docs/architecture.md 16.2, docs/backend-checklist.md
// B2.10, inventory N13): making one, listing one project's, renaming one, archiving and restoring
// one, and deleting one. It is the layer between the HTTP routes and the stored chats: the routes
// read the request and write the answer, and this package decides which rules a request has to
// meet, mints the ids, and publishes the `chat.*` events the client follows.
//
// A chat keeps its own session. The service makes that session's row in the same transaction as the
// chat, in state `starting`; nothing runs until the person sends the chat a message, and the session
// manager starts the agent then. What the service needs from the session manager is in the Sessions
// interface below, and it is optional: a service built without one still serves every route except
// sending a message, and only skips stopping a live session when a chat is archived or deleted.
package chats

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/khanblair/marshal/daemon/internal/keyedlock"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

const (
	// NewChatTitle is what a chat is called until its first message names it. A person can rename a
	// chat at any time. A cheap model writes the title from the first message in Phase 4 (B4.7);
	// until then the daemon writes it from the first words (see titleFromMessage), the way the app's
	// own chat did (apps/web/src/mock/actions/chats.ts).
	NewChatTitle = "New chat"
	// titleWords is how many words of the first message make its title.
	titleWords = 6
	// maxTitleChars is the longest name a chat can have.
	maxTitleChars = 100
	// maxRoleChars is the longest role name a chat can talk to. Role names are the app's own short
	// list (Worker, Tester, Integrator, Orchestrator), so this only stops nonsense.
	maxRoleChars = 60
	// defaultAgentKind and defaultPermissionMode are what a chat gets when the request says
	// nothing: the same defaults a card has (0002_projects.sql).
	defaultAgentKind      = protocol.AgentKindClaude
	defaultPermissionMode = protocol.PermissionModeAutoEdits
)

// Events is the part of the event bus this module uses. cmd/marshald gives it the real bus.
type Events interface {
	Publish(topic, eventType string, data any, critical bool) uint64
}

// Sessions is what a chat needs from the session manager: a chat's own session is a session the way
// a card's is, so the manager owns it rather than this module growing a second one. It is the whole
// of the interface on purpose. The manager never reads the chats table, so this module hands it the
// chat whole when there is a message to send.
type Sessions interface {
	// SendChat puts a message into the chat's session, starting the session when it is not running
	// and resuming it when it has slept. The answer arrives on the chat's topic.
	SendChat(ctx context.Context, chat protocol.Chat, text string) error
	// StopChatSession puts a chat's session to sleep when it is running, and keeps its conversation
	// so the next message resumes it. A chat that has no session running in this process is not an
	// error.
	StopChatSession(ctx context.Context, chatID string) error
	// RemoveChatLogs deletes the on-disk log folder of a chat's session. A chat that never had a
	// session has nothing to remove, and a missing folder is not an error.
	RemoveChatLogs(ctx context.Context, chatID string) error
}

// Deps are the parts the service is built from.
type Deps struct {
	// Store is the open database. It is both the chats and their sessions.
	Store *store.Store
	// Bus publishes chat.created, chat.updated, chat.archived, and chat.deleted.
	Bus Events
	// Sessions sends to a chat's session, stops it, and removes its logs. Optional: without it a chat
	// is still made, listed, renamed, archived, restored, and deleted, and only the live half is
	// skipped. A message cannot be sent without it.
	Sessions Sessions
}

// Service owns a project's chats. It is safe for use by many goroutines.
type Service struct {
	store    *store.Store
	bus      Events
	sessions Sessions
	log      *slog.Logger
	now      func() time.Time

	// locks serializes what changes a chat's session: a message, an archive, a restore, and a
	// delete of one chat follow one another. Without it a message that has passed the archived
	// check could reach the session manager after the archive or the delete had stopped the session,
	// and start an agent for a chat that is put away or gone. The manager never calls back into this
	// module, so taking this lock before the manager's own cannot deadlock.
	locks keyedlock.Locks
}

// Option changes how New builds a Service.
type Option func(*Service)

// WithClock sets the clock that stamps a chat and its session. The default is time.Now.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// WithLogger sets the logger.
func WithLogger(log *slog.Logger) Option {
	return func(s *Service) {
		if log != nil {
			s.log = log
		}
	}
}

// New builds the service. The store and the bus are both required: every change to a chat is
// published, and a service that could not publish would leave every client's list stale.
func New(deps Deps, opts ...Option) (*Service, error) {
	if deps.Store == nil || deps.Bus == nil {
		return nil, fmt.Errorf("chats: a store and the event bus are both required")
	}
	s := &Service{
		store: deps.Store, bus: deps.Bus, sessions: deps.Sessions,
		log: slog.New(slog.DiscardHandler), now: time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// List answers GET /v1/projects/{id}/chats. `archived` picks which half of the list is wanted: the
// live chats of the main list, or the archived ones behind the Archived toggle. An unknown project
// is not found rather than an empty list.
func (s *Service) List(ctx context.Context, projectID string, archived bool) (protocol.ChatListSnapshot, error) {
	if err := s.projectExists(ctx, projectID); err != nil {
		return protocol.ChatListSnapshot{}, err
	}
	var rows []db.Chat
	err := s.store.Read(ctx, func(q *db.Queries) error {
		var err error
		if archived {
			rows, err = q.ListArchivedChatsByProject(ctx, projectID)
		} else {
			rows, err = q.ListChatsByProject(ctx, projectID)
		}
		return err
	})
	if err != nil {
		return protocol.ChatListSnapshot{}, fmt.Errorf("list the chats of project %s: %w", projectID, err)
	}
	chats := make([]protocol.Chat, len(rows))
	for i, row := range rows {
		chats[i] = chatOf(row)
	}
	return protocol.ChatListSnapshot{
		ProjectID: projectID, Chats: chats, ServerTime: protocol.NewTimestamp(s.now()),
	}, nil
}

// Chat answers a read of one chat, for the routes that need the chat they just changed. An unknown
// chat is not found.
func (s *Service) Chat(ctx context.Context, chatID string) (protocol.Chat, error) {
	row, err := s.row(ctx, chatID)
	if err != nil {
		return protocol.Chat{}, err
	}
	return chatOf(row), nil
}

// Create answers POST /v1/projects/{id}/chats. The chat and its session row are written in one
// transaction, so a chat never exists without the session it keeps. chat.created is published
// after the commit.
func (s *Service) Create(ctx context.Context, projectID string, in protocol.CreateChatRequest) (protocol.Chat, error) {
	if err := s.projectExists(ctx, projectID); err != nil {
		return protocol.Chat{}, err
	}
	title, err := checkTitle(in.Title, true)
	if err != nil {
		return protocol.Chat{}, err
	}
	target, err := checkTarget(in.Target)
	if err != nil {
		return protocol.Chat{}, err
	}
	settings, err := checkSettings(in)
	if err != nil {
		return protocol.Chat{}, err
	}
	now := s.now()
	chatID, err := s.newID(now)
	if err != nil {
		return protocol.Chat{}, err
	}
	sessionID, err := s.newID(now)
	if err != nil {
		return protocol.Chat{}, err
	}
	millis := now.UnixMilli()
	params := db.CreateChatParams{
		ID: chatID, ProjectID: projectID, Title: title,
		TargetKind: string(target.Kind), TargetID: target.ID,
		AgentKind: string(settings.agent), Model: settings.model, Thinking: settings.thinking,
		PermissionMode: string(settings.permission),
		CreatedAt:      millis, UpdatedAt: millis, LastActiveAt: millis,
	}
	// The session's own settings come from the agent the session manager starts, which is only
	// known once it starts; the chat carries what the session row starts with.
	sessionParams := db.CreateChatSessionParams{
		ID: sessionID, ChatID: &chatID, AgentKind: string(settings.agent), State: string(protocol.SessionStateStarting),
		Model: settings.model, Thinking: settings.thinking, PermissionMode: string(settings.permission),
		LastActiveAt: millis, CreatedAt: millis, UpdatedAt: millis,
	}
	row, err := s.writeChat(ctx, func(q *db.Queries) (db.Chat, error) {
		if err := q.CreateChat(ctx, params); err != nil {
			return db.Chat{}, err
		}
		if err := q.CreateChatSession(ctx, sessionParams); err != nil {
			return db.Chat{}, err
		}
		return q.GetChat(ctx, chatID)
	})
	if err != nil {
		return protocol.Chat{}, err
	}
	chat := chatOf(row)
	s.publish(protocol.ProjectTopic(projectID), protocol.EventTypeChatCreated, protocol.ChatEventData{Chat: chat})
	s.log.Info("made a project chat", "chat_id", chat.ID, "project_id", projectID, "session_id", sessionID)
	return chat, nil
}

// Update answers PATCH /v1/chats/{id}: rename a chat. A field that is not set is left alone, and a
// name that is empty or only spaces is refused. chat.updated is published after the commit.
func (s *Service) Update(ctx context.Context, chatID string, in protocol.UpdateChatRequest) (protocol.Chat, error) {
	row, err := s.row(ctx, chatID)
	if err != nil {
		return protocol.Chat{}, err
	}
	if in.Title == nil {
		return chatOf(row), nil
	}
	title, err := checkTitle(*in.Title, false)
	if err != nil {
		return protocol.Chat{}, err
	}
	now := s.now().UnixMilli()
	updated, err := s.writeChat(ctx, func(q *db.Queries) (db.Chat, error) {
		if _, err := q.RenameChat(ctx, db.RenameChatParams{Title: title, UpdatedAt: now, ID: chatID}); err != nil {
			return db.Chat{}, err
		}
		return q.GetChat(ctx, chatID)
	})
	if err != nil {
		return protocol.Chat{}, err
	}
	chat := chatOf(updated)
	s.publish(protocol.ProjectTopic(chat.ProjectID), protocol.EventTypeChatUpdated, protocol.ChatEventData{Chat: chat})
	return chat, nil
}

// Archive answers POST /v1/chats/{id}/archive: the chat leaves the main list, and its session is
// put to sleep (architecture.md 16.2). A chat that is already archived keeps the moment it was
// archived, so archiving twice does not move it in the archived list. chat.archived is published
// after the commit.
func (s *Service) Archive(ctx context.Context, chatID string) (protocol.Chat, error) {
	defer s.locks.Lock(chatID)()
	row, err := s.row(ctx, chatID)
	if err != nil {
		return protocol.Chat{}, err
	}
	now := s.now()
	if row.ArchivedAt == nil {
		if err := s.stopSession(ctx, chatID); err != nil {
			return protocol.Chat{}, err
		}
		millis := now.UnixMilli()
		row, err = s.writeChat(ctx, func(q *db.Queries) (db.Chat, error) {
			if _, err := q.ArchiveChat(ctx, db.ArchiveChatParams{ArchivedAt: &millis, UpdatedAt: millis, ID: chatID}); err != nil {
				return db.Chat{}, err
			}
			return q.GetChat(ctx, chatID)
		})
		if err != nil {
			return protocol.Chat{}, err
		}
	}
	chat := chatOf(row)
	s.publish(protocol.ProjectTopic(chat.ProjectID), protocol.EventTypeChatArchived, protocol.ChatArchivedEventData{Chat: chat})
	return chat, nil
}

// Restore answers POST /v1/chats/{id}/restore: the chat comes back to the main list. Its session
// stays asleep until the chat's next message wakes it, the way a card's does (architecture.md
// 16.2). A chat that is not archived is left as it is. chat.archived is published after the
// commit: it carries the chat as it is now, so one event type serves both directions.
func (s *Service) Restore(ctx context.Context, chatID string) (protocol.Chat, error) {
	defer s.locks.Lock(chatID)()
	row, err := s.row(ctx, chatID)
	if err != nil {
		return protocol.Chat{}, err
	}
	if row.ArchivedAt != nil {
		millis := s.now().UnixMilli()
		row, err = s.writeChat(ctx, func(q *db.Queries) (db.Chat, error) {
			if _, err := q.RestoreChat(ctx, db.RestoreChatParams{UpdatedAt: millis, ID: chatID}); err != nil {
				return db.Chat{}, err
			}
			return q.GetChat(ctx, chatID)
		})
		if err != nil {
			return protocol.Chat{}, err
		}
	}
	chat := chatOf(row)
	s.publish(protocol.ProjectTopic(chat.ProjectID), protocol.EventTypeChatArchived, protocol.ChatArchivedEventData{Chat: chat})
	return chat, nil
}

// Remove answers DELETE /v1/chats/{id}: the chat's session is stopped, its logs are deleted, and
// the chat goes, with its messages. Cards the chat made stay on the board (architecture.md 16.2).
// chat.deleted is published after the commit.
func (s *Service) Remove(ctx context.Context, chatID string) error {
	defer s.locks.Lock(chatID)()
	row, err := s.row(ctx, chatID)
	if err != nil {
		return err
	}
	if err := s.stopSession(ctx, chatID); err != nil {
		return err
	}
	if s.sessions != nil {
		if err := s.sessions.RemoveChatLogs(ctx, chatID); err != nil {
			return fmt.Errorf("remove the logs of chat %s: %w", chatID, err)
		}
	}
	if err := s.store.Write(ctx, func(q *db.Queries) error {
		if _, err := q.DeleteChat(ctx, chatID); err != nil {
			return fmt.Errorf("delete chat %s: %w", chatID, err)
		}
		return nil
	}); err != nil {
		return err
	}
	s.publish(protocol.ProjectTopic(row.ProjectID), protocol.EventTypeChatDeleted, protocol.ChatDeletedEventData{
		ChatID: chatID, ProjectID: row.ProjectID,
	})
	s.log.Info("deleted a project chat", "chat_id", chatID, "project_id", row.ProjectID)
	return nil
}

// Send answers POST /v1/chats/{id}/messages: the person's message goes into the chat's own session,
// which is started by it when it is not running. A chat that is not there is not found, and one that
// is archived is refused with its own sentence and reason, because an archived chat is put away and
// must be restored before it talks. Once the session has taken the message, the chat moves to the
// top of its project's list and, if it is still called "New chat", is named after the first words of
// the message; chat.updated is published with the chat as it now is.
func (s *Service) Send(ctx context.Context, chatID, text string) error {
	defer s.locks.Lock(chatID)()
	row, err := s.row(ctx, chatID)
	if err != nil {
		return err
	}
	if row.ArchivedAt != nil {
		return protocol.Refused("This chat is archived. Restore it to keep talking.").
			With("reason", string(protocol.ChatRefusalReasonArchived)).With("chatId", chatID)
	}
	if s.sessions == nil {
		return protocol.Unavailable("Marshal cannot start this chat's agent right now. Try again in a moment.").
			With("chatId", chatID)
	}
	if err := s.sessions.SendChat(ctx, chatOf(row), text); err != nil {
		return err
	}
	s.noteMessage(ctx, row, text)
	return nil
}

// noteMessage records that a chat has just been sent a message: it moves to the top of the list, and
// a chat that is still called "New chat" takes its name from the message. The message is already in
// the session, so a failure here is logged and not returned: the person's words were delivered, and
// the list catches up with the next event or load.
func (s *Service) noteMessage(ctx context.Context, row db.Chat, text string) {
	millis := s.now().UnixMilli()
	title := row.Title
	if row.Title == NewChatTitle {
		if named := titleFromMessage(text); named != "" {
			title = named
		}
	}
	updated, err := s.writeChat(ctx, func(q *db.Queries) (db.Chat, error) {
		if title != row.Title {
			if _, err := q.RenameChat(ctx, db.RenameChatParams{Title: title, UpdatedAt: millis, ID: row.ID}); err != nil {
				return db.Chat{}, err
			}
		}
		if _, err := q.TouchChat(ctx, db.TouchChatParams{LastActiveAt: millis, UpdatedAt: millis, ID: row.ID}); err != nil {
			return db.Chat{}, err
		}
		return q.GetChat(ctx, row.ID)
	})
	if err != nil {
		s.log.Error("could not record a message on its chat", "chat_id", row.ID, "error", err)
		return
	}
	chat := chatOf(updated)
	s.publish(protocol.ProjectTopic(chat.ProjectID), protocol.EventTypeChatUpdated, protocol.ChatEventData{Chat: chat})
}

// titleFromMessage names a chat after the first words of its first message, as the app's own chat
// did: the first six words, with the punctuation that ends a sentence taken off the end, and the
// first letter in capitals. A message with no words in it gives no title, and the chat keeps the
// name it has. The name is cut to the longest a chat can have.
func titleFromMessage(text string) string {
	words := strings.Fields(strings.TrimRight(strings.TrimSpace(text), "?.!"))
	if len(words) == 0 {
		return ""
	}
	title := strings.Join(words[:min(len(words), titleWords)], " ")
	runes := []rune(title)
	if len(runes) > maxTitleChars {
		runes = runes[:maxTitleChars]
	}
	runes[0] = unicode.ToUpper(runes[0])
	return strings.TrimSpace(string(runes))
}

// chatSettings are the agent settings a new chat starts with.
type chatSettings struct {
	agent      protocol.AgentKind
	model      string
	thinking   string
	permission protocol.PermissionMode
}

// checkSettings reads the agent settings of a new chat, refusing one that is not a known word, and
// filling in the defaults the database would otherwise apply.
func checkSettings(in protocol.CreateChatRequest) (chatSettings, error) {
	settings := chatSettings{
		agent: defaultAgentKind, model: strings.TrimSpace(in.Model),
		permission: defaultPermissionMode,
	}
	if in.AgentKind != "" {
		if !in.AgentKind.Valid() {
			return chatSettings{}, protocol.InvalidArgument(
				"That is not an agent Marshal knows. Choose one of: " + joinStrings(protocol.AgentKindValues()) + ".")
		}
		settings.agent = in.AgentKind
	}
	if in.PermissionMode != "" {
		if !in.PermissionMode.Valid() {
			return chatSettings{}, protocol.InvalidArgument(
				"That is not a permission mode. Choose one of: " + joinStrings(protocol.PermissionModeValues()) + ".")
		}
		settings.permission = in.PermissionMode
	}
	if in.Thinking != nil {
		if !in.Thinking.Valid() {
			return chatSettings{}, protocol.InvalidArgument(
				"That is not a thinking mode. Choose one of: " + joinStrings(protocol.ThinkingModeValues()) + ".")
		}
		settings.thinking = string(*in.Thinking)
	}
	return settings, nil
}

// checkTarget reads who a chat talks to. A request with no target means the Orchestrator, which is
// what the app opens a new chat with. A role needs a name and a card needs an opaque id; an
// Orchestrator has no id, and one that carries an id anyway is refused rather than quietly ignored.
func checkTarget(target *protocol.ChatTarget) (protocol.ChatTarget, error) {
	if target == nil {
		return protocol.ChatTarget{Kind: protocol.ChatTargetKindOrchestrator}, nil
	}
	wanted := *target
	if wanted.Kind == "" {
		wanted.Kind = protocol.ChatTargetKindOrchestrator
	}
	if !wanted.Kind.Valid() {
		return protocol.ChatTarget{}, protocol.InvalidArgument(
			"That is not something a chat can talk to. Choose one of: " + joinStrings(protocol.ChatTargetKindValues()) + ".")
	}
	switch wanted.Kind {
	case protocol.ChatTargetKindOrchestrator:
		if wanted.ID != "" {
			return protocol.ChatTarget{}, protocol.InvalidArgument(
				"The Orchestrator has no id. Leave the target id out.")
		}
	case protocol.ChatTargetKindRole:
		name := strings.TrimSpace(wanted.ID)
		if name == "" {
			return protocol.ChatTarget{}, protocol.InvalidArgument("Choose the role this chat talks to.")
		}
		if utf8.RuneCountInString(name) > maxRoleChars {
			return protocol.ChatTarget{}, protocol.InvalidArgument(
				fmt.Sprintf("Role names can have at most %d characters.", maxRoleChars))
		}
		wanted.ID = name
	case protocol.ChatTargetKindCard:
		if !protocol.ValidID(wanted.ID) {
			return protocol.ChatTarget{}, protocol.InvalidArgument("That is not a card id.")
		}
	}
	return wanted, nil
}

// checkTitle reads a chat's name. A request that says nothing gets "New chat"; a name that is empty
// or only spaces is refused, because a chat with no name is not one the list can draw. A name is
// kept as it was typed apart from the spaces around it: people paste, and their spacing matters.
func checkTitle(title string, mayBeEmpty bool) (string, error) {
	// Empty means "use the default" (Create) or "leave it alone" (Update, which never reaches here
	// with an empty title: Update returns early when in.Title is nil). A title that is not empty but
	// trims to nothing (spaces only) is given, and refused, either way: protocol.CreateChatRequest's
	// own doc says so.
	if title == "" && mayBeEmpty {
		return NewChatTitle, nil
	}
	if strings.TrimSpace(title) == "" {
		return "", protocol.InvalidArgument("Chat names can't be empty. The old name is kept.")
	}
	trimmed := strings.TrimSpace(title)
	if utf8.RuneCountInString(trimmed) > maxTitleChars {
		return "", protocol.InvalidArgument(
			fmt.Sprintf("Chat names can have at most %d characters.", maxTitleChars))
	}
	return trimmed, nil
}

// row reads one chat. An unknown chat is not found, which is also the answer for an id that can
// never exist.
func (s *Service) row(ctx context.Context, chatID string) (db.Chat, error) {
	var row db.Chat
	err := s.store.Read(ctx, func(q *db.Queries) error {
		var err error
		row, err = q.GetChat(ctx, chatID)
		return err
	})
	switch {
	case err == nil:
		return row, nil
	case store.IsNotFound(err):
		return db.Chat{}, notFoundChat(chatID)
	default:
		return db.Chat{}, fmt.Errorf("read chat %s: %w", chatID, err)
	}
}

// writeChat runs one write and reads the chat back inside the same transaction, so the answer is
// exactly the row the commit left behind.
func (s *Service) writeChat(ctx context.Context, fn func(*db.Queries) (db.Chat, error)) (db.Chat, error) {
	var row db.Chat
	err := s.store.Write(ctx, func(q *db.Queries) error {
		var err error
		row, err = fn(q)
		return err
	})
	if err != nil {
		return db.Chat{}, err
	}
	return row, nil
}

// projectExists reports whether the project is there, as the not found answer when it is not. The
// chats table points at the project, so a project that does not exist has no chats to list.
func (s *Service) projectExists(ctx context.Context, projectID string) error {
	err := s.store.Read(ctx, func(q *db.Queries) error {
		_, err := q.GetProject(ctx, projectID)
		return err
	})
	switch {
	case err == nil:
		return nil
	case store.IsNotFound(err):
		return protocol.NotFound("project").With("id", projectID)
	default:
		return fmt.Errorf("read project %s: %w", projectID, err)
	}
}

// stopSession puts a chat's session to sleep for an archive, or ends it for a delete. It is a
// no-op without a session manager, which is how the service is built in a test that is only about
// the stored chats.
func (s *Service) stopSession(ctx context.Context, chatID string) error {
	if s.sessions == nil {
		return nil
	}
	if err := s.sessions.StopChatSession(ctx, chatID); err != nil {
		return fmt.Errorf("stop the session of chat %s: %w", chatID, err)
	}
	return nil
}

// newID mints an opaque id for a chat or one of its sessions.
func (s *Service) newID(now time.Time) (string, error) {
	id, err := protocol.NewID(now, rand.Reader)
	if err != nil {
		return "", fmt.Errorf("make a chat id: %w", err)
	}
	return id, nil
}

// publish sends a chat event. Every one of them is critical: a client that missed the chat leaving
// the list would keep drawing it.
func (s *Service) publish(topic protocol.Topic, eventType protocol.EventType, data any) {
	s.bus.Publish(string(topic), string(eventType), data, true)
}

// chatOf turns a stored chat into the wire type.
func chatOf(row db.Chat) protocol.Chat {
	chat := protocol.Chat{
		ID: row.ID, ProjectID: row.ProjectID, Title: row.Title,
		Target:     protocol.ChatTarget{Kind: protocol.ChatTargetKind(row.TargetKind), ID: row.TargetID},
		AgentKind:  protocol.AgentKind(row.AgentKind),
		Model:      row.Model,
		ArchivedAt: store.OptionalTimestamp(row.ArchivedAt),
		CreatedAt:  store.Timestamp(row.CreatedAt),
	}
	if row.Thinking != "" {
		thinking := protocol.ThinkingMode(row.Thinking)
		chat.Thinking = &thinking
	}
	chat.PermissionMode = protocol.PermissionMode(row.PermissionMode)
	chat.LastActiveAt = store.Timestamp(row.LastActiveAt)
	return chat
}

// notFoundChat is the answer for a chat that is not there. It is built the way the services build
// theirs, so an id that cannot exist and one that was deleted read the same.
func notFoundChat(id string) *protocol.Error {
	if len(id) > maxEchoedIDBytes {
		id = id[:maxEchoedIDBytes]
	}
	return protocol.NotFound("chat").With("id", id)
}

// maxEchoedIDBytes cuts an id before it is sent back in an error, so a very long one is not
// repeated in full.
const maxEchoedIDBytes = 64

// joinStrings writes a fixed list of words for a refusal's sentence, in the order the protocol
// lists them.
func joinStrings[T ~string](values []T) string {
	words := make([]string, len(values))
	for i, value := range values {
		words[i] = string(value)
	}
	return strings.Join(slices.Clip(words), ", ")
}
