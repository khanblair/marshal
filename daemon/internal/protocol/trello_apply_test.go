package protocol_test

import (
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The wire shape of the Trello apply flow: what is pending, what an apply answers, and what a
// refresh answers.

func sampleTrelloPending() protocol.TrelloPending {
	fetched := protocol.NewTimestamp(providersNow.Add(-4 * time.Minute))
	return protocol.TrelloPending{
		Linked:    true,
		BoardName: "Mobile app",
		Changes: []protocol.TrelloChange{
			{
				ID: "chg-move-1", CardID: "01M3C107JB041061050R3GG28A", CardTitle: "Fix the report export",
				Kind: protocol.TrelloChangeKindMoveDone, Summary: "Move to Done", ByKind: "person",
				Conflict: true, ConflictNote: "Trello moved this card to Review.",
			},
			{
				ID: "chg-comment-1", CardID: "01M3C107JB041061050R3GG28A", CardTitle: "Fix the report export",
				Kind: protocol.TrelloChangeKindAddComment, Summary: "Add a comment",
				Detail: "The export now streams rows, so large reports no longer time out.", ByKind: "agent",
			},
		},
		Cards: []protocol.TrelloLinkedCard{{
			CardID: "01M3C107JB041061050R3GG28A", URL: "https://trello.com/c/Ab12CdEf",
			ListName: "In review", FromTrello: true,
		}},
		FetchedAt:  &fetched,
		ServerTime: protocol.NewTimestamp(providersNow),
	}
}

func TestTrelloPendingGolden(t *testing.T) {
	testutil.Golden(t, "trello-pending", sampleTrelloPending())
	testutil.Golden(t, "trello-pending-unlinked", protocol.TrelloPending{
		Changes: []protocol.TrelloChange{}, Cards: []protocol.TrelloLinkedCard{}, ServerTime: protocol.NewTimestamp(providersNow),
	})
}

func TestTrelloApplyGolden(t *testing.T) {
	testutil.Golden(t, "trello-apply-request", protocol.ApplyTrelloRequest{
		ChangeIDs: []string{"chg-move-1", "chg-comment-1"}, OverrideIDs: []string{"chg-move-1"},
	})
	testutil.Golden(t, "trello-apply-result", protocol.ApplyTrelloResult{
		Applied: []string{"chg-comment-1"},
		Skipped: []protocol.TrelloApplySkip{{ChangeID: "chg-move-1", Reason: "The Trello card was deleted."}},
		Pending: protocol.TrelloPending{Linked: true, BoardName: "Mobile app", Changes: []protocol.TrelloChange{},
			Cards: []protocol.TrelloLinkedCard{}, ServerTime: protocol.NewTimestamp(providersNow)},
	})
	testutil.Golden(t, "trello-dismiss-request", protocol.DismissTrelloRequest{ChangeIDs: []string{"chg-move-1"}})
	testutil.Golden(t, "trello-refresh-request", protocol.RefreshTrelloRequest{KeepTrelloCardIDs: []string{"01M3C107JB041061050R3GG28A"}})
	testutil.Golden(t, "trello-refresh-result", protocol.RefreshTrelloResult{
		Imported: 3, Updated: 1, Pending: sampleTrelloPending(),
	})
}
