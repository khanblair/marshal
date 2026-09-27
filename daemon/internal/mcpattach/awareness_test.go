package mcpattach

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/mcpserver"
	"github.com/khanblair/marshal/daemon/internal/memory"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The board-awareness summary (docs/marshal-product-scope.md section 11.3, build-plan tasks 7.2 and
// 7.3). It is read at the start of every turn, so these tests are about what it says, what it leaves
// out, and that it stays inside the budget it is built to.

// awareAttacher builds an attacher over a board and a set of holds, without a host or a server: the
// summary is built by the attacher alone.
func awareAttacher(t *testing.T, cards []protocol.Card, claims map[string][]memory.Claim, claimErr error) *Attacher {
	t.Helper()
	byID := make(map[string]protocol.Card, len(cards))
	for _, card := range cards {
		byID[card.ID] = card
	}
	a, err := New(Deps{
		Host:   mcpserver.NewHost(),
		Cards:  &fakeCards{byID: byID, inBoard: cards},
		Notes:  fakeNotes{},
		Claims: fakeClaims{project: claims, err: claimErr},
		Agents: fakeAgents{},
		Harness: func(string) (harness.Config, bool) {
			return harness.Config{Mode: protocol.PermissionModeAutoEdits}, true
		},
		Command: "/usr/local/bin/marshald", Address: "127.0.0.1:47800",
	})
	if err != nil {
		t.Fatalf("build the attacher: %v", err)
	}
	return a
}

// claim is one held path, for building a project's holds.
func claim(cardID, path string) memory.Claim {
	return memory.Claim{CardID: cardID, ProjectID: testProjectID, PathOrPackage: path,
		ClaimedAt: time.UnixMilli(0).UTC()}
}

func TestTurnAwarenessNamesTheOtherCardsAndWhatTheyHold(t *testing.T) {
	cards := []protocol.Card{fixtureCard(), fixtureOther()}
	holds := map[string][]memory.Claim{
		testProjectID: {claim("card-2", "apps/web/src/views/board.tsx")},
	}
	a := awareAttacher(t, cards, holds, nil)

	got := a.TurnAwareness(context.Background(), testCardID)

	// The other card is named by its key, and its own line carries what it is doing and holds.
	for _, want := range []string{"#2", "Ship the board", "reading the diff", "holds",
		"apps/web/src/views/board.tsx"} {
		if !strings.Contains(got, want) {
			t.Errorf("the summary does not mention %q:\n%s", want, got)
		}
	}
	// The card itself is not listed as one of its own other cards.
	if strings.Contains(got, "#1 Add a health check") {
		t.Errorf("the summary names the card it is for:\n%s", got)
	}
}

func TestTurnAwarenessWarnsBothCardsAboutAFileTheyBothHold(t *testing.T) {
	shared := "daemon/internal/api/server.go"
	cards := []protocol.Card{fixtureCard(), fixtureOther()}
	holds := map[string][]memory.Claim{
		testProjectID: {claim(testCardID, shared), claim("card-2", shared)},
	}
	a := awareAttacher(t, cards, holds, nil)

	got := a.TurnAwareness(context.Background(), testCardID)

	// The card that already held the file is told, by name, which file and which other card - the
	// other half of memory.ClaimResult.Conflicts, which warns the card that claimed it last.
	if !strings.Contains(got, shared) {
		t.Errorf("the summary does not name the shared file:\n%s", got)
	}
	if !strings.Contains(got, "#2 Ship the board") {
		t.Errorf("the summary does not name the other card holding it:\n%s", got)
	}
	if !strings.Contains(got, "agree before either edits it") {
		t.Errorf("the summary does not say what to do about it:\n%s", got)
	}
	// It comes ahead of the board, because it is the part an agent must not lose to the budget.
	if i, j := strings.Index(got, shared), strings.Index(got, "Other cards on this board"); i < 0 || i > j {
		t.Errorf("the conflict warning is not ahead of the board:\n%s", got)
	}
}

func TestTurnAwarenessDoesNotWarnAboutAFileOnlyThisCardHolds(t *testing.T) {
	cards := []protocol.Card{fixtureCard(), fixtureOther()}
	holds := map[string][]memory.Claim{
		testProjectID: {claim(testCardID, "daemon/internal/session/send.go")},
	}
	a := awareAttacher(t, cards, holds, nil)

	got := a.TurnAwareness(context.Background(), testCardID)

	if strings.Contains(got, "agree before either edits it") {
		t.Errorf("the summary warned about a file no other card holds:\n%s", got)
	}
}

func TestTurnAwarenessStaysInsideItsBudget(t *testing.T) {
	// A hostile board: far more cards than the summary may name, with titles and notes long enough
	// that a summary built to the board's size would be enormous, and holds long enough to matter.
	cards := []protocol.Card{fixtureCard()}
	var holds []memory.Claim
	for i := 2; i < 200; i++ {
		id := fmt.Sprintf("card-%d", i)
		cards = append(cards, protocol.Card{
			ID: id, ProjectID: testProjectID, Number: i, Key: fmt.Sprintf("%s#%d", testProjectID, i),
			Title:    strings.Repeat("a very long card title ", 10),
			State:    protocol.CardStateWorking,
			DoingNow: strings.Repeat("explaining at length what it is up to ", 10),
		})
		for j := 0; j < 10; j++ {
			holds = append(holds, claim(id, fmt.Sprintf("some/deep/path/number/%d/file-%d.go", i, j)))
		}
	}
	// This card shares a file with another one, which must survive the budget.
	shared := "daemon/internal/api/server.go"
	holds = append(holds, claim(testCardID, shared), claim("card-2", shared))
	a := awareAttacher(t, cards, map[string][]memory.Claim{testProjectID: holds}, nil)

	got := a.TurnAwareness(context.Background(), testCardID)

	if len(got) > awarenessBudget {
		t.Errorf("the summary is %d bytes, want at most %d:\n%s", len(got), awarenessBudget, got)
	}
	if !utf8.ValidString(got) {
		t.Error("the summary is not valid UTF-8: a clipped line split a character")
	}
	// The warning is kept whole even when the board is cut: it is the part that must not be lost.
	if !strings.Contains(got, shared) || !strings.Contains(got, "agree before either edits it") {
		t.Errorf("the budget dropped the conflict warning:\n%s", got)
	}
	// The board is cut rather than grown, and says how much it left out.
	if !strings.Contains(got, "more cards") {
		t.Errorf("the summary does not say how many cards it left out:\n%s", got)
	}
}

func TestTurnAwarenessKeepsTheBoardWhenTheHoldsCannotBeRead(t *testing.T) {
	cards := []protocol.Card{fixtureCard(), fixtureOther()}
	a := awareAttacher(t, cards, nil, errors.New("memory is unreadable"))

	got := a.TurnAwareness(context.Background(), testCardID)

	if !strings.Contains(got, "#2 Ship the board") {
		t.Errorf("an unreadable memory module dropped the board too:\n%s", got)
	}
	if strings.Contains(got, "agree") {
		t.Errorf("the summary warned about something from an unreadable read:\n%s", got)
	}
}

func TestTurnAwarenessIsEmptyForACardThatCannotBeRead(t *testing.T) {
	a := awareAttacher(t, []protocol.Card{fixtureCard(), fixtureOther()}, nil, nil)

	if got := a.TurnAwareness(context.Background(), "no-such-card"); got != "" {
		t.Errorf("TurnAwareness for an unknown card = %q, want empty", got)
	}
}

func TestTurnAwarenessIsEmptyOnABoardThisCardHasToItself(t *testing.T) {
	a := awareAttacher(t, []protocol.Card{fixtureCard()}, nil, nil)

	if got := a.TurnAwareness(context.Background(), testCardID); got != "" {
		t.Errorf("TurnAwareness on a one-card board = %q, want empty", got)
	}
}

func TestClipNeverLengthensAStringAndKeepsItValid(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
	}{
		{name: "already short", in: "short", max: 10},
		{name: "exactly the limit", in: "12345", max: 5},
		{name: "ascii", in: strings.Repeat("x", 100), max: 20},
		{name: "multibyte", in: strings.Repeat("日", 100), max: 20},
		{name: "a zero limit", in: "anything", max: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := clip(tc.in, tc.max)
			if tc.max > 0 && len(got) > tc.max {
				t.Errorf("clip(%q, %d) is %d bytes, want at most %d", tc.in, tc.max, len(got), tc.max)
			}
			if !utf8.ValidString(got) {
				t.Errorf("clip(%q, %d) = %q, which is not valid UTF-8", tc.in, tc.max, got)
			}
			if len(tc.in) <= tc.max && got != tc.in {
				t.Errorf("clip(%q, %d) = %q, want the string untouched", tc.in, tc.max, got)
			}
		})
	}
}
