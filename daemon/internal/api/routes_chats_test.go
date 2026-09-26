package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The project chat routes (docs/backend-checklist.md B2.10): the list, the create, and the four
// changes to one chat, over the real server, the real store, and the real event bus. The rules
// themselves are tested in internal/chats; these tests are about what the routes read, what they
// answer, and what they announce.

// chatListOf reads a project's chats through the API.
func (st *stack) chatListOf(projectID string, archived bool) protocol.ChatListSnapshot {
	st.t.Helper()
	path := "/v1/projects/" + projectID + "/chats"
	if archived {
		path += "?archived=true"
	}
	r := st.do(http.MethodGet, path, nil).want(st.t, http.StatusOK)
	return decode[protocol.ChatListSnapshot](st.t, r)
}

// newChat makes a chat through the API.
func (st *stack) newChat(projectID string, in protocol.CreateChatRequest) protocol.Chat {
	st.t.Helper()
	r := st.do(http.MethodPost, "/v1/projects/"+projectID+"/chats", in)
	return decode[protocol.Chat](st.t, r.want(st.t, http.StatusCreated))
}

func TestChatRoutesMakeRenameArchiveRestoreAndDelete(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")

	// A new project has no chats, and its list is an empty list rather than null.
	if got := st.chatListOf(project.ID, false); got.Chats == nil || len(got.Chats) != 0 {
		t.Fatalf("the new project's chats = %v, want an empty list", got.Chats)
	}

	chat := st.newChat(project.ID, protocol.CreateChatRequest{
		Title:  "JWKS caching question",
		Target: &protocol.ChatTarget{Kind: protocol.ChatTargetKindOrchestrator},
	})
	if chat.Title != "JWKS caching question" || chat.ProjectID != project.ID {
		t.Fatalf("chat = %+v", chat)
	}
	if chat.ArchivedAt != nil {
		t.Errorf("archivedAt = %s, want null for a new chat", chat.ArchivedAt.Time())
	}

	list := st.chatListOf(project.ID, false)
	if len(list.Chats) != 1 || list.Chats[0].ID != chat.ID {
		t.Fatalf("the list = %+v, want the new chat", list.Chats)
	}
	if list.ServerTime.Time().IsZero() {
		t.Error("the list carries no serverTime")
	}

	// Rename.
	name := "Per key limits"
	r := st.do(http.MethodPatch, "/v1/chats/"+chat.ID, protocol.UpdateChatRequest{Title: &name})
	renamed := decode[protocol.Chat](t, r.want(t, http.StatusOK))
	if renamed.Title != name {
		t.Errorf("title = %q, want %q", renamed.Title, name)
	}

	// Archive takes it out of the main list and puts it in the archived one.
	r = st.do(http.MethodPost, "/v1/chats/"+chat.ID+"/archive", nil)
	archived := decode[protocol.Chat](t, r.want(t, http.StatusOK))
	if archived.ArchivedAt == nil {
		t.Fatal("archivedAt is null after archiving")
	}
	if got := st.chatListOf(project.ID, false); len(got.Chats) != 0 {
		t.Errorf("the main list still holds %+v", got.Chats)
	}
	archivedList := st.chatListOf(project.ID, true)
	if len(archivedList.Chats) != 1 || archivedList.Chats[0].ID != chat.ID {
		t.Errorf("the archived list = %+v, want the archived chat", archivedList.Chats)
	}

	// Restore puts it back.
	r = st.do(http.MethodPost, "/v1/chats/"+chat.ID+"/restore", nil)
	restored := decode[protocol.Chat](t, r.want(t, http.StatusOK))
	if restored.ArchivedAt != nil {
		t.Errorf("archivedAt = %s, want null after restoring", restored.ArchivedAt.Time())
	}
	if got := st.chatListOf(project.ID, false); len(got.Chats) != 1 {
		t.Errorf("the main list = %+v, want the restored chat", got.Chats)
	}

	// Delete.
	st.do(http.MethodDelete, "/v1/chats/"+chat.ID, nil).want(t, http.StatusNoContent)
	if got := st.chatListOf(project.ID, false); len(got.Chats) != 0 {
		t.Errorf("the chat survived the delete: %+v", got.Chats)
	}
	st.do(http.MethodGet, "/v1/chats/"+chat.ID, nil).want(t, http.StatusMethodNotAllowed)
}

func TestChatRoutesRefuseAProjectOrChatThatIsNotThere(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	missing := "/v1/chats/01M3C107JB041061050R3GG28Z"

	st.do(http.MethodGet, "/v1/projects/nope/chats", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodPost, "/v1/projects/nope/chats", protocol.CreateChatRequest{}).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodPatch, missing, protocol.UpdateChatRequest{}).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodPost, missing+"/archive", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodPost, missing+"/restore", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodDelete, missing, nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)

	// An id that cannot be an id is not found too, without a service being asked.
	st.do(http.MethodGet, "/v1/chats/not/a/chat", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	if got := st.chatListOf(project.ID, false); len(got.Chats) != 0 {
		t.Errorf("a refused route made a chat: %+v", got.Chats)
	}
}

func TestChatRoutesRefuseWhatTheChatsServiceRefuses(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")

	blank := "   "
	st.do(http.MethodPost, "/v1/projects/"+project.ID+"/chats", protocol.CreateChatRequest{Title: blank}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	st.do(http.MethodPost, "/v1/projects/"+project.ID+"/chats", protocol.CreateChatRequest{
		Target: &protocol.ChatTarget{Kind: protocol.ChatTargetKindRole},
	}).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)

	chat := st.newChat(project.ID, protocol.CreateChatRequest{})
	st.do(http.MethodPatch, "/v1/chats/"+chat.ID, protocol.UpdateChatRequest{Title: &blank}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)

	// The archived filter is true, false, or nothing; anything else is refused rather than read as
	// false, so a client that misspells it is told.
	st.do(http.MethodGet, "/v1/projects/"+project.ID+"/chats?archived=yes", nil).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
}

func TestChatRoutesAreNotRegisteredWithoutTheChatsService(t *testing.T) {
	st := newStack(t, withoutChats())
	project, _ := st.addProject("small-repo")
	st.do(http.MethodGet, "/v1/projects/"+project.ID+"/chats", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}

// A chat's session belongs to the chat, not to a card: the chat row carries it, and the session
// row names the chat. This is what makes "each chat keeps its own session" true at the store.
func TestAChatCreatesItsOwnSession(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	chat := st.newChat(project.ID, protocol.CreateChatRequest{})

	session, err := st.store.Queries().GetSessionByChat(context.Background(), &chat.ID)
	if err != nil {
		t.Fatalf("read the chat's session: %v", err)
	}
	if session.ChatID == nil || *session.ChatID != chat.ID {
		t.Errorf("session chat id = %v, want %s", session.ChatID, chat.ID)
	}
	if session.CardID != "" {
		t.Errorf("session card id = %q, want empty", session.CardID)
	}
	if session.State != string(protocol.SessionStateStarting) {
		t.Errorf("session state = %q, want starting", session.State)
	}
}
