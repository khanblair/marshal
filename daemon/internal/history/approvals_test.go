package history_test

import (
	"context"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// request is a small permission request, enough for AppendApproval's own fields.
func request(title string) agents.PermissionRequested {
	return agents.PermissionRequested{RequestID: "r1", Title: title, Kind: "execute", Command: "go test ./..."}
}

// approvalOf reads a card's newest event as the wire approval block a chat draws, failing the test
// if the newest event is not one.
func approvalOf(t *testing.T, e *env, cardID string) protocol.ChatApproval {
	t.Helper()
	page, err := e.history.Page(context.Background(), cardID, 0, 1)
	if err != nil {
		t.Fatalf("Page: %v", err)
	}
	if len(page.Events) != 1 {
		t.Fatalf("got %d events, want 1", len(page.Events))
	}
	msg, err := history.ChatMessageOf(page.Events[0])
	if err != nil {
		t.Fatalf("ChatMessageOf: %v", err)
	}
	if msg.Approval == nil {
		t.Fatalf("message = %+v, want an approval block", msg)
	}
	return *msg.Approval
}

func TestAppendApprovalStoresTheApprovalsOwnID(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.history.AppendApproval(ctx, e.cardID, e.sessionID, "a1", history.StateWaiting, request("Run the tests")); err != nil {
		t.Fatalf("AppendApproval: %v", err)
	}
	got := approvalOf(t, e, e.cardID)
	if got.ID != "a1" || got.State != protocol.ChatApprovalStateWaiting || got.Command != "go test ./..." {
		t.Errorf("approval = %+v, want id a1, waiting, the command", got)
	}
}

func TestAnApprovalRowWithNoIDAndNoStateIsNeverWaiting(t *testing.T) {
	// Bypass once wrote its note this way. A person has nothing to answer, so no button is drawn.
	e := newEnv(t)
	note := []history.Record{{Kind: history.KindApproval, Summary: "Bypass permissions turned on by you"}}
	if err := e.history.Append(context.Background(), e.cardID, e.sessionID, note); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got := approvalOf(t, e, e.cardID)
	if got.ID != "" || got.State != protocol.ChatApprovalStateApproved {
		t.Errorf("approval = %+v, want no id, approved", got)
	}
}

func TestAppendApprovalWithNoIDIsAnAlreadyDecidedRow(t *testing.T) {
	// The daemon-auto-answer paths (bypass, the harness) never mint an approval id, and their row is
	// already resolved: nothing is ever waiting for a person to answer.
	e := newEnv(t)
	ctx := context.Background()
	if err := e.history.AppendApproval(ctx, e.cardID, e.sessionID, "", history.StateOK, request("Run the tests")); err != nil {
		t.Fatalf("AppendApproval: %v", err)
	}
	got := approvalOf(t, e, e.cardID)
	if got.ID != "" || got.State != protocol.ChatApprovalStateApproved {
		t.Errorf("approval = %+v, want no id, approved", got)
	}
}

func TestResolveApprovalRewritesTheStoredState(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.history.AppendApproval(ctx, e.cardID, e.sessionID, "a1", history.StateWaiting, request("Run the tests")); err != nil {
		t.Fatalf("AppendApproval: %v", err)
	}
	if err := e.history.ResolveApproval(ctx, e.cardID, "a1", history.StateOK); err != nil {
		t.Fatalf("ResolveApproval: %v", err)
	}
	got := approvalOf(t, e, e.cardID)
	if got.ID != "a1" || got.State != protocol.ChatApprovalStateApproved {
		t.Errorf("approval after resolve = %+v, want id a1, approved", got)
	}
}

func TestResolveApprovalWithAnUnmatchedIDIsANoOp(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.history.AppendApproval(ctx, e.cardID, e.sessionID, "a1", history.StateWaiting, request("Run the tests")); err != nil {
		t.Fatalf("AppendApproval: %v", err)
	}
	if err := e.history.ResolveApproval(ctx, e.cardID, "does-not-exist", history.StateOK); err != nil {
		t.Fatalf("ResolveApproval(unmatched id): %v, want no error", err)
	}
	got := approvalOf(t, e, e.cardID)
	if got.ID != "a1" || got.State != protocol.ChatApprovalStateWaiting {
		t.Errorf("approval after a no-op resolve = %+v, want it unchanged (id a1, waiting)", got)
	}
}

func TestResolveApprovalWithAnEmptyIDIsANoOp(t *testing.T) {
	// The row AppendApproval writes for a daemon-auto-answered request has no id, so nothing could
	// ever be found for one; ResolveApproval must not scan the whole history looking for it.
	e := newEnv(t)
	ctx := context.Background()
	if err := e.history.AppendApproval(ctx, e.cardID, e.sessionID, "", history.StateOK, request("Run the tests")); err != nil {
		t.Fatalf("AppendApproval: %v", err)
	}
	if err := e.history.ResolveApproval(ctx, e.cardID, "", history.StateFailed); err != nil {
		t.Fatalf("ResolveApproval(empty id): %v, want no error", err)
	}
	got := approvalOf(t, e, e.cardID)
	if got.State != protocol.ChatApprovalStateApproved {
		t.Errorf("approval after an empty-id resolve = %+v, want it unchanged (approved)", got)
	}
}

func TestResolveApprovalNeverTouchesAnotherCardsRow(t *testing.T) {
	e := newEnv(t)
	other, otherSession := addCard(t, e.store, "2")
	ctx := context.Background()
	// Both cards happen to carry the same approval id, so the test proves the query is scoped by
	// the owner and not merely lucky that ids never collide.
	if err := e.history.AppendApproval(ctx, e.cardID, e.sessionID, "shared", history.StateWaiting, request("Run the tests")); err != nil {
		t.Fatalf("AppendApproval(card 1): %v", err)
	}
	if err := e.history.AppendApproval(ctx, other, otherSession, "shared", history.StateWaiting, request("Run the tests")); err != nil {
		t.Fatalf("AppendApproval(card 2): %v", err)
	}
	if err := e.history.ResolveApproval(ctx, e.cardID, "shared", history.StateOK); err != nil {
		t.Fatalf("ResolveApproval: %v", err)
	}
	if got := approvalOf(t, e, e.cardID); got.State != protocol.ChatApprovalStateApproved {
		t.Errorf("card 1's approval = %+v, want approved", got)
	}
	if got := approvalOf(t, e, other); got.State != protocol.ChatApprovalStateWaiting {
		t.Errorf("card 2's approval = %+v, want it still waiting", got)
	}
}

func TestAppendChatApprovalAndResolveChatApprovalMirrorTheCardVersions(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	chatID, chatSession := addChat(t, e.store, "1")
	if err := e.history.AppendChatApproval(ctx, chatID, chatSession, "a1", history.StateWaiting, request("Run the tests")); err != nil {
		t.Fatalf("AppendChatApproval: %v", err)
	}
	page, err := e.history.PageChat(ctx, chatID, 0, 1)
	if err != nil {
		t.Fatalf("PageChat: %v", err)
	}
	if len(page.Events) != 1 {
		t.Fatalf("got %d events, want 1", len(page.Events))
	}
	msg, err := history.ChatMessageOf(page.Events[0])
	if err != nil {
		t.Fatalf("ChatMessageOf: %v", err)
	}
	if msg.Approval == nil || msg.Approval.ID != "a1" {
		t.Fatalf("message = %+v, want an approval block with id a1", msg)
	}
	if err := e.history.ResolveChatApproval(ctx, chatID, "a1", history.StateFailed); err != nil {
		t.Fatalf("ResolveChatApproval: %v", err)
	}
	page, err = e.history.PageChat(ctx, chatID, 0, 1)
	if err != nil {
		t.Fatalf("PageChat after resolve: %v", err)
	}
	msg, err = history.ChatMessageOf(page.Events[0])
	if err != nil {
		t.Fatalf("ChatMessageOf after resolve: %v", err)
	}
	if msg.Approval == nil || msg.Approval.State != protocol.ChatApprovalStateDenied {
		t.Errorf("message after resolve = %+v, want denied", msg)
	}
}
