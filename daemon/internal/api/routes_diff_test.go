package api_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The card diff routes (docs/backend-checklist.md B2.9, N15). They read a real worktree of the
// project's repository, so the file list and the hunks are Git's own answers.

// startWorktree gives a card a worktree of its project's repository and records it on the card, the
// way starting the card's session does, and returns the folder the agent would work in.
func (st *stack) startWorktree(t *testing.T, project protocol.Project, repo string, card protocol.Card) string {
	t.Helper()
	ctx := context.Background()
	branch := gitx.CardBranchName(project.ID, card.Number, card.Title)
	dir := filepath.Join(projects.WorktreesDir(st.dataDir, project.ID), card.ID)
	spec := gitx.WorktreeSpec{Path: dir, Branch: branch, Base: project.DefaultBranch}
	if err := st.git.AddWorktree(ctx, repo, spec); err != nil {
		t.Fatalf("make a worktree for card %s: %v", card.ID, err)
	}
	if _, err := st.proj.SetWorktree(ctx, card.ID, dir, branch); err != nil {
		t.Fatalf("record the worktree of card %s: %v", card.ID, err)
	}
	return dir
}

// edit writes a file inside a worktree, making its folders, the way an agent edits a file.
func edit(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// changedCard is a card whose worktree holds one edited file and one new one.
func (st *stack) changedCard(t *testing.T) protocol.Card {
	t.Helper()
	project, repo := st.addProject("small-repo")
	card := st.addCard(project.ID, "Refresh the token")
	dir := st.startWorktree(t, project, repo, card)
	edit(t, dir, "src/util.js", "/** Adds two numbers. */\nexport function add(a, b) {\n  return a + b + 1;\n}\n")
	edit(t, dir, "src/new.js", "export const one = 1;\nexport const two = 2;\n")
	return card
}

func TestCardDiffThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, repo := st.addProject("small-repo")
	card := st.addCard(project.ID, "Refresh the token")
	dir := st.startWorktree(t, project, repo, card)
	edit(t, dir, "src/util.js", "/** Adds two numbers. */\nexport function add(a, b) {\n  return a + b + 1;\n}\n")
	edit(t, dir, "src/new.js", "export const one = 1;\n")

	got := st.do(http.MethodGet, "/v1/cards/"+card.ID+"/diff", nil).want(t, http.StatusOK)
	sameShape(t, "card-diff", got.Body)
	answer := decode[protocol.CardDiff](t, got)
	if answer.CardID != card.ID || answer.Base != project.DefaultBranch || answer.Branch == "" {
		t.Errorf("the answer = %+v, want the card, the base branch, and the card's branch", answer)
	}
	if answer.Truncated || answer.FileCount != 2 || len(answer.Files) != 2 {
		t.Fatalf("the answer = %+v, want two whole files", answer)
	}
	if answer.Additions != 2 || answer.Deletions != 1 {
		t.Errorf("the totals = +%d -%d, want +2 -1", answer.Additions, answer.Deletions)
	}
	if answer.Files[0].Path != "src/new.js" || answer.Files[0].Status != protocol.DiffFileStatusAdded {
		t.Errorf("the first file = %+v, want the new file", answer.Files[0])
	}
	if answer.Files[1].Status != protocol.DiffFileStatusModified || answer.Files[1].Large {
		t.Errorf("the second file = %+v, want a small change", answer.Files[1])
	}
}

func TestCardFileHunksThroughHTTP(t *testing.T) {
	st := newStack(t)
	card := st.changedCard(t)

	got := st.do(http.MethodGet, "/v1/cards/"+card.ID+"/diff/src/util.js", nil).want(t, http.StatusOK)
	sameShape(t, "file-hunks", got.Body)
	answer := decode[protocol.FileHunks](t, got)
	if answer.Path != "src/util.js" || answer.Status != protocol.DiffFileStatusModified || answer.Truncated {
		t.Fatalf("the answer = %+v, want one whole changed file", answer)
	}
	if len(answer.Hunks) != 1 || len(answer.Hunks[0].Lines) == 0 {
		t.Fatalf("the hunks = %+v", answer.Hunks)
	}
	// The new file's hunks come from the file itself, since it has no other side in the diff.
	added := decode[protocol.FileHunks](t,
		st.do(http.MethodGet, "/v1/cards/"+card.ID+"/diff/src/new.js", nil).want(t, http.StatusOK))
	if added.Status != protocol.DiffFileStatusAdded || added.Hunks[0].Lines[1].Text != "export const two = 2;" {
		t.Errorf("the new file's hunks = %+v", added)
	}
}

// A card that never started has no worktree, and that is an empty diff rather than an error: the
// Diff tab draws its empty state for such a card.
func TestCardDiffOfACardThatNeverStarted(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Refresh the token")

	got := st.do(http.MethodGet, "/v1/cards/"+card.ID+"/diff", nil).want(t, http.StatusOK)
	sameShape(t, "card-diff-empty", got.Body)
	answer := decode[protocol.CardDiff](t, got)
	if answer.Files == nil || len(answer.Files) != 0 {
		t.Errorf("the answer = %+v, want an empty list and never null", answer)
	}
	if answer.Branch != "" {
		t.Errorf("branch = %q, want empty for a card that never started", answer.Branch)
	}
}

func TestCardDiffRefusesWhatIsNotThere(t *testing.T) {
	st := newStack(t)
	card := st.changedCard(t)
	tests := []struct {
		name    string
		path    string
		message string
	}{
		{"a card that does not exist", "/v1/cards/01M3C107JB041061050R3GG28B/diff", "Marshal cannot find that card. It may have been removed."},
		{"a card id of the wrong shape", "/v1/cards/not-an-id/diff", "Marshal cannot find that card. It may have been removed."},
		{"a file that did not change", "/v1/cards/" + card.ID + "/diff/README.md", "Marshal cannot find that file. It may have been removed."},
		{"a file on a card that does not exist", "/v1/cards/01M3C107JB041061050R3GG28B/diff/src/util.js", "Marshal cannot find that card. It may have been removed."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := st.do(http.MethodGet, tc.path, nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
			if got.Message != tc.message {
				t.Errorf("message = %q\nwant      %q", got.Message, tc.message)
			}
		})
	}
}
