package protocol_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// sampleRoles has a starter role as Marshal ships it, a starter role the project being looked at
// overrides, and a role a person made by duplicating one, so the golden file shows both flags, a
// role with no skills and no MCP servers, and a role whose limits are all zero.
func sampleRoles() []protocol.Role {
	return []protocol.Role{
		{
			ID: "01JD7Q4M2X8K9V0P5T3RB6NHAE", Name: "Worker", Starter: true, Overridden: false,
			Spec: protocol.RoleSpec{
				Skills: []string{"conventional-commits"}, MCP: []string{"marshal", "github"},
				Limits: protocol.RoleLimits{Time: 60, Cost: 5, Rounds: 12}, Backup: "gpt-5",
				Desc: "Does the coding on a card", Agent: "Claude Code", Model: "claude-sonnet-4-5",
				Think: "Medium", Perm: "Auto-accept edits", Strength: "Your choice",
				Instr: "You do the coding on one card.",
			},
		},
		{
			ID: "01JD7Q4M2X8K9V0P5T3RB6NHB7", Name: "Reviewer", Starter: true, Overridden: true,
			Spec: protocol.RoleSpec{
				Skills: []string{"conventional-commits"}, MCP: []string{"marshal", "github"},
				Limits: protocol.RoleLimits{Time: 30, Cost: 3, Rounds: 6}, Backup: "gpt-5",
				Desc: "Reviews every pull request before you do", Agent: "Claude Code",
				Model: "claude-opus-4-1", Think: "High", Perm: "Plan only", Strength: "Strong",
				Instr: "Review the diff against the card's task and acceptance checks.",
			},
		},
		{
			ID: "01JD7Q4M2X8K9V0P5T3RB6NHC3", Name: "Nightly janitor", Starter: false, Overridden: false,
			Spec: protocol.RoleSpec{
				Skills: []string{}, MCP: []string{}, Limits: protocol.RoleLimits{},
				Desc: "Tidies the backlog overnight", Agent: "Built-in agent", Model: "gpt-5-mini",
				Strength: "Medium", Instr: "Tidy up.",
			},
		},
	}
}

func TestRoleListGolden(t *testing.T) {
	testutil.Golden(t, "role-list", protocol.NewRoleList(sampleRoles(), providersNow))
}

func TestCreateRoleRequestGolden(t *testing.T) {
	testutil.Golden(t, "create-role-request", protocol.CreateRoleRequest{
		Name: "Nightly janitor",
		Spec: protocol.RoleSpec{
			Skills: []string{}, MCP: []string{"marshal"}, Limits: protocol.RoleLimits{Time: 10, Cost: 1, Rounds: 3},
			Backup: "gpt-5", Desc: "Tidies the backlog overnight", Agent: "Built-in agent",
			Model: "gpt-5-mini", Think: "Low", Perm: "Plan only", Strength: "Medium", Instr: "Tidy up.",
		},
	})
}

func TestRoleListNeverEncodesNull(t *testing.T) {
	list, err := json.Marshal(protocol.NewRoleList(nil, providersNow))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(list); !strings.Contains(got, `"roles":[]`) {
		t.Errorf("an empty role list encoded as %s, want []", got)
	}
}
