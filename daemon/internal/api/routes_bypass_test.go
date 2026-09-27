package api_test

import (
	"net/http"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The bypass routes (docs/backend-checklist.md B3.2): POST /v1/cards/{id}/bypass turns bypass on
// with the acknowledgement a person gave, DELETE turns it off, and no other way of writing a card
// reaches that mode.

// bypassCard is a card in a fresh project, which is what the routes need.
func bypassCard(t *testing.T, st *stack) protocol.Card {
	t.Helper()
	project, _ := st.addProject("small-repo")
	return st.addCard(project.ID, "Run the tests")
}

func TestBypassRoutesTurnItOnAndOff(t *testing.T) {
	st := newStack(t)
	card := bypassCard(t, st)

	on := decode[protocol.Card](t, st.do(http.MethodPost, "/v1/cards/"+card.ID+"/bypass",
		protocol.BypassRequest{Acknowledged: true}).want(t, http.StatusOK))
	if on.PermissionMode != protocol.PermissionModeBypass {
		t.Errorf("the card after turning bypass on = %q, want bypass", on.PermissionMode)
	}
	if got := st.getCard(card.ID); got.PermissionMode != protocol.PermissionModeBypass {
		t.Errorf("the stored card after turning bypass on = %q, want bypass", got.PermissionMode)
	}

	off := decode[protocol.Card](t, st.do(http.MethodDelete, "/v1/cards/"+card.ID+"/bypass", nil).
		want(t, http.StatusOK))
	if off.PermissionMode != protocol.PermissionModeFullAuto {
		t.Errorf("the card after turning bypass off = %q, want full-auto", off.PermissionMode)
	}
	// Turning it off again is not a refusal: there is nothing to undo, and the card is still read
	// back in the mode the app's own switch leaves it in.
	again := decode[protocol.Card](t, st.do(http.MethodDelete, "/v1/cards/"+card.ID+"/bypass", nil).
		want(t, http.StatusOK))
	if again.PermissionMode != protocol.PermissionModeFullAuto {
		t.Errorf("a second turn off = %q, want full-auto", again.PermissionMode)
	}
}

func TestBypassRouteRefusesAnUnacknowledgedRequest(t *testing.T) {
	st := newStack(t)
	card := bypassCard(t, st)

	// A body that carries no acknowledgement, however it is written, cannot grant bypass.
	got := st.do(http.MethodPost, "/v1/cards/"+card.ID+"/bypass", map[string]any{}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got.Details["reason"] != string(protocol.BypassRefusalReasonUnacknowledged) {
		t.Errorf("reason = %q, want %q", got.Details["reason"], protocol.BypassRefusalReasonUnacknowledged)
	}
	if card := st.getCard(card.ID); card.PermissionMode == protocol.PermissionModeBypass {
		t.Error("a request with no acknowledgement put the card in bypass")
	}

	// The field is not optional in the body, but a body that is not JSON at all is a bad request.
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/bypass", nil).
		want(t, http.StatusBadRequest)

	// An id that cannot be a card's is not found, without asking the sessions anything.
	st.do(http.MethodPost, "/v1/cards/not-an-id/bypass", protocol.BypassRequest{Acknowledged: true}).
		want(t, http.StatusNotFound)
}

func TestBypassRouteIsRefusedForALockedProject(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Run the tests")

	locked := true
	st.do(http.MethodPatch, "/v1/projects/"+project.ID, protocol.UpdateProjectRequest{BypassLocked: &locked}).
		want(t, http.StatusOK)

	got := st.do(http.MethodPost, "/v1/cards/"+card.ID+"/bypass", protocol.BypassRequest{Acknowledged: true}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got.Details["reason"] != string(protocol.BypassRefusalReasonLocked) {
		t.Errorf("reason = %q, want %q", got.Details["reason"], protocol.BypassRefusalReasonLocked)
	}
	if got.Details["projectId"] != project.ID {
		t.Errorf("the refusal names project %q, want %q", got.Details["projectId"], project.ID)
	}
	if card := st.getCard(card.ID); card.PermissionMode == protocol.PermissionModeBypass {
		t.Error("a locked project let bypass through")
	}

	// Turning it off is never locked: the lock is about granting bypass, not about leaving it.
	st.do(http.MethodDelete, "/v1/cards/"+card.ID+"/bypass", nil).want(t, http.StatusOK)
}

// TestBypassIsUnreachableFromACardEdit is the other half of the rule: bypass is granted by the
// call that carries the acknowledgement and by nothing else, so setting the field directly is
// refused rather than quietly accepted.
func TestBypassIsUnreachableFromACardEdit(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")

	// A new card cannot be born in bypass.
	got := st.do(http.MethodPost, "/v1/projects/"+project.ID+"/cards",
		protocol.CreateCardRequest{Title: "Sneak it in", PermissionMode: protocol.PermissionModeBypass}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got.Details["reason"] != string(protocol.BypassRefusalReasonUnacknowledged) {
		t.Errorf("reason = %q, want %q", got.Details["reason"], protocol.BypassRefusalReasonUnacknowledged)
	}

	card := st.addCard(project.ID, "Run the tests")
	bypass := protocol.PermissionModeBypass
	got = st.do(http.MethodPatch, "/v1/cards/"+card.ID, protocol.UpdateCardRequest{PermissionMode: &bypass}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got.Details["reason"] != string(protocol.BypassRefusalReasonUnacknowledged) {
		t.Errorf("reason = %q, want %q", got.Details["reason"], protocol.BypassRefusalReasonUnacknowledged)
	}
	if card := st.getCard(card.ID); card.PermissionMode == protocol.PermissionModeBypass {
		t.Error("an edit put the card in bypass")
	}

	// The modes that do not grant bypass are still set by an edit, so the refusal is about bypass
	// and not about the field.
	full := protocol.PermissionModePlan
	edited := decode[protocol.Card](t, st.do(http.MethodPatch, "/v1/cards/"+card.ID,
		protocol.UpdateCardRequest{PermissionMode: &full}).want(t, http.StatusOK))
	if edited.PermissionMode != protocol.PermissionModePlan {
		t.Errorf("the card after an ordinary edit = %q, want plan", edited.PermissionMode)
	}
}
