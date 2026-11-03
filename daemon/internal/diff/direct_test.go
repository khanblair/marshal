package diff_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/diff"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

func TestDirectCardDiffUsesItsSnapshotAndStaysFrozenAfterTheTurn(t *testing.T) {
	ctx := context.Background()
	g := testutil.Git()
	repo := testutil.Fixture(t, "small-repo")
	writeFile := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("owners.txt", "untracked before the card\n")
	const cardID = "01M3C107JB041061050R3GG28A"
	if _, err := g.SnapshotFolder(ctx, repo, gitx.DirectBaseRef(cardID), "start"); err != nil {
		t.Fatal(err)
	}
	working := protocol.SessionStateWorking
	p := &projects{
		card:    protocol.Card{ID: cardID, ProjectID: "small-repo", State: protocol.CardStateWorking, Session: &working},
		project: protocol.Project{ID: "small-repo", Path: repo, DefaultBranch: "main"},
		path:    repo, branch: "main",
	}
	s, err := diff.New(diff.Deps{Projects: p, Git: g})
	if err != nil {
		t.Fatal(err)
	}
	writeFile("task.txt", "this card\n")
	answer, err := s.Files(ctx, cardID)
	if err != nil || len(answer.Files) != 1 || answer.Files[0].Path != "task.txt" {
		t.Fatalf("live direct diff = %+v, %v", answer, err)
	}
	if _, err := g.SnapshotFolder(ctx, repo, gitx.DirectResultRef(cardID), "end turn"); err != nil {
		t.Fatal(err)
	}
	p.card.State = protocol.CardStateDone
	p.card.Session = nil
	writeFile("later.txt", "another card\n")
	writeFile("task.txt", "later edit\n")
	answer, err = s.Files(ctx, cardID)
	if err != nil || len(answer.Files) != 1 || answer.Files[0].Path != "task.txt" {
		t.Fatalf("frozen direct diff = %+v, %v", answer, err)
	}
	hunks, err := s.Hunks(ctx, cardID, "task.txt")
	if err != nil || len(hunks.Hunks) != 1 || hunks.Hunks[0].Lines[0].Text != "this card" {
		t.Fatalf("frozen direct hunks = %+v, %v", hunks, err)
	}
}
