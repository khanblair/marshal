package projects

import (
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

func TestToProjectFillsTheEffectiveIntegrationBranch(t *testing.T) {
	tests := []struct {
		name, defaultBranch, chosen, want string
	}{
		{"nothing chosen", "main", "", "main"},
		{"a choice", "main", "development", "development"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			row := db.Project{ID: "api", DefaultBranch: tc.defaultBranch, IntegrationBranch: tc.chosen, PackagesJSON: "[]"}
			got, err := toProject(row, protocol.ProjectBadges{})
			if err != nil {
				t.Fatal(err)
			}
			if got.IntegrationBranch != tc.want || got.Target() != tc.want || got.DefaultBranch != tc.defaultBranch {
				t.Errorf("project = %+v, want integration branch %q and default %q", got, tc.want, tc.defaultBranch)
			}
		})
	}
}

func TestToCardCarriesTheWorktreeAndTheMergeFields(t *testing.T) {
	card := toCard(db.Card{
		ID: "c1", ProjectID: "api", Number: 4, WorktreePath: "/data/worktrees/api/c1",
		MergePhase: "resolving", MergeNote: "Resolving 2 conflicts",
	}, nil)
	if card.Worktree != "/data/worktrees/api/c1" || card.MergePhase != protocol.MergePhaseResolving ||
		card.MergeNote != "Resolving 2 conflicts" {
		t.Errorf("card = worktree %q, phase %q, note %q", card.Worktree, card.MergePhase, card.MergeNote)
	}
	plain := toCard(db.Card{ID: "c2", ProjectID: "api", Number: 5}, nil)
	if plain.Worktree != "" || plain.MergePhase != "" || plain.MergeNote != "" {
		t.Errorf("a card that was never started or merged carries %q, %q, %q", plain.Worktree, plain.MergePhase, plain.MergeNote)
	}
}
