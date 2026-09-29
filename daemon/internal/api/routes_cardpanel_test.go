package api_test

import (
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A card's checks, checklists, comments, and members over the API (docs/backend-checklist.md B10.2,
// B10.5, B10.6). The rules are internal/cardpanel's and are tested there; these tests prove the
// routes, the shapes they answer with, and that the caller is who a comment or a tick is by.

func TestChecklistsThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Ship it")
	base := "/v1/cards/" + card.ID + "/checklists"

	made := st.do(http.MethodPost, base, protocol.CreateChecklistRequest{Name: "Done when"}).want(t, http.StatusOK)
	list := decode[protocol.ChecklistList](t, made).Checklists[0]
	if list.Name != "Done when" || list.Items == nil {
		t.Fatalf("checklist = %+v", list)
	}
	yes := true
	st.do(http.MethodPatch, base+"/"+list.ID, protocol.UpdateChecklistRequest{Required: &yes}).want(t, http.StatusOK)
	added := st.do(http.MethodPost, base+"/"+list.ID+"/items", protocol.AddChecklistItemRequest{Text: "Docs updated"}).want(t, http.StatusOK)
	sameShape(t, "checklists", added.Body)
	item := decode[protocol.ChecklistList](t, added).Checklists[0].Items[0]

	ticked := decode[protocol.ChecklistList](t, st.do(http.MethodPut, base+"/"+list.ID+"/items/"+item.ID,
		protocol.TickChecklistItemRequest{Done: true}).want(t, http.StatusOK))
	got := ticked.Checklists[0]
	if !got.Required || !got.Items[0].Done || got.Items[0].DoneByKind != "person" || got.Items[0].DoneByID == "" {
		t.Fatalf("after the tick: %+v", got)
	}
	gone := decode[protocol.ChecklistList](t, st.do(http.MethodDelete, base+"/"+list.ID, nil).want(t, http.StatusOK))
	if len(gone.Checklists) != 0 {
		t.Fatalf("the list is still there: %+v", gone)
	}
	st.do(http.MethodGet, base, nil).want(t, http.StatusOK)
	st.do(http.MethodPatch, base+"/"+list.ID, protocol.UpdateChecklistRequest{Required: &yes}).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}

func TestCommentsThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Ship it")
	base := "/v1/cards/" + card.ID + "/comments"

	posted := st.do(http.MethodPost, base, protocol.PostCommentRequest{
		Body: "see https://example.com/spec",
		Attachments: []protocol.NewAttachment{{
			Kind: protocol.AttachmentKindFile, Name: "notes.txt", MimeType: "text/plain",
			Data: base64.StdEncoding.EncodeToString([]byte("hello")),
		}},
	}).want(t, http.StatusOK)
	sameShape(t, "comments", posted.Body)
	comment := decode[protocol.CommentList](t, posted).Comments[0]
	if comment.AuthorKind != protocol.AuthorKindPerson || comment.AuthorID == "" || len(comment.Attachments) != 2 {
		t.Fatalf("comment = %+v", comment)
	}

	file := st.do(http.MethodGet, "/v1/cards/"+card.ID+"/attachments/"+comment.Attachments[0].ID, nil).want(t, http.StatusOK)
	if string(file.Body) != "hello" {
		t.Fatalf("file = %q", file.Body)
	}
	if h := file.Header.Get("Content-Disposition"); h == "" || h[:10] != "attachment" {
		t.Fatalf("a text file must download, not open: %q", h)
	}
	if file.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("the file is served without nosniff")
	}

	st.do(http.MethodPost, base, protocol.PostCommentRequest{}).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	left := decode[protocol.CommentList](t, st.do(http.MethodDelete, base+"/"+comment.ID, nil).want(t, http.StatusOK))
	if len(left.Comments) != 0 {
		t.Fatalf("comments = %+v", left)
	}
}

func TestMembersThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Ship it")
	me := decode[protocol.Profile](t, st.do(http.MethodGet, "/v1/me", nil).want(t, http.StatusOK))
	route := "/v1/cards/" + card.ID + "/members/"

	added := st.do(http.MethodPut, route+me.ID, nil).want(t, http.StatusOK)
	sameShape(t, "card-members", added.Body)
	if got := decode[protocol.CardMembers](t, added); len(got.UserIDs) != 1 || got.UserIDs[0] != me.ID {
		t.Fatalf("members = %+v", got)
	}
	st.do(http.MethodPut, route+"01M3USER00000000000000000Z", nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	left := decode[protocol.CardMembers](t, st.do(http.MethodDelete, route+me.ID, nil).want(t, http.StatusOK))
	if len(left.UserIDs) != 0 {
		t.Fatalf("members = %+v", left)
	}
}

func TestCardChecksThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Ship it")
	base := "/v1/cards/" + card.ID + "/checks"

	got := st.do(http.MethodGet, base, nil).want(t, http.StatusOK)
	sameShape(t, "card-checks", got.Body)
	if list := decode[protocol.CardCheckList](t, got); len(list.Checks) == 0 {
		t.Fatal("a new card has no checks")
	}
	added := decode[protocol.CardCheckList](t, st.do(http.MethodPost, base, protocol.AddCardCheckRequest{Name: "Smoke", Command: "true"}).want(t, http.StatusOK))
	last := added.Checks[len(added.Checks)-1]
	if last.Name != "Smoke" || last.Status != protocol.CheckStatusPending {
		t.Fatalf("last check = %+v", last)
	}
	// A card that has not started has no worktree to run in, which is said in words.
	st.do(http.MethodPost, base+"/run", nil).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	left := decode[protocol.CardCheckList](t, st.do(http.MethodDelete, base+"/"+last.ID, nil).want(t, http.StatusOK))
	if len(left.Checks) != len(added.Checks)-1 {
		t.Fatalf("the check was not removed: %+v", left)
	}
}

func TestPanelRoutesRefuseAnUnknownCard(t *testing.T) {
	st := newStack(t)
	for _, path := range []string{"/checks", "/checklists", "/comments", "/members"} {
		st.do(http.MethodGet, "/v1/cards/01M3CARD00000000000000000Z"+path, nil).
			apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	}
}

func TestPanelRoutesFollowTheirService(t *testing.T) {
	st := newStack(t, withoutCardPanel())
	st.do(http.MethodGet, "/v1/cards/01M3CARD00000000000000000Z/checks", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}
