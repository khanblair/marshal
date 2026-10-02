package chats_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/chats"
	"github.com/khanblair/marshal/daemon/internal/integrator"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// A system chat is one Marshal keeps for a project itself: the pinned Integrator chat. These tests
// are about making it once, listing it first, refusing what a person may not do to it, and the
// Integrator-chat adapter the merge resolver drives.

// The adapter is what the merge flow depends on, so its shape is checked at compile time.
var _ integrator.ChatSession = (*chats.IntegratorChat)(nil)

// rolesStub is the chats.Roles the service reads the Integrator's model from.
type rolesStub struct {
	model string
	err   error
	asked []string
}

func (r *rolesStub) Role(_ context.Context, name, _ string) (protocol.Role, error) {
	r.asked = append(r.asked, name)
	if r.err != nil {
		return protocol.Role{}, r.err
	}
	return protocol.Role{Name: name, Spec: protocol.RoleSpec{Model: r.model}}, nil
}

// withRoles rebuilds the env's service so it reads roles, keeping the same store and session stub.
func (e *env) withRoles(t *testing.T, roles chats.Roles) {
	t.Helper()
	svc, err := chats.New(
		chats.Deps{Store: e.store, Bus: e.events, Sessions: e.sessions, Roles: roles},
		chats.WithClock(func() time.Time { return testTime }),
	)
	if err != nil {
		t.Fatalf("make the service: %v", err)
	}
	e.service = svc
}

// ensure makes the project's Integrator chat and fails the test when it is refused.
func (e *env) ensure(t *testing.T) protocol.Chat {
	t.Helper()
	chat, err := e.service.EnsureSystemChat(context.Background(), "api", protocol.ChatSystemIntegrator)
	if err != nil {
		t.Fatalf("EnsureSystemChat: %v", err)
	}
	return chat
}

// wantSentence fails the test unless err is a refusal whose sentence holds every word given.
func wantSentence(t *testing.T, err error, words ...string) {
	t.Helper()
	wantCode(t, err, protocol.ErrorCodeRefused)
	var answer *protocol.Error
	if !errors.As(err, &answer) {
		return
	}
	for _, word := range words {
		if !strings.Contains(answer.Message, word) {
			t.Errorf("the refusal %q does not say %q", answer.Message, word)
		}
	}
}

func TestEnsureSystemChatMakesThePinnedIntegratorChat(t *testing.T) {
	e := newEnv(t)
	e.withRoles(t, &rolesStub{model: "claude-opus-4-1"})
	chat := e.ensure(t)

	if chat.System != protocol.ChatSystemIntegrator || chat.Title != "Integrator" {
		t.Errorf("chat = %+v, want the system chat titled Integrator", chat)
	}
	if chat.Target.Kind != protocol.ChatTargetKindRole || chat.Target.ID != "Integrator" {
		t.Errorf("target = %+v, want the Integrator role", chat.Target)
	}
	if chat.AgentKind != protocol.AgentKindClaude || chat.PermissionMode != protocol.PermissionModeAutoEdits {
		t.Errorf("agent / mode = %q / %q, want claude / auto-edits", chat.AgentKind, chat.PermissionMode)
	}
	if chat.Model != "claude-opus-4-1" {
		t.Errorf("model = %q, want the Integrator role's model", chat.Model)
	}
	// Like any chat it keeps a session row from the moment it exists, or the first message could not
	// start its agent.
	session := e.sessionOf(t, chat.ID)
	if session.State != string(sessionStateStarting) || session.Model != "claude-opus-4-1" {
		t.Errorf("session = %+v, want a starting session with the chat's model", session)
	}
}

func TestEnsureSystemChatIsIdempotentAndPublishesOnlyWhenItMakesTheChat(t *testing.T) {
	e := newEnv(t)
	first := e.ensure(t)
	second := e.ensure(t)
	if first.ID != second.ID {
		t.Errorf("a second call made another chat: %s then %s", first.ID, second.ID)
	}
	if got := e.events.types(); len(got) != 1 || got[0] != string(protocol.EventTypeChatCreated) {
		t.Errorf("events = %v, want one chat.created", got)
	}
	list, err := e.service.List(context.Background(), "api", false)
	if err != nil || len(list.Chats) != 1 {
		t.Fatalf("List = %+v, %v, want the one chat", list, err)
	}
}

func TestEnsureSystemChatWorksWithoutARolesReaderAndWhenTheRoleCannotBeRead(t *testing.T) {
	e := newEnv(t)
	if chat := e.ensure(t); chat.Model != "" {
		t.Errorf("model = %q with no roles reader, want the agent's own default", chat.Model)
	}
	f := newEnv(t)
	roles := &rolesStub{err: errors.New("roles are down")}
	f.withRoles(t, roles)
	if chat := f.ensure(t); chat.Model != "" || len(roles.asked) != 1 || roles.asked[0] != "Integrator" {
		t.Errorf("chat = %+v, roles asked = %v, want an empty model after asking for the Integrator", chat, roles.asked)
	}
}

func TestARolesReaderGivenAfterTheServiceIsBuiltIsUsedForTheNextChat(t *testing.T) {
	e := newEnv(t)
	e.service.SetRoles(&rolesStub{model: "claude-opus-4-1"})
	if chat := e.ensure(t); chat.Model != "claude-opus-4-1" {
		t.Errorf("model = %q, want the late roles reader's", chat.Model)
	}
}

func TestEnsureSystemChatRefusesAnUnknownKindAndAnUnknownProject(t *testing.T) {
	e := newEnv(t)
	_, err := e.service.EnsureSystemChat(context.Background(), "api", "orchestrator")
	wantCode(t, err, protocol.ErrorCodeInvalidArgument)
	_, err = e.service.EnsureSystemChat(context.Background(), "nope", protocol.ChatSystemIntegrator)
	wantCode(t, err, protocol.ErrorCodeNotFound)
	if len(e.events.events) != 0 {
		t.Errorf("a refused call published %v", e.events.types())
	}
}

func TestTheSystemChatIsListedFirstWhateverTheOtherChatsDo(t *testing.T) {
	e := newEnv(t)
	other := []protocol.Chat{e.newChat(t, protocol.CreateChatRequest{}), e.newChat(t, protocol.CreateChatRequest{})}
	system := e.ensure(t)
	e.newChat(t, protocol.CreateChatRequest{})
	e.send(t, other[0].ID, "touch this chat so it is the most recent")

	list, err := e.service.List(context.Background(), "api", false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Chats) != 4 || list.Chats[0].ID != system.ID {
		t.Fatalf("the chats are %v, want the system chat first of four", idsOf(list.Chats))
	}
}

func idsOf(list []protocol.Chat) []string {
	out := make([]string, len(list))
	for i, chat := range list {
		out[i] = chat.System + ":" + chat.ID
	}
	return out
}

func TestTheSystemChatCannotBeRenamedArchivedOrDeleted(t *testing.T) {
	e := newEnv(t)
	chat := e.ensure(t)
	title := "My merges"
	before := len(e.events.events)

	_, err := e.service.Update(context.Background(), chat.ID, protocol.UpdateChatRequest{Title: &title})
	wantSentence(t, err, "Integrator", "renamed")
	_, err = e.service.Archive(context.Background(), chat.ID)
	wantSentence(t, err, "Integrator", "archived")
	err = e.service.Remove(context.Background(), chat.ID)
	wantSentence(t, err, "Integrator", "deleted")

	// A refusal comes before any side effect: the session was neither stopped nor stripped of its
	// logs, nothing was announced, and the chat is exactly as it was.
	if len(e.sessions.stopped) != 0 || len(e.sessions.logs) != 0 {
		t.Errorf("a refused change stopped %v and removed the logs of %v", e.sessions.stopped, e.sessions.logs)
	}
	if len(e.events.events) != before {
		t.Errorf("a refused change published %v", e.events.types()[before:])
	}
	got, err := e.service.Chat(context.Background(), chat.ID)
	if err != nil || got.Title != "Integrator" || got.ArchivedAt != nil {
		t.Errorf("chat = %+v, %v, want it untouched", got, err)
	}
}

func TestTheSystemChatCanStillBeAskedToKeepItsNameAndBeSentMessages(t *testing.T) {
	e := newEnv(t)
	chat := e.ensure(t)
	// A rename that names no new title changes nothing, so it is not refused.
	if got, err := e.service.Update(context.Background(), chat.ID, protocol.UpdateChatRequest{}); err != nil || got.ID != chat.ID {
		t.Errorf("Update with no title = %+v, %v, want the chat back", got, err)
	}
	e.send(t, chat.ID, "Merge the next ready card please")
	got, err := e.service.Chat(context.Background(), chat.ID)
	if err != nil || got.Title != "Integrator" {
		t.Errorf("chat = %+v, %v, want the title kept: only a New chat is named by its first message", got, err)
	}
}

func TestEnsureAllSystemChatsGivesEveryProjectItsChat(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.store.Write(ctx, func(q *db.Queries) error {
		return q.CreateProject(ctx, db.CreateProjectParams{
			ID: "web", Name: "web", RepoPath: "/tmp/web", DefaultBranch: "main",
			PackagesJSON: "[]", CreatedAt: testTime.UnixMilli(), UpdatedAt: testTime.UnixMilli(),
		})
	}); err != nil {
		t.Fatalf("write the second project: %v", err)
	}
	if err := e.service.EnsureAllSystemChats(ctx); err != nil {
		t.Fatalf("EnsureAllSystemChats: %v", err)
	}
	if err := e.service.EnsureAllSystemChats(ctx); err != nil {
		t.Fatalf("a second EnsureAllSystemChats: %v", err)
	}
	for _, project := range []string{"api", "web"} {
		list, err := e.service.List(ctx, project, false)
		if err != nil || len(list.Chats) != 1 || list.Chats[0].System != protocol.ChatSystemIntegrator {
			t.Errorf("%s chats = %+v, %v, want only the Integrator chat", project, list.Chats, err)
		}
	}
}

func TestTheDatabaseKeepsOneIntegratorChatPerProject(t *testing.T) {
	e := newEnv(t)
	chat := e.ensure(t)
	ctx := context.Background()
	err := e.store.Write(ctx, func(q *db.Queries) error {
		return q.CreateChat(ctx, db.CreateChatParams{
			ID: "01M3SECOND00000000000000000", ProjectID: "api", Title: "Integrator", TargetKind: "role",
			TargetID: "Integrator", AgentKind: "claude", PermissionMode: "auto-edits", System: protocol.ChatSystemIntegrator,
		})
	})
	if err == nil {
		t.Fatalf("a second Integrator chat was written next to %s", chat.ID)
	}
	if !strings.Contains(err.Error(), "UNIQUE") {
		t.Errorf("err = %v, want a uniqueness failure", err)
	}
}

func TestIntegratorChatSendMakesTheChatAndSendsToItsSession(t *testing.T) {
	e := newEnv(t)
	ic := chats.NewIntegratorChat(e.service)
	if err := ic.Send(context.Background(), "api", "Merge task t-1"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(e.sessions.sent) != 1 {
		t.Fatalf("the session was given %d messages, want 1", len(e.sessions.sent))
	}
	got := e.sessions.sent[0]
	if got.text != "Merge task t-1" || got.chat.System != protocol.ChatSystemIntegrator || got.chat.ProjectID != "api" {
		t.Errorf("sent = %+v, want the task text to the Integrator chat", got)
	}
	// A second task goes to the same chat.
	if err := ic.Send(context.Background(), "api", "Merge task t-2"); err != nil {
		t.Fatalf("second Send: %v", err)
	}
	if len(e.sessions.sent) != 2 || e.sessions.sent[1].chat.ID != got.chat.ID {
		t.Errorf("the second task went to %+v, want the same chat", e.sessions.sent)
	}
}

func TestIntegratorChatResetEndsTheSessionAndKeepsTheChat(t *testing.T) {
	e := newEnv(t)
	ic := chats.NewIntegratorChat(e.service)
	chat := e.ensure(t)
	if err := ic.Reset(context.Background(), "api"); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(e.sessions.reset) != 1 || e.sessions.reset[0] != chat.ID {
		t.Errorf("reset = %v, want the Integrator chat's session", e.sessions.reset)
	}
	// Only the live session and its saved conversation go: the chat stays, so its history does too.
	if got, err := e.service.Chat(context.Background(), chat.ID); err != nil || got.System != protocol.ChatSystemIntegrator {
		t.Errorf("chat after Reset = %+v, %v, want it kept", got, err)
	}
	if len(e.sessions.stopped) != 0 || len(e.sessions.logs) != 0 {
		t.Errorf("a reset stopped %v and removed the logs of %v, want neither", e.sessions.stopped, e.sessions.logs)
	}
}

func TestIntegratorChatResetFailsWithTheSessionManagersError(t *testing.T) {
	e := newEnv(t)
	e.sessions.resetErr = errors.New("the agent will not stop")
	err := chats.NewIntegratorChat(e.service).Reset(context.Background(), "api")
	if err == nil || !strings.Contains(err.Error(), "the agent will not stop") {
		t.Errorf("Reset = %v, want the session manager's error", err)
	}
}

func TestIntegratorChatMakesTheChatForAProjectThatHasNone(t *testing.T) {
	e := newBareEnv(t)
	err := chats.NewIntegratorChat(e.service).Send(context.Background(), "api", "hello")
	wantCode(t, err, protocol.ErrorCodeUnavailable)
	// The chat exists even though there was no session manager to send to.
	list, listErr := e.service.List(context.Background(), "api", false)
	if listErr != nil || len(list.Chats) != 1 {
		t.Errorf("List = %+v, %v, want the Integrator chat made", list, listErr)
	}
}
