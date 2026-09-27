package roles_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/roles"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The roles module is checked against a real database in a temporary directory, so the rows that are
// written are the rows the daemon would write and the seed runs exactly as it does at start-up. No
// test reaches a provider, Git, or a network.

var testNow = time.Date(2026, time.September, 27, 11, 0, 0, 0, time.UTC)

// openTestStore opens a real database in a temporary directory.
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(),
		filepath.Join(t.TempDir(), "marshal.db"), store.WithLogger(nil))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// newService returns a service over a fresh store, with Marshal's starter roles already added.
func newService(t *testing.T) (*roles.Service, *store.Store) {
	t.Helper()
	st := openTestStore(t)
	svc, err := roles.New(roles.Deps{Store: st}, roles.WithClock(func() time.Time { return testNow }))
	if err != nil {
		t.Fatalf("build the roles service: %v", err)
	}
	if err := svc.EnsureStarters(context.Background()); err != nil {
		t.Fatalf("add the starter roles: %v", err)
	}
	return svc, st
}

// addProject writes a project row, so an override has somewhere to point. Nothing else about a
// project matters to the roles module.
func addProject(t *testing.T, st *store.Store, id string) string {
	t.Helper()
	err := st.Write(context.Background(), func(q *db.Queries) error {
		return q.CreateProject(context.Background(), db.CreateProjectParams{
			ID: id, Name: "api-gateway", RepoPath: "/tmp/api-gateway", DefaultBranch: "main",
			Language: "Go", PackagesJSON: "[]",
			CreatedAt: testNow.UnixMilli(), UpdatedAt: testNow.UnixMilli(),
		})
	})
	if err != nil {
		t.Fatalf("add project %s: %v", id, err)
	}
	return id
}

// names is the names of a list, in order.
func names(list protocol.RoleList) []string {
	out := make([]string, 0, len(list.Roles))
	for _, role := range list.Roles {
		out = append(out, role.Name)
	}
	return out
}

// roleNamed finds a role in a list by name.
func roleNamed(t *testing.T, list protocol.RoleList, name string) protocol.Role {
	t.Helper()
	for _, role := range list.Roles {
		if role.Name == name {
			return role
		}
	}
	t.Fatalf("no role called %q in %v", name, names(list))
	return protocol.Role{}
}

// TestEnsureStartersAddsMarshalRolesOnce checks that the eight starter roles are added, in the order
// Marshal ships them, that a second call adds nothing, and that a person's edit survives a restart.
func TestEnsureStartersAddsMarshalRolesOnce(t *testing.T) {
	svc, _ := newService(t)

	list, err := svc.Roles(context.Background(), "")
	if err != nil {
		t.Fatalf("list the roles: %v", err)
	}
	want := []string{
		"Orchestrator", "Worker", "Reviewer", "Integrator",
		"Tester", "Docs writer", "Security checker", "UI checker",
	}
	if got := names(list); len(got) != len(want) {
		t.Fatalf("the starter roles are %v, want %v", got, want)
	}
	for i, name := range want {
		if list.Roles[i].Name != name {
			t.Errorf("role %d is %q, want %q", i, list.Roles[i].Name, name)
		}
		if !list.Roles[i].Starter {
			t.Errorf("%s is not marked a starter role", name)
		}
		if list.Roles[i].Overridden {
			t.Errorf("%s reads as overridden with no project being looked at", name)
		}
		if list.Roles[i].ID == "" {
			t.Errorf("%s has no id", name)
		}
	}

	// The Worker role is the prototype's, field for field (apps/web/src/mock/seed/settings.ts).
	worker := roleNamed(t, list, "Worker")
	wantWorker := protocol.RoleSpec{
		Skills: []string{"conventional-commits"}, MCP: []string{"marshal", "github"},
		Limits: protocol.RoleLimits{Time: 60, Cost: 5, Rounds: 12}, Backup: "gpt-5",
		Desc: "Does the coding on a card", Agent: "Claude Code", Model: "claude-sonnet-4-5",
		Think: "Medium", Perm: "Auto-accept edits", Strength: "Your choice",
		Instr: "You do the coding on one card. Stay inside your worktree, claim the files you change, " +
			"and keep commits small.",
	}
	if !sameSpec(worker.Spec, wantWorker) {
		t.Errorf("Worker is %+v, want %+v", worker.Spec, wantWorker)
	}

	// A person edits a role, and a second start-up must leave the edit alone.
	renamed := "Worker (mine)"
	if _, err := svc.UpdateRole(context.Background(), "Worker", protocol.UpdateRoleRequest{Name: &renamed}); err != nil {
		t.Fatalf("rename Worker: %v", err)
	}
	if err := svc.EnsureStarters(context.Background()); err != nil {
		t.Fatalf("add the starter roles again: %v", err)
	}
	list, err = svc.Roles(context.Background(), "")
	if err != nil {
		t.Fatalf("list the roles: %v", err)
	}
	if len(list.Roles) != len(want) {
		t.Fatalf("a second start-up left %v, want the same eight roles", names(list))
	}
	for _, role := range list.Roles {
		if role.Name == "Worker" {
			t.Fatalf("a second start-up added Worker back beside the person's rename")
		}
	}
}

// TestCreateRoleAddsANewRoleAtTheEnd checks a role a person makes sits after Marshal's own, that its
// lists are never null, and that its name must be free.
func TestCreateRoleAddsANewRoleAtTheEnd(t *testing.T) {
	svc, _ := newService(t)
	in := protocol.CreateRoleRequest{
		Name: "  Nightly janitor  ",
		Spec: protocol.RoleSpec{
			Desc: "Tidies the backlog overnight", Agent: "Built-in agent", Model: "gpt-5-mini",
			Perm: "Plan only", Strength: "Medium", Instr: "Tidy up.",
		},
	}
	list, err := svc.CreateRole(context.Background(), "", in)
	if err != nil {
		t.Fatalf("create the role: %v", err)
	}
	if len(list.Roles) != 9 || list.Roles[8].Name != "Nightly janitor" {
		t.Fatalf("after the create the roles are %v, want the eight starters then Nightly janitor", names(list))
	}
	made := list.Roles[8]
	if made.Starter {
		t.Error("a role a person made is marked a starter role")
	}
	if made.Spec.Skills == nil || made.Spec.MCP == nil {
		t.Error("a role with no skills and no MCP servers has a null list, want []")
	}

	if _, err := svc.CreateRole(context.Background(), "", in); err == nil {
		t.Error("a second role with the same name was allowed")
	}
}

// TestCreateRoleRefusesWhatCannotBeSaved checks the refusals that happen before the store is touched.
func TestCreateRoleRefusesWhatCannotBeSaved(t *testing.T) {
	svc, _ := newService(t)
	cases := []struct {
		what string
		in   protocol.CreateRoleRequest
	}{
		{"an empty name", protocol.CreateRoleRequest{}},
		{"a name with a slash", protocol.CreateRoleRequest{Name: "Docs/writer"}},
		{"a name that is too long", protocol.CreateRoleRequest{Name: strings.Repeat("x", 61)}},
		{"a negative limit", protocol.CreateRoleRequest{Name: "Cheap", Spec: protocol.RoleSpec{
			Limits: protocol.RoleLimits{Cost: -1},
		}}},
		{"an empty skill name", protocol.CreateRoleRequest{Name: "Tidy", Spec: protocol.RoleSpec{
			Skills: []string{"go-testing", " "},
		}}},
	}
	for _, c := range cases {
		if _, err := svc.CreateRole(context.Background(), "", c.in); err == nil {
			t.Errorf("%s was allowed", c.what)
		}
	}
}

// TestUpdateRoleChangesOnlyWhatIsSent checks a rename, a spec replacement, and a body that sets
// nothing at all.
func TestUpdateRoleChangesOnlyWhatIsSent(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	if _, err := svc.UpdateRole(ctx, "Worker", protocol.UpdateRoleRequest{}); err != nil {
		t.Fatalf("update with nothing set: %v", err)
	}
	before, err := svc.Role(ctx, "Worker", "")
	if err != nil {
		t.Fatalf("read Worker: %v", err)
	}

	spec := before.Spec
	spec.Model = "gpt-5-codex"
	list, err := svc.UpdateRole(ctx, "Worker", protocol.UpdateRoleRequest{Spec: &spec})
	if err != nil {
		t.Fatalf("replace the spec: %v", err)
	}
	edited := roleNamed(t, list, "Worker")
	if edited.Spec.Model != "gpt-5-codex" {
		t.Errorf("the model is %q, want gpt-5-codex", edited.Spec.Model)
	}
	if edited.Spec.Desc != before.Spec.Desc {
		t.Errorf("the description changed to %q", edited.Spec.Desc)
	}

	renamed := "Implementer"
	list, err = svc.UpdateRole(ctx, "Worker", protocol.UpdateRoleRequest{Name: &renamed})
	if err != nil {
		t.Fatalf("rename Worker: %v", err)
	}
	if _, err := svc.Role(ctx, "Worker", ""); err == nil {
		t.Error("the old name still names a role")
	}
	if got := roleNamed(t, list, "Implementer").Spec.Model; got != "gpt-5-codex" {
		t.Errorf("the rename lost the model change: %q", got)
	}

	taken := "Reviewer"
	if _, err := svc.UpdateRole(ctx, "Implementer", protocol.UpdateRoleRequest{Name: &taken}); err == nil {
		t.Error("a role was renamed onto another role's name")
	}
}

// TestDeleteRoleRefusesAStarterRoleAndRemovesAPersonsRole checks the one rule that keeps the eight
// roles the screens are built around in place.
func TestDeleteRoleRefusesAStarterRoleAndRemovesAPersonsRole(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	if _, err := svc.DeleteRole(ctx, "Worker", ""); err == nil {
		t.Error("a starter role was deleted")
	}
	if _, err := svc.CreateRole(ctx, "", protocol.CreateRoleRequest{Name: "Nightly janitor"}); err != nil {
		t.Fatalf("create a role: %v", err)
	}
	list, err := svc.DeleteRole(ctx, "Nightly janitor", "")
	if err != nil {
		t.Fatalf("delete the role: %v", err)
	}
	if len(list.Roles) != 8 {
		t.Fatalf("after the delete the roles are %v, want the eight starters", names(list))
	}
}

// TestOverrideResetClearsTheFlagAndLeavesTheSpec checks the rule the prototype's confirmResetRole
// follows: a reset clears the overridden flag and does not put the starter text back.
func TestOverrideResetClearsTheFlagAndLeavesTheSpec(t *testing.T) {
	svc, st := newService(t)
	ctx := context.Background()
	project := addProject(t, st, "01JD7Q4M2X8K9V0P5T3RB6NHAE")

	if _, err := svc.CreateRole(ctx, "", protocol.CreateRoleRequest{
		Name: "Nightly janitor", Spec: protocol.RoleSpec{Instr: "Tidy up."},
	}); err != nil {
		t.Fatalf("create a role: %v", err)
	}
	spec, err := svc.Role(ctx, "Nightly janitor", "")
	if err != nil {
		t.Fatalf("read the role: %v", err)
	}
	overridable := spec.Spec
	overridable.Instr = "Tidy up, but only in the mornings."

	list, err := svc.SetRoleOverride(ctx, "Nightly janitor", project, overridable)
	if err != nil {
		t.Fatalf("override the role: %v", err)
	}
	if !roleNamed(t, list, "Nightly janitor").Overridden {
		t.Error("the overridden role does not read as overridden")
	}
	if roleNamed(t, list, "Worker").Overridden {
		t.Error("a role the project does not override reads as overridden")
	}

	// A list with no project being looked at never says a role is overridden.
	plain, err := svc.Roles(ctx, "")
	if err != nil {
		t.Fatalf("list the roles: %v", err)
	}
	for _, role := range plain.Roles {
		if role.Overridden {
			t.Errorf("%s reads as overridden with no project being looked at", role.Name)
		}
	}

	list, err = svc.ResetRole(ctx, "Nightly janitor", project)
	if err != nil {
		t.Fatalf("reset the role: %v", err)
	}
	reset := roleNamed(t, list, "Nightly janitor")
	if reset.Overridden {
		t.Error("the role still reads as overridden after the reset")
	}
	if reset.Spec.Instr != "Tidy up." {
		t.Errorf("the reset changed the role's own instructions to %q, want the role left as it was", reset.Spec.Instr)
	}

	// Resetting again is not an error: the answer is the same list.
	if _, err := svc.ResetRole(ctx, "Nightly janitor", project); err != nil {
		t.Fatalf("reset twice: %v", err)
	}
}

// TestOverrideNeedsARealProject checks the two refusals an override can get: no project named, and a
// project that is not there.
func TestOverrideNeedsARealProject(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	if _, err := svc.SetRoleOverride(ctx, "Worker", "", protocol.RoleSpec{}); err == nil {
		t.Error("an override with no project was allowed")
	}
	if _, err := svc.SetRoleOverride(ctx, "Worker", "01JD7Q4M2X8K9V0P5T3RB6NHAE", protocol.RoleSpec{}); err == nil {
		t.Error("an override for a project that is not there was allowed")
	}
	if _, err := svc.ResetRole(ctx, "Worker", ""); err == nil {
		t.Error("a reset with no project was allowed")
	}
	if _, err := svc.Role(ctx, "Nobody", ""); err == nil {
		t.Error("a role that is not there was read")
	}
}

// TestAnExportedRoleImports checks the shape that moves between machines: the document a role exports
// is the document an import posts, so the two cannot drift apart.
func TestAnExportedRoleImports(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	exported, err := svc.Role(ctx, "Reviewer", "")
	if err != nil {
		t.Fatalf("read Reviewer: %v", err)
	}
	list, err := svc.CreateRole(ctx, "", protocol.CreateRoleRequest{Name: "Reviewer (mine)", Spec: exported.Spec})
	if err != nil {
		t.Fatalf("import the role: %v", err)
	}
	imported := roleNamed(t, list, "Reviewer (mine)")
	if imported.Starter {
		t.Error("an imported role is marked a starter role")
	}
	if !sameSpec(imported.Spec, exported.Spec) {
		t.Errorf("the imported role is %+v, want %+v", imported.Spec, exported.Spec)
	}
}

// sameSpec compares two role bodies, treating a nil list and an empty list as the same thing.
func sameSpec(a, b protocol.RoleSpec) bool {
	return slices(a.Skills) == slices(b.Skills) && slices(a.MCP) == slices(b.MCP) &&
		a.Limits == b.Limits && a.Backup == b.Backup && a.Desc == b.Desc && a.Agent == b.Agent &&
		a.Model == b.Model && a.Think == b.Think && a.Perm == b.Perm && a.Strength == b.Strength &&
		a.Instr == b.Instr
}

// slices renders a list of names so two lists can be compared as text.
func slices(list []string) string {
	out := ""
	for i, each := range list {
		if i > 0 {
			out += "\x00"
		}
		out += each
	}
	return out
}
