package roles_test

import (
	"context"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/roles"
)

// LimitsFor is what the session manager's harness reads a card's role through (B5.3, build-plan
// 5.3). It reads a role when a card runs, so the ceilings it answers with are always the current
// ones, and it is never an error for a role not to be there: a card naming a role somebody deleted
// runs unlimited rather than being stopped for a limit nobody set.

// A starter role's own ceilings are read and mapped onto the harness's names for the same three
// numbers.
func TestAStarterRolesLimitsAreRead(t *testing.T) {
	svc, _ := newService(t)
	limits, found, err := svc.LimitsFor(context.Background(), "", "Worker")
	if err != nil {
		t.Fatalf("LimitsFor: %v", err)
	}
	if !found {
		t.Fatal("the Worker role was not found")
	}
	want := harness.Limits{TurnMinutes: 60, CostDollars: 5, Rounds: 12}
	if limits != want {
		t.Errorf("limits = %+v, want %+v", limits, want)
	}
}

// A role that is not there, and a name that is not a role's, are not errors: they report false, and
// the caller limits nothing.
func TestAnUnknownRoleIsNotAnError(t *testing.T) {
	svc, _ := newService(t)
	for _, name := range []string{"", "   ", "No Such Role"} {
		limits, found, err := svc.LimitsFor(context.Background(), "", name)
		if err != nil {
			t.Fatalf("LimitsFor(%q): %v", name, err)
		}
		if found {
			t.Errorf("LimitsFor(%q) found a role", name)
		}
		if !limits.IsZero() {
			t.Errorf("LimitsFor(%q) = %+v, want no ceilings", name, limits)
		}
	}
}

// A role a person made is read like any other, with whatever ceilings it was saved with.
func TestARoleAPersonMadeIsRead(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.CreateRole(context.Background(), "", protocol.CreateRoleRequest{
		Name: "Night shift",
		Spec: protocol.RoleSpec{
			Desc: "Runs the long jobs", Agent: "claude", Model: "opus",
			Instr: "Work slowly and carefully.", Limits: protocol.RoleLimits{Time: 90, Cost: 25, Rounds: 4},
		},
	})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	limits, found, err := svc.LimitsFor(context.Background(), "", "Night shift")
	if err != nil {
		t.Fatalf("LimitsFor: %v", err)
	}
	if !found {
		t.Fatal("the role a person made was not found")
	}
	want := harness.Limits{TurnMinutes: 90, CostDollars: 25, Rounds: 4}
	if limits != want {
		t.Errorf("limits = %+v, want %+v", limits, want)
	}
}

// A project's own version of a role is what its cards run under, so the override's ceilings are
// what LimitsFor answers for that project - and the role's own are still what the rest see.
func TestAProjectsOwnRoleWinsForThatProjectOnly(t *testing.T) {
	svc, st := newService(t)
	project := addProject(t, st, "01M3C107JB041061050R3GG28A")

	worker, err := svc.Role(context.Background(), "Worker", "")
	if err != nil {
		t.Fatalf("Role: %v", err)
	}
	override := worker.Spec
	override.Instr = "This project's own Worker."
	override.Limits = protocol.RoleLimits{Time: 15, Cost: 2, Rounds: 3}
	if _, err := svc.SetRoleOverride(context.Background(), "Worker", project, override); err != nil {
		t.Fatalf("SetRoleOverride: %v", err)
	}

	limits, found, err := svc.LimitsFor(context.Background(), project, "Worker")
	if err != nil {
		t.Fatalf("LimitsFor: %v", err)
	}
	if !found {
		t.Fatal("the overridden role was not found")
	}
	if want := (harness.Limits{TurnMinutes: 15, CostDollars: 2, Rounds: 3}); limits != want {
		t.Errorf("the project's limits = %+v, want %+v", limits, want)
	}

	// Another project, and no project at all, still read the role's own ceilings.
	for _, other := range []string{"", "01M3C107JB041061050R3GG28C"} {
		limits, found, err := svc.LimitsFor(context.Background(), other, "Worker")
		if err != nil {
			t.Fatalf("LimitsFor(%q): %v", other, err)
		}
		if !found {
			t.Fatalf("the role was not found for %q", other)
		}
		if want := (harness.Limits{TurnMinutes: 60, CostDollars: 5, Rounds: 12}); limits != want {
			t.Errorf("the limits for %q = %+v, want the role's own %+v", other, limits, want)
		}
	}
}

// A role whose ceilings are all zero is still a role: it is found, and its ceilings limit nothing.
func TestARoleWithNoCeilingsIsStillFound(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.CreateRole(context.Background(), "", protocol.CreateRoleRequest{
		Name: "Unlimited",
		Spec: protocol.RoleSpec{Desc: "No ceilings at all", Agent: "claude", Model: "opus", Instr: "Go."},
	})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	limits, found, err := svc.LimitsFor(context.Background(), "", "Unlimited")
	if err != nil {
		t.Fatalf("LimitsFor: %v", err)
	}
	if !found {
		t.Fatal("a role with no ceilings was not found")
	}
	if !limits.IsZero() {
		t.Errorf("limits = %+v, want no ceilings", limits)
	}
}

// The service satisfies the interface the session manager names, so the daemon can wire one to the
// other without either package knowing the other's types.
var _ interface {
	LimitsFor(ctx context.Context, projectID, roleName string) (harness.Limits, bool, error)
} = (*roles.Service)(nil)
