package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The role routes (build-plan task 5.1, B5.1, N18). The rules are in internal/roles and are tested
// there; these tests prove the routes read the path, the query, and the body, answer in the wire
// type, and switch the whole list, which is what a screen redraws from.

const rolesPath = "/v1/roles"

// roleRow finds one role in a list by its name.
func roleRow(t *testing.T, list protocol.RoleList, name string) protocol.Role {
	t.Helper()
	for _, role := range list.Roles {
		if role.Name == name {
			return role
		}
	}
	t.Fatalf("the list has no role called %q; it has %d roles", name, len(list.Roles))
	return protocol.Role{}
}

// TestListRolesCarriesMarshalRoles is the read the Settings > Roles screen makes: Marshal's eight
// starter roles, in the order the screens show them, with no project being looked at.
func TestListRolesCarriesMarshalRoles(t *testing.T) {
	st := newStack(t)
	r := st.do(http.MethodGet, rolesPath, nil).want(t, http.StatusOK)
	sameShape(t, "role-list", r.Body)

	list := decode[protocol.RoleList](t, r)
	if len(list.Roles) != 8 {
		t.Fatalf("the list has %d roles, want Marshal's eight", len(list.Roles))
	}
	if list.ServerTime.Time().IsZero() {
		t.Error("serverTime is zero, want the daemon's time")
	}
	if got := list.Roles[0].Name; got != "Orchestrator" {
		t.Errorf("the first role is %q, want Orchestrator", got)
	}
	for _, role := range list.Roles {
		if !role.Starter {
			t.Errorf("%s is not marked a starter role", role.Name)
		}
		if role.Overridden {
			t.Errorf("%s reads as overridden with no project being looked at", role.Name)
		}
		if role.ID == "" {
			t.Errorf("%s has no id", role.Name)
		}
	}
}

// TestOneRoleIsReadByItsName checks the single read, which is what the editor opens with.
func TestOneRoleIsReadByItsName(t *testing.T) {
	st := newStack(t)
	r := st.do(http.MethodGet, rolesPath+"/Worker", nil).want(t, http.StatusOK)
	role := decode[protocol.Role](t, r)
	if role.Name != "Worker" || role.Spec.Model != "claude-sonnet-4-5" {
		t.Errorf("read %q with model %q, want Worker with claude-sonnet-4-5", role.Name, role.Spec.Model)
	}

	// A name in the path may hold a space: "Docs writer" is one of Marshal's own roles.
	r = st.do(http.MethodGet, rolesPath+"/Docs%20writer", nil).want(t, http.StatusOK)
	if got := decode[protocol.Role](t, r).Name; got != "Docs writer" {
		t.Errorf("read %q, want Docs writer", got)
	}

	st.do(http.MethodGet, rolesPath+"/Nobody", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}

// TestCreatingARoleIsRefusedWhenTheNameIsTaken is the import rule: importing a role must never
// overwrite one that is already there.
func TestCreatingARoleIsRefusedWhenTheNameIsTaken(t *testing.T) {
	st := newStack(t)
	body := protocol.CreateRoleRequest{
		Name: "Worker",
		Spec: protocol.RoleSpec{Desc: "Someone else's Worker"},
	}
	got := st.do(http.MethodPost, rolesPath, body).
		apiError(t, http.StatusConflict, protocol.ErrorCodeConflict)
	if !strings.Contains(got.Message, "Worker") {
		t.Errorf("the message does not name the role: %q", got.Message)
	}
	if got.Details["name"] != "Worker" {
		t.Errorf("the error does not carry the name, got %v", got.Details)
	}

	// The refused create changed nothing.
	list := decode[protocol.RoleList](t, st.do(http.MethodGet, rolesPath, nil).want(t, http.StatusOK))
	if len(list.Roles) != 8 {
		t.Errorf("after the refused create the list has %d roles, want eight", len(list.Roles))
	}
	if got := roleRow(t, list, "Worker").Spec.Model; got != "claude-sonnet-4-5" {
		t.Errorf("the refused create overwrote Worker's model with %q", got)
	}
}

// TestARoleIsCreatedEditedAndDeletedThroughTheAPI walks the whole life of a role a person makes.
func TestARoleIsCreatedEditedAndDeletedThroughTheAPI(t *testing.T) {
	st := newStack(t)
	created := st.do(http.MethodPost, rolesPath, protocol.CreateRoleRequest{
		Name: "Nightly janitor",
		Spec: protocol.RoleSpec{
			Desc: "Tidies the backlog overnight", Agent: "Built-in agent", Model: "gpt-5-mini",
			Perm: "Plan only", Strength: "Medium", Instr: "Tidy up.",
		},
	}).want(t, http.StatusOK)
	list := decode[protocol.RoleList](t, created)
	if len(list.Roles) != 9 || list.Roles[8].Name != "Nightly janitor" {
		t.Fatalf("after the create the list has %d roles, last %q", len(list.Roles), list.Roles[len(list.Roles)-1].Name)
	}
	if roleRow(t, list, "Nightly janitor").Spec.Skills == nil {
		t.Error("a role with no skills has a null list, want []")
	}

	model := "gpt-5-codex"
	edited := st.do(http.MethodPatch, rolesPath+"/Nightly%20janitor",
		protocol.UpdateRoleRequest{Spec: &protocol.RoleSpec{
			Desc: "Tidies the backlog overnight", Agent: "Built-in agent", Model: model,
			Perm: "Plan only", Strength: "Medium", Instr: "Tidy up.",
		}}).want(t, http.StatusOK)
	if got := roleRow(t, decode[protocol.RoleList](t, edited), "Nightly janitor").Spec.Model; got != model {
		t.Errorf("the model is %q, want %q", got, model)
	}

	renamed := "Morning janitor"
	after := st.do(http.MethodPatch, rolesPath+"/Nightly%20janitor",
		protocol.UpdateRoleRequest{Name: &renamed}).want(t, http.StatusOK)
	list = decode[protocol.RoleList](t, after)
	roleRow(t, list, "Morning janitor")
	if got := roleRow(t, list, "Morning janitor").Spec.Model; got != model {
		t.Errorf("the rename lost the edit: model is %q, want %q", got, model)
	}

	deleted := st.do(http.MethodDelete, rolesPath+"/Morning%20janitor", nil).want(t, http.StatusOK)
	if n := len(decode[protocol.RoleList](t, deleted).Roles); n != 8 {
		t.Errorf("after the delete the list has %d roles, want eight", n)
	}
}

// TestMarshalRolesCannotBeDeleted checks the one rule that keeps the roles the screens are built
// around in place: a starter role is reset, not deleted.
func TestMarshalRolesCannotBeDeleted(t *testing.T) {
	st := newStack(t)
	got := st.do(http.MethodDelete, rolesPath+"/Worker", nil).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if !strings.Contains(got.Message, "Worker") {
		t.Errorf("the message does not name the role: %q", got.Message)
	}
}

// TestOverridingARoleIsPerProjectAndResettingDoesNotRestoreIt checks the override routes: the flag
// follows the project in the query, and a reset clears the flag without putting the role back.
func TestOverridingARoleIsPerProjectAndResettingDoesNotRestoreIt(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")

	override := protocol.RoleSpec{
		Desc: "Does the coding on a card", Agent: "Claude Code", Model: "claude-haiku-4-5",
		Think: "Low", Perm: "Plan only", Strength: "Medium", Instr: "Keep it cheap.",
	}
	body := st.do(http.MethodPut, rolesPath+"/Worker/override?project="+project.ID, override).
		want(t, http.StatusOK)
	list := decode[protocol.RoleList](t, body)
	if !roleRow(t, list, "Worker").Overridden {
		t.Error("the overridden role is not marked overridden")
	}

	// The same list without the project says nothing is overridden.
	plain := decode[protocol.RoleList](t, st.do(http.MethodGet, rolesPath, nil).want(t, http.StatusOK))
	if roleRow(t, plain, "Worker").Overridden {
		t.Error("a role reads as overridden with no project being looked at")
	}

	reset := st.do(http.MethodPost, rolesPath+"/Worker/reset?project="+project.ID, nil).
		want(t, http.StatusOK)
	list = decode[protocol.RoleList](t, reset)
	worker := roleRow(t, list, "Worker")
	if worker.Overridden {
		t.Error("the role still reads as overridden after the reset")
	}
	if worker.Spec.Model != "claude-sonnet-4-5" {
		t.Errorf("the reset changed the role itself: model is %q, want claude-sonnet-4-5", worker.Spec.Model)
	}
}

// TestRoleRoutesRefuseWhatIsNotAllowed checks the refusals a screen can run into.
func TestRoleRoutesRefuseWhatIsNotAllowed(t *testing.T) {
	st := newStack(t)
	// A role with no name cannot be made.
	st.do(http.MethodPost, rolesPath, protocol.CreateRoleRequest{}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	// A limit below zero cannot be made.
	st.do(http.MethodPost, rolesPath, protocol.CreateRoleRequest{
		Name: "Cheap", Spec: protocol.RoleSpec{Limits: protocol.RoleLimits{Cost: -1}},
	}).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	// An override needs the project it is for.
	st.do(http.MethodPut, rolesPath+"/Worker/override", protocol.RoleSpec{}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	// A reset needs the project it is for.
	st.do(http.MethodPost, rolesPath+"/Worker/reset", nil).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	// A role that is not there cannot be edited or deleted.
	st.do(http.MethodPatch, rolesPath+"/Nobody", protocol.UpdateRoleRequest{}).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodDelete, rolesPath+"/Nobody", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}

// TestRoleRoutesAreRegisteredOnlyWhenTheServiceIsThere checks the routes follow the server's rule:
// a route whose service was not given is not registered at all, so it answers 404 and not 500.
func TestRoleRoutesAreRegisteredOnlyWhenTheServiceIsThere(t *testing.T) {
	st := newStack(t, withoutRoles())
	st.do(http.MethodGet, rolesPath, nil).want(t, http.StatusNotFound)
}
