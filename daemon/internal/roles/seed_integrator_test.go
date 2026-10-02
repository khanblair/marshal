package roles_test

import (
	"context"
	"strings"
	"testing"
)

// TestIntegratorStarterRoleDescribesTheOneSessionFlow checks that the Integrator's instructions name
// the three tools the chats workstream registers and the rules the agent must keep, and that the
// role's other fields are the ones it has always had.
func TestIntegratorStarterRoleDescribesTheOneSessionFlow(t *testing.T) {
	svc, _ := newService(t)
	list, err := svc.Roles(context.Background(), "")
	if err != nil {
		t.Fatalf("list the roles: %v", err)
	}
	spec := roleNamed(t, list, "Integrator").Spec

	for _, want := range []string{
		"merge_context", "merge_report", "ask_owner", "task_id", "integrator workspace", "one session per project",
		"by intent", "git add", "Never commit, switch branches, push", "confident to false", "questions",
		"owner's uncommitted work", "prefer the owner's version",
	} {
		if !strings.Contains(spec.Instr, want) {
			t.Errorf("the Integrator's instructions lack %q: %s", want, spec.Instr)
		}
	}
	if strings.Contains(spec.Instr, "merge-tree") {
		t.Errorf("the Integrator's instructions still tell it to dry-run the merge: %s", spec.Instr)
	}
	if spec.Desc == "" || strings.Contains(spec.Desc, "Merges finished work") {
		t.Errorf("the Integrator's description is %q", spec.Desc)
	}
	if spec.Agent != "Claude Code" || spec.Think != "High" || spec.Perm != "Full auto" || spec.Strength != "Strong" {
		t.Errorf("the Integrator's other fields changed: %+v", spec)
	}
}
