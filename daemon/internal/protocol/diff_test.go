package protocol_test

import (
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The golden samples of a card's diff (docs/backend-checklist.md B2.9, docs/backend-inventory.md
// N15). The list carries one file of every status, a binary file, a large file, and a list that was
// cut short; the hunks carry one line of every kind. A wire shape that changes shows up here and in
// the app's own test of the same file.

// diffTime is the moment every sample is stamped with.
func diffTime() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) }

// cardDiff is one card's file list, in path order and without hunks.
func cardDiff() protocol.CardDiff {
	return protocol.CardDiff{
		CardID: "01M3C107JB041061050R3GG28A",
		Base:   "main",
		Branch: "marshal/api-41-refresh-the-token",
		Files: []protocol.ChangedFile{
			{
				Path: "assets/logo.png", Status: protocol.DiffFileStatusModified, Binary: true,
			},
			{
				Path: "go.sum", Status: protocol.DiffFileStatusAdded, Additions: 1240, Large: true,
			},
			{
				Path: "internal/auth/legacy.go", Status: protocol.DiffFileStatusDeleted, Deletions: 41,
			},
			{
				Path: "internal/auth/middleware.go", Status: protocol.DiffFileStatusModified,
				Additions: 19, Deletions: 3,
			},
			{
				Path: "internal/auth/refresh.go", Status: protocol.DiffFileStatusModified,
				Additions: 38, Deletions: 9,
			},
			{
				Path: "internal/auth/session.go", OldPath: "internal/auth/token.go",
				Status: protocol.DiffFileStatusRenamed, Additions: 2, Deletions: 2,
			},
		},
		FileCount: 1203, Additions: 1299, Deletions: 55, Truncated: true,
		ServerTime: protocol.NewTimestamp(diffTime()),
	}
}

// fileHunks is one file's hunks, as the screen loads them when the file is opened.
func fileHunks() protocol.FileHunks {
	return protocol.FileHunks{
		Path:   "internal/auth/refresh.go",
		Status: protocol.DiffFileStatusModified,
		Hunks: []protocol.DiffHunk{{
			Header: "@@ -41,5 +41,8 @@ type Client struct",
			Lines: []protocol.DiffLine{
				{Kind: protocol.DiffLineKindContext, OldLine: 41, NewLine: 41, Text: "type Client struct {"},
				{Kind: protocol.DiffLineKindRemoved, OldLine: 42, Text: "\thttp *http.Client"},
				{Kind: protocol.DiffLineKindAdded, NewLine: 42, Text: "\thttp     *http.Client"},
				{Kind: protocol.DiffLineKindAdded, NewLine: 43, Text: "\tgroup    singleflight.Group"},
				{Kind: protocol.DiffLineKindContext, OldLine: 43, NewLine: 44, Text: "}"},
			},
		}},
		ServerTime: protocol.NewTimestamp(diffTime()),
	}
}

func TestCardDiffGolden(t *testing.T) {
	testutil.Golden(t, "card-diff", cardDiff())
	testutil.Golden(t, "file-hunks", fileHunks())
}

// An empty diff is a list of no files, never null: the Diff tab draws its empty state for a card
// that never started.
func TestCardDiffOfACardThatNeverStarted(t *testing.T) {
	empty := protocol.CardDiff{
		CardID: "01M3C107JB041061050R3GG28B", Base: "main", Files: []protocol.ChangedFile{},
		ServerTime: protocol.NewTimestamp(diffTime()),
	}
	testutil.Golden(t, "card-diff-empty", empty)
	if empty.Files == nil {
		t.Error("an empty diff carries a null file list")
	}
}

// The file statuses are Git's own words, because a status a person reads in Git must mean the same
// thing on the card's Diff tab.
func TestDiffFileStatusesCarryTheWordsGitUses(t *testing.T) {
	want := []string{"added", "modified", "deleted", "renamed"}
	if got := names(protocol.DiffFileStatusValues()); !equalStrings(got, want) {
		t.Errorf("DiffFileStatus = %v, want %v", got, want)
	}
	for _, status := range protocol.DiffFileStatusValues() {
		if !status.Valid() {
			t.Errorf("%q is in DiffFileStatusValues but is not valid", status)
		}
	}
	if protocol.DiffFileStatus("changed").Valid() {
		t.Error("an unknown file status is valid")
	}
	if got := names(protocol.DiffLineKindValues()); !equalStrings(got, []string{"context", "added", "removed"}) {
		t.Errorf("DiffLineKind = %v", got)
	}
	if protocol.DiffLineKind("+").Valid() {
		t.Error("a sign is not a wire value; the wire says added")
	}
}
