package roles

import (
	"context"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The eight roles Marshal ships. They are the prototype's own starter roles, word for word
// (apps/web/src/mock/seed/settings.ts): the same names in the same order, the same description, agent,
// model, thinking label, permission label, strength, and instructions, the same skills and MCP
// servers, the same three limits, and the same backup model. The order is the order the screens show
// them in, so it is kept here and the table is seeded in it.
//
// A person may edit any of them. Editing a starter role changes its spec in place; there is no second
// copy to fall back to, which is why resetting a role clears only the overridden flag.
var starterRoles = []starter{
	{
		name: "Orchestrator",
		spec: protocol.RoleSpec{
			Skills: []string{"conventional-commits"}, MCP: starterMCP, Limits: starterLimits, Backup: starterBackup,
			Desc:  "Plans work and splits goals into cards",
			Agent: "Claude Code", Model: "claude-opus-4-1", Think: "High",
			Perm: "Plan only", Strength: "Strong or medium",
			Instr: "You plan work for this project. Explore the codebase and memory, propose a plan, " +
				"and create cards with the right roles and dependencies after the user approves.",
		},
	},
	{
		name: "Worker",
		spec: protocol.RoleSpec{
			Skills: []string{"conventional-commits"}, MCP: starterMCP, Limits: starterLimits, Backup: starterBackup,
			Desc:  "Does the coding on a card",
			Agent: "Claude Code", Model: "claude-sonnet-4-5", Think: "Medium",
			Perm: "Auto-accept edits", Strength: "Your choice",
			Instr: "You do the coding on one card. Stay inside your worktree, claim the files you change, " +
				"and keep commits small.",
		},
	},
	{
		name: "Reviewer",
		spec: protocol.RoleSpec{
			Skills: []string{"conventional-commits"}, MCP: starterMCP, Limits: starterLimits, Backup: starterBackup,
			Desc:  "Reviews every pull request before you do",
			Agent: "Claude Code", Model: "claude-opus-4-1", Think: "High",
			Perm: "Plan only", Strength: "Strong",
			Instr: "Review the diff against the card's task and acceptance checks. Leave specific comments. " +
				"Approve only when every check passes.",
		},
	},
	{
		name: "Integrator",
		spec: protocol.RoleSpec{
			Skills: []string{"conventional-commits"}, MCP: starterMCP, Limits: starterLimits, Backup: starterBackup,
			Desc:  "Merges finished work into the target branch",
			Agent: "Claude Code", Model: "claude-opus-4-1", Think: "High",
			Perm: "Full auto", Strength: "Strong",
			Instr: "Merge one card at a time. Dry-run with git merge-tree first. Resolve conflicts by intent. " +
				"Stop and explain when you are not confident.",
		},
	},
	{
		name: "Tester",
		spec: protocol.RoleSpec{
			Skills: []string{"go-testing", "playwright"}, MCP: starterMCP, Limits: starterLimits, Backup: starterBackup,
			Desc:  "Writes and runs tests",
			Agent: "Codex", Model: "gpt-5-codex", Think: "Medium",
			Perm: "Full auto", Strength: "Medium",
			Instr: "Write focused tests for the change and run them. Report flaky tests separately from " +
				"real failures.",
		},
	},
	{
		name: "Docs writer",
		spec: protocol.RoleSpec{
			Skills: []string{"conventional-commits"}, MCP: starterMCP, Limits: starterLimits, Backup: starterBackup,
			Desc:  "Writes and updates documentation",
			Agent: "Built-in agent", Model: "claude-haiku-4-5", Think: "Low",
			Perm: "Auto-accept edits", Strength: "Medium",
			Instr: "Update docs to match the code. Use plain, short sentences.",
		},
	},
	{
		name: "Security checker",
		spec: protocol.RoleSpec{
			Skills: []string{"conventional-commits"}, MCP: starterMCP, Limits: starterLimits, Backup: starterBackup,
			Desc:  "Looks for security problems in changes",
			Agent: "Claude Code", Model: "claude-opus-4-1", Think: "Extra high",
			Perm: "Plan only", Strength: "Strong",
			Instr: "Look for injection, auth bypass, secrets, and unsafe dependencies in the diff.",
		},
	},
	{
		name: "UI checker",
		spec: protocol.RoleSpec{
			Skills: []string{"screenshot-compare"}, MCP: starterMCP, Limits: starterLimits, Backup: starterBackup,
			Desc:  "Checks UI changes with previews and screenshots",
			Agent: "Claude Code", Model: "claude-sonnet-4-5", Think: "Medium",
			Perm: "Plan only", Strength: "Medium",
			Instr: "Open the live preview, capture before and after screenshots, and compare them against " +
				"the card.",
		},
	},
}

// starter is one role Marshal ships: what it is called and its body.
type starter struct {
	name string
	spec protocol.RoleSpec
}

// The three values every starter role shares, spelled out once so a change to one of them is one
// change. They are copied into each role rather than shared, so a later edit of a role cannot reach
// another role's list.
var (
	starterMCP    = []string{"marshal", "github"}
	starterLimits = protocol.RoleLimits{Time: 60, Cost: 5, Rounds: 12}
	starterBackup = "gpt-5"
)

// EnsureStarters adds any of Marshal's starter roles that are missing. It is safe to call at every
// start-up.
//
// A person may edit a starter role's name as well as its body, so a starter role is not found by its
// name: Marshal's starter roles are the ones marked a starter role, and they are added until there
// are as many of them as Marshal ships. That way a role a person renamed is not added back beside
// their rename, their edits survive every start-up, and a role is never doubled.
//
// It is written to leave the table alone once the starters are all there, so a normal start-up writes
// nothing.
func (s *Service) EnsureStarters(ctx context.Context) error {
	added := 0
	err := s.store.Write(ctx, func(q *db.Queries) error {
		rows, err := q.ListRoles(ctx)
		if err != nil {
			return fmt.Errorf("list roles: %w", err)
		}
		have := 0
		for _, row := range rows {
			if row.IsStarter != 0 {
				have++
			}
		}
		// A table written by a newer version of Marshal could hold more starter roles than this
		// one knows; there is then nothing to add.
		if have >= len(starterRoles) {
			return nil
		}
		for _, role := range starterRoles[have:] {
			spec, err := encodeSpec(role.spec)
			if err != nil {
				return err
			}
			id, err := s.newID()
			if err != nil {
				return fmt.Errorf("make an id for the starter role %q: %w", role.name, err)
			}
			params := db.CreateStarterRoleParams{ID: id, Name: role.name, SpecJSON: spec}
			changed, err := q.CreateStarterRole(ctx, params)
			if err != nil {
				return fmt.Errorf("add the starter role %q: %w", role.name, err)
			}
			added += int(changed)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if added > 0 {
		s.log.Info("added the starter roles", "count", added)
	}
	return nil
}
