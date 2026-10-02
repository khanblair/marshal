package quality_test

import (
	"context"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/quality"
)

func TestTheDiffIsReadAgainstTheIntegrationBranch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "big.go", padding(900))
	git := &fakeGit{files: []gitx.DiffFile{{Path: "big.go", Status: gitx.DiffStatusAdded}}, head: "abc123"}
	st := openTestStore(t)
	seedProject(t, st, testProjectID, "main")
	seedCard(t, st, testCardID, testProjectID)
	svc := newService(t, st, quality.Deps{
		Cards: &fakeCards{card: protocol.Card{ID: testCardID, ProjectID: testProjectID}},
		Projects: &fakeProjects{project: protocol.Project{
			ID: testProjectID, DefaultBranch: "main", IntegrationBranch: "development",
		}, path: dir},
		Git: git,
	})
	if _, err := svc.Check(context.Background(), testCardID); err != nil {
		t.Fatalf("check the card: %v", err)
	}
	if len(git.bases) != 1 || git.bases[0] != "development" {
		t.Fatalf("the diff was read against %v, want development", git.bases)
	}
}
